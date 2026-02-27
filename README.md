# KubeManager Lite

![KubeManager Lite](assets/kubemanager_lite.png)

> A lightweight, cross-platform desktop application for managing Docker containers and Kubernetes pods with real-time monitoring — built with Go and Svelte, no Electron required.

![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-blue)
![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)
![Wails](https://img.shields.io/badge/wails-v2.11-red)
![Svelte](https://img.shields.io/badge/svelte-3-FF3E00?logo=svelte)
![License](https://img.shields.io/badge/license-MIT-green)

---

## What is KubeManager Lite?

KubeManager Lite is a **native desktop application** that provides a unified interface for managing Docker containers and Kubernetes pods from a single window. It is designed to be fast, minimal, and resource-efficient — everything that Electron-based tools are not.

The application connects to your local Docker daemon (via socket) and your Kubernetes cluster (via `~/.kube/config`), streaming real-time CPU and memory metrics, log output, and lifecycle events directly to a reactive UI — all without polling.

---

## Why KubeManager Lite?

### vs. Electron-based dashboards (Lens, Portainer Desktop, etc.)

| Concern | Electron apps | KubeManager Lite |
|---|---|---|
| Binary size | 150–300 MB | ~15 MB |
| Startup time | 3–8 seconds | < 1 second |
| Memory at rest | 200–400 MB | ~30 MB |
| Node.js runtime required | Yes | No |
| Native OS titlebar | Opt-in, complex | Native by default |

### vs. CLI tools (kubectl, docker CLI)

KubeManager Lite does not replace the CLI — it complements it. When you want to:
- Watch logs from multiple containers side-by-side
- Quickly start/stop containers without remembering IDs
- Monitor CPU/memory visually without running `watch docker stats`

...a proper UI is faster and clearer than chained terminal commands.

### Key Advantages

- **Zero polling:** CPU/memory stats use Docker's `stream=true` event API. Log lines are pushed via goroutine streams. No periodic HTTP requests.
- **Backpressure by design:** A dedicated Hub with a 500-message buffered channel and 50ms batch aggregator protects the UI from log storms (verified under 939 million messages/minute — see [Benchmarks](#benchmarks)).
- **Auto-reconnect:** If Docker or the K8s cluster goes down, the app retries automatically with exponential backoff (1s → 30s cap) instead of requiring a restart.
- **Native binary:** Ships as a single executable per platform. No JavaScript runtime, no Chromium bundle, no `node_modules`.
- **Full test coverage:** Unit, integration, E2E, and stress tests — all running in CI on every push.

---

## Features

### Docker
- List all containers (running, stopped, exited) with real-time CPU% and memory (MB)
- Visual CPU/memory progress bars per container
- Start, Stop, Restart actions with toast error notifications on failure
- Real-time log streaming in a built-in xterm.js terminal
- Instant lifecycle detection (start/stop/die events) via Docker Events API — no polling delay

### Kubernetes
- Connect via `~/.kube/config` (current context)
- List pods across all namespaces with namespace selector
- Per-pod status badges (Running, Pending, Failed, Succeeded, Unknown)
- Pod ready state and restart count display
- Real-time log streaming for any pod/container via `client-go`

### UI
- Split-pane log viewer shared between Docker and Kubernetes tabs
- macOS native titlebar integration
- Auto-dismiss toast notifications for action errors
- Connection status indicators with retry countdown for both Docker and K8s

---

## Technology Stack

| Layer | Technology | Version |
|---|---|---|
| Desktop framework | [Wails v2](https://wails.io) | v2.11.0 |
| Backend language | Go | 1.25 |
| Docker SDK | docker/docker | v27.1.1 |
| Kubernetes client | k8s.io/client-go | v0.35.1 |
| Frontend framework | Svelte | 3.49 |
| Frontend language | TypeScript | 4.6 |
| Build tool | Vite | 3.0 |
| Terminal emulator | xterm.js | 6.0 |
| E2E testing | Playwright | 1.58 |
| Go linter | golangci-lint | latest |

### Why Wails?

Wails bridges a Go backend to a web frontend using the **platform's native WebView** (WebKit on macOS/Linux, WebView2 on Windows). This means:
- No bundled Chromium — the OS-provided renderer is used
- Go code runs natively, not in a sandboxed Node process
- IPC between Go and JavaScript is handled by the Wails runtime, not a custom HTTP server
- Final binary is ~10–20 MB vs ~150 MB for Electron equivalents

---

## Architecture

### High-Level Overview

```
┌─────────────────────────────────────────────────┐
│                  Wails App                      │
│                                                 │
│  ┌──────────────┐        ┌───────────────────┐  │
│  │  Go Backend  │◄──────►│  Svelte Frontend  │  │
│  │              │ events │  (WebView)        │  │
│  │  app.go      │──────► │  App.svelte       │  │
│  │  docker/     │        │  ContainerList    │  │
│  │  kubernetes/ │        │  PodList          │  │
│  │  streaming/  │        │  LogViewer        │  │
│  │  reconnect/  │        │  stores/          │  │
│  └──────────────┘        └───────────────────┘  │
└─────────────────────────────────────────────────┘
         │                         ▲
         ▼                         │
  Docker daemon              User actions
  K8s cluster               (Start/Stop/Logs)
```

### Backend Packages

```
backend/
  docker/       Docker SDK wrapper
                  containers.go  — list, start, stop, restart, CPU/mem stats
                  logs.go        — log streaming goroutine
                  events.go      — Docker Events API watcher (lifecycle)
                  client.go      — connection + auto-reconnect

  kubernetes/   Kubernetes client-go wrapper
                  pods.go        — pod listing, namespace discovery
                  logs.go        — pod log streaming goroutine
                  client.go      — kubeconfig connection + auto-reconnect

  streaming/    Central backpressure Hub
                  hub.go         — buffered channel + batch aggregator

  reconnect/    Exponential backoff
                  backoff.go     — 1s → 30s cap, WithBackoff() wrapper
```

### Backpressure Hub

The Hub is the most critical component of the system. It decouples fast log producers (one goroutine per container/pod) from the slower JavaScript frontend.

```
[Docker stream] ──┐
[K8s stream]    ──┼──► [chan LogMessage, cap=500] ──► [aggregator goroutine] ──► Wails "log:batch"
[Pod stream]    ──┘         non-blocking send              50ms / 100 msgs
                             (drop on full)                whichever first
```

**Properties:**
- `Hub.Send()` is **non-blocking** — if the channel is full, the message is discarded via `select/default`, never blocking the producer
- The aggregator batches up to 100 messages or flushes every 50ms (whichever comes first), emitting a single `log:batch` Wails event
- Memory is **strictly bounded**: fixed 500-message channel + zero allocations in `Send`
- Verified zero-allocation: `Hub.Send` = **4.4 ns/op, 0 B/op** under parallel contention

```go
// Hub constants
ChannelBufferSize = 500      // max messages in-flight before backpressure
FlushInterval     = 50ms     // ~20 UI updates per second
MaxBatchSize      = 100      // max messages per Wails event payload
```

### Event Flow

All backend-to-frontend communication uses Wails events:

| Event | Direction | Payload | Purpose |
|---|---|---|---|
| `stats:update` | Backend → Frontend | `{ id, cpu, memMB }` | Container metrics (~1/s per container) |
| `log:batch` | Backend → Frontend | `LogMessage[]` | Batched log lines from Hub |
| `container:lifecycle` | Backend → Frontend | `{ action, id, name }` | start/stop/die events |
| `connection:status` | Backend → Frontend | `{ state, message, retryIn, attempt }` | Docker/K8s connection state |

### Auto-Reconnect

Both the Docker and Kubernetes clients use `reconnect.WithBackoff()` to handle transient failures:

```
attempt 1  →  retry in  1s
attempt 2  →  retry in  2s
attempt 3  →  retry in  4s
attempt 4  →  retry in  8s
attempt 5  →  retry in 16s
attempt 6+ →  retry in 30s  (cap)
```

The frontend receives `connection:status` events on every state change and shows a live countdown indicator.

### Frontend State Management

All UI state lives in Svelte writable stores (`frontend/src/stores/containers.ts`):

| Store | Type | Purpose |
|---|---|---|
| `containers` | `ContainerInfo[]` | Docker container list |
| `pods` | `PodInfo[]` | Kubernetes pod list |
| `logLines` | `Map<id, string[]>` | Log lines per container (max 2000) |
| `activeLogContainerId` | `string \| null` | Currently viewed log target |
| `dockerConnected` | `boolean` | Docker connection state |
| `k8sConnected` | `boolean` | Kubernetes connection state |
| `toasts` | `Toast[]` | Active toast notifications |

---

## Benchmarks

> All benchmarks run on **Apple M4 Pro** (darwin/arm64), Go 1.25.
> Full source: `backend/streaming/hub_stress_test.go`

### Scenario 1 — 50 Containers Logging Simultaneously (5s)

| Metric | Value |
|---|---|
| Producers | 50 concurrent goroutines |
| Messages sent | 75,352,293 |
| Messages delivered to frontend | 10,100 |
| Flush events emitted | 101 |
| Goroutine delta after stop | −1 (no leak) |
| Heap growth | 0.1 MB |
| Result | PASS |

50 goroutines fired at full speed for 5 seconds. The Hub absorbed **75 million messages**, delivered a controlled **10,100 batched updates** to the frontend, and released all memory cleanly. Heap grew by just **0.1 MB**.

### Scenario 2 — Channel at Capacity for 60 Seconds

| Metric | Value |
|---|---|
| Producers | 10 goroutines at full speed |
| Messages sent | 939,308,963 |
| Messages delivered | 120,300 |
| Messages dropped (backpressure) | 939,188,563 |
| Drop rate | ~100% (intentional overflow) |
| Flush events emitted | 1,203 |
| Watchdog failures | 0 |
| Result | PASS |

Under an intentional flood of **939 million messages in 60 seconds**, the Hub never deadlocked, never crashed, and remained fully responsive throughout (confirmed by a watchdog check every 5 seconds). The `default` branch in the non-blocking `Send` discarded overflow instantly without blocking any producer goroutine.

### Scenario 3 — 1,000 Log Lines per Second (10s)

| Metric | Value |
|---|---|
| Producers | 5 containers × 200 lines/s |
| Messages sent | 9,984 |
| Messages delivered | 9,984 |
| Delivery rate | **100.0%** |
| Flush events | 201 |
| Result | PASS |

At a sustained 1,000 lines/second, the Hub achieved **zero message loss**. Flush events (201 over 10s) match the expected cadence of the 50ms flush interval precisely. This represents a realistic heavy workload and confirms the buffer and flush configuration are well-calibrated.

### Microbenchmarks

> Run: `go test -bench=. -benchmem ./backend/streaming/`

| Benchmark | ops/sec | ns/op | Allocations | Notes |
|---|---|---|---|---|
| `BenchmarkHub_Send` (parallel) | ~225,000,000 | **4.4 ns** | 0 B/op · 0 allocs | 14 goroutines, M4 Pro |
| `BenchmarkHub_Send_Serial` | ~32,000,000 | **31.4 ns** | 0 B/op · 0 allocs | Single goroutine |

`Hub.Send` is a **zero-allocation, sub-5-nanosecond operation** under full parallel contention. The Hub can sustain over **200 million send attempts per second** — far beyond any realistic container monitoring workload.

---

## Test Coverage

KubeManager Lite has four layers of testing, all running in CI on every push.

### Unit Tests

```bash
go test -v -race -count=1 ./...
```

| Package | What is tested |
|---|---|
| `backend/streaming` | Backpressure: channel full → drop; batch size limit; flush interval; Hub lifecycle |
| `backend/docker` | `calculateCPUPercent` formula; `bytesToMB` conversion; container info mapping |
| `backend/kubernetes` | `toPodInfo` struct mapping; restart count aggregation; ready state logic |

### Integration Tests

```bash
# Requires running Docker daemon
go test -v -tags integration -timeout 60s ./backend/docker/...

# Requires kind cluster
go test -v -tags integration -timeout 60s ./backend/kubernetes/...
```

| Target | What is tested |
|---|---|
| Docker client | Ping succeeds with daemon running |
| Docker containers | `ListContainers` returns expected containers; `StopContainer` changes state |
| Docker logs | Stream opens, receives lines, closes cleanly on context cancel |
| Kubernetes | Pod listing, namespace discovery, log streaming against kind cluster |

### End-to-End Tests (Playwright)

```bash
cd frontend
npx playwright install   # first time only
npm run test:e2e
```

E2E tests run against the **Vite dev server** with the Wails runtime mocked via `frontend/e2e/fixtures.ts`. Fast, deterministic, and independent of real infrastructure.

| User Flow | Status |
|---|---|
| Docker tab active by default | PASS |
| Docker status dot shows connected | PASS |
| Container list loads and shows running containers | PASS |
| Container count badge shows correct number | PASS |
| All visible containers show RUNNING status badge | PASS |
| Clicking container name opens log viewer | PASS |
| Log viewer receives log lines | PASS |
| Closing log viewer hides the panel | PASS |
| Switches to Kubernetes tab | PASS |
| Pod list loads with pods | PASS |
| Namespace selector shows default namespace | PASS |
| Pods show Ready status | PASS |
| Clicking logs button opens log viewer for pod | PASS |
| Can switch between Docker and Kubernetes tabs | PASS |
| Switching tabs closes open log viewer | PASS |

### Stress Tests

```bash
go test -v -tags stress -timeout 120s ./backend/streaming/
```

See [Benchmarks](#benchmarks) above for full results.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| Go 1.25+ | [https://go.dev/dl](https://go.dev/dl) |
| Node.js 18+ | Required for the Svelte frontend build |
| Wails CLI v2 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Docker daemon | Running locally for Docker features |
| kubectl + kubeconfig | `~/.kube/config` for Kubernetes features |

### Platform-specific dependencies

**Linux:**
```bash
# Ubuntu/Debian (must use Ubuntu 22.04 — webkit2gtk-4.0-dev unavailable on 24.04)
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev
```

**Windows:**
WebView2 runtime is included in Windows 10/11. No additional dependencies.

**macOS:**
WebKit is included with the OS. No additional dependencies.

---

## Getting Started

### Clone and install dependencies

```bash
git clone https://github.com/guycanella/kubemanager_lite.git
cd kubemanager_lite
go mod download
cd frontend && npm install && cd ..
```

### Run in development mode

```bash
wails dev
```

This starts the app with hot reload — Go file changes restart the backend, Svelte changes are hot-reloaded in the WebView.

### Run tests

```bash
# Unit tests
go test -v -race -count=1 ./...

# Integration tests (Docker)
go test -v -tags integration ./backend/docker/...

# Integration tests (Kubernetes — requires kind)
go test -v -tags integration ./backend/kubernetes/...

# Stress tests
go test -v -tags stress -timeout 120s ./backend/streaming/

# E2E tests
cd frontend
npx playwright install   # first time only
npm run test:e2e
```

---

## Building

### Current platform

```bash
wails build
```

### Cross-platform targets

```bash
wails build -platform darwin/arm64    # macOS Apple Silicon
wails build -platform darwin/amd64    # macOS Intel
wails build -platform windows/amd64   # Windows
wails build -platform linux/amd64     # Linux
```

Binaries are placed in `build/bin/`.

---

## CI/CD Pipeline

The GitHub Actions pipeline runs all jobs on every push to `main` and on pull requests.

```
┌───────────────────────────────────────────────────┐
│  Parallel (no dependencies)                       │
│  ├── lint         (golangci-lint + svelte-check)  │
│  └── test-unit    (go test ./... + stress tests)  │
└──────────────────┬────────────────────────────────┘
                   │ after test-unit passes
┌──────────────────▼────────────────────────────────┐
│  Parallel                                         │
│  ├── test-integration-docker                      │
│  ├── test-integration-k8s   (kind cluster)        │
│  └── test-e2e               (Playwright)          │
└───────────────────────────────────────────────────┘
                   │ after lint + test-unit pass
┌──────────────────▼────────────────────────────────┐
│  Parallel builds                                  │
│  ├── build-macos   (arm64 + amd64)                │
│  ├── build-windows (amd64)                        │
│  └── build-linux   (amd64, ubuntu-22.04)          │
└──────────────────┬────────────────────────────────┘
                   │ only on v* tag push
┌──────────────────▼────────────────────────────────┐
│  release  →  GitHub Release with all artifacts    │
└───────────────────────────────────────────────────┘
```

| Job | Runner | What it does |
|---|---|---|
| `lint` | ubuntu-latest | `golangci-lint` + `svelte-check` |
| `test-unit` | ubuntu-latest | `go test ./...` + stress tests |
| `test-integration-docker` | ubuntu-latest | Docker integration tests with real daemon |
| `test-integration-k8s` | ubuntu-latest | K8s integration tests with kind cluster |
| `test-e2e` | ubuntu-latest | Playwright against Vite + mocked Wails runtime |
| `build-macos` | macos-latest | arm64 + amd64 `.app` bundles |
| `build-windows` | windows-latest | amd64 `.exe` |
| `build-linux` | ubuntu-22.04 | amd64 binary |
| `release` | ubuntu-latest | GitHub Release on `v*` tag push |

---

## Project Structure

```
kubemanager_lite/
├── app.go                        Wails app struct — public methods = frontend bindings
├── main.go                       Entry point, Wails options
├── go.mod / go.sum
│
├── backend/
│   ├── docker/
│   │   ├── client.go             Connection + auto-reconnect
│   │   ├── containers.go         List, start, stop, restart, CPU/mem stats
│   │   ├── logs.go               Log streaming goroutine
│   │   └── events.go             Docker Events API watcher
│   ├── kubernetes/
│   │   ├── client.go             kubeconfig connection + auto-reconnect
│   │   ├── pods.go               Pod listing, namespace discovery
│   │   └── logs.go               Pod log streaming goroutine
│   ├── streaming/
│   │   └── hub.go                Central backpressure Hub
│   └── reconnect/
│       └── backoff.go            Exponential backoff (1s → 30s)
│
└── frontend/
    ├── package.json
    ├── vite.config.ts
    ├── src/
    │   ├── App.svelte            Root: tabs, titlebar, connection status, toasts
    │   ├── stores/
    │   │   └── containers.ts     All Svelte stores + helper functions
    │   └── components/
    │       ├── ContainerList.svelte  Docker table with CPU/mem bars + actions
    │       ├── PodList.svelte        K8s table with namespace selector
    │       ├── LogViewer.svelte      Split-pane xterm.js terminal
    │       ├── StatusBadge.svelte    Status indicator badge
    │       └── Toast.svelte          Auto-dismiss notifications
    └── e2e/
        ├── fixtures.ts           Wails runtime mock for Playwright
        └── *.spec.ts             E2E test specs
```

---

