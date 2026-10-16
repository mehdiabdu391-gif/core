package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseByteSize(t *testing.T) {
	cases := map[string]int64{"0": 0, "4096": 4096, "512KB": 512 << 10, "10mb": 10 << 20, "1G": 1 << 30, " 2 MB ": 2 << 20}
	for in, want := range cases {
		got, err := parseByteSize(in)
		if err != nil || got != want {
			t.Errorf("parseByteSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ten", "-1MB", "1TB"} {
		if _, err := parseByteSize(bad); err == nil {
			t.Errorf("parseByteSize(%q) should fail", bad)
		}
	}
}

func TestNormalizeLogsConfig(t *testing.T) {
	l, err := normalizeLogsConfig(LogsConfig{})
	if err != nil || l.MaxSize != defaultLogMaxSize || l.MaxFiles != defaultLogMaxFiles {
		t.Fatalf("defaults = %+v, %v", l, err)
	}
	five := 5
	l, err = normalizeLogsConfig(LogsConfig{MaxSize: "0", MaxFiles: &five})
	if err != nil || l.MaxSize != 0 || l.MaxFiles != 5 {
		t.Fatalf("got %+v, %v", l, err)
	}
	if _, err := normalizeLogsConfig(LogsConfig{MaxSize: "100"}); err == nil {
		t.Fatal("tiny max_size should fail")
	}
	zero := 0
	if _, err := normalizeLogsConfig(LogsConfig{MaxFiles: &zero}); err == nil {
		t.Fatal("max_files 0 should fail")
	}
}

func openTestSink(t *testing.T, limits LogLimits) (*logSink, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "web.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	s := newLogSink(f, limits)
	t.Cleanup(s.close)
	return s, path
}

func TestLogSinkRotatesAndKeepsMaxFiles(t *testing.T) {
	s, path := openTestSink(t, LogLimits{MaxSize: 100, MaxFiles: 2})
	line := strings.Repeat("x", 39) + "\n" // 40 bytes: two lines fit, the third rotates
	for i := 0; i < 20; i++ {
		s.writeLine(line)
	}
	for _, p := range []string{path, path + ".1", path + ".2"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("%s missing: %v", filepath.Base(p), err)
		}
		if info.Size() > 100 {
			t.Fatalf("%s is %d bytes, over the 100 byte limit", filepath.Base(p), info.Size())
		}
		data, _ := os.ReadFile(p)
		if len(data)%40 != 0 {
			t.Fatalf("%s holds a split line", filepath.Base(p))
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("kept more than max_files rotated logs")
	}
	got := rotatedLogs(path)
	if len(got) != 2 || got[0] != path+".2" || got[1] != path+".1" {
		t.Fatalf("rotatedLogs = %v", got)
	}
}

func TestLogSinkNoRotationWhenDisabled(t *testing.T) {
	s, path := openTestSink(t, LogLimits{MaxSize: 0, MaxFiles: 3})
	for i := 0; i < 100; i++ {
		s.writeLine("0123456789\n")
	}
	if info, _ := os.Stat(path); info.Size() != 1100 {
		t.Fatalf("size = %d", info.Size())
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("rotated with max_size 0")
	}
}

func TestLogSinkOversizedLineStillWritten(t *testing.T) {
	s, path := openTestSink(t, LogLimits{MaxSize: 10, MaxFiles: 1})
	s.writeLine("a long line over the limit\n")
	s.writeLine("b\n")
	cur, _ := os.ReadFile(path)
	old, _ := os.ReadFile(path + ".1")
	if string(old) != "a long line over the limit\n" || string(cur) != "b\n" {
		t.Fatalf("old=%q cur=%q", old, cur)
	}
}

func TestLogTailFollowsRotation(t *testing.T) {
	s, path := openTestSink(t, LogLimits{MaxSize: 4096, MaxFiles: 3})
	var tail logTail
	out := captureStdout(t, func() {
		s.writeLine("one\n")
		if err := tail.poll(path, false, false, ""); err != nil {
			t.Fatal(err)
		}
		s.writeLine("two\n") // written to the old file, not yet read
		s.mu.Lock()
		s.rotateLocked()
		s.mu.Unlock()
		s.writeLine("three\n")
		if err := tail.poll(path, false, false, ""); err != nil {
			t.Fatal(err)
		}
	})
	if out != "one\ntwo\nthree\n" {
		t.Fatalf("followed output = %q", out)
	}
}

func TestLogsConfigInCoreYAML(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	defer setLogLimits(LogLimits{MaxSize: defaultLogMaxSize, MaxFiles: defaultLogMaxFiles})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	yaml := "logs:\n  max_size: 1MB\n  max_files: 7\nservices:\n  web:\n    command: echo\n"
	if err := os.WriteFile("core.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if l := currentLogLimits(); l.MaxSize != 1<<20 || l.MaxFiles != 7 {
		t.Fatalf("limits = %+v", l)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	return <-done
}
