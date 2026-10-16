//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
)

// sameCoreProcess reports whether pid is running the same executable as this
// process, which guards against acting on a PID the OS has reused.
func sameCoreProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	currentExe, err := os.Executable()
	if err != nil {
		return false
	}
	targetExe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return false
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return false
	}
	targetExe, err = filepath.EvalSymlinks(targetExe)
	if err != nil {
		return false
	}
	return currentExe == targetExe
}
