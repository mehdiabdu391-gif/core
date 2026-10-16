package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RecoveryAction string

const (
	RecoveryNoAction RecoveryAction = "NO_ACTION"
	RecoveryRestart  RecoveryAction = "RESTART"
	RecoveryEscalate RecoveryAction = "ESCALATE"
)

type RecoveryDecision struct {
	Service          string
	Action           RecoveryAction
	Reason           string
	RiskScore        int
	Failures         int
	Restarts         int
	MaxRestarts      int
	Backoff          time.Duration
	HasHealthcheck   bool
	AffectedServices []string
}

func PlanRecovery(
	services map[string]Service,
	runtime map[string]ServiceRuntime,
	records []IncidentRecord,
	root string,
) (RecoveryDecision, error) {
	service, ok := services[root]
	if !ok {
		return RecoveryDecision{}, fmt.Errorf("unknown service %q", root)
	}

	risks := calculateRisk(records)

	riskScore := 0
	failures := 0

	for _, risk := range risks {
		if risk.Service == root {
			riskScore = risk.Score
			failures = risk.Failures
			break
		}
	}

	restarts := 0
	if state, ok := runtime[root]; ok {
		restarts = state.Restarts
	}

	action := RecoveryRestart
	reason := "service failure detected; restart policy permits recovery"

	if service.Restart == RestartNo {
		action = RecoveryEscalate
		reason = "restart policy is disabled"
	}

	if service.MaxRestarts > 0 && restarts >= service.MaxRestarts {
		action = RecoveryEscalate
		reason = "maximum restart limit has been reached"
	}

	impact, err := BuildFailureImpact(services, root)
	if err != nil {
		return RecoveryDecision{}, err
	}

	return RecoveryDecision{
		Service:          root,
		Action:           action,
		Reason:           reason,
		RiskScore:        riskScore,
		Failures:         failures,
		Restarts:         restarts,
		MaxRestarts:      service.MaxRestarts,
		Backoff:          service.Backoff,
		HasHealthcheck:   service.Healthcheck != nil,
		AffectedServices: append([]string(nil), impact.AllAffected...),
	}, nil
}

func FormatRecoveryDecision(decision RecoveryDecision) string {
	var b strings.Builder

	fmt.Fprintln(&b, "CORE RECOVERY")
	fmt.Fprintln(&b, "=============")
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "SERVICE: %s\n", decision.Service)
	fmt.Fprintf(&b, "RISK SCORE: %d\n", decision.RiskScore)
	fmt.Fprintf(&b, "HISTORICAL FAILURES: %d\n", decision.Failures)
	fmt.Fprintf(&b, "CURRENT RESTARTS: %d\n", decision.Restarts)
	fmt.Fprintf(&b, "MAX RESTARTS: %d\n", decision.MaxRestarts)
	fmt.Fprintf(&b, "BACKOFF: %s\n", decision.Backoff)
	fmt.Fprintf(&b, "HEALTHCHECK: %t\n", decision.HasHealthcheck)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "AFFECTED SERVICES")

	if len(decision.AffectedServices) == 0 {
		fmt.Fprintln(&b, "  none")
	} else {
		names := append([]string(nil), decision.AffectedServices...)
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "PROPOSED ACTION")
	fmt.Fprintf(&b, "  %s\n", decision.Action)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "REASON")
	fmt.Fprintf(&b, "  %s\n", decision.Reason)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "MODE")
	fmt.Fprintln(&b, "  DRY-RUN")
	fmt.Fprintln(&b, "  ACTION NOT EXECUTED")

	return b.String()
}
