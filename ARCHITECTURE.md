# Architecture

This document explains the key design decisions behind KubeManager Lite, the *why* behind the choices, not just the *what*.

---

## Why Wails instead of Electron

The most common choice for cross-platform desktop apps with a web frontend is Electron. KubeManager Lite uses [Wails v2](https://wails.io) instead, for three concrete reasons:

**Binary size.** Electron bundles a full Chromium instance (~150 MB) and a Node.js runtime into every app. Wails uses the WebView already provided by the operating system, WebKit on macOS/Linux, and WebView2 on Windows. The result is a ~15 MB binary vs ~150–300 MB for equivalent Electron apps.

**No JavaScript runtime in the backend.** With Electron, both the frontend and the backend run in Node.js processes. With Wails, the backend is a native Go binary. This means the full Go standard library, direct access to OS sockets (Docker's `/var/run/docker.sock`), and native goroutine concurrency, none of which require a JavaScript shim.

**IPC without HTTP.** Electron apps typically communicate between the main process and the renderer via IPC channels or a local HTTP server. Wails generates typed JavaScript bindings from Go method signatures at build time. Calling `ListContainers()` from Svelte invokes the Go method directly through the WebView bridge (no HTTP), no serialisation overhead beyond JSON.

The tradeoff: Wails is less mature than Electron, the ecosystem is smaller, and testing requires mocking the WebView bridge (see [Testing](#testing)).

---

## Project layout

```
kubemanager_lite/
├── app.go                  Public methods on App = Wails frontend bindings
├── main.go                 Wails bootstrap (window config, menu, startup hooks)
│
├── backend/
│   ├── docker/             Docker SDK wrapper
│   ├── kubernetes/         Kubernetes client-go wrapper
│   ├── streaming/          Backpressure Hub
│   └── reconnect/          Exponential backoff
│
└── frontend/
    ├── src/
    │   ├── App.svelte       Root component — tabs, titlebar, connection status
    │   ├── stores/          All reactive state (Svelte writable stores)
    │   └── components/      ContainerList, PodList, LogViewer, Toast, StatusBadge
    ├── e2e/                 Playwright tests + Wails runtime mock
    └── wailsjs/             Auto-generated JS bindings (do not edit)
```

`app.go` is the boundary between Go and JavaScript. Every public method on `App` is automatically exposed to the frontend by Wails. Adding a new backend feature means: implement the logic in a `backend/` package, call it from a new method on `App`, run `wails generate module` to regenerate the bindings, then call it from Svelte like any async function.

---

## Backpressure Hub

The Hub is the most important component in the system. Without it, the app would be unusable under any real workload.

### The problem

Docker and Kubernetes log streams produce data as fast as the daemon can send it, easily thousands of lines per second per container. The Wails event system emits events synchronously to the JavaScript runtime. If a Go goroutine calls `runtime.EventsEmit` on every log line, it will flood the WebView's event queue, freeze the UI, and eventually crash the renderer.

### The solution

```
[Docker log goroutine] ──┐
[K8s log goroutine]    ──┼──► [chan LogMessage, cap=500] ──► [aggregator] ──► "log:batch" event
[Pod log goroutine]    ──┘         non-blocking send           50ms / 100 msgs
                                   (drop if full)              whichever first
```

Every log producer calls `Hub.Send()`, which does a non-blocking channel send:

```go
select {
case h.ch <- msg:
default:
    // channel full — drop silently, never block the producer
}
```

A single aggregator goroutine reads from the channel and accumulates messages into a batch. It flushes when either 100 messages accumulate or 50ms elapses, whichever comes first. The frontend receives at most ~20 Wails events per second regardless of how many containers are logging or at what rate.

### Why this design

**Alternative 1 — emit every line directly:** Simple, but causes UI freeze under any real load. Eliminated immediately.

**Alternative 2 — time-based polling:** The frontend polls Go every N milliseconds for new log lines. Adds latency equal to the poll interval, wastes CPU when nothing is happening, and requires a shared buffer with locking. Rejected.

**Alternative 3 — buffered channel + batch aggregator (current design):** Producers never block. Memory is strictly bounded (fixed channel capacity). The aggregator is the only goroutine that touches the Wails runtime. Throughput is decoupled from UI frame rate.

### Properties verified under stress

| Property | How verified |
|---|---|
| Producers never block | `Hub.Send` = 4.4 ns/op, 0 B/op under 14 parallel goroutines |
| Memory bounded | 939M messages in 60s → heap growth < 1 MB |
| No goroutine leak | goroutine delta = −1 after `Hub.Stop()` (the aggregator exits) |
| No deadlock | 60s flood test with watchdog every 5s — zero failures |

Full stress test source: `backend/streaming/hub_stress_test.go`

---

## Docker integration

### Why `stream=true` instead of polling

Docker's stats API has two modes: a one-shot snapshot (`stream=false`) and a continuous stream (`stream=true`). Polling with `stream=false` on a N-second interval means:

- One HTTP round-trip per container per interval
- CPU% calculation requires two consecutive samples anyway (it's a delta)
- Stats are always N/2 seconds stale on average

With `stream=true`, Docker pushes a new stats object approximately every second per container. The Go goroutine in `containers.go` keeps a persistent connection open, calculates the CPU delta between consecutive samples, and emits a `stats:update` Wails event. Zero polling, always fresh.

The tradeoff: one persistent connection per container. With 50 containers this is 50 open HTTP connections to the Docker socket, negligible on any modern machine.

### Docker SDK v27 pin rationale

The `go.mod` pins `github.com/docker/docker` at `v27.1.1`. This is intentional. The Docker SDK does not follow standard semver for its Go module, major version bumps in the SDK do not always correspond to breaking API changes, and minor version bumps sometimes do. v27 was pinned because it is the version that shipped the stable `container.StatsResponseReader` interface used in `containers.go`. Upgrading without testing against a live daemon risks subtle behavioural changes in the stats stream format.

### Why Docker Events instead of polling for lifecycle

`docker events` is a server-sent stream of all daemon events (container start, stop, die, restart, pause, etc.). The alternative: polling `ListContainers` every N seconds, has an inherent delay equal to the poll interval and wastes bandwidth fetching the full container list repeatedly.

`backend/docker/events.go` opens a single event stream and translates each event into a `container:lifecycle` Wails event. The frontend calls `loadContainers()` immediately on receipt. The result is sub-100ms UI updates on any container state change.

---

## Kubernetes integration

### Connection via kubeconfig

The Kubernetes client reads `~/.kube/config` (or `$KUBECONFIG`) at startup using `client-go`'s `clientcmd.BuildConfigFromFlags`. This is the same mechanism used by `kubectl`. It means KubeManager Lite works with any cluster the user already has configured, local (minikube, kind, Docker Desktop), cloud (GKE, EKS, AKS), or on-premise without any additional configuration.

If the file does not exist, the Kubernetes tab is disabled gracefully. The app does not require a cluster to be available.

### Log streaming via client-go

Kubernetes pod logs are streamed using `client-go`'s `CoreV1().Pods().GetLogs()` with `Follow: true`. This is equivalent to `kubectl logs -f`. The stream is connected to the same Hub used for Docker logs, so the same backpressure and batching logic applies transparently.

---

## Auto-reconnect

Both clients implement the same reconnect pattern via `reconnect.WithBackoff()`:

```
attempt 1  →  wait  1s
attempt 2  →  wait  2s
attempt 3  →  wait  4s
attempt 4  →  wait  8s
attempt 5  →  wait 16s
attempt 6+ →  wait 30s  (cap)
```

There are two scenarios:

**Initial connection failure** (daemon not running at app startup): `app.go` launches a goroutine that calls `WithBackoff` until the connection succeeds, then calls `setupDockerClient` / `setupK8sClient` to initialise all dependent components.

**Runtime disconnection** (daemon goes down after app is running): Each client runs a `monitorLoop` goroutine that pings every 5s (Docker) or 10s (Kubernetes). On failure, it calls `WithBackoff` to re-establish the connection.

On every state change, `WithBackoff` calls `EmitConnectionStatus` on the `App`, which emits a `connection:status` Wails event. The frontend updates the status dot on the tab (grey → amber pulsing → green) and shows a live retry countdown tooltip.

---

## Frontend state management

All UI state lives in Svelte writable stores in `frontend/src/stores/containers.ts`. There is no external state management library, Svelte's built-in reactivity is sufficient.

The stores are updated from two sources:

**Wails event listeners** (backend-initiated): `stats:update`, `log:batch`, `container:lifecycle`, `connection:status`. These are registered in `onMount` in the relevant components and cleaned up in `onDestroy`.

**Direct Go calls** (user-initiated): `ListContainers()`, `ListPods()`, `StartContainer()`, etc. These are called on user action and on lifecycle events that signal a state change.

The split-pane log viewer is controlled by `activeLogContainerId`: a single nullable store that both the Docker and Kubernetes tabs read. Setting it to `null` closes the panel; setting it to a container/pod ID opens it and starts the log stream.

---

## Testing strategy

### The Wails testing problem

Playwright and other browser-based test frameworks run in a standard Chromium instance. Wails applications rely on `window.go`, a runtime bridge injected by the Wails WebView that exposes Go methods to JavaScript. This bridge does not exist in standard Chromium.

Without it, every call to `ListContainers()`, `DockerStatus()`, etc. throws a runtime error, and the UI never renders any data.

### The solution: Playwright fixtures

`frontend/e2e/fixtures.ts` replaces `window.go` and `window.runtime` with in-memory mocks that return deterministic test data. The E2E tests run against the Vite dev server (not `wails dev`) with these mocks injected before the page loads.

This makes E2E tests:
- **Fast** — no Go compilation, no Docker/K8s infrastructure
- **Deterministic** — mock data is fixed, no race conditions from real containers
- **Portable** — run identically on any machine and in CI

The tradeoff is that E2E tests do not exercise the Go backend. That responsibility belongs to the integration tests, which run against a real Docker daemon and a kind cluster in CI.

### Test layers summary

| Layer | Tool | What it covers | Infrastructure needed |
|---|---|---|---|
| Unit | `go test` | Pure logic (CPU%, pod mapping, Hub behaviour) | None |
| Integration | `go test -tags integration` | Real Docker + K8s API calls | Docker daemon, kind cluster |
| E2E | Playwright | Full user flows in the UI | None (mocked) |
| Stress | `go test -tags stress` | Hub under extreme load | None |