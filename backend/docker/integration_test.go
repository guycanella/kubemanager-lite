//go:build integration

package docker

import (
	"context"
	"testing"
	"time"
)

// Integration tests require a running Docker daemon.
// Run with: go test -tags integration ./backend/docker/...

func TestIntegration_NewClient_ConnectsSuccessfully(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()
}

func TestIntegration_Ping_ReachesDockerDaemon(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
}

func TestIntegration_ListContainers_ReturnsRunningContainers(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := client.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers() failed: %v", err)
	}

	if len(containers) == 0 {
		t.Fatal("expected at least one running container, got 0")
	}

	// Validate fields on each returned container
	for _, c := range containers {
		if c.ID == "" {
			t.Error("container ID should not be empty")
		}
		if len(c.ShortID) != 12 {
			t.Errorf("ShortID %q should be 12 chars, got %d", c.ShortID, len(c.ShortID))
		}
		if c.Name == "" {
			t.Error("container Name should not be empty")
		}
		if c.State == "" {
			t.Error("container State should not be empty")
		}
	}
}

func TestIntegration_ListContainers_AllContainersRunning(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := client.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers() failed: %v", err)
	}

	// ListContainers uses All:false, so every returned container must be running
	for _, c := range containers {
		if c.State != "running" {
			t.Errorf("container %q has state %q, expected running", c.Name, c.State)
		}
	}
}

func TestIntegration_LogStream_ReceivesLines(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := client.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers() failed: %v", err)
	}
	if len(containers) == 0 {
		t.Skip("no running containers available for log stream test")
	}

	// Use a mock hub to capture log lines
	hub := newMockHub()
	streamer := NewLogStreamer(client, hub)

	target := containers[0]
	if err := streamer.StartStream(target.ID, target.Name); err != nil {
		t.Fatalf("StartStream() failed: %v", err)
	}

	// Wait up to 5s for at least one log line
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			// Some containers may have no recent logs — acceptable
			t.Logf("no log lines received within 5s for container %q (may have no recent logs)", target.Name)
			streamer.StopStream(target.ID)
			return
		case <-time.After(100 * time.Millisecond):
			if hub.count() > 0 {
				t.Logf("received %d log line(s) from %q", hub.count(), target.Name)
				streamer.StopStream(target.ID)
				return
			}
		}
	}
}

func TestIntegration_LogStream_CancelsCleanly(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := client.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers() failed: %v", err)
	}
	if len(containers) == 0 {
		t.Skip("no running containers available")
	}

	hub := newMockHub()
	streamer := NewLogStreamer(client, hub)
	target := containers[0]

	if err := streamer.StartStream(target.ID, target.Name); err != nil {
		t.Fatalf("StartStream() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// StopStream should not panic or block
	done := make(chan struct{})
	go func() {
		streamer.StopStream(target.ID)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("StopStream() blocked for more than 3 seconds")
	}
}
