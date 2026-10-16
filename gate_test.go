package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestGateTransitions(t *testing.T) {
	g := newServiceGate()
	if g.isReady() || g.isFailed() {
		t.Fatal("new gate must be neither ready nor failed")
	}

	_, _, _, changed := g.snapshot()
	g.markReady()
	assertClosed(t, changed, "markReady did not broadcast")
	if !g.isReady() {
		t.Fatal("gate should be ready")
	}

	_, _, _, changed = g.snapshot()
	g.markDown(&testError{"database crashed"})
	assertClosed(t, changed, "markDown did not broadcast")
	if g.isReady() || g.isFailed() {
		t.Fatal("down gate must be neither ready nor failed")
	}

	g.markReady()
	if !g.isReady() {
		t.Fatal("gate must become ready again after recovering")
	}

	_, _, _, changed = g.snapshot()
	g.markFailed(&testError{"crash loop"})
	assertClosed(t, changed, "markFailed did not broadcast")
	if !g.isFailed() || g.isReady() {
		t.Fatal("gate should be failed")
	}
	if got := g.failure(); got == nil || got.Error() != "crash loop" {
		t.Fatalf("unexpected failure: %v", got)
	}

	g.markReady()
	if g.isReady() {
		t.Fatal("a failed gate must never become ready")
	}
}

func TestGateFailureDefaultsError(t *testing.T) {
	g := newServiceGate()
	g.markFailed(nil)
	if !errors.Is(g.failure(), errDependencyFailed) {
		t.Fatalf("unexpected failure: %v", g.failure())
	}
}

func TestWatchDependenciesFiresOnDown(t *testing.T) {
	dep := newServiceGate()
	dep.markReady()
	stop := make(chan struct{})
	defer close(stop)

	down := watchDependencies([]*serviceGate{dep}, stop)
	select {
	case <-down:
		t.Fatal("watch fired while dependency was ready")
	case <-time.After(50 * time.Millisecond):
	}

	dep.markDown(&testError{"database crashed"})
	assertClosed(t, down, "watch did not fire after dependency went down")
}

func assertClosed(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
}

func chdirTemp(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}
