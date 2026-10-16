package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type processResult struct {
	err            error
	intentional    bool
	dependencyDown bool
}
type processWait struct {
	once sync.Once
	done chan struct{}
	err  error
}

func newProcessWait() *processWait   { return &processWait{done: make(chan struct{})} }
func (w *processWait) set(err error) { w.once.Do(func() { w.err = err; close(w.done) }) }
func (w *processWait) result() error { <-w.done; return w.err }

type logSink struct {
	mu   sync.Mutex
	file *os.File
}

// runProcess starts one instance of a service and supervises it until it
// exits, a dependency stops being ready (depDown closes), or shutdown closes.
// depDown may be nil when the service has no dependencies.
func runProcess(service Service, shutdown <-chan struct{}, depDown <-chan struct{}, depReason func() string, onPID func(int), onReady func(int)) processResult {
	cmd := exec.Command(service.Command, service.Args...)
	if service.WorkingDir != "" {
		cmd.Dir = service.WorkingDir
	}
	if len(service.Environment) > 0 {
		cmd.Env = append(os.Environ(), service.Environment...)
	}
	configureProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return processResult{err: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return processResult{err: err}
	}
	logFile, err := openServiceLog(service.Name)
	if err != nil {
		return processResult{err: fmt.Errorf("opening service log: %w", err)}
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		fmt.Printf("[%s] Failed to start: %v\n", service.Name, err)
		return processResult{err: err}
	}
	onPID(cmd.Process.Pid)
	fmt.Printf("[%s] Started PID=%d\n", service.Name, cmd.Process.Pid)
	sink := &logSink{file: logFile}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamOutput(stdout, service.Name, sink) }()
	go func() { defer wg.Done(); streamOutput(stderr, service.Name, sink) }()
	pw := newProcessWait()
	go func() { pw.set(cmd.Wait()) }()
	if service.Healthcheck == nil {
		onReady(cmd.Process.Pid)
	} else if err := waitForHealth(service, shutdown, depDown, pw); err != nil {
		stopProcessTree(cmd, pw.done, service.StopTimeout)
		_ = pw.result()
		wg.Wait()
		_ = logFile.Close()
		switch {
		case isClosed(shutdown):
			return processResult{intentional: true}
		case isClosed(depDown):
			fmt.Printf("[%s] %s; stopping...\n", service.Name, depReason())
			return processResult{err: errDependencyFailed, dependencyDown: true}
		}
		fmt.Printf("[%s] %v\n", service.Name, err)
		return processResult{err: err}
	} else {
		onReady(cmd.Process.Pid)
		fmt.Printf("[%s] Health check passed\n", service.Name)
	}
	select {
	case <-pw.done:
		err := pw.err
		wg.Wait()
		_ = logFile.Close()
		if err != nil {
			fmt.Printf("[%s] Exited: %v\n", service.Name, err)
			return processResult{err: err}
		}
		fmt.Printf("[%s] Exited normally\n", service.Name)
		return processResult{}
	case <-depDown:
		fmt.Printf("[%s] %s; stopping...\n", service.Name, depReason())
		stopProcessTree(cmd, pw.done, service.StopTimeout)
		_ = pw.result()
		wg.Wait()
		_ = logFile.Close()
		return processResult{err: errDependencyFailed, dependencyDown: true}
	case <-shutdown:
		fmt.Printf("[%s] Stopping...\n", service.Name)
		stopProcessTree(cmd, pw.done, service.StopTimeout)
		_ = pw.result()
		wg.Wait()
		_ = logFile.Close()
		return processResult{intentional: true}
	}
}
func isClosed(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func waitForHealth(service Service, shutdown <-chan struct{}, depDown <-chan struct{}, pw *processWait) error {
	h := service.Healthcheck
	interval, err := healthDuration(h.Interval, time.Second)
	if err != nil {
		return err
	}
	timeout, err := healthDuration(h.Timeout, 2*time.Second)
	if err != nil {
		return err
	}
	retries := h.Retries
	if retries <= 0 {
		retries = 30
	}
	var last error
	for attempt := 0; attempt < retries; attempt++ {
		select {
		case <-pw.done:
			return fmt.Errorf("process exited before health check passed: %v", pw.err)
		case <-shutdown:
			return errShutdown
		case <-depDown:
			return errDependencyFailed
		default:
		}
		if err := checkHealth(service, timeout); err == nil {
			return nil
		} else {
			last = err
		}
		t := time.NewTimer(interval)
		select {
		case <-t.C:
		case <-shutdown:
			t.Stop()
			return errShutdown
		case <-depDown:
			t.Stop()
			return errDependencyFailed
		case <-pw.done:
			t.Stop()
			return fmt.Errorf("process exited during health check: %v", pw.err)
		}
	}
	return fmt.Errorf("health check failed after %d attempts: %v", retries, last)
}
func healthDuration(v string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid healthcheck duration %q", v)
	}
	return d, nil
}
func checkHealth(service Service, timeout time.Duration) error {
	h := service.Healthcheck
	switch strings.ToLower(strings.TrimSpace(h.Type)) {
	case "tcp":
		host := h.Host
		if host == "" {
			host = "127.0.0.1"
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, h.Port), timeout)
		if err != nil {
			return err
		}
		return conn.Close()
	case "http":
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
		if err != nil {
			return err
		}
		client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("http status %d", resp.StatusCode)
		}
		return nil
	case "command":
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, h.Command[0], h.Command[1:]...)
		if service.WorkingDir != "" {
			cmd.Dir = service.WorkingDir
		}
		if len(service.Environment) > 0 {
			cmd.Env = append(os.Environ(), service.Environment...)
		}
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		return cmd.Run()
	default:
		return fmt.Errorf("unknown healthcheck type %q", h.Type)
	}
}
func streamOutput(pipe io.ReadCloser, name string, sink *logSink) {
	defer pipe.Close()
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Printf("[%s] %s\n", name, line)
		sink.mu.Lock()
		_, _ = fmt.Fprintf(sink.file, "[%s] %s\n", time.Now().UTC().Format(time.RFC3339Nano), line)
		sink.mu.Unlock()
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		sink.mu.Lock()
		_, _ = fmt.Fprintf(sink.file, "[%s] [core] output error: %v\n", time.Now().UTC().Format(time.RFC3339Nano), err)
		sink.mu.Unlock()
	}
}
func openServiceLog(name string) (*os.File, error) {
	if !validServiceName(name) {
		return nil, fmt.Errorf("invalid service log name %q", name)
	}
	if err := ensureCoreDirs(); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(logDir, name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
