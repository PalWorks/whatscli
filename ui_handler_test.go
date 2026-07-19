package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rivo/tview"
)

// TestUpdateQueue_FIFOOrdering exercises the real ordering machinery
// (AppContext.enqueue feeding AppContext.processUpdates). Every UiHandler
// method funnels through this same single-consumer queue, so proving the queue
// preserves FIFO order proves render ordering is deterministic — the property
// that the old goroutine-per-update design could not guarantee.
func TestUpdateQueue_FIFOOrdering(t *testing.T) {
	ctx := &AppContext{}
	ctx.initUpdates()

	var mu sync.Mutex
	var got []int
	// Synchronous sink: the single consumer invokes each closure in order.
	ctx.draw = func(f func()) { f() }
	go ctx.processUpdates()

	const n = 250
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		ctx.enqueue(func() {
			mu.Lock()
			got = append(got, i)
			mu.Unlock()
			wg.Done()
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != n {
		t.Fatalf("expected %d applied updates, got %d", n, len(got))
	}
	for i := 0; i < n; i++ {
		if got[i] != i {
			t.Fatalf("update applied out of order at index %d: got %d, want %d", i, got[i], i)
		}
	}
}

// TestEnqueue_NeverBlocksWhenConsumerStalled guards the deadlock-safety
// invariant behind the unbounded queue: a producer (crucially, the command
// loop, which calls uiHandler.PrintError/PrintText directly) must never block
// on the UI update queue, even when the consumer is fully stalled waiting on
// the tview main loop. If enqueue could block, a full queue would freeze the
// command loop, which would stop draining CommandChannel, which would freeze
// the main loop — a permanent deadlock. This test would hang under a bounded
// blocking channel.
func TestEnqueue_NeverBlocksWhenConsumerStalled(t *testing.T) {
	ctx := &AppContext{}
	ctx.initUpdates()

	block := make(chan struct{})
	ctx.draw = func(f func()) { <-block } // simulate a stalled main loop
	go ctx.processUpdates()

	done := make(chan struct{})
	go func() {
		// Far more than any bounded buffer would hold.
		for i := 0; i < 100000; i++ {
			ctx.enqueue(func() {})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("enqueue blocked while the consumer was stalled: producers can deadlock the command loop")
	}
	close(block)
}

// TestUiHandler_RoutesThroughQueue confirms a UiHandler method does not mutate
// the UI synchronously on the caller's goroutine but instead schedules the work
// on the update queue, and that the scheduled work lands in the TextView.
func TestUiHandler_RoutesThroughQueue(t *testing.T) {
	ctx := &AppContext{TextView: tview.NewTextView()}
	ctx.initUpdates()
	applied := make(chan struct{}, 16)
	ctx.draw = func(f func()) {
		f()
		applied <- struct{}{}
	}
	go ctx.processUpdates()

	h := UiHandler{ctx: ctx}
	h.PrintText("hello-queue")

	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("PrintText was never applied through the update queue")
	}

	if !strings.Contains(ctx.TextView.GetText(false), "hello-queue") {
		t.Errorf("expected TextView to contain the printed text, got %q", ctx.TextView.GetText(false))
	}
}
