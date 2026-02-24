package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"kubemanager_lite/backend/streaming"

	corev1 "k8s.io/api/core/v1"
)

// PodLogStreamer manages active log streams per pod.
// Mirrors the same architecture as the Docker LogStreamer —
// each pod gets its own goroutine and context cancel function.
type PodLogStreamer struct {
	client  *Client
	hub     *streaming.Hub
	streams map[string]context.CancelFunc
}

func NewPodLogStreamer(client *Client, hub *streaming.Hub) *PodLogStreamer {
	return &PodLogStreamer{
		client:  client,
		hub:     hub,
		streams: make(map[string]context.CancelFunc),
	}
}

func streamKey(namespace, podName string) string {
	return namespace + "/" + podName
}

func (ps *PodLogStreamer) StartStream(namespace, podName, containerName string) error {
	key := streamKey(namespace, podName)

	if _, exists := ps.streams[key]; exists {
		return nil
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

func (ps *PodLogStreamer) StopStream(namespace, podName string) {
	key := streamKey(namespace, podName)
	if cancel, exists := ps.streams[key]; exists {
		cancel()
		delete(ps.streams, key)
	}
}

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
		Follow:     true,
		TailLines:  int64Ptr(50),
		Timestamps: true,
	}

	if containerName != "" {
		opts.Container = containerName
	}

	req := ps.client.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open log stream for pod %s/%s: %w", namespace, podName, err)
	}
	defer stream.Close()

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
			return nil
		}
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
