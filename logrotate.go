package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultLogMaxSize  int64 = 10 << 20
	defaultLogMaxFiles       = 3
	maxLogMaxFiles           = 100
)

// LogsConfig is the optional top-level "logs" block in core.yaml.
type LogsConfig struct {
	MaxSize  string `yaml:"max_size"`
	MaxFiles *int   `yaml:"max_files"`
}

// LogLimits controls rotation of .core/logs/NAME.log. MaxSize 0 disables
// rotation. MaxFiles is how many rotated files (NAME.log.1 ... NAME.log.N)
// are kept beside the live log.
type LogLimits struct {
	MaxSize  int64
	MaxFiles int
}

var (
	logLimitsMu sync.Mutex
	logLimits   = LogLimits{MaxSize: defaultLogMaxSize, MaxFiles: defaultLogMaxFiles}
)

func currentLogLimits() LogLimits {
	logLimitsMu.Lock()
	defer logLimitsMu.Unlock()
	return logLimits
}

func setLogLimits(l LogLimits) {
	logLimitsMu.Lock()
	logLimits = l
	logLimitsMu.Unlock()
}

func normalizeLogsConfig(cfg LogsConfig) (LogLimits, error) {
	limits := LogLimits{MaxSize: defaultLogMaxSize, MaxFiles: defaultLogMaxFiles}
	if strings.TrimSpace(cfg.MaxSize) != "" {
		n, err := parseByteSize(cfg.MaxSize)
		if err != nil {
			return limits, fmt.Errorf("logs.max_size: %w", err)
		}
		if n != 0 && n < 4<<10 {
			return limits, fmt.Errorf("logs.max_size must be 0 (no rotation) or at least 4KB")
		}
		limits.MaxSize = n
	}
	if cfg.MaxFiles != nil {
		if *cfg.MaxFiles < 1 || *cfg.MaxFiles > maxLogMaxFiles {
			return limits, fmt.Errorf("logs.max_files must be between 1 and %d", maxLogMaxFiles)
		}
		limits.MaxFiles = *cfg.MaxFiles
	}
	return limits, nil
}

// parseByteSize accepts plain bytes or a number with B, KB, MB or GB
// (1024-based, case-insensitive): "10MB", "512kb", "1048576".
func parseByteSize(s string) (int64, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(v, u.suffix) {
			v = strings.TrimSpace(strings.TrimSuffix(v, u.suffix))
			mult = u.mult
			break
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	if n > (1<<62)/mult {
		return 0, fmt.Errorf("size %q is too large", s)
	}
	return n * mult, nil
}

// logSink is one service's log file. Writes go through it so the file can be
// rotated between lines; the mutex is shared by the stdout and stderr readers.
type logSink struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	size   int64
	limits LogLimits
}

func newLogSink(f *os.File, limits LogLimits) *logSink {
	s := &logSink{file: f, path: f.Name(), limits: limits}
	if info, err := f.Stat(); err == nil {
		s.size = info.Size()
	}
	return s
}

// writeLine writes one complete line. It rotates first when the line would
// push the live file past MaxSize, so no line is ever split across files.
// The caller must not hold mu.
func (s *logSink) writeLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return
	}
	if s.limits.MaxSize > 0 && s.size > 0 && s.size+int64(len(line)) > s.limits.MaxSize {
		s.rotateLocked()
	}
	n, _ := s.file.WriteString(line)
	s.size += int64(n)
}

func (s *logSink) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}

// rotateLocked shifts NAME.log.(N-1) -> NAME.log.N ... NAME.log -> NAME.log.1
// and reopens an empty NAME.log. If the rename fails (on Windows another
// process may hold the file open), it keeps writing to the current file and
// tries again on a later line.
func (s *logSink) rotateLocked() {
	if s.path == "" {
		return
	}
	_ = s.file.Close()
	n := s.limits.MaxFiles
	if n < 1 {
		n = 1
	}
	_ = os.Remove(rotatedLogName(s.path, n))
	for i := n - 1; i >= 1; i-- {
		_ = os.Rename(rotatedLogName(s.path, i), rotatedLogName(s.path, i+1))
	}
	renameErr := os.Rename(s.path, rotatedLogName(s.path, 1))
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		s.file = nil
		return
	}
	s.file = f
	if renameErr != nil {
		// Still appending to the same file. Back off so a stuck rename is
		// retried after another half MaxSize of output, not on every line.
		s.size = s.limits.MaxSize / 2
		return
	}
	s.size = 0
}

func rotatedLogName(path string, i int) string {
	return path + "." + strconv.Itoa(i)
}

// rotatedLogs returns NAME.log.N ... NAME.log.1 that exist, oldest first.
func rotatedLogs(path string) []string {
	dir := filepath.Dir(path)
	base := filepath.Base(path) + "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	nums := []int{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, base) {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(name, base)); err == nil && n > 0 {
			nums = append(nums, n)
		}
	}
	// Highest number is oldest.
	sort.Sort(sort.Reverse(sort.IntSlice(nums)))
	out := make([]string, 0, len(nums))
	for _, n := range nums {
		out = append(out, rotatedLogName(path, n))
	}
	return out
}
