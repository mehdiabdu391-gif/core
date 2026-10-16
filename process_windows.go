//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func configureProcess(cmd *exec.Cmd) {}
func stopProcessTree(cmd *exec.Cmd, done <-chan struct{}, timeout time.Duration) {
	if cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/PID", fmt.Sprint(cmd.Process.Pid), "/T", "/F")
	_ = kill.Run()
	if done != nil {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			_ = cmd.Process.Kill()
		}
	}
}
func signalProcess(pid int) error {
	return exec.Command("taskkill", "/PID", fmt.Sprint(pid), "/T", "/F").Run()
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
