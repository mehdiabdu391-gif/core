package main

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestWatchStopRequest(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := ensureCoreDirs(); err != nil {
		t.Fatal(err)
	}
	// A leftover request from an earlier run must not stop the new one.
	_ = os.WriteFile(stopRequestFile, []byte("123"), 0o600)
	requested, quit := watchStopRequest(123)
	defer close(quit)
	if _, err := os.Stat(stopRequestFile); !os.IsNotExist(err) {
		t.Fatal("stale stop request was not removed")
	}
	// A request for another supervisor is ignored.
	_ = os.WriteFile(stopRequestFile, []byte("999"), 0o600)
	select {
	case <-requested:
		t.Fatal("stopped on a request for a different PID")
	case <-time.After(600 * time.Millisecond):
	}
	_ = os.WriteFile(stopRequestFile, []byte(strconv.Itoa(123)), 0o600)
	select {
	case <-requested:
	case <-time.After(2 * time.Second):
		t.Fatal("stop request not noticed")
	}
}
