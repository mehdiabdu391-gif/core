package main

import (
	"testing"
	"time"
)

func TestGenerateSuggestionsCritical(t *testing.T) {
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	records := []IncidentRecord{
		predictionTestIncident("redis", base),
		predictionTestIncident("redis", base.Add(1*time.Hour)),
		predictionTestIncident("redis", base.Add(2*time.Hour)),
	}

	suggestions := generateSuggestions(records)

	if len(suggestions) != 1 {
		t.Fatalf("suggestions = %d, want 1", len(suggestions))
	}

	s := suggestions[0]

	if s.Service != "redis" {
		t.Fatalf("service = %q, want redis", s.Service)
	}

	if s.Suggested != "backoff: 30s" {
		t.Fatalf("suggested = %q, want %q", s.Suggested, "backoff: 30s")
	}

	if s.Confidence != 84 {
		t.Fatalf("confidence = %d, want 84", s.Confidence)
	}
}

func TestGenerateSuggestionsStable(t *testing.T) {
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	records := []IncidentRecord{
		predictionTestIncident("batch", base.Add(-29*24*time.Hour)),
		predictionTestIncident("batch", base.Add(-14*24*time.Hour)),
		predictionTestIncident("batch", base),
	}

	suggestions := generateSuggestions(records)

	if len(suggestions) != 0 {
		t.Fatalf("suggestions = %d, want 0", len(suggestions))
	}
}

func TestGenerateSuggestionsNoIncidents(t *testing.T) {
	suggestions := generateSuggestions(nil)

	if len(suggestions) != 0 {
		t.Fatalf("suggestions = %d, want 0", len(suggestions))
	}
}
