package main

import (
	"fmt"
	"sort"
)

type RiskScore struct {
	Service       string
	Failures      int
	AffectedTotal int
	Criticality   int
	Score         int
	LastError     string
}

func calculateRisk(records []IncidentRecord) []RiskScore {
	summaries := summarizeIncidents(records)

	out := make([]RiskScore, 0, len(summaries))

	for _, s := range summaries {
		// Deterministic historical risk score.
		//
		// Failure frequency is the strongest signal.
		// Criticality captures how much dependency damage occurred.
		// AffectedTotal adds cumulative blast-radius evidence.
		score := (s.Failures * 10) +
			(s.Criticality * 20) +
			s.AffectedTotal

		out = append(out, RiskScore{
			Service:       s.Service,
			Failures:      s.Failures,
			AffectedTotal: s.AffectedTotal,
			Criticality:   s.Criticality,
			Score:         score,
			LastError:     s.LastError,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Service < out[j].Service
	})

	return out
}

func formatRiskReport(records []IncidentRecord) string {
	risk := calculateRisk(records)

	if len(risk) == 0 {
		return "\nCORE RISK\n=========\nNo historical incidents recorded.\n"
	}

	var out string

	out += "\nCORE RISK\n"
	out += "=========\n\n"

	out += fmt.Sprintf("SERVICES ANALYZED: %d\n\n", len(risk))

	for i, r := range risk {
		out += fmt.Sprintf(
			"%d. %s\n"+
				"   Risk score: %d\n"+
				"   Failures: %d\n"+
				"   Criticality: %d\n"+
				"   Affected services: %d\n"+
				"   Last error: %s\n\n",
			i+1,
			r.Service,
			r.Score,
			r.Failures,
			r.Criticality,
			r.AffectedTotal,
			r.LastError,
		)
	}

	top := risk[0]

	out += "PRIORITY\n"
	out += "========\n"
	out += fmt.Sprintf(
		"Monitor %s first.\n"+
			"Historical risk score: %d\n"+
			"Historical failures: %d\n"+
			"Historical affected services: %d\n",
		top.Service,
		top.Score,
		top.Failures,
		top.AffectedTotal,
	)

	return out
}
