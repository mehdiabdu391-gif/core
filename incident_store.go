package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var incidentDir = ".core/incidents"

type IncidentRecord struct {
	ID               string              `json:"id"`
	Timestamp        time.Time           `json:"timestamp"`
	Service          string              `json:"service"`
	Error            string              `json:"error"`
	RootService      string              `json:"root_service"`
	DirectDependents []string            `json:"direct_dependents"`
	Indirect         []string            `json:"indirect"`
	AffectedServices []string            `json:"affected_services"`
	Paths            map[string][]string `json:"paths"`
}

func saveIncident(record IncidentRecord) error {
	if err := os.MkdirAll(incidentDir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(incidentDir, record.ID+".json")
	return atomicWrite(path, data, 0o600)
}

func newIncidentID(t time.Time, service string) string {
	safe := filepath.Base(service)
	return fmt.Sprintf("%s-%s-%d", t.UTC().Format("20060102T150405.000000000Z"), safe, t.UnixNano())
}

func listIncidents() ([]IncidentRecord, error) {
	entries, err := os.ReadDir(incidentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	records := make([]IncidentRecord, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(incidentDir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var record IncidentRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("reading incident %s: %w", entry.Name(), err)
		}

		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	return records, nil
}
