package docker

import (
	"context"
	"fmt"

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
type LifecycleEmitter interface {
	EmitLifecycle(event LifecycleEvent)
}

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

func (ew *EventWatcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	ew.cancel = cancel

	go func() {
		if err := ew.watch(ctx); err != nil {
			if ctx.Err() == nil {
				fmt.Printf("[EventWatcher] Error watching Docker events: %v\n", err)
			}
		}
	}()
}

func (ew *EventWatcher) Stop() {
	if ew.cancel != nil {
		ew.cancel()
	}
}

func (ew *EventWatcher) watch(ctx context.Context) error {
	// Filter: only container events, only lifecycle actions
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
