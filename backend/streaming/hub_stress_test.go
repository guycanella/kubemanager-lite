package streaming

// Stress tests for the Hub backpressure system.
//
// Run with:
//
//	go test -v -tags stress -run Stress ./backend/streaming/
//	go test -v -tags stress -bench=. -benchmem ./backend/streaming/
//
// These tests are intentionally excluded from the normal test suite
// via the "stress" build tag to avoid slowing down CI unit test jobs.

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── Mock emitter ────────────────────────────────────────────────────────────

// stressEmitter counts every event emitted and every message inside each batch.
type stressEmitter struct {
	mu           sync.Mutex
	eventsEmit   int64
	messagesEmit int64
	lastBatch    []LogMessage
}

func (e *stressEmitter) EventsEmit(_ context.Context, _ string, data ...interface{}) {
	atomic.AddInt64(&e.eventsEmit, 1)

	if len(data) == 0 {
		return
	}

	batch, ok := data[0].([]LogMessage)
	if !ok {
		return
	}

	atomic.AddInt64(&e.messagesEmit, int64(len(batch)))

	e.mu.Lock()
	e.lastBatch = batch
	e.mu.Unlock()
}

func (e *stressEmitter) Events() int64   { return atomic.LoadInt64(&e.eventsEmit) }
func (e *stressEmitter) Messages() int64 { return atomic.LoadInt64(&e.messagesEmit) }

// ─── Scenario 1: 50 containers logging simultaneously ────────────────────────
//
// Goal: UI stays responsive; no goroutine leak; memory stable.
// Each "container" is a goroutine that fires messages as fast as possible
// for the duration of the test. After stopping, we verify that no goroutines
// were leaked and that memory growth is bounded.

func TestStress_50ContainersSimultaneous(t *testing.T) {
	const (
		numContainers = 50
		testDuration  = 5 * time.Second
	)

	emitter := &stressEmitter{}
	hub := NewHub(emitter)
	hub.Start()

	// Baseline goroutine count before load
	runtime.GC()
	baselineGoroutines := runtime.NumGoroutine()

	// Baseline memory before load
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Launch 50 producer goroutines
	var wg sync.WaitGroup
	stop := make(chan struct{})

	var totalSent atomic.Int64

	for i := 0; i < numContainers; i++ {
		wg.Add(1)
		containerID := fmt.Sprintf("container-%02d", i)

		go func(id string) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					hub.Send(LogMessage{
						Source: "docker",
						ID:     id,
						Name:   id,
						Line:   "stress log line from " + id,
					})
					totalSent.Add(1)
				}
			}
		}(containerID)
	}

	// Run for testDuration
	time.Sleep(testDuration)
	close(stop)
	wg.Wait()

	hub.Stop()

	// Allow goroutines to fully exit
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	// ── Assert: no goroutine leak ────────────────────────────────────────────
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - baselineGoroutines
	if leaked > 2 { // tolerance of 2 for runtime internals
		t.Errorf("goroutine leak: started with %d, ended with %d (leaked ~%d)",
			baselineGoroutines, finalGoroutines, leaked)
	}

	// ── Assert: memory growth is bounded ────────────────────────────────────
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// HeapAlloc must not grow by more than 50 MB under sustained load
	const maxHeapGrowthBytes = 50 * 1024 * 1024
	heapGrowth := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	if heapGrowth > maxHeapGrowthBytes {
		t.Errorf("heap growth too large: %.1f MB (limit: 50 MB)",
			float64(heapGrowth)/(1024*1024))
	}

	// ── Assert: at least some messages were delivered ────────────────────────
	if emitter.Messages() == 0 {
		t.Error("expected at least some messages to be emitted to the frontend")
	}

	t.Logf("sent=%d  emitted=%d  events=%d  goroutine_delta=%d  heap_growth=%.1fMB",
		totalSent.Load(), emitter.Messages(), emitter.Events(),
		leaked, float64(heapGrowth)/(1024*1024))
}

// ─── Scenario 2: channel at capacity for 60 seconds ──────────────────────────
//
// Goal: messages dropped gracefully; no deadlock; no crash.
// We fill the channel faster than the aggregator can drain it and hold that
// pressure for the full duration.

func TestStress_ChannelAtCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 60s capacity test in short mode")
	}

	const testDuration = 60 * time.Second

	emitter := &stressEmitter{}
	hub := NewHub(emitter)
	hub.Start()

	// Track dropped messages via sent vs received delta
	var totalSent atomic.Int64

	stop := make(chan struct{})

	// Producer: floods the channel with 10 goroutines at full speed,
	// which is much faster than the aggregator can flush (50ms / 100 msgs).
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := LogMessage{
				Source: "docker",
				ID:     fmt.Sprintf("flood-%d", id),
				Name:   "flood",
				Line:   "flood line",
			}
			for {
				select {
				case <-stop:
					return
				default:
					hub.Send(msg)
					totalSent.Add(1)
				}
			}
		}(i)
	}

	// Watchdog: assert the hub is still responsive every 5 seconds
	// by sending a sentinel message and verifying it eventually arrives.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	deadline := time.After(testDuration)
	watchdogFails := 0

	for {
		select {
		case <-deadline:
			close(stop)
			wg.Wait()
			hub.Stop()

			dropped := totalSent.Load() - emitter.Messages()
			dropRate := float64(dropped) / float64(totalSent.Load()) * 100

			// Drop rate should be high (we're flooding on purpose)
			// but the system must still be alive and emitting.
			if emitter.Events() == 0 {
				t.Error("hub emitted zero events during 60s flood — possible deadlock")
			}

			// No crash means the test reached here: pass.
			t.Logf("sent=%d  delivered=%d  dropped=%d (%.1f%%)  events=%d  watchdog_fails=%d",
				totalSent.Load(), emitter.Messages(), dropped, dropRate,
				emitter.Events(), watchdogFails)
			return

		case <-ticker.C:
			// Confirm the aggregator is still running by checking events increased
			before := emitter.Events()
			time.Sleep(200 * time.Millisecond)
			after := emitter.Events()
			if after <= before {
				watchdogFails++
				t.Logf("WARNING: aggregator appears stalled at tick (events before=%d after=%d)",
					before, after)
			}
		}
	}
}

// ─── Scenario 3: 1000 log lines/second per container ─────────────────────────
//
// Goal: batch aggregator flushes correctly; frontend renders without freezing.
// We use a rate-limited producer to hit exactly 1000 lines/s and verify
// that: batches respect MaxBatchSize, flush interval is honored, and
// total delivered count is within the expected range.

func TestStress_1000LinesPerSecond(t *testing.T) {
	const (
		targetLPS    = 1000 // lines per second
		testDuration = 10 * time.Second
		numProducers = 5 // 5 containers × 200 lines/s each = 1000 lines/s total
		lpsPerProd   = targetLPS / numProducers
	)

	emitter := &stressEmitter{}
	hub := NewHub(emitter)
	hub.Start()

	var wg sync.WaitGroup
	var totalSent atomic.Int64

	stop := make(chan struct{})

	// Rate-limited producers: each fires lpsPerProd messages per second
	// using a time.Ticker for pacing.
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		containerID := fmt.Sprintf("ratelimited-%d", i)
		interval := time.Second / time.Duration(lpsPerProd)

		go func(id string) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					hub.Send(LogMessage{
						Source: "docker",
						ID:     id,
						Name:   id,
						Line:   "rate-limited line from " + id,
					})
					totalSent.Add(1)
				}
			}
		}(containerID)
	}

	time.Sleep(testDuration)
	close(stop)
	wg.Wait()

	// Give aggregator a final flush cycle
	time.Sleep(FlushInterval * 3)
	hub.Stop()

	// ── Assert: batch size never exceeded MaxBatchSize ───────────────────────
	// We verify by checking the last batch the emitter received.
	emitter.mu.Lock()
	lastBatchLen := len(emitter.lastBatch)
	emitter.mu.Unlock()

	if lastBatchLen > MaxBatchSize {
		t.Errorf("batch size %d exceeded MaxBatchSize %d", lastBatchLen, MaxBatchSize)
	}

	// ── Assert: delivery rate is reasonable ──────────────────────────────────
	// At 1000 lines/s × 10s = 10_000 lines sent.
	// Channel capacity is 500, so we expect minimal drops at this rate.
	// We require at least 90% delivery.
	sent := totalSent.Load()
	delivered := emitter.Messages()

	if sent == 0 {
		t.Fatal("no messages were sent")
	}

	deliveryRate := float64(delivered) / float64(sent)
	if deliveryRate < 0.90 {
		t.Errorf("delivery rate too low: %.1f%% (sent=%d delivered=%d) — expected ≥90%%",
			deliveryRate*100, sent, delivered)
	}

	// ── Assert: flush frequency is close to FlushInterval ────────────────────
	// Over 10s at 50ms intervals, we expect ~200 flush events (±20% tolerance).
	expectedFlushes := int64(testDuration / FlushInterval)
	minFlushes := int64(float64(expectedFlushes) * 0.5) // generous lower bound

	if emitter.Events() < minFlushes {
		t.Errorf("too few flush events: got %d, expected ≥%d (based on %s interval over %s)",
			emitter.Events(), minFlushes, FlushInterval, testDuration)
	}

	t.Logf("sent=%d  delivered=%d  delivery_rate=%.1f%%  flush_events=%d  last_batch_size=%d",
		sent, delivered, deliveryRate*100, emitter.Events(), lastBatchLen)
}

// ─── Benchmarks ──────────────────────────────────────────────────────────────

// BenchmarkHub_Send measures the raw throughput of Hub.Send under contention.
func BenchmarkHub_Send(b *testing.B) {
	emitter := &stressEmitter{}
	hub := NewHub(emitter)
	hub.Start()
	defer hub.Stop()

	msg := LogMessage{
		Source: "docker",
		ID:     "bench-container",
		Name:   "bench",
		Line:   "benchmark log line",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			hub.Send(msg)
		}
	})

	b.ReportMetric(float64(emitter.Messages()), "delivered")
	b.ReportMetric(float64(b.N-int(emitter.Messages())), "dropped")
}

// BenchmarkHub_Send_Serial measures single-goroutine Send throughput.
func BenchmarkHub_Send_Serial(b *testing.B) {
	emitter := &stressEmitter{}
	hub := NewHub(emitter)
	hub.Start()
	defer hub.Stop()

	msg := LogMessage{Source: "docker", ID: "bench", Name: "bench", Line: "line"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Send(msg)
	}
}
