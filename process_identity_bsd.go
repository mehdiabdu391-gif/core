//go:build !linux && !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// sameCoreProcess reports whether pid is running the same executable as this
// process. macOS and the BSDs have no /proc, so ask ps for the command path.
func sameCoreProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	currentExe, err := os.Executable()
	if err != nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	target := strings.TrimSpace(string(out))
	if target == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
		currentExe = resolved
	}
	if filepath.IsAbs(target) {
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
		}
		return target == currentExe
	}
	return filepath.Base(target) == filepath.Base(currentExe)
}
