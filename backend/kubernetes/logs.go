package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"kubemanager_lite/backend/streaming"

	corev1 "k8s.io/api/core/v1"
)

// PodLogHub is the interface accepted by PodLogStreamer to forward log messages.
type PodLogHub interface {
	Send(msg streaming.LogMessage)
}

// PodLogStreamer manages active log streams per pod.
// Mirrors the same architecture as the Docker LogStreamer —
// each pod gets its own goroutine and context cancel function.
type PodLogStreamer struct {
	client  *Client
	hub     PodLogHub
	streams map[string]context.CancelFunc // key: namespace/podName
}

// NewPodLogStreamer creates a PodLogStreamer connected to the Hub.
func NewPodLogStreamer(client *Client, hub PodLogHub) *PodLogStreamer {
	return &PodLogStreamer{
		client:  client,
		hub:     hub,
		streams: make(map[string]context.CancelFunc),
	}
}

// streamKey returns a unique key for a pod stream.
func streamKey(namespace, podName string) string {
	return namespace + "/" + podName
}

// StartStream opens a log stream for a pod.
// If the pod has multiple containers, it streams the first one by default.
// Pass containerName to target a specific container.
func (ps *PodLogStreamer) StartStream(namespace, podName, containerName string) error {
	key := streamKey(namespace, podName)

	if _, exists := ps.streams[key]; exists {
		return nil // stream already active
	}

	ctx, cancel := context.WithCancel(context.Background())
	ps.streams[key] = cancel

	go func() {
		defer delete(ps.streams, key)

		if err := ps.streamLogs(ctx, namespace, podName, containerName); err != nil {
			if ctx.Err() == nil {
				fmt.Printf("[PodLogStreamer] Error streaming logs for %s/%s: %v\n", namespace, podName, err)
			}
		}
	}()

	return nil
}

// StopStream cancels the log stream for a pod.
func (ps *PodLogStreamer) StopStream(namespace, podName string) {
	key := streamKey(namespace, podName)
	if cancel, exists := ps.streams[key]; exists {
		cancel()
		delete(ps.streams, key)
	}
}

// StopAll cancels all active pod log streams. Called on app shutdown.
func (ps *PodLogStreamer) StopAll() {
	for key, cancel := range ps.streams {
		cancel()
		delete(ps.streams, key)
	}
}

// streamLogs opens a log stream from the Kubernetes API and forwards
// each line to the Hub, using the same backpressure mechanism as Docker logs.
func (ps *PodLogStreamer) streamLogs(ctx context.Context, namespace, podName, containerName string) error {
	opts := &corev1.PodLogOptions{
		Follow:     true,         // keep stream open (equivalent to tail -f)
		TailLines:  int64Ptr(50), // last 50 lines on connect
		Timestamps: true,         // include timestamps in each line
	}

	// Target a specific container if provided
	if containerName != "" {
		opts.Container = containerName
	}

	req := ps.client.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open log stream for pod %s/%s: %w", namespace, podName, err)
	}
	defer stream.Close()

	// Use the pod name as the display name in the LogViewer
	displayName := fmt.Sprintf("%s/%s", namespace, podName)

	scanner := bufio.NewScanner(stream)

	const maxLineSize = 1024 * 1024 // 1MB per line
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		ps.hub.Send(streaming.LogMessage{
			Source: "kubernetes",
			ID:     streamKey(namespace, podName),
			Name:   displayName,
			Line:   scanner.Text(),
		})
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil // expected cancellation
		}
		// io.EOF is normal when the pod terminates
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("error reading log stream: %w", err)
	}

	return nil
}

// int64Ptr returns a pointer to an int64 value,
// required by the Kubernetes API PodLogOptions.
func int64Ptr(i int64) *int64 {
	return &i
}
