package main

import (
	"testing"
	"time"
)

func predictionTestIncident(service string, timestamp time.Time) IncidentRecord {
	return IncidentRecord{
		ID:        timestamp.Format(time.RFC3339Nano),
		Timestamp: timestamp,
		Service:   service,
		Error:     "test failure",
	}
}

func findPrediction(predictions []FailurePrediction, service string) FailurePrediction {
	for _, p := range predictions {
		if p.Service == service {
			return p
		}
	}
	return FailurePrediction{}
}

func TestPredictFailuresTemporalBehavior(t *testing.T) {
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		times         []time.Time
		wantTrend     RiskTrend
		maxConfidence int
	}{
		{
			name: "three failures over two hours",
			times: []time.Time{
				base,
				base.Add(1 * time.Hour),
				base.Add(2 * time.Hour),
			},
			wantTrend:     RiskCritical,
			maxConfidence: 95,
		},
		{
			name: "three failures spread across twenty nine days",
			times: []time.Time{
				base.Add(-29 * 24 * time.Hour),
				base.Add(-14 * 24 * time.Hour),
				base,
			},
			wantTrend:     RiskStable,
			maxConfidence: 60,
		},
		{
			name: "ten failures over five minutes",
			times: func() []time.Time {
				out := make([]time.Time, 10)
				for i := range out {
					out[i] = base.Add(time.Duration(i) * 30 * time.Second)
				}
				return out
			}(),
			wantTrend:     RiskCritical,
			maxConfidence: 98,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var records []IncidentRecord

			for _, ts := range tt.times {
				records = append(records, predictionTestIncident("test", ts))
			}

			predictions := predictFailures(records)
			got := findPrediction(predictions, "test")

			if got.Trend != tt.wantTrend {
				t.Fatalf(
					"trend = %s, want %s; reason=%s; interval=%s",
					got.Trend,
					tt.wantTrend,
					got.Reason,
					got.AverageInterval,
				)
			}

			if got.Confidence > tt.maxConfidence {
				t.Fatalf(
					"confidence = %d, unexpectedly above %d",
					got.Confidence,
					tt.maxConfidence,
				)
			}

			if got.AverageInterval <= 0 {
				t.Fatal("expected a positive average failure interval")
			}
		})
	}
}
