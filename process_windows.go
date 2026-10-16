//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// kernel32 is declared in lock_windows.go.
var procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")

const ctrlBreakEvent = 1

// configureProcess starts each service in its own process group so it can be
// sent CTRL_BREAK on its own, without also interrupting Core.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// stopProcessTree asks the service to exit with CTRL_BREAK (the Windows
// counterpart of SIGTERM; Go programs see it as os.Interrupt and Node as
// SIGBREAK), waits up to timeout, then ends the whole tree with taskkill /F.
// If no console is attached, CTRL_BREAK can't be delivered and the tree is
// ended at once.
func stopProcessTree(cmd *exec.Cmd, done <-chan struct{}, timeout time.Duration) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if done == nil || !sendCtrlBreak(pid) {
		forceKillTree(pid)
		if done != nil {
			waitOrKill(cmd, done, timeout)
		}
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
		forceKillTree(pid)
		waitOrKill(cmd, done, timeout)
	}
}

func waitOrKill(cmd *exec.Cmd, done <-chan struct{}, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		_ = cmd.Process.Kill()
	}
}

func sendCtrlBreak(pid int) bool {
	if err := procGenerateConsoleCtrlEvent.Find(); err != nil {
		return false
	}
	r, _, _ := procGenerateConsoleCtrlEvent.Call(uintptr(ctrlBreakEvent), uintptr(pid))
	return r != 0
}

func forceKillTree(pid int) {
	_ = exec.Command("taskkill", "/PID", fmt.Sprint(pid), "/T", "/F").Run()
}

// signalProcess asks the supervisor with this PID to shut down gracefully by
// writing a stop request it polls for. Windows has no SIGTERM to send to a
// process in another console.
func signalProcess(pid int) error {
	if err := ensureCoreDirs(); err != nil {
		return err
	}
	return os.WriteFile(stopRequestFile, []byte(strconv.Itoa(pid)), 0o600)
}

// forceStopSupervisor ends the supervisor and its services when a graceful
// stop timed out.
func forceStopSupervisor(pid int) bool {
	forceKillTree(pid)
	return true
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	return !strings.Contains(string(out), "INFO:")
}
func currentPID() int { return os.Getpid() }
