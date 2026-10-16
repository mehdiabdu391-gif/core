package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func isCoreProject() bool {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1] == "core"
		}
	}

	return false
}

func DetectServices() ([]Service, error) {
	if fileExists("core.yaml") {
		return servicesFromConfig()
	}

	if fileExists("package.json") {
		s, err := detectNode()
		if err != nil {
			return nil, err
		}
		if len(s) > 0 {
			return s, nil
		}
	}

	if fileExists("docker-compose.yml") || fileExists("docker-compose.yaml") {
		return []Service{{
			Name:        "docker",
			Command:     "docker",
			Args:        []string{"compose", "up"},
			Restart:     RestartNo,
			StopTimeout: 10 * time.Second,
		}}, nil
	}

	if fileExists("go.mod") && !isCoreProject() {
		return []Service{{
			Name:        "go",
			Command:     "go",
			Args:        []string{"run", "."},
			Restart:     RestartOnFailure,
			Backoff:     time.Second,
			StopTimeout: 5 * time.Second,
		}}, nil
	}

	p, err := detectPython()
	if err != nil {
		return nil, err
	}
	if p != nil {
		return []Service{*p}, nil
	}

	return nil, nil
}

func detectNode() ([]Service, error) {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return nil, fmt.Errorf("reading package.json: %w", err)
	}
	if len(data) > 4<<20 {
		return nil, fmt.Errorf("package.json is too large")
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}

	for _, script := range []string{"dev", "start", "serve"} {
		if _, ok := pkg.Scripts[script]; ok {
			return []Service{{
				Name:        "web",
				Command:     "npm",
				Args:        []string{"run", script},
				Restart:     RestartOnFailure,
				Backoff:     time.Second,
				StopTimeout: 5 * time.Second,
			}}, nil
		}
	}

	return nil, nil
}

func detectPython() (*Service, error) {
	for _, f := range []string{"main.py", "app.py", "server.py"} {
		if fileExists(f) {
			return &Service{
				Name:        "python",
				Command:     pythonCommand(),
				Args:        []string{f},
				Restart:     RestartOnFailure,
				Backoff:     time.Second,
				StopTimeout: 5 * time.Second,
			}, nil
		}
	}

	files, err := filepath.Glob("*.py")
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	if len(files) == 0 {
		return nil, nil
	}

	return &Service{
		Name:        "python",
		Command:     pythonCommand(),
		Args:        []string{files[0]},
		Restart:     RestartOnFailure,
		Backoff:     time.Second,
		StopTimeout: 5 * time.Second,
	}, nil
}

// pythonCommand prefers python3, which is the only name on many systems.
func pythonCommand() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
