package main

import (
	"errors"
	"sync"
	"time"
)

var errDependencyFailed = errors.New("dependency failed")

const (
	RestartNo        = "no"
	RestartOnFailure = "on-failure"
	RestartAlways    = "always"
)

const (
	StatusWaiting    = "waiting"
	StatusStarting   = "starting"
	StatusRunning    = "running"
	StatusReady      = "ready"
	StatusUnhealthy  = "unhealthy"
	StatusRestarting = "restarting"
	StatusStopped    = "stopped"
	StatusFailed     = "failed"
)

type Config struct {
	Services map[string]ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	DependsOn   []string          `yaml:"depends_on"`
	Restart     string            `yaml:"restart"`
	MaxRestarts int               `yaml:"max_restarts"`
	Backoff     string            `yaml:"backoff"`
	Environment map[string]string `yaml:"environment"`
	WorkingDir  string            `yaml:"working_dir"`
	StopTimeout string            `yaml:"stop_timeout"`
	Healthcheck *Healthcheck      `yaml:"healthcheck"`
}

type Healthcheck struct {
	Type     string   `yaml:"type"`
	URL      string   `yaml:"url"`
	Host     string   `yaml:"host"`
	Port     int      `yaml:"port"`
	Command  []string `yaml:"command"`
	Interval string   `yaml:"interval"`
	Timeout  string   `yaml:"timeout"`
	Retries  int      `yaml:"retries"`
}

type Service struct {
	Name        string
	Command     string
	Args        []string
	DependsOn   []string
	Restart     string
	MaxRestarts int
	Backoff     time.Duration
	Environment []string
	WorkingDir  string
	StopTimeout time.Duration
	Healthcheck *Healthcheck
}

type ServiceRuntime struct {
	PID       int       `json:"pid"`
	Status    string    `json:"status"`
	Restarts  int       `json:"restarts"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error,omitempty"`
}

type RuntimeState struct {
	PID       int                       `json:"pid"`
	StartedAt time.Time                 `json:"started_at"`
	Project   string                    `json:"project"`
	Lifecycle string                    `json:"lifecycle"`
	Services  map[string]ServiceRuntime `json:"services"`
}

// serviceGate publishes a service's readiness to its dependents.
//
// A gate moves between three states:
//   - ready:   the service is up (and healthy, when it has a healthcheck)
//   - down:    the service is not ready but may still recover (starting or restarting)
//   - failed:  the service has stopped for good; dependents must give up
//
// Every state change closes the current changed channel and replaces it, so
// any number of waiters can observe transitions without missing one.
type serviceGate struct {
	mu      sync.Mutex
	ready   bool
	failed  bool
	err     error
	changed chan struct{}
}

func newServiceGate() *serviceGate {
	return &serviceGate{changed: make(chan struct{})}
}

func (g *serviceGate) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

// snapshot returns the current state and a channel closed on the next change.
func (g *serviceGate) snapshot() (ready, failed bool, err error, changed <-chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ready, g.failed, g.err, g.changed
}

// markReady records that the service is up. It has no effect once failed.
func (g *serviceGate) markReady() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failed || g.ready {
		return
	}
	g.ready = true
	g.err = nil
	g.notifyLocked()
}

// markDown records that the service is not ready but may recover.
func (g *serviceGate) markDown(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failed || !g.ready {
		if err != nil && !g.failed {
			g.err = err
		}
		return
	}
	g.ready = false
	g.err = err
	g.notifyLocked()
}

// markFailed records that the service has stopped permanently.
func (g *serviceGate) markFailed(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failed {
		return
	}
	if err == nil {
		err = errDependencyFailed
	}
	g.ready = false
	g.failed = true
	g.err = err
	g.notifyLocked()
}

func (g *serviceGate) isReady() bool  { r, _, _, _ := g.snapshot(); return r }
func (g *serviceGate) isFailed() bool { _, f, _, _ := g.snapshot(); return f }
func (g *serviceGate) failure() error {
	_, _, err, _ := g.snapshot()
	if err == nil {
		return errDependencyFailed
	}
	return err
}

const (
	CoreStarting   = "starting"
	CoreRunning    = "running"
	CoreStopping   = "stopping"
	CoreStopped    = "stopped"
	CoreFailed     = "failed"
	CoreRecovering = "recovering"
)
