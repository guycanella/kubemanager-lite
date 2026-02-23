package streaming

// Package streaming implements the central backpressure system of KubeManager.
//
// Flow:
//   [Goroutine Container A] ──┐
//   [Goroutine Container B] ──┼──► [Buffered Channel] ──► [Aggregator] ──► [Wails Event]
//   [Goroutine Container C] ──┘         (cap: 500)              (50ms)

import (
	"context"
	"sync"
	"time"
)

const (
	// ChannelBufferSize defines the maximum capacity of the channel before applying backpressure
	// (discarding oldest messages).
	// apply backpressure (discard oldest messages).
	ChannelBufferSize = 500

	// FlushInterval defines the interval at which the aggregator sends the accumulated batch
	// to the frontend. 50ms = ~20 updates per second.
	FlushInterval = 50 * time.Millisecond

	// MaxBatchSize defines the maximum size of a single batch sent to the frontend,
	// avoiding a burst of logs causing a huge payload at once.
	MaxBatchSize = 100
)

// LogMessage represents a log line from a container or pod.
type LogMessage struct {
	// Source identifies the origin: "docker" or "kubernetes"
	Source string `json:"source"`
	// ID is the ID of the container (Docker) or the name of the pod (Kubernetes)
	ID string `json:"id"`
	// Name is the readable name of the container/pod
	Name string `json:"name"`
	// Line is the content of the log line
	Line string `json:"line"`
	// Timestamp is the moment when the log was captured (Unix milliseconds)
	Timestamp int64 `json:"timestamp"`
}

// EventEmitter is the interface that the Wails runtime implements to emit events.
// We define it as an interface to facilitate unit tests.
type EventEmitter interface {
	EventsEmit(ctx context.Context, eventName string, optionalData ...interface{})
}

// Hub is the center of the streaming system. It receives LogMessages from multiple
// goroutines and delivers them to the frontend in a controlled manner.
type Hub struct {
	// channel is the central buffer. All monitoring goroutines write here. The finite capacity
	// is intentional: if the channel is full, the oldest message is discarded (non-blocking send).
	channel chan LogMessage
	// emitter is the function to send events to the Wails
	emitter EventEmitter
	// ctx and cancel control the lifecycle of the Hub
	ctx    context.Context
	cancel context.CancelFunc
	// mu protects the internal state during shutdown
	mu sync.Mutex
	// running indicates if the aggregator is active
	running bool
}

// NewHub creates a new Hub with the EventEmitter of the Wails.
// It should be called only once during the app initialization.
func NewHub(emitter EventEmitter) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		channel: make(chan LogMessage, ChannelBufferSize),
		emitter: emitter,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts the aggregator goroutine that drains the channel and sends
// batches to the frontend. It should be called after NewHub.
func (h *Hub) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return
	}

	h.running = true
	go h.aggregator()
}

// Stop gracefully shuts down the Hub, waiting for the aggregator to drain
// the remaining messages in the channel before closing.
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return
	}

	h.cancel()
	h.running = false
}

// Send sends a LogMessage to the Hub in a non-blocking manner.
// If the channel is full, the message is discarded silently
// (backpressure: we prioritize the stability of the UI over the completeness of the logs).
func (h *Hub) Send(msg LogMessage) {
	msg.Timestamp = time.Now().UnixMilli()

	select {
	case h.channel <- msg:
		// message enqueued successfully
	default:
		// channel full: discard the newest message (backpressure active)
		// In production, you could increment a metrics counter here.
	}
}

// aggregator is the central goroutine. It drains the channel in batches
// at each FlushInterval and emits a single Wails event with all the messages.
// This avoids hundreds of JavaScript events per second.
func (h *Hub) aggregator() {
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			// Context canceled: drain the remaining channel before exiting
			h.flush()
			return

		case <-ticker.C:
			h.flush()
		}
	}
}

// flush collects up to MaxBatchSize messages from the channel and emits
// a single "log:batch" event to the frontend.
func (h *Hub) flush() {
	batch := make([]LogMessage, 0, MaxBatchSize)

	// Drain the channel in a non-blocking manner up to the batch limit
	for i := 0; i < MaxBatchSize; i++ {
		select {
		case msg := <-h.channel:
			batch = append(batch, msg)
		default:
			// channel empty, stop draining
			goto emit
		}
	}

emit:
	if len(batch) == 0 {
		return
	}

	// Emit a single event with the complete batch.
	// The frontend receives an array and processes it at once, avoiding
	// re-renders multiple Svelte components.
	h.emitter.EventsEmit(h.ctx, "log:batch", batch)
}
