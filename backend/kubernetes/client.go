package kubernetes

// Package kubernetes provides integration with Kubernetes clusters
// via the default kubeconfig file ~/.kube/config.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	reconnectpkg "kubemanager_lite/backend/reconnect"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client encapsulates the official Kubernetes clientset.
type Client struct {
	clientset *kubernetes.Clientset
}

// NewClient creates a connection to the Kubernetes cluster using the default kubeconfig
// system default (~/.kube/config).
//
// Returns a descriptive error if the file does not exist or the cluster is
// inaccessible — we handle this gracefully in the UI (K8s tab disabled).
func NewClient() (*Client, error) {
	kubeconfigPath, err := defaultKubeconfigPath()
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("error reading kubeconfig in %s: %w", kubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes client: %w", err)
	}

	return &Client{clientset: clientset}, nil
}

// IsAvailable checks if the K8s cluster is accessible.
// The frontend uses this to enable/disable the Pods tab.
func (c *Client) IsAvailable(ctx context.Context) bool {
	_, err := c.clientset.Discovery().ServerVersion()
	return err == nil
}

// Raw exposes the underlying clientset for use in other packages.
func (c *Client) Raw() *kubernetes.Clientset {
	return c.clientset
}

// Monitor starts a background health-check goroutine.
// It checks cluster availability every 10 seconds. On failure, it enters
// exponential backoff until the cluster is reachable again.
func (c *Client) Monitor(ctx context.Context, emitter reconnectpkg.StatusEmitter) {
	go c.monitorLoop(ctx, emitter)
}

func (c *Client) monitorLoop(ctx context.Context, emitter reconnectpkg.StatusEmitter) {
	const healthInterval = 10 * time.Second
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.IsAvailable(ctx) {
				_ = reconnectpkg.WithBackoff(ctx, "kubernetes", emitter, func(ctx context.Context) error {
					if !c.IsAvailable(ctx) {
						return fmt.Errorf("cluster unreachable")
					}
					return nil
				})
				ticker.Reset(healthInterval)
			}
		}
	}
}

// defaultKubeconfigPath returns the default path for the kubeconfig.
// Respects the KUBECONFIG variable if defined, otherwise uses ~/.kube/config.
func defaultKubeconfigPath() (string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory: %w", err)
	}

	path := filepath.Join(home, ".kube", "config")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("kubeconfig not found in %s — configure a K8s cluster", path)
	}

	return path, nil
}
