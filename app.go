package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	dockerpkg "kubemanager_lite/backend/docker"
	k8spkg "kubemanager_lite/backend/kubernetes"
	"kubemanager_lite/backend/streaming"
)

type App struct {
	ctx           context.Context
	hub           *streaming.Hub
	dockerClient  *dockerpkg.Client
	k8sClient     *k8spkg.Client
	logStreamer   *dockerpkg.LogStreamer
	statsStreamer *dockerpkg.StatsStreamer
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.hub = streaming.NewHub(&wailsEmitter{ctx: ctx})
	a.hub.Start()

	dockerClient, err := dockerpkg.NewClient()
	if err != nil {
		fmt.Printf("[App] Warning: Docker not available: %v\n", err)
	} else {
		a.dockerClient = dockerClient
		a.logStreamer = dockerpkg.NewLogStreamer(dockerClient, a.hub)

		a.statsStreamer = dockerpkg.NewStatsStreamer(dockerClient, a)
		fmt.Println("[App] Docker connected successfully")
	}

	k8sClient, err := k8spkg.NewClient()
	if err != nil {
		fmt.Printf("[App] Info: Kubernetes not configured: %v\n", err)
	} else {
		a.k8sClient = k8sClient
		fmt.Println("[App] Kubernetes connected successfully")
	}
}

// shutdown is called by Wails when the app closes.
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("[App] Shutting down KubeManager Lite...")

	if a.logStreamer != nil {
		a.logStreamer.StopAll()
	}

	if a.statsStreamer != nil {
		a.statsStreamer.StopAll()
	}

	if a.hub != nil {
		a.hub.Stop()
	}

	if a.dockerClient != nil {
		a.dockerClient.Close()
	}
}

// EmitStats implements the dockerpkg.StatsEmitter interface.
// Called by StatsStreamer on every ~1s tick per container.
// Emits a "stats:update" event directly to the frontend — no polling needed.
func (a *App) EmitStats(update dockerpkg.StatsUpdate) {
	runtime.EventsEmit(a.ctx, "stats:update", update)
}

// =============================================================================
// Bindings — Docker
// =============================================================================

func (a *App) DockerStatus() bool {
	if a.dockerClient == nil {
		return false
	}
	return a.dockerClient.Ping(a.ctx) == nil
}

func (a *App) ListContainers() ([]dockerpkg.ContainerInfo, error) {
	if a.dockerClient == nil {
		return nil, fmt.Errorf("Docker is not available")
	}
	return a.dockerClient.ListContainers(a.ctx)
}

func (a *App) StartContainer(containerID string) error {
	if a.dockerClient == nil {
		return fmt.Errorf("Docker is not available")
	}
	return a.dockerClient.StartContainer(a.ctx, containerID)
}

func (a *App) StopContainer(containerID string) error {
	if a.dockerClient == nil {
		return fmt.Errorf("Docker is not available")
	}
	return a.dockerClient.StopContainer(a.ctx, containerID)
}

func (a *App) RestartContainer(containerID string) error {
	if a.dockerClient == nil {
		return fmt.Errorf("Docker is not available")
	}
	return a.dockerClient.RestartContainer(a.ctx, containerID)
}

func (a *App) StartLogStream(containerID, containerName string) error {
	if a.logStreamer == nil {
		return fmt.Errorf("Docker is not available")
	}
	return a.logStreamer.StartStream(containerID, containerName)
}

func (a *App) StopLogStream(containerID string) {
	if a.logStreamer != nil {
		a.logStreamer.StopStream(containerID)
	}
}

func (a *App) StartStatsStream(containerID string) {
	if a.statsStreamer != nil {
		a.statsStreamer.StartStream(containerID)
	}
}

func (a *App) StopStatsStream(containerID string) {
	if a.statsStreamer != nil {
		a.statsStreamer.StopStream(containerID)
	}
}

// =============================================================================
// Bindings — Kubernetes
// =============================================================================

func (a *App) K8sStatus() bool {
	if a.k8sClient == nil {
		return false
	}
	return a.k8sClient.IsAvailable(a.ctx)
}

func (a *App) ListNamespaces() ([]string, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("Kubernetes is not configured")
	}
	return a.k8sClient.ListNamespaces(a.ctx)
}

func (a *App) ListPods(namespace string) ([]k8spkg.PodInfo, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("Kubernetes is not configured")
	}
	return a.k8sClient.ListPods(a.ctx, namespace)
}

// =============================================================================
// wailsEmitter — adapter for the streaming.EventEmitter interface
// =============================================================================

// wailsEmitter adapts runtime.EventsEmit to the streaming.EventEmitter interface,
// keeping the Hub decoupled from the Wails runtime.
type wailsEmitter struct {
	ctx context.Context
}

func (e *wailsEmitter) EventsEmit(ctx context.Context, eventName string, optionalData ...interface{}) {
	runtime.EventsEmit(e.ctx, eventName, optionalData...)
}
