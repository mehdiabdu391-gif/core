//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCoreBinaryBuilds(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "core")

	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	if _, err := os.Stat(bin); err != nil {
		t.Fatal("core binary was not produced")
	}
}

func TestCoreHelp(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "core")

	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	out, err := exec.Command(bin, "help").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "Core") {
		t.Fatal("unexpected help output")
	}
}

func TestCoreEndToEndDependencyFailure(t *testing.T) {
	tmp := t.TempDir()

	coreBin := filepath.Join(tmp, "core")
	build := exec.Command("go", "build", "-o", coreBin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building core: %v\n%s", err, out)
	}

	helper := filepath.Join(tmp, "helper")

	helperSource := filepath.Join(tmp, "helper.go")
	if err := os.WriteFile(helperSource, []byte(`
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	name := "unknown"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}

	fmt.Printf("%s READY\n", name)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	for {
		select {
		case sig := <-signals:
			fmt.Printf("%s received %s\n", name, sig)
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	buildHelper := exec.Command("go", "build", "-o", helper, helperSource)
	if out, err := buildHelper.CombinedOutput(); err != nil {
		t.Fatalf("building helper: %v\n%s", err, out)
	}

	config := fmt.Sprintf(`
services:
  db:
    command: %s
    args: ["db"]
    restart: "no"
    stop_timeout: "2s"

  api:
    command: %s
    args: ["api"]
    depends_on: ["db"]
    restart: "no"
    stop_timeout: "2s"
`, helper, helper)

	if err := os.WriteFile(filepath.Join(tmp, "core.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(coreBin, "start")
	cmd.Dir = tmp

	outputFile := filepath.Join(tmp, "core-output.log")
	out, err := os.Create(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting core: %v", err)
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}()

	statePath := filepath.Join(tmp, ".core", "state.json")

	waitFor := func(condition func() bool, timeout time.Duration, description string) {
		t.Helper()

		deadline := time.Now().Add(timeout)

		for time.Now().Before(deadline) {
			if condition() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}

		data, _ := os.ReadFile(outputFile)
		t.Fatalf("timeout waiting for %s\ncore output:\n%s", description, data)
	}

	waitFor(func() bool {
		data, err := os.ReadFile(statePath)
		if err != nil {
			return false
		}

		var current RuntimeState
		if json.Unmarshal(data, &current) != nil {
			return false
		}

		db, dbOK := current.Services["db"]
		api, apiOK := current.Services["api"]

		return dbOK &&
			apiOK &&
			db.Status == StatusReady &&
			api.Status == StatusReady
	}, 10*time.Second, "db and api to become ready")

	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var state RuntimeState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatal(err)
	}

	dbPID := state.Services["db"].PID
	apiPID := state.Services["api"].PID

	if dbPID <= 0 {
		t.Fatalf("invalid db PID: %d", dbPID)
	}

	if apiPID <= 0 {
		t.Fatalf("invalid api PID: %d", apiPID)
	}

	if err := syscall.Kill(dbPID, syscall.SIGKILL); err != nil {
		t.Fatalf("stopping db: %v", err)
	}
	waitFor(func() bool {
		data, err := os.ReadFile(outputFile)
		if err != nil {
			return false
		}
		text := string(data)
		return strings.Contains(text, "[api] Dependency failed; stopping...") &&
			strings.Contains(text, "[api] api received terminated")
	}, 10*time.Second, "api to stop after db failure")

	if err := syscall.Kill(apiPID, 0); err == nil {
		t.Fatalf("api process %d is still alive after dependency failure", apiPID)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Logf("core shutdown signal: %v", err)
	}

	_ = cmd.Wait()

}
