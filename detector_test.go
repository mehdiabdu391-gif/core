package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsCoreProject(t *testing.T) {
	dir := t.TempDir()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module core\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !isCoreProject() {
		t.Fatal("expected Core project to be detected")
	}
}

func TestNonCoreGoProjectIsNotCore(t *testing.T) {
	dir := t.TempDir()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/myapp\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if isCoreProject() {
		t.Fatal("expected non-Core Go project not to be detected as Core")
	}
}

func TestDetectServicesDoesNotDetectCoreAsGoService(t *testing.T) {
	dir := t.TempDir()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module core\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	services, err := DetectServices()
	if err != nil {
		t.Fatal(err)
	}

	if len(services) != 0 {
		t.Fatalf("expected no services for Core itself, got %d: %#v", len(services), services)
	}
}
