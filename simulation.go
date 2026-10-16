package main

import (
	"fmt"
	"sort"
	"strings"
)

type SimulationOutcome string

const (
	SimulationFailed   SimulationOutcome = "FAILED"
	SimulationImpacted SimulationOutcome = "IMPACTED"
)

type SimulationResult struct {
	RootService string
	Impact      FailureImpact
	Outcomes    map[string]SimulationOutcome
}

func SimulateFailure(services map[string]Service, root string) (SimulationResult, error) {
	impact, err := BuildFailureImpact(services, root)
	if err != nil {
		return SimulationResult{}, err
	}

	outcomes := make(map[string]SimulationOutcome)

	outcomes[root] = SimulationFailed

	for _, service := range impact.AllAffected {
		outcomes[service] = SimulationImpacted
	}

	return SimulationResult{
		RootService: root,
		Impact:      impact,
		Outcomes:    outcomes,
	}, nil
}

func FormatSimulation(result SimulationResult) string {
	var b strings.Builder

	fmt.Fprintln(&b, "FAILURE SIMULATION")
	fmt.Fprintln(&b, "==================")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "ROOT FAILURE")
	fmt.Fprintf(&b, "  %s\n\n", result.RootService)

	fmt.Fprintln(&b, "DIRECT DEPENDENTS")
	if len(result.Impact.DirectDependents) == 0 {
		fmt.Fprintln(&b, "  none")
	} else {
		for _, name := range result.Impact.DirectDependents {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "INDIRECT DEPENDENTS")
	if len(result.Impact.Indirect) == 0 {
		fmt.Fprintln(&b, "  none")
	} else {
		for _, name := range result.Impact.Indirect {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	fmt.Fprintf(&b, "\nTOTAL AFFECTED: %d\n", len(result.Impact.AllAffected))

	if len(result.Impact.Paths) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "DEPENDENCY PATHS")

		names := make([]string, 0, len(result.Impact.Paths))
		for name := range result.Impact.Paths {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			fmt.Fprintf(&b, "  %s\n", strings.Join(result.Impact.Paths[name], " -> "))
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "OUTCOME")

	names := make([]string, 0, len(result.Outcomes))
	for name := range result.Outcomes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(&b, "  %-16s %s\n", name, result.Outcomes[name])
	}

	return b.String()
}
