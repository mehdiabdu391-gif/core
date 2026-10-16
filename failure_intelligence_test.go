package main

import (
	"strings"
	"testing"
)

func TestBuildFailureImpact(t *testing.T) {
	services := map[string]Service{
		"database": {
			Name: "database",
		},
		"api": {
			Name:      "api",
			DependsOn: []string{"database"},
		},
		"worker": {
			Name:      "worker",
			DependsOn: []string{"api"},
		},
		"frontend": {
			Name:      "frontend",
			DependsOn: []string{"api"},
		},
		"independent": {
			Name: "independent",
		},
	}

	impact, err := BuildFailureImpact(services, "database")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(impact.DirectDependents) != 1 ||
		impact.DirectDependents[0] != "api" {
		t.Fatalf("unexpected direct dependents: %v", impact.DirectDependents)
	}

	if len(impact.Indirect) != 2 {
		t.Fatalf("unexpected indirect dependents: %v", impact.Indirect)
	}

	if len(impact.AllAffected) != 3 {
		t.Fatalf("unexpected affected services: %v", impact.AllAffected)
	}

	wantPaths := map[string]string{
		"api":      "database -> api",
		"frontend": "database -> api -> frontend",
		"worker":   "database -> api -> worker",
	}

	for service, want := range wantPaths {
		got, ok := impact.Paths[service]
		if !ok {
			t.Fatalf("missing path for %s", service)
		}

		if strings.Join(got, " -> ") != want {
			t.Fatalf("path for %s = %q, want %q",
				service, strings.Join(got, " -> "), want)
		}
	}
}

func TestBuildFailureImpactUnknownService(t *testing.T) {
	services := map[string]Service{
		"api": {
			Name: "api",
		},
	}

	_, err := BuildFailureImpact(services, "missing")
	if err == nil {
		t.Fatal("expected error for unknown root service")
	}
}
