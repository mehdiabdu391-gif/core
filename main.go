package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "start", "run":
		run()
	case "stop":
		stop()
	case "restart":
		restart()
	case "status":
		status()
	case "incidents":
		incidents()
	case "risk":
		risk()
	case "predict":
		predict()
	case "suggest":
		suggest()
	case "explain":
		explain()
	case "attach":
		attachService()
	case "simulate":
		simulate()
	case "what-if":
		whatIf()
	case "cut":
		minimumCut()
	case "recover":
		recoverService()
	case "logs":
		logs()
	case "validate":
		validate()
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}
func suggest() {
	records, err := listIncidents()
	if err != nil {
		fmt.Printf("Unable to read incident history: %v\n", err)
		return
	}
	fmt.Print(formatSuggestReport(records))
}

func predict() {
	records, err := listIncidents()
	if err != nil {
		fmt.Printf("Unable to read incident history: %v\n", err)
		return
	}
	fmt.Print(formatPredictionReport(records))
}

func risk() {
	records, err := listIncidents()
	if err != nil {
		fmt.Printf("Unable to read incident history: %v\n", err)
		return
	}
	fmt.Print(formatRiskReport(records))
}

func run() {
	lock, err := lockProject()
	if err != nil {
		fatal(err)
	}
	defer func() {
		_ = releaseProjectLock(lock)
		_ = lock.Close()
	}()
	if err := ensureNoRunningCore(); err != nil {
		fatal(err)
	}
	services, err := DetectServices()
	if err != nil {
		fatal(err)
	}
	if len(services) == 0 {
		fmt.Println("No services detected. Create core.yaml or add a supported project file.")
		return
	}
	shutdown := make(chan struct{})
	stopRequested, stopWatch := watchStopRequest(os.Getpid())
	defer close(stopWatch)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	sup := NewSupervisor(services, shutdown)
	done := make(chan error, 1)
	go func() { done <- sup.Run() }()
	select {
	case sig := <-signals:
		fmt.Printf("\nReceived %s; stopping...\n", sig)
		close(shutdown)
		if err := <-done; err != nil {
			fatal(err)
		}
	case <-stopRequested:
		fmt.Println("\nStop requested; stopping...")
		close(shutdown)
		if err := <-done; err != nil {
			fatal(err)
		}
	case err := <-done:
		if err != nil {
			fatal(err)
		}
	}
}

// stopRunningCore asks a running supervisor to shut down and waits for it.
// It reports whether a supervisor was running.
func stopRunningCore() (bool, error) {
	inspection := inspectState()
	switch inspection.Status {
	case StateMissing:
		return false, nil
	case StateStale:
		_ = removeState()
		return false, nil
	case StateCorrupt:
		return false, fmt.Errorf("Core state is corrupt (%s); remove %s if no supervisor is running", inspection.Reason, stateFile)
	}
	pid := inspection.State.PID
	fmt.Printf("Stopping Core (PID %d)...\n", pid)
	if err := signalProcess(pid); err != nil {
		return true, err
	}
	for i := 0; i < 300; i++ {
		if !processAlive(pid) {
			_ = removeState()
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if forceStopSupervisor(pid) {
		for i := 0; i < 50; i++ {
			if !processAlive(pid) {
				_ = removeState()
				_ = os.Remove(stopRequestFile)
				fmt.Println("Core did not stop within 30 seconds and was ended forcefully.")
				return true, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return true, fmt.Errorf("Core (PID %d) did not exit within 30 seconds", pid)
}

const stopRequestFile = ".core/stop.request"

// watchStopRequest polls for a stop request addressed to pid (written by
// "core stop" on Windows) and closes the returned channel when one arrives.
// A leftover request from an earlier run is removed first. Close quit to
// end the watch.
func watchStopRequest(pid int) (<-chan struct{}, chan struct{}) {
	_ = os.Remove(stopRequestFile)
	requested := make(chan struct{})
	quit := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				data, err := os.ReadFile(stopRequestFile)
				if err != nil {
					continue
				}
				_ = os.Remove(stopRequestFile)
				if strings.TrimSpace(string(data)) == strconv.Itoa(pid) {
					close(requested)
					return
				}
			}
		}
	}()
	return requested, quit
}

func stop() {
	running, err := stopRunningCore()
	if err != nil {
		fatal(err)
	}
	if !running {
		fmt.Println("Core is not running.")
		return
	}
	fmt.Println("Core stopped.")
}

func restart() {
	if _, err := stopRunningCore(); err != nil {
		fatal(err)
	}
	run()
}

func loadServiceMap() (map[string]Service, error) {
	services, err := DetectServices()
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no services detected; create core.yaml or add a supported project file")
	}
	m := make(map[string]Service, len(services))
	for _, svc := range services {
		m[svc.Name] = svc
	}
	return m, nil
}

func status() {
	inspection := inspectState()

	if inspection.Status == StateMissing {
		fmt.Println("CORE STATUS")
		fmt.Println("Status: STOPPED")
		fmt.Println("Reason: no Core state exists")
		return
	}

	s := inspection.State

	fmt.Printf("CORE STATUS\nSupervisor: %d\nProject: %s\nStarted: %s\nStatus: %s\nReason: %s\n\n",
		s.PID,
		s.Project,
		s.StartedAt.Format(time.RFC3339),
		inspection.Status,
		inspection.Reason,
	)

	names := make([]string, 0, len(s.Services))
	for n := range s.Services {
		names = append(names, n)
	}
	sort.Strings(names)

	fmt.Printf("%-18s %-8s %-14s %-8s\n", "SERVICE", "PID", "STATUS", "RESTARTS")
	for _, n := range names {
		r := s.Services[n]
		fmt.Printf("%-18s %-8s %-14s %-8d\n", n, pidString(r.PID), r.Status, r.Restarts)
	}
}
func logs() {
	service := ""
	follow := false
	for _, a := range os.Args[2:] {
		if a == "-f" || a == "--follow" {
			follow = true
		} else if service == "" {
			service = a
		} else {
			fatal(fmt.Errorf("unexpected argument %q", a))
		}
	}
	if err := showLogs(service, follow); err != nil {
		fatal(err)
	}
}
func validate() {
	services, err := DetectServices()
	if err != nil {
		fatal(err)
	}
	if len(services) == 0 {
		fatal(fmt.Errorf("no services detected; create core.yaml or add a supported project file"))
	}
	m := map[string]Service{}
	for _, s := range services {
		m[s.Name] = s
	}
	if err := ValidateServices(m); err != nil {
		fatal(err)
	}
	fmt.Printf("Configuration valid: %d service(s)\n", len(services))
}
func pidString(pid int) string {
	if pid == 0 {
		return "-"
	}
	return strconv.Itoa(pid)
}
func fatal(err error) { fmt.Fprintf(os.Stderr, "Error: %v\n", err); os.Exit(1) }
func printHelp() {
	fmt.Print(`Core — lightweight process supervisor

Usage:
  core [command] [args]

Commands:
  start               Start detected/configured services (default)
  run                 Alias for start
  stop                Stop the running Core supervisor
  restart             Stop the running supervisor and start again here
  status              Show service status
  logs [NAME] [-f]    Show all logs or one service's log; -f follows
  validate            Validate detected/configured services
  explain SERVICE     Explain a service's place in the dependency graph
  attach NAME PATH    Add an executable to core.yaml as a service
  incidents           Show incident history
  risk                Show historical failure risk
  predict             Show failure predictions
  suggest             Suggest configuration changes from observed failures
  simulate SERVICE    Simulate failure impact
  what-if SERVICE     Counterfactual impact (or --rank)
  cut SERVICE         Minimum failure set (or --rank)
  recover SERVICE     Plan a safe recovery
  help                Show help

Configuration:
  core.yaml           Optional explicit service configuration
`)
}

func incidents() {
	records, err := listIncidents()
	if err != nil {
		fmt.Printf("Failed to read incident history: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(formatIncidentHistory(records))
}

func simulate() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: core simulate SERVICE")
		return
	}

	serviceMap, err := loadServiceMap()
	if err != nil {
		fatal(err)
	}

	result, err := SimulateFailure(serviceMap, os.Args[2])
	if err != nil {
		fmt.Printf("Simulation failed: %v\n", err)
		return
	}

	fmt.Print(FormatSimulation(result))
}

func recoverService() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: core recover SERVICE")
		return
	}

	services, err := loadServiceMap()
	if err != nil {
		fatal(err)
	}

	records, err := listIncidents()
	if err != nil {
		fmt.Printf("Unable to read incident history: %v\n", err)
		return
	}

	decision, err := PlanRecovery(
		services,
		map[string]ServiceRuntime{},
		records,
		os.Args[2],
	)
	if err != nil {
		fmt.Printf("Unable to plan recovery: %v\n", err)
		return
	}

	fmt.Print(FormatRecoveryDecision(decision))
}

func whatIf() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: core what-if SERVICE")
		fmt.Println("       core what-if --rank")
		return
	}

	services, err := loadServiceMap()
	if err != nil {
		fatal(err)
	}

	records, err := listIncidents()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	switch os.Args[2] {
	case "--rank":
		if len(os.Args) != 3 {
			fmt.Println("Usage: core what-if --rank")
			return
		}

		results, err := RankCounterfactuals(services, records)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Print(FormatCounterfactualRanking(results))

	default:
		if len(os.Args) != 3 {
			fmt.Println("Usage: core what-if SERVICE")
			return
		}

		result, err := AnalyzeCounterfactual(
			services,
			records,
			os.Args[2],
		)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Print(FormatCounterfactual(result))
	}
}

func minimumCut() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: core cut SERVICE")
		fmt.Println("       core cut --rank")
		return
	}

	services, err := loadServiceMap()
	if err != nil {
		fatal(err)
	}

	if os.Args[2] == "--rank" {
		if len(os.Args) != 3 {
			fmt.Println("Usage: core cut --rank")
			return
		}

		results := make([]MinimumCut, 0, len(services))

		names := make([]string, 0, len(services))
		for name := range services {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			result, err := AnalyzeMinimumCut(services, name)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			results = append(results, result)
		}

		sort.Slice(results, func(i, j int) bool {
			if results[i].Isolated != results[j].Isolated {
				return !results[i].Isolated
			}

			if results[i].Size != results[j].Size {
				return results[i].Size < results[j].Size
			}

			return results[i].Target < results[j].Target
		})

		fmt.Println("MINIMUM FAILURE CUT ANALYSIS")
		fmt.Println("=============================")
		fmt.Println()

		for i, result := range results {
			fmt.Printf("#%d %s\n", i+1, result.Target)

			if result.Isolated {
				fmt.Println("   cut: none")
			} else {
				fmt.Printf("   cut size: %d\n", result.Size)
				fmt.Printf("   failure set: %v\n", result.Services)
			}

			fmt.Println()
		}

		return
	}

	if len(os.Args) != 3 {
		fmt.Println("Usage: core cut SERVICE")
		return
	}

	result, err := AnalyzeMinimumCut(services, os.Args[2])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Print(FormatMinimumCut(result))
}
