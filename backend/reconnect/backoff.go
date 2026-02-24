// Package reconnect provides exponential backoff retry logic for
// infrastructure client connections (Docker and Kubernetes).
//
// Backoff schedule:
//
//	attempt 1 → wait 1s
//	attempt 2 → wait 2s
//	attempt 3 → wait 4s
//	attempt 4 → wait 8s
//	attempt 5 → wait 16s
//	attempt 6+ → wait 30s (cap)
package reconnect

import (
	"context"
	"fmt"
	"time"
)

const (
	initialDelay = 1 * time.Second
	maxDelay     = 30 * time.Second
	multiplier   = 2
)

// StatusType represents the current connection state.
type StatusType string

const (
	StatusConnected    StatusType = "connected"
	StatusReconnecting StatusType = "reconnecting"
	StatusFailed       StatusType = "failed"
)

// Status is the payload emitted to the frontend on every connection change.
type Status struct {
	// Source identifies which client changed: "docker" or "kubernetes"
	Source string `json:"source"`

	// State is the current connection state
	State StatusType `json:"state"`

	// Message is a human-readable description
	Message string `json:"message"`

	// RetryIn is the number of seconds until the next reconnect attempt (0 if connected)
	RetryIn int `json:"retryIn"`

	// Attempt is the current retry attempt number (0 if connected)
	Attempt int `json:"attempt"`
}

// StatusEmitter is the interface used to push connection status to the frontend.
type StatusEmitter interface {
	EmitConnectionStatus(status Status)
}

// ConnectFunc is the function called on each reconnect attempt.
// It should return nil on success, or an error if the connection failed.
type ConnectFunc func(ctx context.Context) error

// WithBackoff runs connectFn repeatedly with exponential backoff until it
// succeeds or the context is cancelled. On each failure, it emits a
// StatusReconnecting event with the countdown to the next attempt.
//
// When the connection is restored, it emits StatusConnected and returns nil.
func WithBackoff(ctx context.Context, source string, emitter StatusEmitter, connectFn ConnectFunc) error {
	delay := initialDelay
	attempt := 0

	for {
		attempt++

		err := connectFn(ctx)
		if err == nil {
			// Connection restored
			emitter.EmitConnectionStatus(Status{
				Source:  source,
				State:   StatusConnected,
				Message: fmt.Sprintf("%s reconnected successfully", source),
			})
			return nil
		}

		// Check if context was cancelled before waiting
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Emit reconnecting status with countdown
		emitter.EmitConnectionStatus(Status{
			Source:  source,
			State:   StatusReconnecting,
			Message: fmt.Sprintf("Connection to %s lost. Retrying in %ds... (attempt %d)", source, int(delay.Seconds()), attempt),
			RetryIn: int(delay.Seconds()),
			Attempt: attempt,
		})

		fmt.Printf("[Reconnect] %s unavailable (attempt %d), retrying in %s: %v\n", source, attempt, delay, err)

		// Wait for the backoff duration or context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		// Increase delay exponentially up to the cap
		delay = min(delay*multiplier, maxDelay)
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
