package docker

import (
	"bufio"
	"context"
	"fmt"

	"kubemanager_lite/backend/streaming"

	"github.com/docker/docker/api/types/container"
)

// LogStreamer manages active log streams by container.
// We use a map of cancel functions to be able to stop the stream of a container
// the stream of a container when the user closes the logs tab.
type LogStreamer struct {
	client  *Client
	hub     *streaming.Hub
	streams map[string]context.CancelFunc
}

// NewLogStreamer creates a LogStreamer connected to the backpressure Hub.
func NewLogStreamer(client *Client, hub *streaming.Hub) *LogStreamer {
	return &LogStreamer{
		client:  client,
		hub:     hub,
		streams: make(map[string]context.CancelFunc),
	}
}

// StartStream starts the streaming of logs of a specific container.
// Each container receives its own goroutine and cancel context.
//
// The flow is:
//  1. Open log stream of Docker (follow=true, timestamps=true)
//  2. Read line by line via bufio.Scanner
//  3. Send each line to the central Hub (that applies the backpressure)
//
// If the stream is already active for this container, do not open a second one.
func (ls *LogStreamer) StartStream(containerID, containerName string) error {
	// Avoid duplicate streams
	if _, exists := ls.streams[containerID]; exists {
		return nil
	}

	// Context with cancel — we save the cancel func to be able to stop later
	ctx, cancel := context.WithCancel(context.Background())
	ls.streams[containerID] = cancel

	go func() {
		defer func() {
			// Cleanup: remove the stream from the map when closing
			delete(ls.streams, containerID)
		}()

		if err := ls.streamLogs(ctx, containerID, containerName); err != nil {
			// We don't log context.Canceled — it is the expected behavior
			// when the user stops the stream manually.
			if ctx.Err() == nil {
				fmt.Printf("[LogStreamer] Error in stream of %s: %v\n", containerName, err)
			}
		}
	}()

	return nil
}

// StopStream stops the log stream of a container.
// The context.Cancel() makes the reading goroutine naturally exit.
func (ls *LogStreamer) StopStream(containerID string) {
	if cancel, exists := ls.streams[containerID]; exists {
		cancel()
	}
}

// StopAll stops all active streams. Called on application shutdown.
func (ls *LogStreamer) StopAll() {
	for id, cancel := range ls.streams {
		cancel()
		delete(ls.streams, id)
	}
}

// ActiveStreams returns the IDs of containers with active streams.
func (ls *LogStreamer) ActiveStreams() []string {
	ids := make([]string, 0, len(ls.streams))
	for id := range ls.streams {
		ids = append(ids, id)
	}
	return ids
}

// streamLogs reads the continuous logs of a container.
// This function runs inside a goroutine and blocks until:
//   - The context is canceled (user closes the tab)
//   - The container is stopped
//   - An I/O error occurs
func (ls *LogStreamer) streamLogs(ctx context.Context, containerID, containerName string) error {
	reader, err := ls.client.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true, // keeps the connection open (tail -f)
		Timestamps: true, // includes timestamp in each line
		Tail:       "50", // last 50 lines when connecting (does not overload at the beginning)
	})
	if err != nil {
		return fmt.Errorf("failed to open log stream: %w", err)
	}
	defer reader.Close()

	// bufio.Scanner reads line by line efficiently.
	// Docker multiplexes stdout/stderr into a single stream with an 8 byte header.
	// The Scanner removes these headers automatically for us.
	scanner := bufio.NewScanner(reader)

	// Increase the default buffer to handle very long log lines
	const maxLineSize = 1024 * 1024 // 1MB per line
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		// Check if the context was canceled before processing the next line
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()

		// Remove the 8 byte header from the Docker multiplexer if present
		// Format: [STREAM_TYPE(1)] [0 0 0(3)] [SIZE(4)] [PAYLOAD]
		if len(line) > 8 {
			line = line[8:]
		}

		ls.hub.Send(streaming.LogMessage{
			Source: "docker",
			ID:     containerID,
			Name:   containerName,
			Line:   line,
		})
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil // expected cancellation
		}
		return fmt.Errorf("error in reading the stream: %w", err)
	}

	return nil
}
