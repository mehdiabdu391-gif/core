package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const coreDir = ".core"
const stateFile = ".core/state.json"
const logDir = ".core/logs"

var stateMu sync.Mutex

func ensureCoreDirs() error {
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	return nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp := path + fmt.Sprintf(".%d.tmp", os.Getpid())
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func saveState(state RuntimeState) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	if err := ensureCoreDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(stateFile, data, 0o600)
}
func loadState() (RuntimeState, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return RuntimeState{}, err
	}
	var s RuntimeState
	if err := json.Unmarshal(data, &s); err != nil {
		return RuntimeState{}, err
	}
	return s, nil
}
func removeState() error {
	err := os.Remove(stateFile)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func newRuntimeState() RuntimeState {
	project, _ := os.Getwd()
	return RuntimeState{PID: os.Getpid(), StartedAt: time.Now().UTC(), Project: project, Services: map[string]ServiceRuntime{}}
}
func stateExists() bool { _, err := os.Stat(stateFile); return err == nil }
func ensureNoRunningCore() error {
	inspection := inspectState()

	switch inspection.Status {
	case StateMissing:
		return nil

	case StateStale:
		return removeState()

	case StateRunning:
		return fmt.Errorf("Core is already running (PID %d)", inspection.State.PID)

	case StateCorrupt:
		return fmt.Errorf("Core state is corrupt: %s", inspection.Reason)

	default:
		return fmt.Errorf("unknown Core state status: %s", inspection.Status)
	}
}
func lockProject() (*os.File, error) {
	if err := ensureCoreDirs(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(coreDir, "core.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := acquireProjectLock(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("Core is already running in this project")
	}
	return f, nil
}
