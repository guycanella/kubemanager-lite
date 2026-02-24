package streaming_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"kubemanager_lite/backend/streaming"
)

// mockEmitter captures emitted events for assertion in tests.
type mockEmitter struct {
	mu     sync.Mutex
	events [][]streaming.LogMessage
}

func (m *mockEmitter) EventsEmit(_ context.Context, _ string, data ...interface{}) {
	if len(data) == 0 {
		return
	}
	batch, ok := data[0].([]streaming.LogMessage)
	if !ok {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, batch)
}

func (m *mockEmitter) totalMessages() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, batch := range m.events {
		total += len(batch)
	}
	return total
}

func (m *mockEmitter) batchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHub_SendAndReceive(t *testing.T) {
	emitter := &mockEmitter{}
	hub := streaming.NewHub(emitter)
	hub.Start()
	defer hub.Stop()

	hub.Send(streaming.LogMessage{Source: "docker", ID: "abc", Name: "test", Line: "hello"})
	hub.Send(streaming.LogMessage{Source: "docker", ID: "abc", Name: "test", Line: "world"})

	// Wait for flush interval + buffer
	time.Sleep(streaming.FlushInterval * 3)

	if got := emitter.totalMessages(); got != 2 {
		t.Errorf("expected 2 messages, got %d", got)
	}
}

func TestHub_BackpressureDropsMessagesWhenFull(t *testing.T) {
	emitter := &mockEmitter{}
	hub := streaming.NewHub(emitter)
	// Do NOT start the hub — aggregator won't drain the channel,
	// so we can fill it to capacity and verify drops.

	overflow := streaming.ChannelBufferSize + 50

	for i := 0; i < overflow; i++ {
		hub.Send(streaming.LogMessage{Line: "line"})
	}

	// Start hub and wait for a full flush cycle
	hub.Start()
	defer hub.Stop()
	time.Sleep(streaming.FlushInterval * 4)

	received := emitter.totalMessages()
	if received > streaming.ChannelBufferSize {
		t.Errorf("expected at most %d messages (buffer cap), got %d", streaming.ChannelBufferSize, received)
	}
	if received == 0 {
		t.Error("expected some messages to be delivered, got 0")
	}
}

func TestHub_BatchSizeRespected(t *testing.T) {
	emitter := &mockEmitter{}
	hub := streaming.NewHub(emitter)
	hub.Start()
	defer hub.Stop()

	// Send exactly MaxBatchSize + 10 messages in quick succession
	total := streaming.MaxBatchSize + 10
	for i := 0; i < total; i++ {
		hub.Send(streaming.LogMessage{Line: "line"})
	}

	// Wait for two flush cycles to drain everything
	time.Sleep(streaming.FlushInterval * 5)

	// Each individual batch should never exceed MaxBatchSize
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	for i, batch := range emitter.events {
		if len(batch) > streaming.MaxBatchSize {
			t.Errorf("batch %d has %d messages, exceeds MaxBatchSize of %d", i, len(batch), streaming.MaxBatchSize)
		}
	}
}

func TestHub_EmitsNothingWhenChannelEmpty(t *testing.T) {
	emitter := &mockEmitter{}
	hub := streaming.NewHub(emitter)
	hub.Start()
	defer hub.Stop()

	// Wait for a couple of flush cycles without sending anything
	time.Sleep(streaming.FlushInterval * 4)

	if got := emitter.batchCount(); got != 0 {
		t.Errorf("expected 0 batches emitted on empty channel, got %d", got)
	}
}

func TestHub_StopsGracefully(t *testing.T) {
	emitter := &mockEmitter{}
	hub := streaming.NewHub(emitter)
	hub.Start()

	hub.Send(streaming.LogMessage{Line: "before stop"})

	hub.Stop()

	// Sending after stop should not panic (non-blocking send)
	hub.Send(streaming.LogMessage{Line: "after stop"})
}

func TestHub_TimestampSetOnSend(t *testing.T) {
	emitter := &mockEmitter{}
	hub := streaming.NewHub(emitter)
	hub.Start()
	defer hub.Stop()

	before := time.Now().UnixMilli()
	hub.Send(streaming.LogMessage{Line: "ts test"})
	time.Sleep(streaming.FlushInterval * 3)
	after := time.Now().UnixMilli()

	emitter.mu.Lock()
	defer emitter.mu.Unlock()

	if len(emitter.events) == 0 || len(emitter.events[0]) == 0 {
		t.Fatal("expected at least one message")
	}

	ts := emitter.events[0][0].Timestamp
	if ts < before || ts > after {
		t.Errorf("timestamp %d out of expected range [%d, %d]", ts, before, after)
	}
}
