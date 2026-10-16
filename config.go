package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"go.yaml.in/yaml/v3"
)

const (
	maxServices    = 256
	maxServiceName = 64
	maxArgs        = 256
	maxEnv         = 256
	maxDuration    = 24 * time.Hour
)

func LoadConfig() ([]Service, error) {
	data, err := os.ReadFile("core.yaml")
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, fmt.Errorf("core.yaml is too large")
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid core.yaml: %w", err)
	}
	if len(config.Services) == 0 {
		return nil, fmt.Errorf("core.yaml contains no services")
	}
	limits, err := normalizeLogsConfig(config.Logs)
	if err != nil {
		return nil, err
	}
	setLogLimits(limits)
	if len(config.Services) > maxServices {
		return nil, fmt.Errorf("too many services: maximum is %d", maxServices)
	}
	names := make([]string, 0, len(config.Services))
	for n := range config.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	services := make([]Service, 0, len(names))
	for _, name := range names {
		s, err := normalizeService(name, config.Services[name])
		if err != nil {
			return nil, err
		}
		services = append(services, s)
	}
	return services, nil
}

func validServiceName(name string) bool {
	if name == "" || len(name) > maxServiceName || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func normalizeService(name string, cfg ServiceConfig) (Service, error) {
	if !validServiceName(name) {
		return Service{}, fmt.Errorf("invalid service name %q", name)
	}
	cmd := strings.TrimSpace(cfg.Command)
	if cmd == "" {
		return Service{}, fmt.Errorf("service %q has no command", name)
	}
	if len(cfg.Args) > maxArgs {
		return Service{}, fmt.Errorf("service %q has too many args", name)
	}
	restart := strings.ToLower(strings.TrimSpace(cfg.Restart))
	if restart == "" {
		restart = RestartNo
	}
	if restart != RestartNo && restart != RestartOnFailure && restart != RestartAlways {
		return Service{}, fmt.Errorf("service %q has invalid restart policy %q", name, cfg.Restart)
	}
	if cfg.MaxRestarts < 0 {
		return Service{}, fmt.Errorf("service %q max_restarts cannot be negative", name)
	}
	backoff, err := parseDuration(cfg.Backoff, time.Second, "backoff")
	if err != nil {
		return Service{}, fmt.Errorf("service %q: %w", name, err)
	}
	stopTimeout, err := parseDuration(cfg.StopTimeout, 5*time.Second, "stop_timeout")
	if err != nil {
		return Service{}, fmt.Errorf("service %q: %w", name, err)
	}
	if len(cfg.Environment) > maxEnv {
		return Service{}, fmt.Errorf("service %q has too many environment variables", name)
	}
	env := make([]string, 0, len(cfg.Environment))
	for k, v := range cfg.Environment {
		if !validEnvKey(k) {
			return Service{}, fmt.Errorf("service %q has invalid environment key %q", name, k)
		}
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	wd := strings.TrimSpace(cfg.WorkingDir)
	if cfg.Healthcheck != nil {
		if err := validateHealthcheck(*cfg.Healthcheck); err != nil {
			return Service{}, fmt.Errorf("service %q: %w", name, err)
		}
	}
	deps := append([]string(nil), cfg.DependsOn...)
	return Service{Name: name, Command: cmd, Args: append([]string(nil), cfg.Args...), DependsOn: deps, Restart: restart, MaxRestarts: cfg.MaxRestarts, Backoff: backoff, Environment: env, WorkingDir: wd, StopTimeout: stopTimeout, Healthcheck: cfg.Healthcheck}, nil
}

func parseDuration(v string, def time.Duration, field string) (time.Duration, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	if d <= 0 || d > maxDuration {
		return 0, fmt.Errorf("%s must be >0 and <=24h", field)
	}
	return d, nil
}
func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		if i == 0 && (r >= '0' && r <= '9') {
			return false
		}
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
func validateHealthcheck(h Healthcheck) error {
	t := strings.ToLower(strings.TrimSpace(h.Type))
	if t != "tcp" && t != "http" && t != "command" {
		return fmt.Errorf("unknown healthcheck type %q", h.Type)
	}
	if h.Retries < 0 || h.Retries > 100000 {
		return fmt.Errorf("healthcheck retries out of range")
	}
	if h.Interval != "" {
		if _, err := parseDuration(h.Interval, time.Second, "healthcheck interval"); err != nil {
			return err
		}
	}
	if h.Timeout != "" {
		if _, err := parseDuration(h.Timeout, 2*time.Second, "healthcheck timeout"); err != nil {
			return err
		}
	}
	switch t {
	case "tcp":
		if h.Port < 1 || h.Port > 65535 {
			return fmt.Errorf("tcp healthcheck requires port 1-65535")
		}
	case "http":
		if strings.TrimSpace(h.URL) == "" {
			return fmt.Errorf("http healthcheck requires url")
		}
	case "command":
		if len(h.Command) == 0 || len(h.Command) > maxArgs {
			return fmt.Errorf("command healthcheck requires 1-%d args", maxArgs)
		}
	}
	return nil
}

func servicesFromConfig() ([]Service, error) { return LoadConfig() }
