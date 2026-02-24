package kubernetes

import (
	"context"
	"fmt"
	"time"

	"kubemanager_lite/backend/reconnect"
)

const pingInterval = 15 * time.Second

// ClusterWatcher periodically checks Kubernetes cluster connectivity.
// Unlike the Docker EventWatcher (which reacts to a stream dropping),
// K8s has no persistent event stream for connectivity — so we ping on
// a fixed interval and trigger reconnect when the ping fails.
//
// On reconnect, the client is fully recreated by re-reading ~/.kube/config.
// This is intentional: the cluster endpoint may change between restarts
// (e.g. minikube assigns a new port on each start).
type ClusterWatcher struct {
	// client is a pointer to the App's k8sClient field.
	// On successful reconnect, we replace it with a fresh client.
	clientRef **Client

	emitter reconnect.StatusEmitter
	cancel  context.CancelFunc
}

// NewClusterWatcher creates a ClusterWatcher.
// clientRef must be a pointer to the *Client field in App so we can
// replace it atomically when the cluster endpoint changes.
func NewClusterWatcher(clientRef **Client, emitter reconnect.StatusEmitter) *ClusterWatcher {
	return &ClusterWatcher{
		clientRef: clientRef,
		emitter:   emitter,
	}
}

// Start begins the connectivity check loop in a background goroutine.
func (cw *ClusterWatcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	cw.cancel = cancel

	go cw.watch(ctx)
}

// Stop cancels the watcher and any pending reconnect attempts.
func (cw *ClusterWatcher) Stop() {
	if cw.cancel != nil {
		cw.cancel()
	}
}

// watch runs the ping loop. When a ping fails, it hands off to
// reconnect.WithBackoff which retries until the cluster is reachable again.
func (cw *ClusterWatcher) watch(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if *cw.clientRef == nil {
				continue
			}

			if !(*cw.clientRef).IsAvailable(ctx) {
				fmt.Println("[ClusterWatcher] Kubernetes cluster unreachable, starting reconnect...")

				err := reconnect.WithBackoff(ctx, "Kubernetes", cw.emitter, cw.tryReconnect)
				if err != nil && ctx.Err() == nil {
					fmt.Printf("[ClusterWatcher] Kubernetes reconnect failed permanently: %v\n", err)
				}
			}
		}
	}
}

// tryReconnect attempts to create a fresh K8s client by re-reading kubeconfig.
// Called by WithBackoff on every retry attempt.
func (cw *ClusterWatcher) tryReconnect(ctx context.Context) error {
	newClient, err := NewClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	if !newClient.IsAvailable(ctx) {
		return fmt.Errorf("cluster reachable but not ready")
	}

	// Atomically replace the client so all subsequent calls use the new endpoint
	*cw.clientRef = newClient
	fmt.Println("[ClusterWatcher] Kubernetes client refreshed with new endpoint")
	return nil
}
