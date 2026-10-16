package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateUnknownDependency(t *testing.T) {
	m := map[string]Service{"api": {Name: "api", DependsOn: []string{"db"}}}
	if err := ValidateServices(m); err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("got %v", err)
	}
}
func TestValidateCycle(t *testing.T) {
	m := map[string]Service{"a": {Name: "a", DependsOn: []string{"b"}}, "b": {Name: "b", DependsOn: []string{"a"}}}
	if err := ValidateServices(m); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("got %v", err)
	}
}
func TestBackoff(t *testing.T) {
	if got := backoffDelay(time.Second, 1); got != time.Second {
		t.Fatal(got)
	}
	if got := backoffDelay(time.Second, 5); got != 16*time.Second {
		t.Fatal(got)
	}
	if got := backoffDelay(time.Second, 20); got != 30*time.Second {
		t.Fatal(got)
	}
}
func TestServiceName(t *testing.T) {
	for _, ok := range []struct {
		n string
		b bool
	}{{"api", true}, {"api-v2", true}, {"../x", false}, {"a/b", false}, {"", false}} {
		if validServiceName(ok.n) != ok.b {
			t.Fatalf("%q", ok.n)
		}
	}
}

type closedReader struct{}

func (closedReader) Read([]byte) (int, error) { return 0, os.ErrClosed }
func (closedReader) Close() error             { return nil }

func TestStreamOutputIgnoresExpectedClosedPipe(t *testing.T) {
	f, err := os.CreateTemp("", "core-log-test-*")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()

	streamOutput(closedReader{}, "web", &logSink{file: f})

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "output error") {
		t.Fatalf("unexpected pipe-close error in log: %q", data)
	}
}
