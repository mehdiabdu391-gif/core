//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func sameCoreProcess(pid int) bool {
	if pid <= 0 {
		return false
	}

	current := filepath.Base(os.Args[0])
	current = strings.ToLower(strings.TrimSuffix(current, ".exe"))

	out, err := exec.Command(
		"tasklist",
		"/FI",
		"PID eq "+strconv.Itoa(pid),
		"/FO",
		"CSV",
		"/NH",
	).Output()
	if err != nil {
		return false
	}

	fields := strings.Split(string(out), ",")
	if len(fields) == 0 {
		return false
	}

	image := strings.Trim(fields[0], "\"")
	image = strings.ToLower(strings.TrimSuffix(image, ".exe"))

	return image == current
}
