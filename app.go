package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	dockerpkg "kubemanager_lite/backend/docker"
	k8spkg "kubemanager_lite/backend/kubernetes"
	reconnectpkg "kubemanager_lite/backend/reconnect"
	"kubemanager_lite/backend/streaming"
)

type App struct {
	ctx            context.Context
	hub            *streaming.Hub
	dockerClient   *dockerpkg.Client
	k8sClient      *k8spkg.Client
	logStreamer    *dockerpkg.LogStreamer
	statsStreamer  *dockerpkg.StatsStreamer
	eventWatcher   *dockerpkg.EventWatcher
	podLogStreamer *k8spkg.PodLogStreamer
	clusterWatcher *k8spkg.ClusterWatcher
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
		fmt.Printf("[App] Warning: Docker not available: %v — starting retry loop\n", err)
		go a.retryDockerConnect(ctx)
	} else {
		a.setupDockerClient(dockerClient)
		dockerClient.Monitor(ctx, a)
		fmt.Println("[App] Docker connected successfully")
	}

	k8sClient, err := k8spkg.NewClient()
	if err != nil {
		fmt.Printf("[App] Info: Kubernetes not configured: %v — starting retry loop\n", err)
		go a.retryK8sConnect(ctx)
	} else {
		a.setupK8sClient(k8sClient)
		k8sClient.Monitor(ctx, a)
		fmt.Println("[App] Kubernetes connected successfully")
	}
}

// setupDockerClient initialises all Docker-dependent components.
// Safe to call from a goroutine — pointer assignment is atomic on 64-bit.
func (a *App) setupDockerClient(cli *dockerpkg.Client) {
	a.dockerClient = cli
	a.logStreamer = dockerpkg.NewLogStreamer(cli, a.hub)
	a.statsStreamer = dockerpkg.NewStatsStreamer(cli, a)
	a.eventWatcher = dockerpkg.NewEventWatcher(cli, a)
	a.eventWatcher.Start()
}

// setupK8sClient initialises all Kubernetes-dependent components.
func (a *App) setupK8sClient(cli *k8spkg.Client) {
	a.k8sClient = cli
	a.podLogStreamer = k8spkg.NewPodLogStreamer(cli, a.hub)
	a.clusterWatcher = k8spkg.NewClusterWatcher(&a.k8sClient, a)
	a.clusterWatcher.Start()
}

// retryDockerConnect keeps trying to establish the initial Docker connection
// using exponential backoff. Called only when startup fails.
func (a *App) retryDockerConnect(ctx context.Context) {
	_ = reconnectpkg.WithBackoff(ctx, "docker", a, func(ctx context.Context) error {
		cli, err := dockerpkg.NewClient()
		if err != nil {
			return err
		}
		if err := cli.Ping(ctx); err != nil {
			_ = cli.Close()
			return err
		}
		a.setupDockerClient(cli)
		cli.Monitor(ctx, a)
		fmt.Println("[App] Docker connected after retry")
		return nil
	})
}

// retryK8sConnect keeps trying to establish the initial Kubernetes connection
// using exponential backoff. Called only when startup fails.
func (a *App) retryK8sConnect(ctx context.Context) {
	_ = reconnectpkg.WithBackoff(ctx, "kubernetes", a, func(ctx context.Context) error {
		cli, err := k8spkg.NewClient()
		if err != nil {
			return err
		}
		if !cli.IsAvailable(ctx) {
			return fmt.Errorf("cluster unreachable")
		}
		a.setupK8sClient(cli)
		cli.Monitor(ctx, a)
		fmt.Println("[App] Kubernetes connected after retry")
		return nil
	})
}

// shutdown is called by Wails when the app closes.
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("[App] Shutting down KubeManager Lite...")

	if a.eventWatcher != nil {
		a.eventWatcher.Stop()
	}

	if a.podLogStreamer != nil {
		a.podLogStreamer.StopAll()
	}

	if a.clusterWatcher != nil {
		a.clusterWatcher.Stop()
	}

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

// EmitLifecycle implements dockerpkg.LifecycleEmitter.
// Fired on container start/stop/die/restart/pause/unpause/destroy.
// The frontend listens to "container:lifecycle" and refreshes the list immediately.
func (a *App) EmitLifecycle(event dockerpkg.LifecycleEvent) {
	runtime.EventsEmit(a.ctx, "container:lifecycle", event)
}

// EmitConnectionStatus implements reconnect.StatusEmitter.
// Fired on every connection state change (reconnecting, connected, failed).
func (a *App) EmitConnectionStatus(status reconnectpkg.Status) {
	runtime.EventsEmit(a.ctx, "connection:status", status)
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

func (a *App) StartPodLogStream(namespace, podName, containerName string) error {
	if a.podLogStreamer == nil {
		return fmt.Errorf("Kubernetes is not configured")
	}
	return a.podLogStreamer.StartStream(namespace, podName, containerName)
}

func (a *App) StopPodLogStream(namespace, podName string) {
	if a.podLogStreamer != nil {
		a.podLogStreamer.StopStream(namespace, podName)
	}
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
