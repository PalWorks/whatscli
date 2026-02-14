package messages

import (
	"container/heap"
	"testing"
	"time"
)

func TestPriorityQueue(t *testing.T) {
	now := time.Now().Unix()

	// data setup
	c1 := &Conversation{Name: "Old Unpinned", LastMsgTime: now - 3600, IsPinned: false}
	c2 := &Conversation{Name: "New Unpinned", LastMsgTime: now - 60, IsPinned: false}
	c3 := &Conversation{Name: "Old Pinned", LastMsgTime: now - 7200, IsPinned: true}
	c4 := &Conversation{Name: "New Pinned", LastMsgTime: now - 120, IsPinned: true}

	// Initialize logic
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	// Test Push
	heap.Push(&pq, c1)
	heap.Push(&pq, c2)
	heap.Push(&pq, c3)
	heap.Push(&pq, c4)

	if pq.Len() != 4 {
		t.Errorf("Expected length 4, got %d", pq.Len())
	}

	// Test Pop Order
	// Expected Order:
	// 1. New Pinned (c4) - Pinned & Newer than c3
	// 2. Old Pinned (c3) - Pinned
	// 3. New Unpinned (c2) - Unpinned & Newer than c1
	// 4. Old Unpinned (c1) - Unpinned

	expectedOrder := []*Conversation{c4, c3, c2, c1}

	for i, expected := range expectedOrder {
		item := heap.Pop(&pq).(*Conversation)
		if item != expected {
			t.Errorf("Index %d: expected %s, got %s", i, expected.Name, item.Name)
		}
	}

	// Test Fix (Update)
	// Reset PQ
	heap.Push(&pq, c1)
	heap.Push(&pq, c2)
	heap.Push(&pq, c3)

	// c1 is currently last (Old Unpinned). Let's pin it.
	// New state: c1 (Pinned, Old), c3 (Pinned, Older), c2 (Unpinned) -> c1 should jump to top or near top?
	// c3 Time: now-7200. c1 Time: now-3600.
	// If both pinned, c1 is newer, so c1 should be #1.

	pq.Update(c1, c1.LastMsgTime, true) // Pin c1

	first := heap.Pop(&pq).(*Conversation)
	if first != c1 {
		t.Errorf("After pinning c1, expected it to be first (newer than c3), but got %s", first.Name)
	}

	second := heap.Pop(&pq).(*Conversation)
	if second != c3 {
		t.Errorf("Expected c3 second, got %s", second.Name)
	}
}
