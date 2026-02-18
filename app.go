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
	ctx context.Context
	// Hub central of backpressure — receives logs from all goroutines
	hub *streaming.Hub
	// Infrastructure clients
	dockerClient *dockerpkg.Client
	k8sClient    *k8spkg.Client // can be nil if kubeconfig does not exist
	// LogStreamer manages active streams by container
	logStreamer *dockerpkg.LogStreamer
}

// NewApp creates the App instance. Called by main.go.
func NewApp() *App {
	return &App{}
}

// startup is called by Wails when the app starts.
// Here we initialize all services.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize the Hub with the emitter of Wails
	// wailsEmitter adapts the runtime.EventsEmit to our EventEmitter interface
	a.hub = streaming.NewHub(&wailsEmitter{ctx: ctx})
	a.hub.Start()

	// Connect to Docker
	dockerClient, err := dockerpkg.NewClient()
	if err != nil {
		fmt.Printf("[App] Warning: Docker not available: %v\n", err)
		// Not fatal — the app starts and shows error status in the UI
	} else {
		a.dockerClient = dockerClient
		a.logStreamer = dockerpkg.NewLogStreamer(dockerClient, a.hub)
		fmt.Println("[App] Docker connected successfully")
	}

	// Try to connect to Kubernetes (optional)
	k8sClient, err := k8spkg.NewClient()
	if err != nil {
		fmt.Printf("[App] Info: Kubernetes not configured: %v\n", err)
		// Expected if there is no kubeconfig — the K8s tab will be disabled
	} else {
		a.k8sClient = k8sClient
		fmt.Println("[App] Kubernetes connected successfully")
	}
}

// shutdown is called by Wails when the app closes.
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("[App] Closing KubeManager Lite...")

	if a.logStreamer != nil {
		a.logStreamer.StopAll()
	}

	if a.hub != nil {
		a.hub.Stop()
	}

	if a.dockerClient != nil {
		a.dockerClient.Close()
	}
}

// =============================================================================
// Bindings — Docker
// Methods below are exposed directly to the frontend via Wails
// =============================================================================

func (a *App) DockerStatus() bool {
	if a.dockerClient == nil {
		return false
	}
	return a.dockerClient.Ping(a.ctx) == nil
}

func (a *App) ListContainers() ([]dockerpkg.ContainerInfo, error) {
	if a.dockerClient == nil {
		return nil, fmt.Errorf("Docker not available")
	}
	return a.dockerClient.ListContainers(a.ctx)
}

func (a *App) GetContainerStats(containerID string) (*dockerpkg.ContainerInfo, error) {
	if a.dockerClient == nil {
		return nil, fmt.Errorf("Docker not available")
	}
	return a.dockerClient.GetContainerStats(a.ctx, containerID)
}

func (a *App) StartContainer(containerID string) error {
	if a.dockerClient == nil {
		return fmt.Errorf("Docker not available")
	}
	return a.dockerClient.StartContainer(a.ctx, containerID)
}

func (a *App) StopContainer(containerID string) error {
	if a.dockerClient == nil {
		return fmt.Errorf("Docker not available")
	}
	return a.dockerClient.StopContainer(a.ctx, containerID)
}

func (a *App) RestartContainer(containerID string) error {
	if a.dockerClient == nil {
		return fmt.Errorf("Docker not available")
	}
	return a.dockerClient.RestartContainer(a.ctx, containerID)
}

func (a *App) StartLogStream(containerID, containerName string) error {
	if a.logStreamer == nil {
		return fmt.Errorf("Docker not available")
	}
	return a.logStreamer.StartStream(containerID, containerName)
}

func (a *App) StopLogStream(containerID string) {
	if a.logStreamer != nil {
		a.logStreamer.StopStream(containerID)
	}
}

// =============================================================================
// Bindings — Kubernetes
// =============================================================================

// K8sStatus checks if the Kubernetes cluster is accessible.
func (a *App) K8sStatus() bool {
	if a.k8sClient == nil {
		return false
	}
	return a.k8sClient.IsAvailable(a.ctx)
}

func (a *App) ListNamespaces() ([]string, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("Kubernetes not configured")
	}
	return a.k8sClient.ListNamespaces(a.ctx)
}

func (a *App) ListPods(namespace string) ([]k8spkg.PodInfo, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("Kubernetes not configured")
	}
	return a.k8sClient.ListPods(a.ctx, namespace)
}

// =============================================================================
// wailsEmitter — adapter for the EventEmitter interface
// =============================================================================

type wailsEmitter struct {
	ctx context.Context
}

func (e *wailsEmitter) EventsEmit(ctx context.Context, eventName string, optionalData ...interface{}) {
	runtime.EventsEmit(e.ctx, eventName, optionalData...)
}
