package main

import (
	"fmt"
	"os"
	"strings"
)

type StateStatus string

const (
	StateMissing StateStatus = "STOPPED"
	StateRunning StateStatus = "RUNNING"
	StateStale   StateStatus = "STALE"
	StateCorrupt StateStatus = "CORRUPT"
)

type StateInspection struct {
	Status StateStatus
	State  RuntimeState
	Reason string
}

func inspectState() StateInspection {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return StateInspection{
				Status: StateMissing,
				Reason: "state file does not exist",
			}
		}

		return StateInspection{
			Status: StateCorrupt,
			Reason: fmt.Sprintf("cannot read state file: %v", err),
		}
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return StateInspection{
			Status: StateCorrupt,
			Reason: "state file is empty",
		}
	}

	state, err := loadState()
	if err != nil {
		return StateInspection{
			Status: StateCorrupt,
			Reason: fmt.Sprintf("invalid state JSON: %v", err),
		}
	}

	if state.PID <= 0 {
		return StateInspection{
			Status: StateCorrupt,
			State:  state,
			Reason: "state contains an invalid supervisor PID",
		}
	}

	if state.StartedAt.IsZero() {
		return StateInspection{
			Status: StateCorrupt,
			State:  state,
			Reason: "state contains no supervisor start time",
		}
	}

	if strings.TrimSpace(state.Project) == "" {
		return StateInspection{
			Status: StateCorrupt,
			State:  state,
			Reason: "state contains no project path",
		}
	}

	if !processAlive(state.PID) {
		return StateInspection{
			Status: StateStale,
			State:  state,
			Reason: fmt.Sprintf("supervisor PID %d is no longer alive", state.PID),
		}
	}

	if state.PID == os.Getpid() {
		return StateInspection{
			Status: StateRunning,
			State:  state,
			Reason: "state belongs to the current Core process",
		}
	}

	if !sameCoreProcess(state.PID) {
		return StateInspection{
			Status: StateStale,
			State:  state,
			Reason: fmt.Sprintf("PID %d is alive but does not belong to Core", state.PID),
		}
	}

	return StateInspection{
		Status: StateRunning,
		State:  state,
		Reason: fmt.Sprintf("Core supervisor PID %d is alive", state.PID),
	}
}
