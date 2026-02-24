package docker

import (
	"context"
	"fmt"

	"kubemanager_lite/backend/reconnect"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// LifecycleEvent represents a container lifecycle change emitted to the frontend.
type LifecycleEvent struct {
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	Action        string `json:"action"` // "start", "stop", "die", "restart", "pause", "unpause"
}

// LifecycleEmitter is the interface used to push lifecycle events to the frontend.
// The App struct implements both LifecycleEmitter and reconnect.StatusEmitter.
type LifecycleEmitter interface {
	EmitLifecycle(event LifecycleEvent)
	reconnect.StatusEmitter
}

// EventWatcher watches the Docker daemon event stream and notifies the frontend
// of container lifecycle changes in real time.
type EventWatcher struct {
	client  *Client
	emitter LifecycleEmitter
	cancel  context.CancelFunc
}

// NewEventWatcher creates an EventWatcher connected to the given emitter.
func NewEventWatcher(client *Client, emitter LifecycleEmitter) *EventWatcher {
	return &EventWatcher{
		client:  client,
		emitter: emitter,
	}
}

// Start begins watching Docker events in a background goroutine.
// If the event stream drops (e.g. Docker daemon restart), it automatically
// reconnects using exponential backoff.
func (ew *EventWatcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	ew.cancel = cancel

	go func() {
		err := reconnect.WithBackoff(ctx, "Docker", ew.emitter, func(ctx context.Context) error {
			return ew.watch(ctx)
		})
		if err != nil && ctx.Err() == nil {
			fmt.Printf("[EventWatcher] Permanent failure watching Docker events: %v\n", err)
		}
	}()
}

// Stop cancels the event stream and any pending reconnect attempts.
func (ew *EventWatcher) Stop() {
	if ew.cancel != nil {
		ew.cancel()
	}
}

// watch subscribes to the Docker event stream and forwards container
// lifecycle events to the frontend. Returns an error if the stream drops,
// which triggers a reconnect attempt from the caller (WithBackoff).
func (ew *EventWatcher) watch(ctx context.Context) error {
	filter := filters.NewArgs(
		filters.Arg("type", string(events.ContainerEventType)),
		filters.Arg("event", "start"),
		filters.Arg("event", "stop"),
		filters.Arg("event", "die"),
		filters.Arg("event", "restart"),
		filters.Arg("event", "pause"),
		filters.Arg("event", "unpause"),
		filters.Arg("event", "destroy"),
	)

	eventCh, errCh := ew.client.cli.Events(ctx, events.ListOptions{
		Filters: filter,
	})

	for {
		select {
		case <-ctx.Done():
			return nil

		case err := <-errCh:
			// Return the error so WithBackoff can trigger a reconnect
			return fmt.Errorf("event stream closed: %w", err)

		case msg := <-eventCh:
			name := msg.Actor.Attributes["name"]
			if name == "" {
				id := msg.Actor.ID
				if len(id) > 12 {
					id = id[:12]
				}
				name = id
			}

			ew.emitter.EmitLifecycle(LifecycleEvent{
				ContainerID:   msg.Actor.ID,
				ContainerName: name,
				Action:        string(msg.Action),
			})
		}
	}
}
