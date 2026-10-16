# Core

**A process supervisor that remembers.**

Core starts your services in the right order, waits until each one is truly ready, brings them back when they fall over, and keeps every word they print. When something breaks, it doesn't just restart it and move on. It writes down what happened, who got hurt, and how far the damage spread. Over time that history turns into answers: which service is fragile, what a failure would take down, and the one weak link that can cut a service off from everything it needs.

It's one small binary. There's no daemon, no account, no cloud, and nothing to install beyond Go. It runs the same on a laptop, a server, a Windows box, or an Android phone in Termux.

```text
[api] Waiting for database...
[cache] Started PID=20027
[database] Started PID=20028
[api] Dependencies ready
[api] Started PID=20030
[worker] Started PID=20036
[database] Exited: exit status 1

[CORE] Failure intelligence
[CORE] Service: database
ROOT FAILURE
  database
DIRECT DEPENDENTS
  api
INDIRECT DEPENDENTS
  worker
TOTAL AFFECTED: 2

[database] Restarting in 200ms...
[api] Dependency restarting; stopping...
[worker] Dependency restarting; stopping...
```

That's real output. The database crashed, so Core stopped the api and the worker that depend on it instead of letting them flail against a dead connection. It restarted the database, waited for it to come back, then brought the others back up in order. And it wrote the whole incident down.

---

## Contents

- [Why Core](#why-core)
- [Install](#install)
- [Your first five minutes](#your-first-five-minutes)
- [Describing your services](#describing-your-services)
- [Logs that look after themselves](#logs-that-look-after-themselves)
- [How Core thinks](#how-core-thinks)
- [Failure intelligence](#failure-intelligence)
- [Every command](#every-command)
- [What Core keeps on disk](#what-core-keeps-on-disk)
- [Platforms](#platforms)
- [Security](#security)
- [Honest limits](#honest-limits)
- [License](#license)

---

## Why Core

Most projects grow into a little constellation: a database, a cache, an API, a worker, a frontend dev server. Starting them by hand means five terminals and a ritual you have to remember. Starting them with a script means that the first crash quietly leaves half your stack running against nothing.

Core is for that gap. It's what you reach for when Docker Compose is too heavy, systemd is too far away, and `npm run dev & go run . &` has burned you one too many times.

It cares about three things:

1. **Order.** Nothing starts until the things it depends on are actually ready, not just launched.
2. **Recovery.** Crashes are expected. Core restarts with backoff, stops dependents while their dependency is down, and resumes them when it's back.
3. **Memory.** Every failure becomes a record you can learn from, not a line that scrolled off the screen.

---

## Install

You need Go 1.23 or newer.

```sh
git clone https://github.com/mehdiabdu391-gif/core.git
cd core
go build -o core .
```

That's the whole install. Move `core` somewhere on your `PATH` if you like.

**On Android (Termux):**

```sh
pkg install golang git
git clone https://github.com/mehdiabdu391-gif/core.git ~/core
cd ~/core && go build -o core .
```

**Check it's healthy:**

```sh
go vet ./...
go test ./...
```

---

## Your first five minutes

**No config at all.** In a project folder, just run:

```sh
./core
```

Core looks around and finds one service to run. It checks for a `package.json` with a `dev`, `start` or `serve` script, then `docker-compose.yml`, then `go.mod`, then a Python entry point (`main.py`, `app.py` or `server.py`, run with `python3`). For a single app, that's often all you need.

**A real stack.** Copy the example and shape it to your project:

```sh
cp core.yaml.example core.yaml
./core validate     # catches typos, missing dependencies and cycles before anything runs
./core              # start everything; Ctrl-C stops it all cleanly
```

**From a second terminal** in the same folder:

```sh
./core status        # who's running, who's waiting, who's failed
./core logs api -f   # follow one service's output live
./core logs -f       # follow everyone at once, each line tagged with its service
./core stop          # shut the whole stack down gracefully
```

Only one supervisor can run per project. A second `./core` in the same folder will tell you one is already running instead of starting a duplicate.

---

## Describing your services

A `core.yaml` is a list of services and how they relate. Here's a full one, with every field in use:

```yaml
logs:
  max_size: 10MB
  max_files: 3

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
    stop_timeout: 5s       # graceful stop, then forced after this
    working_dir: ./api
    environment:
      PORT: "8080"
    healthcheck:
      type: http
      url: http://127.0.0.1:8080/health
```

What each field means:

| Field | What it does |
|---|---|
| `command`, `args` | The program and its arguments. Run directly, never through a shell, so there's no quoting surprises and no injection. |
| `depends_on` | Services that must be *ready* (not just started) before this one starts. |
| `restart` | `no` leaves it down. `on-failure` restarts after a non-zero exit or a healthcheck that never passes. `always` restarts no matter how it exited. |
| `max_restarts` | How many restarts before Core gives up and marks it failed. `0` means keep trying forever. |
| `backoff` | The first wait before restarting. It doubles each time, up to 30 seconds, so a crash loop doesn't eat your CPU. |
| `stop_timeout` | How long a service gets to shut down politely (SIGTERM, or CTRL_BREAK on Windows) before it's forced. Default 5s. |
| `working_dir` | Where the command runs. |
| `environment` | Extra environment variables, added on top of Core's own. |
| `healthcheck` | How Core knows the service is truly ready. See below. |

**Healthchecks** are the difference between "the process exists" and "the process works":

- `tcp`: ready once something accepts connections on `host` (default `127.0.0.1`) and `port`.
- `http`: ready once `url` answers with a 2xx or 3xx. Redirects aren't followed, so a login redirect doesn't count as healthy by accident.
- `command`: ready once your own command exits 0, for anything the others can't express.

Each check tries every `interval` (default 1s), waits up to `timeout` (default 2s) per attempt, and gives up after `retries` (default 30). A service with no healthcheck counts as ready the moment it starts.

**Adding a binary quickly:**

```sh
./core attach indexer "$PWD/bin/indexer"
```

This adds a service called `indexer` to `core.yaml` for you. The path must be absolute and point to an executable file.

---

## Logs that look after themselves

Everything a service prints, on stdout and stderr, goes to your terminal tagged with its name *and* into `.core/logs/NAME.log` with a precise UTC timestamp on every line. Logs append across runs, so yesterday's crash is still there today.

They also rotate, so a chatty service can never fill your disk. When `NAME.log` reaches `max_size`, it becomes `NAME.log.1`, older files shift up, and only `max_files` old files are kept. A service's logs never take more than `max_size × (max_files + 1)`, which is 40MB with the defaults. Rotation happens between lines, so no line is ever cut in half.

```yaml
logs:
  max_size: 10MB   # B, KB, MB or GB; 0 turns rotation off
  max_files: 3     # old files kept per service, 1 to 100
```

Leave the block out and you get these defaults, which also apply when Core detects your service without a `core.yaml`.

Reading them back:

```sh
./core logs api       # the whole story, oldest rotated file first, then the live one
./core logs api -f    # follow live; keeps going across rotations without dropping a line
./core logs -f        # every service at once
```

---

## How Core thinks

These are the rules Core lives by. Knowing them means it never surprises you.

**Ready means ready.** A service is ready when its healthcheck passes, or the moment it starts if it has none. Dependents wait for *ready*, so your API never races a database that's still booting.

**When a dependency falls, its dependents step aside.** If the database goes down, Core stops the API and anything above it rather than leaving them to spew connection errors. They wait. When the database is ready again, they come back in order. Those restarts don't count against the dependents' own `max_restarts`, because it wasn't their fault.

**When a dependency is gone for good, Core says so.** If a dependency can't come back (its policy is `no`, or it used up its `max_restarts`), everything that depends on it is marked failed too, with the reason, instead of waiting forever.

**Restarts are patient, and they forgive.** The restart policy applies from the very first start, including a healthcheck that never passes. Each retry waits twice as long as the last, up to 30 seconds. And once a process has stayed ready for 60 seconds, its restart count and backoff reset. A service that hiccups once a day never "runs out" of restarts.

**A clean exit isn't a failure.** A service that exits with code 0 under `no` or `on-failure` is marked stopped, not failed, and its dependents carry on. That's exactly what you want for one-shot jobs like migrations or seed scripts.

**Shutdown is thorough.** Ctrl-C, SIGTERM or `core stop` stops every service and its entire process tree, including the grandchildren that `npm` and `go run` like to spawn. Each service is asked politely first and given `stop_timeout` to finish. Only then is it forced.

- On Linux, macOS and Android: SIGTERM to the service's process group, then SIGKILL.
- On Windows: CTRL_BREAK to the service's own process group (Go programs receive it as an interrupt, Node as `SIGBREAK`), then `taskkill /T /F`. `core stop` reaches the supervisor through a small request file, `.core/stop.request`, and ends it forcefully only if it hasn't finished within 30 seconds.

**The exit code tells the truth.** `core` exits non-zero if any service ended up failed, unless you stopped it yourself. So it plays nicely in scripts and CI.

---

## Failure intelligence

This is the part that makes Core more than a process runner.

Every failure is saved to `.core/incidents/` as a small JSON record: which service failed, the error, when, and exactly which services were hit, directly and indirectly. The analysis commands read that history together with your dependency graph. They're all read-only. **Core never changes your config or your running services on its own.** It advises; you decide.

Here's what they told us after the crash at the top of this page.

### `core incidents`: what happened

The raw history, grouped by service.

```text
TOTAL INCIDENTS: 3
AFFECTED SERVICES: 6

SERVICE: database
  Failures: 3
  Total affected: 6
  Criticality: 2
  Last error: exit status 1
```

### `core risk`: what to worry about first

A score per service, built from how often it fails, how much damage each failure did, and how far that damage reached. Highest first.

```text
1. database
   Risk score: 76
   Failures: 3
   Criticality: 2
   Affected services: 6

PRIORITY
Monitor database first.
```

### `core predict`: where things are heading

Compares recent failures with older ones to show whether a service is getting better, steady, or worse, and how sure it is.

```text
SERVICE: database
  Trend: CRITICAL
  Confidence: 94%
  Reason: failures are occurring at a very high frequency
  Average failure interval: 1s
```

### `core suggest`: what to change

Concrete config advice, written as the YAML you'd actually paste.

```text
1. SERVICE: database
   Confidence: 94%
   Recommendation: Increase restart backoff to reduce restart thrashing.
   Suggested change: backoff: 30s
```

### `core explain SERVICE`: where something sits

```text
SERVICE: api
DEPENDS ON: cache, database
AFFECTS: worker
```

### `core simulate SERVICE`: what if it fails right now?

Walks your graph and shows every service a failure would reach, and the path the damage would travel.

```text
ROOT FAILURE
  database
TOTAL AFFECTED: 2

DEPENDENCY PATHS
  database -> api
  database -> api -> worker
```

### `core what-if SERVICE`: the full counterfactual

Simulation plus history: the blast radius, how deep it goes, what's happened before, and what recovery would look like. Add `--rank` to compare every service at once.

```text
PREDICTED EFFECTS
  api                DEPENDENCY_LOST
  database           FAILED
  worker             AT_RISK

BLAST RADIUS
  2 service(s) affected
  2 propagation depth

RECOVERY
  Action: RESTART
```

### `core cut SERVICE`: the weakest link

Finds the smallest set of services whose failure would cut the target off from everything it ultimately depends on. If the answer is one service, that's your single point of failure. Add `--rank` to see it for the whole stack.

```text
TARGET
  worker

MINIMUM CUT
  1 service(s)
  - api

INTERPRETATION
  A single service failure can sever the target from its dependency roots.
```

Here the worker relies on two roots, the database and the cache, but everything flows through the api. Harden the api and you've protected the worker.

### `core recover SERVICE`: a safe plan

Weighs the service's risk, history, restart budget and dependents, then proposes an action: restart, wait, or escalate to you. It always runs as a dry run and never acts.

```text
PROPOSED ACTION
  ESCALATE
REASON
  restart policy is disabled
MODE
  DRY-RUN
  ACTION NOT EXECUTED
```

---

## Every command

```text
Running
  core [start]            Start every service (run is an alias)
  core stop               Gracefully stop the running supervisor
  core restart            Stop it, then start again in this terminal
  core status             Supervisor and per-service state
  core logs [NAME] [-f]   Show logs; -f follows live
  core validate           Check the config and dependency graph without running anything
  core explain SERVICE    A service's dependencies and dependents
  core attach NAME PATH   Add an executable to core.yaml

Understanding failures
  core incidents          Failure history
  core risk               Risk score per service
  core predict            Trends and predictions from history
  core suggest            Config suggestions from history
  core simulate SERVICE   What a failure of SERVICE would reach
  core what-if SERVICE    Full counterfactual impact (or --rank)
  core cut SERVICE        Smallest failure set that isolates SERVICE (or --rank)
  core recover SERVICE    A dry-run recovery plan
```

---

## What Core keeps on disk

Everything lives in `.core/` inside your project. Nothing is written anywhere else, and nothing leaves your machine. Add `.core/` to your `.gitignore`.

| Path | What's in it |
|---|---|
| `.core/state.json` | The live state of the supervisor and every service. Written atomically, so it's never half-written, with `0600` permissions so only you can read it. |
| `.core/core.lock` | Stops two supervisors from fighting over one project. |
| `.core/logs/NAME.log` | Each service's output, timestamped, rotated into `NAME.log.1`, `NAME.log.2`, and so on. |
| `.core/incidents/` | One JSON record per failure. This is Core's memory. Delete it to start the history fresh. |
| `.core/stop.request` | Briefly present on Windows while `core stop` is asking the supervisor to shut down. |

`core stop` and `core status` check that the process in `state.json` really is Core, not some unrelated program that happened to reuse the PID after a reboot.

---

## Platforms

| Platform | Status |
|---|---|
| Linux (amd64, arm64) | Fully supported |
| macOS (Intel, Apple Silicon) | Fully supported |
| Android (Termux) | Fully supported. Core was built and used on a phone. |
| Windows | Supported, with the graceful-stop details in [How Core thinks](#how-core-thinks) |

Every push runs `go vet`, the full test suite and the race detector on Linux, macOS and Windows, and cross-builds for every platform above.

---

## Security

Core runs your commands with your own privileges, nothing more. That makes a `core.yaml` executable configuration, just like a shell script: **never run one you don't trust.** Commands are never passed through a shell, environment variable names are validated, and service names are checked so a crafted name can't write logs outside `.core/logs`.

---

## Honest limits

No tool is perfect, and you should know where this one's edges are.

- **Windows graceful stop needs a console.** If Core runs without a console window, or a service ignores CTRL_BREAK, Windows services are ended with `taskkill /F` after `stop_timeout`.
- **On Unix, Core won't kill a stuck supervisor.** If the supervisor ignores `core stop` for 30 seconds, you're told, rather than having it killed and its services left orphaned.
- **The analytics learn from your history.** On a fresh project with no failures yet, `risk`, `predict` and `suggest` have little to say. Give them time.

---

## License

Copyright 2026 Mehdi Abdu Mohammed. All rights not granted by the license are reserved.

Core is **source-available** under the [PolyForm Noncommercial License 1.0.0](LICENSE).

- **Free** for personal projects, learning, research, hobby work, and noncommercial organizations such as charities, schools and public bodies.
- **Companies** and any commercial use need a commercial license. See [COMMERCIAL.md](COMMERCIAL.md). It's a short conversation.

Bug reports and ideas are very welcome in [Issues](https://github.com/mehdiabdu391-gif/core/issues). To keep ownership clear, outside pull requests aren't accepted.

---

<p align="center">Built with care in Addis Ababa by <a href="https://github.com/mehdiabdu391-gif">Mehdi Abdu Mohammed</a>.</p>
