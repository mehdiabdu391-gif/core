package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Suggestion struct {
	Service        string
	Reason         string
	Recommendation string
	Current        string
	Suggested      string
	Confidence     int
}

func generateSuggestions(records []IncidentRecord) []Suggestion {
	if len(records) == 0 {
		return nil
	}

	predictions := predictFailures(records)
	risk := calculateRisk(records)

	riskByService := make(map[string]RiskScore, len(risk))
	for _, r := range risk {
		riskByService[r.Service] = r
	}

	var out []Suggestion

	for _, p := range predictions {
		r, ok := riskByService[p.Service]
		if !ok {
			continue
		}

		switch p.Trend {
		case RiskCritical:
			out = append(out, Suggestion{
				Service: p.Service,
				Reason: fmt.Sprintf(
					"%d failures occurred in the recent period with a risk score of %d.",
					p.RecentIncidents,
					r.Score,
				),
				Recommendation: "Increase restart backoff to reduce restart thrashing.",
				Suggested:      "backoff: 30s",
				Confidence:     p.Confidence,
			})

		case RiskRising:
			out = append(out, Suggestion{
				Service: p.Service,
				Reason: fmt.Sprintf(
					"Failure frequency is increasing; %d recent incidents exceed the older period.",
					p.RecentIncidents,
				),
				Recommendation: "Consider increasing restart backoff and reviewing the service dependency chain.",
				Suggested:      "review backoff and depends_on",
				Confidence:     p.Confidence,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Service < out[j].Service
	})

	return out
}

func formatSuggestReport(records []IncidentRecord) string {
	suggestions := generateSuggestions(records)

	var out strings.Builder

	out.WriteString("\nCORE SUGGEST\n")
	out.WriteString("============\n\n")

	if len(suggestions) == 0 {
		out.WriteString("No actionable recommendations from available incident history.\n")
		return out.String()
	}

	for i, s := range suggestions {
		fmt.Fprintf(&out, "%d. SERVICE: %s\n", i+1, s.Service)
		fmt.Fprintf(&out, "   Confidence: %d%%\n", s.Confidence)
		fmt.Fprintf(&out, "   Reason: %s\n", s.Reason)
		fmt.Fprintf(&out, "   Recommendation: %s\n", s.Recommendation)
		fmt.Fprintf(&out, "   Suggested change: %s\n\n", s.Suggested)
	}

	out.WriteString("IMPORTANT\n")
	out.WriteString("=========\n")
	out.WriteString("These are advisory recommendations only.\n")
	out.WriteString("Core will not modify your configuration or runtime automatically.\n")

	return out.String()
}

func formatSuggestionYAML(s Suggestion) string {
	var out strings.Builder

	out.WriteString("services:\n")
	fmt.Fprintf(&out, "  %s:\n", s.Service)

	if strings.HasPrefix(s.Suggested, "backoff:") {
		value := strings.TrimSpace(strings.TrimPrefix(s.Suggested, "backoff:"))
		fmt.Fprintf(&out, "    backoff: %s\n", strconv.Quote(value))
	}

	return out.String()
}

func suggestionGeneratedAt() time.Time {
	return time.Now().UTC()
}
