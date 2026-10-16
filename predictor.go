package main

import (
	"fmt"
	"sort"
	"time"
)

type RiskTrend string

const (
	RiskInsufficient RiskTrend = "INSUFFICIENT_DATA"
	RiskStable       RiskTrend = "STABLE"
	RiskRising       RiskTrend = "RISING"
	RiskCritical     RiskTrend = "CRITICAL"
)

type FailurePrediction struct {
	Service         string
	Trend           RiskTrend
	Incidents       int
	RecentIncidents int
	OlderIncidents  int
	AverageInterval time.Duration
	LastIncident    time.Time
	Confidence      int
	Reason          string
}

func predictFailures(records []IncidentRecord) []FailurePrediction {
	grouped := make(map[string][]IncidentRecord)

	for _, record := range records {
		grouped[record.Service] = append(grouped[record.Service], record)
	}

	out := make([]FailurePrediction, 0, len(grouped))

	for service, incidents := range grouped {
		sort.Slice(incidents, func(i, j int) bool {
			return incidents[i].Timestamp.Before(incidents[j].Timestamp)
		})

		p := FailurePrediction{
			Service:   service,
			Incidents: len(incidents),
		}

		p.LastIncident = incidents[len(incidents)-1].Timestamp

		if len(incidents) < 2 {
			p.Trend = RiskInsufficient
			p.RecentIncidents = 1
			p.Confidence = 0
			p.Reason = "not enough historical failures"
			out = append(out, p)
			continue
		}

		for i := 1; i < len(incidents); i++ {
			p.AverageInterval += incidents[i].Timestamp.Sub(incidents[i-1].Timestamp)
		}

		p.AverageInterval /= time.Duration(len(incidents) - 1)

		cutoff := p.LastIncident.Add(-30 * 24 * time.Hour)

		for _, incident := range incidents {
			if !incident.Timestamp.Before(cutoff) {
				p.RecentIncidents++
			} else {
				p.OlderIncidents++
			}
		}

		/*
			Temporal classification.

			The interval between failures is the primary signal.
			The recent/older distribution is supporting evidence.

			This prevents a service with occasional failures over
			weeks from being incorrectly classified as unstable.
		*/
		switch {
		case p.AverageInterval <= 10*time.Minute && p.RecentIncidents >= 2:
			p.Trend = RiskCritical
			p.Confidence = minInt(98, 85+p.RecentIncidents*3)
			p.Reason = "failures are occurring at a very high frequency"

		case p.AverageInterval <= time.Hour && p.RecentIncidents >= 3:
			p.Trend = RiskCritical
			p.Confidence = minInt(95, 75+p.RecentIncidents*3)
			p.Reason = "repeated recent failures indicate severe instability"

		case p.AverageInterval <= 24*time.Hour && p.RecentIncidents >= 3:
			p.Trend = RiskRising
			p.Confidence = 75
			p.Reason = "recent failures are recurring within a short operational interval"

		case p.AverageInterval <= 7*24*time.Hour && p.RecentIncidents > p.OlderIncidents:
			p.Trend = RiskRising
			p.Confidence = 70
			p.Reason = "failure frequency is increasing within a short operational interval"

		default:
			p.Trend = RiskStable
			p.Confidence = 60
			p.Reason = "failures recur, but no accelerating pattern was detected"
		}

		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		rank := func(t RiskTrend) int {
			switch t {
			case RiskCritical:
				return 3
			case RiskRising:
				return 2
			case RiskStable:
				return 1
			default:
				return 0
			}
		}

		if rank(out[i].Trend) != rank(out[j].Trend) {
			return rank(out[i].Trend) > rank(out[j].Trend)
		}

		return out[i].Service < out[j].Service
	})

	return out
}

func formatPredictionReport(records []IncidentRecord) string {
	predictions := predictFailures(records)

	var out string
	out += "\nFAILURE PREDICTION\n"
	out += "==================\n\n"

	if len(predictions) == 0 {
		out += "No historical incidents recorded.\n"
		return out
	}

	for _, p := range predictions {
		out += fmt.Sprintf(
			"SERVICE: %s\n"+
				"  Trend: %s\n"+
				"  Incidents: %d\n"+
				"  Recent incidents: %d\n"+
				"  Older incidents: %d\n"+
				"  Confidence: %d%%\n"+
				"  Reason: %s\n",
			p.Service,
			p.Trend,
			p.Incidents,
			p.RecentIncidents,
			p.OlderIncidents,
			p.Confidence,
			p.Reason,
		)

		if p.AverageInterval > 0 {
			out += fmt.Sprintf(
				"  Average failure interval: %s\n",
				p.AverageInterval.Round(time.Second),
			)
		}

		out += "\n"
	}

	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
