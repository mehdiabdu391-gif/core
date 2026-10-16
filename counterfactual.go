package main

import (
	"fmt"
	"sort"
	"strings"
)

type CounterfactualOutcome string

const (
	CounterfactualFailed         CounterfactualOutcome = "FAILED"
	CounterfactualDependencyLost CounterfactualOutcome = "DEPENDENCY_LOST"
	CounterfactualAtRisk         CounterfactualOutcome = "AT_RISK"
)

type CounterfactualAssessment string

const (
	CounterfactualIsolated       CounterfactualAssessment = "ISOLATED"
	CounterfactualModerateImpact CounterfactualAssessment = "MODERATE_IMPACT"
	CounterfactualHighImpact     CounterfactualAssessment = "HIGH_IMPACT"
)

type CounterfactualAnalysis struct {
	RootService         string
	Impact              FailureImpact
	Outcomes            map[string]CounterfactualOutcome
	PropagationDepth    int
	HistoricalFailure   int
	HistoricalAffected  int
	HistoricalPathCount int
	Recovery            RecoveryDecision
	Assessment          CounterfactualAssessment
	Recommendation      string
}

func AnalyzeCounterfactual(
	services map[string]Service,
	records []IncidentRecord,
	root string,
) (CounterfactualAnalysis, error) {
	if _, ok := services[root]; !ok {
		return CounterfactualAnalysis{}, fmt.Errorf("unknown service %q", root)
	}

	impact, err := BuildFailureImpact(services, root)
	if err != nil {
		return CounterfactualAnalysis{}, err
	}

	outcomes := make(map[string]CounterfactualOutcome)
	outcomes[root] = CounterfactualFailed

	for _, name := range impact.DirectDependents {
		outcomes[name] = CounterfactualDependencyLost
	}

	for _, name := range impact.Indirect {
		outcomes[name] = CounterfactualAtRisk
	}

	failures := 0
	affected := 0
	historicalPaths := 0

	for _, record := range records {
		if record.Service != root && record.RootService != root {
			continue
		}

		failures++

		if len(record.AffectedServices) > affected {
			affected = len(record.AffectedServices)
		}

		historicalPaths += len(record.Paths)
	}

	propagationDepth := 0
	for _, path := range impact.Paths {
		depth := len(path) - 1
		if depth > propagationDepth {
			propagationDepth = depth
		}
	}

	runtime := map[string]ServiceRuntime{}
	recovery, err := PlanRecovery(services, runtime, records, root)
	if err != nil {
		return CounterfactualAnalysis{}, err
	}

	assessment := assessCounterfactual(
		len(impact.AllAffected),
		len(impact.DirectDependents),
		len(impact.Indirect),
		propagationDepth,
	)

	recommendation := "No downstream dependency impact detected."

	switch {
	case assessment == CounterfactualHighImpact &&
		recovery.Action == RecoveryEscalate:
		recommendation = "Protect this service before relying on automatic recovery."
	case len(impact.AllAffected) > 0:
		recommendation = "Protect this service because its failure propagates to dependent services."
	case failures > 0:
		recommendation = "Investigate the service because historical failures provide operational evidence."
	}

	return CounterfactualAnalysis{
		RootService:         root,
		Impact:              impact,
		Outcomes:            outcomes,
		PropagationDepth:    propagationDepth,
		HistoricalFailure:   failures,
		HistoricalAffected:  affected,
		HistoricalPathCount: historicalPaths,
		Recovery:            recovery,
		Assessment:          assessment,
		Recommendation:      recommendation,
	}, nil
}

func assessCounterfactual(
	affected int,
	direct int,
	indirect int,
	depth int,
) CounterfactualAssessment {
	switch {
	case affected == 0:
		return CounterfactualIsolated
	case direct > 0 && indirect > 0:
		return CounterfactualHighImpact
	case depth >= 2:
		return CounterfactualHighImpact
	default:
		return CounterfactualModerateImpact
	}
}

func assessmentRank(assessment CounterfactualAssessment) int {
	switch assessment {
	case CounterfactualHighImpact:
		return 3
	case CounterfactualModerateImpact:
		return 2
	case CounterfactualIsolated:
		return 1
	default:
		return 0
	}
}

func RankCounterfactuals(
	services map[string]Service,
	records []IncidentRecord,
) ([]CounterfactualAnalysis, error) {
	names := make([]string, 0, len(services))

	for name := range services {
		names = append(names, name)
	}

	sort.Strings(names)

	results := make([]CounterfactualAnalysis, 0, len(names))

	for _, name := range names {
		analysis, err := AnalyzeCounterfactual(services, records, name)
		if err != nil {
			return nil, err
		}

		results = append(results, analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		if assessmentRank(results[i].Assessment) != assessmentRank(results[j].Assessment) {
			return assessmentRank(results[i].Assessment) > assessmentRank(results[j].Assessment)
		}

		if len(results[i].Impact.AllAffected) != len(results[j].Impact.AllAffected) {
			return len(results[i].Impact.AllAffected) > len(results[j].Impact.AllAffected)
		}

		if results[i].PropagationDepth != results[j].PropagationDepth {
			return results[i].PropagationDepth > results[j].PropagationDepth
		}

		if len(results[i].Impact.DirectDependents) != len(results[j].Impact.DirectDependents) {
			return len(results[i].Impact.DirectDependents) > len(results[j].Impact.DirectDependents)
		}

		if len(results[i].Impact.Indirect) != len(results[j].Impact.Indirect) {
			return len(results[i].Impact.Indirect) > len(results[j].Impact.Indirect)
		}

		if results[i].HistoricalFailure != results[j].HistoricalFailure {
			return results[i].HistoricalFailure > results[j].HistoricalFailure
		}

		if results[i].HistoricalPathCount != results[j].HistoricalPathCount {
			return results[i].HistoricalPathCount > results[j].HistoricalPathCount
		}

		return results[i].RootService < results[j].RootService
	})

	return results, nil
}

func FormatCounterfactual(a CounterfactualAnalysis) string {
	var b strings.Builder

	fmt.Fprintln(&b, "COUNTERFACTUAL ANALYSIS")
	fmt.Fprintln(&b, "======================")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "SCENARIO")
	fmt.Fprintf(&b, "  %s -> FAILED\n\n", a.RootService)

	fmt.Fprintln(&b, "PREDICTED EFFECTS")

	names := make([]string, 0, len(a.Outcomes))
	for name := range a.Outcomes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(&b, "  %-18s %s\n", name, a.Outcomes[name])
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "BLAST RADIUS")
	fmt.Fprintf(&b, "  %d service(s) affected\n", len(a.Impact.AllAffected))
	fmt.Fprintf(&b, "  %d direct dependents\n", len(a.Impact.DirectDependents))
	fmt.Fprintf(&b, "  %d indirect dependents\n", len(a.Impact.Indirect))
	fmt.Fprintf(&b, "  %d propagation depth\n", a.PropagationDepth)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "HISTORICAL EVIDENCE")
	fmt.Fprintf(&b, "  Failures involving service: %d\n", a.HistoricalFailure)
	fmt.Fprintf(&b, "  Maximum historical affected services: %d\n", a.HistoricalAffected)
	fmt.Fprintf(&b, "  Historical dependency paths: %d\n", a.HistoricalPathCount)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "RECOVERY")
	fmt.Fprintf(&b, "  Action: %s\n", a.Recovery.Action)
	fmt.Fprintf(&b, "  Reason: %s\n", a.Recovery.Reason)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "ASSESSMENT")
	fmt.Fprintf(&b, "  %s\n", a.Assessment)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "RECOMMENDATION")
	fmt.Fprintf(&b, "  %s\n", a.Recommendation)

	return b.String()
}

func FormatCounterfactualRanking(results []CounterfactualAnalysis) string {
	var b strings.Builder

	fmt.Fprintln(&b, "FAILURE SURFACE ANALYSIS")
	fmt.Fprintln(&b, "========================")
	fmt.Fprintln(&b)

	if len(results) == 0 {
		fmt.Fprintln(&b, "No services available.")
		return b.String()
	}

	for i, result := range results {
		fmt.Fprintf(&b, "#%d %s\n", i+1, result.RootService)
		fmt.Fprintf(&b, "   assessment: %s\n", result.Assessment)
		fmt.Fprintf(&b, "   downstream: %d services\n", len(result.Impact.AllAffected))
		fmt.Fprintf(&b, "   propagation depth: %d\n", result.PropagationDepth)
		fmt.Fprintf(&b, "   direct dependents: %d\n", len(result.Impact.DirectDependents))
		fmt.Fprintf(&b, "   indirect dependents: %d\n", len(result.Impact.Indirect))
		fmt.Fprintf(&b, "   historical failures: %d\n", result.HistoricalFailure)
		fmt.Fprintf(&b, "   historical paths: %d\n", result.HistoricalPathCount)
		fmt.Fprintln(&b)
	}

	return b.String()
}
