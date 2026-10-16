package main

import "testing"

func TestBuildFailureImpactChain(t *testing.T) {
	services := map[string]Service{
		"database": {Name: "database"},
		"backend":  {Name: "backend", DependsOn: []string{"database"}},
		"api":      {Name: "api", DependsOn: []string{"backend"}},
		"frontend": {Name: "frontend", DependsOn: []string{"api"}},
	}

	impact, err := BuildFailureImpact(services, "database")
	if err != nil {
		t.Fatalf("BuildFailureImpact: %v", err)
	}

	if len(impact.DirectDependents) != 1 ||
		impact.DirectDependents[0] != "backend" {
		t.Fatalf("unexpected direct dependents: %#v", impact.DirectDependents)
	}

	if len(impact.Indirect) != 2 {
		t.Fatalf("unexpected indirect dependents: %#v", impact.Indirect)
	}

	if len(impact.AllAffected) != 3 {
		t.Fatalf("unexpected affected count: %d", len(impact.AllAffected))
	}

	wantPath := []string{"database", "backend", "api", "frontend"}
	gotPath := impact.Paths["frontend"]

	if len(gotPath) != len(wantPath) {
		t.Fatalf("frontend path: %#v", gotPath)
	}

	for i := range wantPath {
		if gotPath[i] != wantPath[i] {
			t.Fatalf("frontend path: got %#v want %#v", gotPath, wantPath)
		}
	}
}

func TestBuildFailureImpactNoDependents(t *testing.T) {
	services := map[string]Service{
		"lab": {Name: "lab"},
	}

	impact, err := BuildFailureImpact(services, "lab")
	if err != nil {
		t.Fatalf("BuildFailureImpact: %v", err)
	}

	if len(impact.DirectDependents) != 0 {
		t.Fatalf("unexpected direct dependents: %#v", impact.DirectDependents)
	}

	if len(impact.Indirect) != 0 {
		t.Fatalf("unexpected indirect dependents: %#v", impact.Indirect)
	}

	if len(impact.AllAffected) != 0 {
		t.Fatalf("unexpected affected services: %#v", impact.AllAffected)
	}
}

func TestSimulateFailure(t *testing.T) {
	services := map[string]Service{
		"database": {Name: "database"},
		"api":      {Name: "api", DependsOn: []string{"database"}},
	}

	result, err := SimulateFailure(services, "database")
	if err != nil {
		t.Fatalf("SimulateFailure: %v", err)
	}

	if result.Outcomes["database"] != SimulationFailed {
		t.Fatalf("database outcome: %v", result.Outcomes["database"])
	}

	if result.Outcomes["api"] != SimulationImpacted {
		t.Fatalf("api outcome: %v", result.Outcomes["api"])
	}
}
