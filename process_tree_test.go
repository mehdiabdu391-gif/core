//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestProcessGroupKillsDescendant(t *testing.T) {
	cmd := exec.Command("sh", "-c", `
		trap 'exit 0' TERM
		sleep 300 &
		child=$!
		echo $child
		wait
	`)

	configureProcess(cmd)

	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	pid := cmd.Process.Pid
	t.Logf("parent PID=%d", pid)

	buf := make([]byte, 64)
	n, _ := output.Read(buf)

	childPIDText := ""
	for _, b := range buf[:n] {
		if b >= '0' && b <= '9' {
			childPIDText += string(b)
		}
	}

	childPID, err := strconv.Atoi(childPIDText)
	if err != nil {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		t.Fatalf("could not determine child PID: %v", err)
	}

	t.Logf("child PID=%d", childPID)

	time.Sleep(200 * time.Millisecond)

	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		t.Fatalf("group SIGTERM: %v", err)
	}

	done := make(chan error, 1)

	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		t.Log("parent terminated")
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		t.Fatal("parent did not terminate")
	}

	time.Sleep(200 * time.Millisecond)

	alive := false

	if p, err := os.FindProcess(childPID); err == nil {
		alive = p.Signal(syscall.Signal(0)) == nil
	}

	if alive {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatalf("descendant PID=%d survived process-group termination", childPID)
	}

	t.Logf("descendant PID=%d terminated", childPID)
}
