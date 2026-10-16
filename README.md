# Core

Core is a small, local process supervisor. One binary loads (or detects) your service graph, starts services in dependency order, waits for readiness, restarts failures with bounded backoff, streams and stores logs, and cleans up whole process trees on shutdown.

It also keeps a history of failures and can reason about it: which services are risky, what a failure would take down, and the smallest set of failures that isolates a service.

## Build

Requires Go 1.23 or newer.

```sh
go build -o core .
go test ./...
```

Builds on Linux, macOS and Windows.

## Quick start

```sh
cp core.yaml.example core.yaml   # edit to taste
./core validate                  # check the config before starting
./core                           # start everything (Ctrl-C to stop)
```

From another terminal in the same project:

```sh
./core status
./core logs api -f
./core stop
```

Without a `core.yaml`, Core detects a single service from the project: `package.json` (`dev`, `start` or `serve` script), `docker-compose.yml`, `go.mod`, or a Python entry point (`main.py`, `app.py`, `server.py`).

## Configuration

```yaml
services:
  database:
    command: docker
    args: [compose, up, postgres]
    restart: always
    healthcheck:
      type: tcp            # tcp | http | command
      port: 5432
      interval: 2s
      timeout: 1s
      retries: 30

  api:
    command: go
    args: [run, ./api]
    depends_on: [database]
    restart: on-failure    # no (default) | on-failure | always
    max_restarts: 10       # 0 = unlimited
    backoff: 1s            # doubles per restart, capped at 30s
    stop_timeout: 5s       # SIGTERM, then SIGKILL after this
    working_dir: ./api
    environment:
      PORT: "8080"
    healthcheck:
      type: http
      url: http://127.0.0.1:8080/health
```

Field reference:

| Field | Meaning |
|---|---|
| `command`, `args` | Executable and arguments. Not run through a shell. |
| `depends_on` | Services that must be ready before this one starts. |
| `restart` | `no`, `on-failure` (non-zero exit or failed healthcheck) or `always`. |
| `max_restarts` | Restarts allowed before the service is marked failed. `0` means unlimited. |
| `backoff` | First restart delay; doubles each time up to 30s. |
| `stop_timeout` | Grace period between SIGTERM and SIGKILL. Default 5s. |
| `healthcheck` | `tcp` (`host`, `port`), `http` (`url`, 2xx/3xx passes, redirects are not followed) or `command` (exit 0 passes). `interval` default 1s, `timeout` default 2s, `retries` default 30. |

## How supervision works

- **Readiness.** A service is ready when its healthcheck passes, or as soon as it starts if it has none.
- **Dependencies.** A service starts only once every dependency is ready. If a dependency goes down, its dependents are stopped and wait. When the dependency is ready again they start again, and this does not count against their own `max_restarts`. If a dependency fails for good (restart policy `no`, or it exceeded `max_restarts`), its dependents are marked failed too.
- **Restarts.** The restart policy applies from the first start, including a healthcheck that never passes. After a process has been ready for 60 seconds, its restart counter and backoff reset, so occasional crashes don't add up to a crash loop.
- **Clean exits.** A service that exits 0 under `no` or `on-failure` is stopped, not failed, and its dependents keep running. That suits one-shot jobs like migrations.
- **Shutdown.** Ctrl-C, SIGTERM or `core stop` stops every service's whole process tree. On Windows, process trees are ended with `taskkill /T`.
- **Exit code.** `core` exits non-zero if any service ended failed, unless you stopped it yourself.

## Commands

```text
core [start]            Start services (run is an alias)
core stop               Stop the running supervisor
core restart            Stop the running supervisor and start again in this terminal
core status             Supervisor and per-service status
core logs [NAME] [-f]   Show logs; -f follows
core validate           Check configuration and the dependency graph
core explain SERVICE    Show a service's dependencies and dependents

core attach NAME PATH   Add an executable to core.yaml
core incidents          Failure history
core risk               Historical failure risk per service
core predict            Failure predictions from history
core suggest            Configuration suggestions from history
core simulate SERVICE   What a failure of SERVICE would affect
core what-if SERVICE    Counterfactual impact (or --rank)
core cut SERVICE        Minimum failure set that isolates SERVICE (or --rank)
core recover SERVICE    Plan a safe recovery
```

## Files

Core keeps its working files in `.core/` in the project directory. Add it to `.gitignore`.

- `.core/state.json`: live supervisor and service state, written atomically with `0600` permissions.
- `.core/core.lock`: prevents two supervisors in one project.
- `.core/logs/NAME.log`: per-service output with timestamps, appended across runs.
- `.core/incidents/`: one JSON record per failure, used by the analysis commands.

## Security

Core runs configured commands with the privileges of the user running it. A `core.yaml` is executable configuration, so never run one you don't trust. Service names are validated, so they can't escape `.core/logs`.

## Limits

- Logs are not rotated. Truncate or delete `.core/logs/*.log` when they grow.
- On Windows, services are stopped forcefully, with no graceful SIGTERM phase.

## License

Copyright 2026 Mehdi Abdu Mohammed. All rights not granted by the license are reserved.

Core is source-available under the [PolyForm Noncommercial License 1.0.0](LICENSE). It is free for personal and other noncommercial use. Companies need a commercial license from the author; see [COMMERCIAL.md](COMMERCIAL.md).
