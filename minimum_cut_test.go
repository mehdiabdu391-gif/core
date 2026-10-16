package main

import (
	"strings"
	"testing"
)

func cutServices(names ...string) []string {
	result := make([]string, len(names))
	copy(result, names)
	return result
}

func TestAnalyzeMinimumCutChain(t *testing.T) {
	services := map[string]Service{
		"database": {Name: "database"},
		"api": {
			Name:      "api",
			DependsOn: []string{"database"},
		},
		"worker": {
			Name:      "worker",
			DependsOn: []string{"api"},
		},
	}

	result, err := AnalyzeMinimumCut(services, "worker")
	if err != nil {
		t.Fatal(err)
	}

	if result.Size != 1 {
		t.Fatalf("expected cut size 1, got %d", result.Size)
	}

	if len(result.Services) != 1 || result.Services[0] != "api" {
		t.Fatalf("expected {api}, got %v", result.Services)
	}
}

func TestAnalyzeMinimumCutRedundantDependencies(t *testing.T) {
	services := map[string]Service{
		"database": {Name: "database"},
		"api-a": {
			Name:      "api-a",
			DependsOn: []string{"database"},
		},
		"api-b": {
			Name:      "api-b",
			DependsOn: []string{"database"},
		},
		"worker": {
			Name:      "worker",
			DependsOn: []string{"api-a", "api-b"},
		},
	}

	result, err := AnalyzeMinimumCut(services, "worker")
	if err != nil {
		t.Fatal(err)
	}

	if result.Size != 2 {
		t.Fatalf("expected cut size 2, got %d", result.Size)
	}

	expected := cutServices("api-a", "api-b")

	for i := range expected {
		if result.Services[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, result.Services)
		}
	}
}

func TestAnalyzeMinimumCutLeaf(t *testing.T) {
	services := map[string]Service{
		"worker": {Name: "worker"},
	}

	result, err := AnalyzeMinimumCut(services, "worker")
	if err != nil {
		t.Fatal(err)
	}

	if !result.Isolated {
		t.Fatal("expected isolated service")
	}

	if result.Size != 0 {
		t.Fatalf("expected cut size 0, got %d", result.Size)
	}
}

func TestAnalyzeMinimumCutUnknownService(t *testing.T) {
	services := map[string]Service{
		"database": {Name: "database"},
	}

	if _, err := AnalyzeMinimumCut(services, "missing"); err == nil {
		t.Fatal("expected unknown service error")
	}
}

func TestFormatMinimumCut(t *testing.T) {
	result := MinimumCut{
		Target:   "worker",
		Services: []string{"api-a", "api-b"},
		Size:     2,
		Roots:    []string{"database"},
	}

	output := FormatMinimumCut(result)

	for _, expected := range []string{
		"MINIMUM FAILURE CUT",
		"worker",
		"2 service(s)",
		"api-a",
		"api-b",
		"database",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}
