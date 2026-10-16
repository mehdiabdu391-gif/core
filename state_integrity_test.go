package main

import (
	"os"
	"strings"
	"testing"
)

func TestInspectStateMissing(t *testing.T) {
	_ = os.Remove(stateFile)

	got := inspectState()

	if got.Status != StateMissing {
		t.Fatalf("expected StateMissing, got %q", got.Status)
	}
}

func TestInspectStateCorrupt(t *testing.T) {
	chdirTemp(t)

	if err := ensureCoreDirs(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(stateFile, []byte("THIS IS NOT JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := inspectState()

	if got.Status != StateCorrupt {
		t.Fatalf("expected StateCorrupt, got %q", got.Status)
	}

	if !strings.Contains(got.Reason, "invalid state JSON") {
		t.Fatalf("expected invalid JSON reason, got %q", got.Reason)
	}
}

func TestEnsureNoRunningCoreRejectsCorruptState(t *testing.T) {
	chdirTemp(t)

	if err := ensureCoreDirs(); err != nil {
		t.Fatal(err)
	}

	original := []byte("THIS IS NOT JSON")

	if err := os.WriteFile(stateFile, original, 0o600); err != nil {
		t.Fatal(err)
	}

	err := ensureNoRunningCore()

	if err == nil {
		t.Fatal("expected corrupt state to be rejected")
	}

	if !strings.Contains(err.Error(), "Core state is corrupt") {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != string(original) {
		t.Fatal("corrupt state was modified or deleted")
	}
}
