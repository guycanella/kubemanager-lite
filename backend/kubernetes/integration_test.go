//go:build integration

package kubernetes

import (
	"context"
	"testing"
	"time"
)

// Integration tests require a running Kubernetes cluster (~/.kube/config).
// Run with: go test -tags integration ./backend/kubernetes/...

func TestIntegration_NewClient_ConnectsSuccessfully(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestIntegration_IsAvailable_ReturnsTrue(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Kubernetes not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !client.IsAvailable(ctx) {
		t.Fatal("IsAvailable() returned false for a reachable cluster")
	}
}

func TestIntegration_ListNamespaces_ReturnsAtLeastDefault(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Kubernetes not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	namespaces, err := client.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces() failed: %v", err)
	}

	if len(namespaces) == 0 {
		t.Fatal("expected at least one namespace, got 0")
	}

	hasDefault := false
	for _, ns := range namespaces {
		if ns == "default" {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		t.Errorf("expected 'default' namespace to exist, got: %v", namespaces)
	}
}

func TestIntegration_ListPods_DefaultNamespace(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Kubernetes not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := client.ListPods(ctx, "default")
	if err != nil {
		t.Fatalf("ListPods() failed: %v", err)
	}

	if len(pods) == 0 {
		t.Skip("no pods in default namespace, skipping field validation")
	}

	for _, pod := range pods {
		if pod.Name == "" {
			t.Error("pod Name should not be empty")
		}
		if pod.Namespace != "default" {
			t.Errorf("pod Namespace = %q, want %q", pod.Namespace, "default")
		}
		if pod.Status == "" {
			t.Error("pod Status should not be empty")
		}
	}
}

func TestIntegration_ListPods_AllNamespaces(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Kubernetes not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Empty string = all namespaces
	pods, err := client.ListPods(ctx, "")
	if err != nil {
		t.Fatalf("ListPods(all namespaces) failed: %v", err)
	}

	t.Logf("found %d pod(s) across all namespaces", len(pods))
}

func TestIntegration_PodLogStream_OpensAndCancels(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("Kubernetes not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := client.ListPods(ctx, "default")
	if err != nil {
		t.Fatalf("ListPods() failed: %v", err)
	}
	if len(pods) == 0 {
		t.Skip("no pods in default namespace for log stream test")
	}

	hub := newMockHub()
	streamer := NewPodLogStreamer(client, hub)

	target := pods[0]
	if err := streamer.StartStream(target.Namespace, target.Name, ""); err != nil {
		t.Fatalf("StartStream() failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// StopStream should not block
	done := make(chan struct{})
	go func() {
		streamer.StopStream(target.Namespace, target.Name)
		close(done)
	}()

	select {
	case <-done:
		t.Logf("log stream for pod %q stopped cleanly, received %d line(s)", target.Name, hub.count())
	case <-time.After(3 * time.Second):
		t.Fatal("StopStream() blocked for more than 3 seconds")
	}
}
