package main

import (
	"strings"
	"testing"
	"time"
)

func testCounterfactualServices() map[string]Service {
	return map[string]Service{
		"database": {
			Name:    "database",
			Restart: RestartAlways,
		},
		"api": {
			Name:      "api",
			DependsOn: []string{"database"},
			Restart:   RestartAlways,
		},
		"worker": {
			Name:      "worker",
			DependsOn: []string{"api"},
			Restart:   RestartAlways,
		},
	}
}

func TestAnalyzeCounterfactualDatabase(t *testing.T) {
	services := testCounterfactualServices()

	result, err := AnalyzeCounterfactual(services, nil, "database")
	if err != nil {
		t.Fatal(err)
	}

	if result.RootService != "database" {
		t.Fatalf("root = %q, want database", result.RootService)
	}

	if result.Outcomes["database"] != CounterfactualFailed {
		t.Fatalf("database outcome = %q, want FAILED", result.Outcomes["database"])
	}

	if result.Outcomes["api"] != CounterfactualDependencyLost {
		t.Fatalf("api outcome = %q, want DEPENDENCY_LOST", result.Outcomes["api"])
	}

	if result.Outcomes["worker"] != CounterfactualAtRisk {
		t.Fatalf("worker outcome = %q, want AT_RISK", result.Outcomes["worker"])
	}

	if len(result.Impact.DirectDependents) != 1 {
		t.Fatalf("direct dependents = %d, want 1", len(result.Impact.DirectDependents))
	}

	if len(result.Impact.Indirect) != 1 {
		t.Fatalf("indirect dependents = %d, want 1", len(result.Impact.Indirect))
	}

	if result.PropagationDepth != 2 {
		t.Fatalf("propagation depth = %d, want 2", result.PropagationDepth)
	}

	if result.HistoricalFailure != 0 {
		t.Fatalf("historical failures = %d, want 0", result.HistoricalFailure)
	}

	if result.HistoricalAffected != 0 {
		t.Fatalf("historical affected = %d, want 0", result.HistoricalAffected)
	}

	if result.HistoricalPathCount != 0 {
		t.Fatalf("historical paths = %d, want 0", result.HistoricalPathCount)
	}

	if result.Assessment != CounterfactualHighImpact {
		t.Fatalf("assessment = %q, want %q", result.Assessment, CounterfactualHighImpact)
	}
}

func TestAnalyzeCounterfactualLeaf(t *testing.T) {
	services := testCounterfactualServices()

	result, err := AnalyzeCounterfactual(services, nil, "worker")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Impact.AllAffected) != 0 {
		t.Fatalf("affected = %d, want 0", len(result.Impact.AllAffected))
	}

	if result.PropagationDepth != 0 {
		t.Fatalf("propagation depth = %d, want 0", result.PropagationDepth)
	}

	if len(result.Outcomes) != 1 {
		t.Fatalf("outcomes = %d, want 1", len(result.Outcomes))
	}

	if result.Outcomes["worker"] != CounterfactualFailed {
		t.Fatalf("worker outcome = %q, want FAILED", result.Outcomes["worker"])
	}

	if result.Assessment != CounterfactualIsolated {
		t.Fatalf("assessment = %q, want %q", result.Assessment, CounterfactualIsolated)
	}
}

func TestAnalyzeCounterfactualDirectDependent(t *testing.T) {
	services := testCounterfactualServices()

	result, err := AnalyzeCounterfactual(services, nil, "api")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Impact.AllAffected) != 1 {
		t.Fatalf("affected = %d, want 1", len(result.Impact.AllAffected))
	}

	if len(result.Impact.DirectDependents) != 1 {
		t.Fatalf("direct dependents = %d, want 1", len(result.Impact.DirectDependents))
	}

	if len(result.Impact.Indirect) != 0 {
		t.Fatalf("indirect dependents = %d, want 0", len(result.Impact.Indirect))
	}

	if result.PropagationDepth != 1 {
		t.Fatalf("propagation depth = %d, want 1", result.PropagationDepth)
	}

	if result.Assessment != CounterfactualModerateImpact {
		t.Fatalf("assessment = %q, want %q", result.Assessment, CounterfactualModerateImpact)
	}
}

func TestAnalyzeCounterfactualUnknownService(t *testing.T) {
	services := testCounterfactualServices()

	_, err := AnalyzeCounterfactual(services, nil, "cache")
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
}

func TestCounterfactualHistoricalEvidence(t *testing.T) {
	services := testCounterfactualServices()

	now := time.Now()

	records := []IncidentRecord{
		{
			ID:               "incident-1",
			Timestamp:        now.Add(-2 * time.Hour),
			Service:          "database",
			RootService:      "database",
			Error:            "database crashed",
			AffectedServices: []string{"api", "worker"},
			Paths: map[string][]string{
				"api":    {"database", "api"},
				"worker": {"database", "api", "worker"},
			},
		},
		{
			ID:               "incident-2",
			Timestamp:        now.Add(-1 * time.Hour),
			Service:          "database",
			RootService:      "database",
			Error:            "database unavailable",
			AffectedServices: []string{"api"},
			Paths: map[string][]string{
				"api": {"database", "api"},
			},
		},
	}

	result, err := AnalyzeCounterfactual(services, records, "database")
	if err != nil {
		t.Fatal(err)
	}

	if result.HistoricalFailure != 2 {
		t.Fatalf("historical failures = %d, want 2", result.HistoricalFailure)
	}

	if result.HistoricalAffected != 2 {
		t.Fatalf("historical affected = %d, want 2", result.HistoricalAffected)
	}

	if result.HistoricalPathCount != 3 {
		t.Fatalf("historical paths = %d, want 3", result.HistoricalPathCount)
	}

	if result.PropagationDepth != 2 {
		t.Fatalf("propagation depth = %d, want 2", result.PropagationDepth)
	}

	if result.Assessment != CounterfactualHighImpact {
		t.Fatalf("assessment = %q, want %q", result.Assessment, CounterfactualHighImpact)
	}
}

func TestCounterfactualEscalatesWithoutRestartPolicy(t *testing.T) {
	services := testCounterfactualServices()

	services["database"] = Service{
		Name:    "database",
		Restart: RestartNo,
	}

	result, err := AnalyzeCounterfactual(services, nil, "database")
	if err != nil {
		t.Fatal(err)
	}

	if result.Recovery.Action != RecoveryEscalate {
		t.Fatalf(
			"recovery action = %q, want %q",
			result.Recovery.Action,
			RecoveryEscalate,
		)
	}

	if !strings.Contains(result.Recommendation, "Protect") {
		t.Fatalf(
			"recommendation = %q, want protection recommendation",
			result.Recommendation,
		)
	}
}

func TestRankCounterfactuals(t *testing.T) {
	services := testCounterfactualServices()

	results, err := RankCounterfactuals(services, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}

	if results[0].RootService != "database" {
		t.Fatalf("rank #1 = %q, want database", results[0].RootService)
	}

	if results[0].Assessment != CounterfactualHighImpact {
		t.Fatalf("database assessment = %q, want %q",
			results[0].Assessment,
			CounterfactualHighImpact)
	}

	if results[1].RootService != "api" {
		t.Fatalf("rank #2 = %q, want api", results[1].RootService)
	}

	if results[1].Assessment != CounterfactualModerateImpact {
		t.Fatalf("api assessment = %q, want %q",
			results[1].Assessment,
			CounterfactualModerateImpact)
	}

	if results[2].RootService != "worker" {
		t.Fatalf("rank #3 = %q, want worker", results[2].RootService)
	}

	if results[2].Assessment != CounterfactualIsolated {
		t.Fatalf("worker assessment = %q, want %q",
			results[2].Assessment,
			CounterfactualIsolated)
	}
}

func TestCounterfactualRankingUsesEvidence(t *testing.T) {
	services := testCounterfactualServices()

	records := []IncidentRecord{
		{
			ID:          "incident-1",
			Service:     "api",
			RootService: "api",
			Paths: map[string][]string{
				"worker": {"api", "worker"},
			},
		},
	}

	results, err := RankCounterfactuals(services, records)
	if err != nil {
		t.Fatal(err)
	}

	if results[0].RootService != "database" {
		t.Fatalf("rank #1 = %q, want database", results[0].RootService)
	}

	if results[1].RootService != "api" {
		t.Fatalf("rank #2 = %q, want api", results[1].RootService)
	}

	if results[1].HistoricalFailure != 1 {
		t.Fatalf("api historical failures = %d, want 1",
			results[1].HistoricalFailure)
	}

	if results[1].HistoricalPathCount != 1 {
		t.Fatalf("api historical paths = %d, want 1",
			results[1].HistoricalPathCount)
	}
}

func TestFormatCounterfactual(t *testing.T) {
	services := testCounterfactualServices()

	result, err := AnalyzeCounterfactual(services, nil, "database")
	if err != nil {
		t.Fatal(err)
	}

	output := FormatCounterfactual(result)

	for _, expected := range []string{
		"COUNTERFACTUAL ANALYSIS",
		"database -> FAILED",
		"DEPENDENCY_LOST",
		"AT_RISK",
		"BLAST RADIUS",
		"2 propagation depth",
		"HISTORICAL EVIDENCE",
		"Historical dependency paths",
		"RECOVERY",
		"ASSESSMENT",
		"HIGH_IMPACT",
		"RECOMMENDATION",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}

	for _, forbidden := range []string{
		"COUNTERFACTUAL SCORE",
		"CONFIDENCE",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains removed concept %q:\n%s", forbidden, output)
		}
	}
}

func TestFormatCounterfactualRanking(t *testing.T) {
	services := testCounterfactualServices()

	results, err := RankCounterfactuals(services, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := FormatCounterfactualRanking(results)

	for _, expected := range []string{
		"FAILURE SURFACE ANALYSIS",
		"#1 database",
		"assessment: HIGH_IMPACT",
		"downstream: 2 services",
		"propagation depth: 2",
		"historical failures: 0",
		"historical paths: 0",
		"#2 api",
		"assessment: MODERATE_IMPACT",
		"#3 worker",
		"assessment: ISOLATED",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("ranking output missing %q:\n%s", expected, output)
		}
	}

	for _, forbidden := range []string{
		"score:",
		"confidence:",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("ranking output contains removed concept %q:\n%s", forbidden, output)
		}
	}
}
