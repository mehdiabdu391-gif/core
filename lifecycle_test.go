package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleConstants(t *testing.T) {
	if CoreStarting != "starting" {
		t.Fatalf("CoreStarting = %q", CoreStarting)
	}
	if CoreRunning != "running" {
		t.Fatalf("CoreRunning = %q", CoreRunning)
	}
	if CoreStopping != "stopping" {
		t.Fatalf("CoreStopping = %q", CoreStopping)
	}
	if CoreStopped != "stopped" {
		t.Fatalf("CoreStopped = %q", CoreStopped)
	}
	if CoreFailed != "failed" {
		t.Fatalf("CoreFailed = %q", CoreFailed)
	}
	if CoreRecovering != "recovering" {
		t.Fatalf("CoreRecovering = %q", CoreRecovering)
	}
}

func TestNewRuntimeStateStartsWithExpectedLifecycle(t *testing.T) {
	state := newRuntimeState()

	if state.PID <= 0 {
		t.Fatalf("PID = %d, want positive PID", state.PID)
	}

	if state.StartedAt.IsZero() {
		t.Fatal("StartedAt is zero")
	}

	if state.Project == "" {
		t.Fatal("Project is empty")
	}

	if state.Lifecycle != "" {
		t.Fatalf("new runtime lifecycle = %q, want empty before supervisor starts", state.Lifecycle)
	}

	if state.Services == nil {
		t.Fatal("Services map is nil")
	}
}

func TestSetLifecyclePersistsState(t *testing.T) {
	dir := t.TempDir()

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	state := newRuntimeState()
	s := &Supervisor{
		state: state,
	}

	s.setLifecycle(CoreStarting)

	saved, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if saved.Lifecycle != CoreStarting {
		t.Fatalf("persisted lifecycle = %q, want %q", saved.Lifecycle, CoreStarting)
	}

	s.setLifecycle(CoreRunning)

	saved, err = loadState()
	if err != nil {
		t.Fatalf("loadState after running: %v", err)
	}

	if saved.Lifecycle != CoreRunning {
		t.Fatalf("persisted lifecycle = %q, want %q", saved.Lifecycle, CoreRunning)
	}
}

func TestLifecycleTransitionSequence(t *testing.T) {
	dir := t.TempDir()

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	state := newRuntimeState()
	s := &Supervisor{
		state: state,
	}

	sequence := []string{
		CoreStarting,
		CoreRunning,
		CoreStopping,
		CoreStopped,
	}

	for _, expected := range sequence {
		s.setLifecycle(expected)

		saved, err := loadState()
		if err != nil {
			t.Fatalf("loadState at %q: %v", expected, err)
		}

		if saved.Lifecycle != expected {
			t.Fatalf("lifecycle = %q, want %q", saved.Lifecycle, expected)
		}
	}
}

func TestFailedLifecyclePersists(t *testing.T) {
	dir := t.TempDir()

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	state := newRuntimeState()
	s := &Supervisor{
		state: state,
	}

	s.setLifecycle(CoreRunning)
	s.setLifecycle(CoreFailed)

	saved, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if saved.Lifecycle != CoreFailed {
		t.Fatalf("persisted lifecycle = %q, want %q", saved.Lifecycle, CoreFailed)
	}
}

func TestLifecyclePreservesRuntimeIdentity(t *testing.T) {
	dir := t.TempDir()

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	state := RuntimeState{
		PID:       12345,
		StartedAt: time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
		Project:   filepath.Clean(dir),
		Lifecycle: CoreRunning,
		Services:  map[string]ServiceRuntime{},
	}

	if err := saveState(state); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	s := &Supervisor{state: state}
	s.setLifecycle(CoreStopping)

	saved, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if saved.PID != state.PID {
		t.Fatalf("PID changed: got %d, want %d", saved.PID, state.PID)
	}

	if !saved.StartedAt.Equal(state.StartedAt) {
		t.Fatalf("StartedAt changed: got %v, want %v", saved.StartedAt, state.StartedAt)
	}

	if saved.Project != state.Project {
		t.Fatalf("Project changed: got %q, want %q", saved.Project, state.Project)
	}

	if saved.Lifecycle != CoreStopping {
		t.Fatalf("Lifecycle = %q, want %q", saved.Lifecycle, CoreStopping)
	}
}
