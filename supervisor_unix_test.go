//go:build !windows

package main

import (
	"strings"
	"testing"
	"time"
)

func TestPermanentDependencyFailureFailsDependent(t *testing.T) {
	chdirTemp(t)
	db := Service{Name: "database", Command: "sh"}
	api := Service{Name: "api", Command: "sh", DependsOn: []string{"database"}}
	shutdown := make(chan struct{})
	s := NewSupervisor([]Service{db, api}, shutdown)

	s.gates["database"].markFailed(&testError{"database crashed"})

	done := make(chan struct{})
	go func() { s.runService(api); close(done) }()
	assertClosed(t, done, "dependent did not give up")

	if !s.gates["api"].isFailed() {
		t.Fatal("api failure was not propagated")
	}
	if err := s.gates["api"].failure(); !strings.Contains(err.Error(), "database") {
		t.Fatalf("unexpected api failure: %v", err)
	}
	if got := s.state.Services["api"].Status; got != StatusFailed {
		t.Fatalf("api status = %q", got)
	}
}

// A dependency that crashes and is restarted must not take its dependents down
// for good: they stop, wait for it, and start again.
func TestDependentRecoversAfterDependencyRestart(t *testing.T) {
	chdirTemp(t)
	db := Service{
		Name:    "db",
		Command: "sh",
		Args:    []string{"-c", "if [ -f crashed ]; then sleep 30; else touch crashed; sleep 0.5; exit 1; fi"},
		Restart: RestartAlways, Backoff: 100 * time.Millisecond, StopTimeout: time.Second,
	}
	api := Service{
		Name:      "api",
		Command:   "sh",
		Args:      []string{"-c", "sleep 30"},
		DependsOn: []string{"db"},
		Restart:   RestartOnFailure, MaxRestarts: 2, Backoff: 100 * time.Millisecond, StopTimeout: time.Second,
	}
	shutdown := make(chan struct{})
	s := NewSupervisor([]Service{db, api}, shutdown)
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run() }()

	deadline := time.Now().Add(5 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		s.stateMu.Lock()
		dbState, apiState := s.state.Services["db"], s.state.Services["api"]
		s.stateMu.Unlock()
		if apiState.Status == StatusFailed {
			t.Fatalf("api failed permanently: %s", apiState.Error)
		}
		if dbState.Restarts == 1 && dbState.Status == StatusReady && apiState.Status == StatusReady {
			recovered = true
			if apiState.Restarts != 0 {
				t.Fatalf("dependency restart must not spend api restarts, got %d", apiState.Restarts)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(shutdown)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !recovered {
		t.Fatal("api did not come back after db restarted")
	}
}

func TestFailureBeforeReadyHonoursRestartPolicy(t *testing.T) {
	chdirTemp(t)
	svc := Service{
		Name: "flaky", Command: "sh", Args: []string{"-c", "exit 1"},
		Restart: RestartOnFailure, MaxRestarts: 2, Backoff: 10 * time.Millisecond, StopTimeout: time.Second,
	}
	shutdown := make(chan struct{})
	s := NewSupervisor([]Service{svc}, shutdown)
	err := s.Run()
	if err == nil {
		t.Fatal("Run should report the failed service")
	}
	if !s.gates["flaky"].isFailed() {
		t.Fatal("service should end failed")
	}
	if got := s.state.Services["flaky"].Restarts; got != 2 {
		t.Fatalf("restarts = %d, want 2", got)
	}
}
