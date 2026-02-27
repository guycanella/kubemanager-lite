# Contributing

Thank you for your interest in contributing to KubeManager Lite.

---

## Prerequisites

Before you start, make sure you have the following installed:

| Tool | Version | Install |
|---|---|---|
| Go | 1.25+ | https://go.dev/dl |
| Node.js | 18+ | https://nodejs.org |
| Wails CLI | v2.11 | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0` |
| Docker | any | https://docs.docker.com/get-docker |
| kubectl + kubeconfig | any | https://kubernetes.io/docs/tasks/tools |

For integration tests you also need a local Kubernetes cluster. [minikube](https://minikube.sigs.k8s.io) or [kind](https://kind.sigs.k8s.io) both work.

---

## Getting started

```bash
git clone https://github.com/guycanella/kubemanager-lite.git
cd kubemanager-lite
go mod download
cd frontend && npm install && cd ..
wails dev
```

`wails dev` starts the app with hot reload — Go changes restart the backend, Svelte changes are reflected instantly in the WebView.

---

## Branch naming

```
feat/<short-description>     new feature
fix/<short-description>      bug fix
chore/<short-description>    tooling, deps, CI changes
docs/<short-description>     documentation only
```

Examples: `feat/pod-exec-shell`, `fix/log-viewer-scroll`, `chore/bump-wails-v2.12`

---

## Commit conventions

This project follows [Conventional Commits](https://www.conventionalcommits.org).

```
feat: add pod exec shell support
fix: prevent log viewer scroll jump on new lines
chore: bump wails to v2.12.0
docs: add reconnect flow to ARCHITECTURE.md
test: add integration test for StopContainer
```

Breaking changes must include `BREAKING CHANGE:` in the commit footer:

```
feat: replace Hub flush interval config

BREAKING CHANGE: HubOptions.FlushMs removed, use HubOptions.FlushInterval (time.Duration)
```

---

## Running tests

```bash
# Unit tests (fast, no infrastructure needed)
go test -v -race -count=1 ./...

# Integration tests — Docker (requires running Docker daemon)
go test -v -tags integration -timeout 60s ./backend/docker/...

# Integration tests — Kubernetes (requires kubeconfig + reachable cluster)
go test -v -tags integration -timeout 60s ./backend/kubernetes/...

# Stress tests
go test -v -tags stress -timeout 120s ./backend/streaming/

# E2E tests (no infrastructure needed — Wails runtime is mocked)
cd frontend
npx playwright install --with-deps chromium   # first time only
npm run test:e2e
```

All of these run automatically in CI on every push and pull request.

---

## Adding a new backend binding

A "binding" is a Go method exposed to the Svelte frontend via Wails. Here is the full flow:

**1. Implement the logic in a `backend/` package**

Keep business logic out of `app.go`. Write it in the appropriate package (`docker/`, `kubernetes/`, etc.) with a clear, testable function signature.

```go
// backend/docker/containers.go
func (c *Client) InspectContainer(ctx context.Context, id string) (ContainerDetail, error) {
    // ...
}
```

**2. Add a public method on `App` in `app.go`**

Public methods on `App` are automatically exposed to the frontend by Wails. The method must be exported (capital letter) and can return up to two values — a result and an error.

```go
// app.go
func (a *App) InspectContainer(containerID string) (dockerpkg.ContainerDetail, error) {
    if a.dockerClient == nil {
        return dockerpkg.ContainerDetail{}, fmt.Errorf("Docker is not available")
    }
    return a.dockerClient.InspectContainer(a.ctx, containerID)
}
```

**3. Regenerate the Wails bindings**

```bash
wails generate module
```

This updates `frontend/wailsjs/go/main/App.js` and `App.d.ts` automatically. Do not edit these files by hand.

**4. Call the binding from Svelte**

```ts
import { InspectContainer } from '../wailsjs/go/main/App';

const detail = await InspectContainer(containerId);
```

**5. Update the Wails mock for E2E tests**

If the new binding is called during any user flow covered by Playwright, add it to `frontend/e2e/fixtures.ts` so E2E tests do not break:

```ts
// frontend/e2e/fixtures.ts
window.go.main.App.InspectContainer = async (id: string) => ({
  id,
  image: 'alpine:latest',
  // ...
});
```

---

## Pull request checklist

Before opening a PR, make sure:

- [ ] All existing tests pass (`go test ./...` + `npm run test:e2e`)
- [ ] New logic has unit tests
- [ ] New `App` methods have the `dockerClient == nil` / `k8sClient == nil` guard
- [ ] `wails generate module` has been run if `app.go` changed
- [ ] E2E mock updated in `fixtures.ts` if new bindings are called in user flows
- [ ] Commit messages follow Conventional Commits