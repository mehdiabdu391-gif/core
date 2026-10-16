package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type IncidentSummary struct {
	Service       string
	Failures      int
	AffectedTotal int
	Criticality   int
	LastIncident  time.Time
	LastError     string
}

func summarizeIncidents(records []IncidentRecord) []IncidentSummary {
	summaries := make(map[string]*IncidentSummary)

	for _, r := range records {
		s := summaries[r.Service]
		if s == nil {
			s = &IncidentSummary{
				Service: r.Service,
			}
			summaries[r.Service] = s
		}

		s.Failures++
		s.AffectedTotal += len(r.AffectedServices)

		criticality := len(r.AffectedServices)
		if criticality == 0 {
			criticality = 1
		}
		if criticality > s.Criticality {
			s.Criticality = criticality
		}

		if r.Timestamp.After(s.LastIncident) {
			s.LastIncident = r.Timestamp
			s.LastError = r.Error
		}
	}

	out := make([]IncidentSummary, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, *s)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Failures != out[j].Failures {
			return out[i].Failures > out[j].Failures
		}
		if out[i].Criticality != out[j].Criticality {
			return out[i].Criticality > out[j].Criticality
		}
		return out[i].Service < out[j].Service
	})

	return out
}

func formatIncidentHistory(records []IncidentRecord) string {
	var out strings.Builder

	out.WriteString("\nINCIDENT HISTORY\n")
	out.WriteString("================\n\n")

	if len(records) == 0 {
		out.WriteString("No incidents recorded.\n")
		return out.String()
	}

	summaries := summarizeIncidents(records)

	fmt.Fprintf(&out, "TOTAL INCIDENTS: %d\n", len(records))
	fmt.Fprintf(&out, "AFFECTED SERVICES: %d\n\n", totalAffectedServices(records))

	for _, s := range summaries {
		fmt.Fprintf(&out, "SERVICE: %s\n", s.Service)
		fmt.Fprintf(&out, "  Failures: %d\n", s.Failures)
		fmt.Fprintf(&out, "  Total affected: %d\n", s.AffectedTotal)
		fmt.Fprintf(&out, "  Criticality: %d\n", s.Criticality)

		if !s.LastIncident.IsZero() {
			fmt.Fprintf(&out, "  Last incident: %s\n",
				s.LastIncident.Format(time.RFC3339))
		}

		if s.LastError != "" {
			fmt.Fprintf(&out, "  Last error: %s\n", s.LastError)
		}

		out.WriteString("\n")
	}

	return out.String()
}

func totalAffectedServices(records []IncidentRecord) int {
	total := 0
	for _, r := range records {
		total += len(r.AffectedServices)
	}
	return total
}
