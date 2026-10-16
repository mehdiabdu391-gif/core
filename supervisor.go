package main

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type Supervisor struct {
	services map[string]Service
	gates    map[string]*serviceGate
	shutdown <-chan struct{}
	state    RuntimeState
	failed   bool
	stateMu  sync.Mutex
	wg       sync.WaitGroup
}

func NewSupervisor(services []Service, shutdown <-chan struct{}) *Supervisor {
	m := make(map[string]Service, len(services))
	g := make(map[string]*serviceGate, len(services))
	for _, s := range services {
		m[s.Name] = s
		g[s.Name] = newServiceGate()
	}
	return &Supervisor{services: m, gates: g, shutdown: shutdown, state: newRuntimeState()}
}
func (s *Supervisor) Run() error {
	if err := ValidateServices(s.services); err != nil {
		return err
	}
	if err := ensureCoreDirs(); err != nil {
		return err
	}

	s.stateMu.Lock()
	s.state.Lifecycle = CoreStarting
	err := saveState(s.state)
	s.stateMu.Unlock()
	if err != nil {
		return err
	}

	names := sortedServiceNames(s.services)
	for _, name := range names {
		s.wg.Add(1)
		go func(svc Service) {
			defer s.wg.Done()
			s.runService(svc)
		}(s.services[name])
	}

	s.setLifecycle(CoreRunning)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	interrupted := false
	select {
	case <-s.shutdown:
		interrupted = true
		s.setLifecycle(CoreStopping)
		<-done
	case <-done:
	}

	s.stateMu.Lock()
	failed := s.failed
	s.stateMu.Unlock()

	if failed {
		s.setLifecycle(CoreFailed)
	} else {
		s.setLifecycle(CoreStopped)
	}

	if err := removeState(); err != nil {
		return err
	}
	if failed && !interrupted {
		return fmt.Errorf("one or more services failed; see `core logs` and `core incidents`")
	}
	return nil
}

// stableRunWindow is how long a process must stay up after becoming ready
// before its restart counter and backoff are reset.
const stableRunWindow = 60 * time.Second

func (s *Supervisor) runService(service Service) {
	gate := s.gates[service.Name]
	deps := make([]*serviceGate, 0, len(service.DependsOn))
	for _, dep := range service.DependsOn {
		deps = append(deps, s.gates[dep])
	}

	restarts := 0
	for {
		if len(deps) > 0 {
			s.setState(service.Name, ServiceRuntime{Status: StatusWaiting, Restarts: restarts})
			if err := s.waitForDependencies(service, deps); err != nil {
				if err == errShutdown {
					s.setState(service.Name, ServiceRuntime{Status: StatusStopped, Restarts: restarts})
					return
				}
				gate.markFailed(err)
				s.markFailed(service.Name, restarts, err)
				return
			}
		}
		select {
		case <-s.shutdown:
			s.setState(service.Name, ServiceRuntime{Status: StatusStopped, Restarts: restarts})
			return
		default:
		}

		stopWatch := make(chan struct{})
		var depDown <-chan struct{}
		if len(deps) > 0 {
			depDown = watchDependencies(deps, stopWatch)
		}
		var readyAt time.Time
		s.setState(service.Name, ServiceRuntime{Status: StatusStarting, Restarts: restarts})
		depReason := func() string {
			for _, g := range deps {
				if g.isFailed() {
					return "Dependency failed"
				}
			}
			return "Dependency restarting"
		}
		result := runProcess(service, s.shutdown, depDown, depReason, func(pid int) {
			s.setState(service.Name, ServiceRuntime{PID: pid, Status: StatusRunning, Restarts: restarts, StartedAt: time.Now().UTC()})
		}, func(pid int) {
			readyAt = time.Now()
			gate.markReady()
			s.setState(service.Name, ServiceRuntime{PID: pid, Status: StatusReady, Restarts: restarts, StartedAt: time.Now().UTC()})
		})
		close(stopWatch)

		switch {
		case result.intentional:
			s.setState(service.Name, ServiceRuntime{Status: StatusStopped, Restarts: restarts})
			return
		case result.dependencyDown:
			// Not this service's fault: wait for the dependency and start again
			// without spending a restart.
			gate.markDown(result.err)
			continue
		case result.err == nil && service.Restart != RestartAlways:
			// A clean exit (for example a one-shot migration) keeps the gate
			// ready so dependents are not torn down.
			s.setState(service.Name, ServiceRuntime{Status: StatusStopped, Restarts: restarts})
			return
		}

		if result.err != nil {
			s.reportFailure(service.Name, result.err)
		}
		gate.markDown(result.err)

		if service.Restart == RestartNo {
			err := result.err
			if err == nil {
				err = fmt.Errorf("process exited")
			}
			gate.markFailed(err)
			s.markFailed(service.Name, restarts, err)
			return
		}

		if !readyAt.IsZero() && time.Since(readyAt) >= stableRunWindow {
			restarts = 0
		}
		restarts++
		if service.MaxRestarts > 0 && restarts > service.MaxRestarts {
			err := fmt.Errorf("crash loop: exceeded max_restarts=%d", service.MaxRestarts)
			gate.markFailed(err)
			s.markFailed(service.Name, restarts-1, err)
			return
		}
		delay := backoffDelay(service.Backoff, restarts)
		fmt.Printf("[%s] Restarting in %s...\n", service.Name, delay)
		s.setState(service.Name, ServiceRuntime{Status: StatusRestarting, Restarts: restarts, Error: errString(result.err)})
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-s.shutdown:
			timer.Stop()
			s.setState(service.Name, ServiceRuntime{Status: StatusStopped, Restarts: restarts})
			return
		}
	}
}

var errShutdown = errors.New("shutdown requested")

// waitForDependencies blocks until every dependency is ready. It returns an
// error naming the first dependency that failed permanently, or errShutdown.
func (s *Supervisor) waitForDependencies(service Service, deps []*serviceGate) error {
	announced := false
	for {
		allReady := true
		waits := make([]<-chan struct{}, 0, len(deps)+1)
		for i, g := range deps {
			ready, failed, err, changed := g.snapshot()
			if failed {
				if err == nil {
					err = errDependencyFailed
				}
				return fmt.Errorf("dependency %q failed: %w", service.DependsOn[i], err)
			}
			if !ready {
				allReady = false
				if !announced {
					fmt.Printf("[%s] Waiting for %s...\n", service.Name, service.DependsOn[i])
				}
			}
			waits = append(waits, changed)
		}
		if allReady {
			if announced {
				fmt.Printf("[%s] Dependencies ready\n", service.Name)
			} else {
				for _, dep := range service.DependsOn {
					fmt.Printf("[%s] Dependency ready: %s\n", service.Name, dep)
				}
			}
			return nil
		}
		announced = true
		if waitAny(append(waits, s.shutdown)) == len(waits) {
			return errShutdown
		}
	}
}

// watchDependencies returns a channel that is closed as soon as any
// dependency stops being ready. Closing stop ends the watch.
func watchDependencies(deps []*serviceGate, stop <-chan struct{}) <-chan struct{} {
	down := make(chan struct{})
	go func() {
		for {
			waits := make([]<-chan struct{}, 0, len(deps)+1)
			for _, g := range deps {
				ready, _, _, changed := g.snapshot()
				if !ready {
					close(down)
					return
				}
				waits = append(waits, changed)
			}
			if waitAny(append(waits, stop)) == len(waits) {
				return
			}
		}
	}()
	return down
}

// waitAny blocks until one of chans is closed or receives, and returns its index.
func waitAny(chans []<-chan struct{}) int {
	cases := make([]reflect.SelectCase, len(chans))
	for i, ch := range chans {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)}
	}
	i, _, _ := reflect.Select(cases)
	return i
}

func (s *Supervisor) markFailed(name string, restarts int, err error) {
	s.stateMu.Lock()
	s.failed = true
	s.stateMu.Unlock()
	s.setState(name, ServiceRuntime{Status: StatusFailed, Restarts: restarts, Error: errString(err)})
}

// setState records a service's runtime status and persists a consistent
// snapshot. The lock is held across the write so snapshots never land out of
// order and the services map is never read while another goroutine writes it.
func (s *Supervisor) setState(name string, r ServiceRuntime) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state.Services == nil {
		s.state.Services = map[string]ServiceRuntime{}
	}
	s.state.Services[name] = r
	_ = saveState(s.state)
}

func sortedServiceNames(m map[string]Service) []string {
	n := make([]string, 0, len(m))
	for k := range m {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
func backoffDelay(base time.Duration, restarts int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	d := base
	for i := 1; i < restarts; i++ {
		if d >= 30*time.Second/2 {
			d = 30 * time.Second
			break
		}
		d *= 2
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
func errString(err error) string {
	if err == nil {
		return "process exited"
	}
	return err.Error()
}

func ValidateServices(services map[string]Service) error {
	for name, s := range services {
		if !validServiceName(name) {
			return fmt.Errorf("invalid service name %q", name)
		}
		for _, dep := range s.DependsOn {
			if _, ok := services[dep]; !ok {
				return fmt.Errorf("service %q depends on unknown service %q", name, dep)
			}
		}
	}

	state := map[string]int{}
	var visit func(string, []string) error

	visit = func(name string, path []string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf(
				"dependency cycle detected: %s",
				strings.Join(append(path, name), " -> "),
			)
		case 2:
			return nil
		}

		state[name] = 1
		for _, d := range services[name].DependsOn {
			if err := visit(d, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}

	for _, n := range sortedServiceNames(services) {
		if err := visit(n, nil); err != nil {
			return err
		}
	}

	return nil
}

func (s *Supervisor) reportFailure(service string, err error) {
	impact, impactErr := BuildFailureImpact(s.services, service)
	if impactErr != nil {
		fmt.Printf("[%s] Failure analysis unavailable: %v\n", service, impactErr)
		return
	}

	now := time.Now().UTC()
	errorText := errString(err)

	record := IncidentRecord{
		ID:               newIncidentID(now, service),
		Timestamp:        now,
		Service:          service,
		Error:            errorText,
		RootService:      impact.RootService,
		DirectDependents: append([]string(nil), impact.DirectDependents...),
		Indirect:         append([]string(nil), impact.Indirect...),
		AffectedServices: append([]string(nil), impact.AllAffected...),
		Paths:            impact.Paths,
	}

	if saveErr := saveIncident(record); saveErr != nil {
		fmt.Printf("[CORE] Incident persistence failed: %v\n", saveErr)
	}

	fmt.Printf("\n[CORE] Failure intelligence\n")
	fmt.Printf("[CORE] Service: %s\n", service)
	if err != nil {
		fmt.Printf("[CORE] Error: %v\n", err)
	}
	fmt.Print(FormatFailureImpact(impact))
}

func (s *Supervisor) setLifecycle(lifecycle string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state.Lifecycle = lifecycle
	_ = saveState(s.state)
}
