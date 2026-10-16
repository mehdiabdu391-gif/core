package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIncidentStoreRoundTrip(t *testing.T) {
	oldDir := incidentDir
	incidentDir = filepath.Join(t.TempDir(), "incidents")
	t.Cleanup(func() {
		incidentDir = oldDir
	})

	now := time.Now().UTC().Truncate(time.Second)

	record := IncidentRecord{
		ID:               newIncidentID(now, "database"),
		Timestamp:        now,
		Service:          "database",
		Error:            "exit status 1",
		RootService:      "database",
		DirectDependents: []string{"api"},
		Indirect:         []string{"worker"},
		AffectedServices: []string{"api", "worker"},
		Paths: map[string][]string{
			"api":    {"database", "api"},
			"worker": {"database", "api", "worker"},
		},
	}

	if err := saveIncident(record); err != nil {
		t.Fatalf("saveIncident: %v", err)
	}

	loaded, err := listIncidents()
	if err != nil {
		t.Fatalf("listIncidents: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("got %d incidents, want 1", len(loaded))
	}

	got := loaded[0]

	if got.ID != record.ID {
		t.Fatalf("ID: got %q want %q", got.ID, record.ID)
	}

	if got.Service != "database" {
		t.Fatalf("Service: got %q want database", got.Service)
	}

	if got.Error != "exit status 1" {
		t.Fatalf("Error: got %q", got.Error)
	}

	if len(got.DirectDependents) != 1 || got.DirectDependents[0] != "api" {
		t.Fatalf("DirectDependents: %#v", got.DirectDependents)
	}

	if len(got.Indirect) != 1 || got.Indirect[0] != "worker" {
		t.Fatalf("Indirect: %#v", got.Indirect)
	}

	if len(got.AffectedServices) != 2 {
		t.Fatalf("AffectedServices: %#v", got.AffectedServices)
	}

	if got.Paths["worker"][2] != "worker" {
		t.Fatalf("worker path: %#v", got.Paths["worker"])
	}

	files, err := os.ReadDir(incidentDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Fatalf("stored files: got %d want 1", len(files))
	}
}

func TestIncidentIDIsUnique(t *testing.T) {
	t1 := time.Unix(100, 1).UTC()
	t2 := time.Unix(100, 2).UTC()

	a := newIncidentID(t1, "api")
	b := newIncidentID(t2, "api")

	if a == b {
		t.Fatal("incident IDs must be unique")
	}
}
