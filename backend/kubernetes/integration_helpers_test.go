//go:build integration

package kubernetes

import (
	"sync"

	"kubemanager_lite/backend/streaming"
)

type mockHub struct {
	mu   sync.Mutex
	msgs []streaming.LogMessage
}

func newMockHub() *mockHub {
	return &mockHub{}
}

func (h *mockHub) Send(msg streaming.LogMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, msg)
}

func (h *mockHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.msgs)
}
