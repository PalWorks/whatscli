package messages

import (
	"container/heap"
)

// A PriorityQueue implements heap.Interface and holds Conversations.
// It prioritizes Pinned chats first, then sorts by LastMsgTime (descending).
// Note: This matches container/heap's MinHeap implementation where "Less" means "Higher Priority" (comes first).
type PriorityQueue []*Conversation

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Rule 1: Pinned chats always come before (are "Less" than) unpinned chats.
	if pq[i].IsPinned && !pq[j].IsPinned {
		return true
	}
	if !pq[i].IsPinned && pq[j].IsPinned {
		return false
	}

	// Rule 2: Within the same group (both pinned or both unpinned),
	// newer chats (larger timestamp) come before older chats.
	return pq[i].LastMsgTime > pq[j].LastMsgTime
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

// Push adds an item to the queue. 
// DO NOT use this directly; use heap.Push(pq, item).
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Conversation)
	item.Index = n
	*pq = append(*pq, item)
}

// Pop removes the last item from the slice.
// DO NOT use this directly; use heap.Pop(pq).
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.Index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// Update modifies the priority of an item in the queue.
func (pq *PriorityQueue) Update(item *Conversation, lastMsgTime int64, isPinned bool) {
	item.LastMsgTime = lastMsgTime
	item.IsPinned = isPinned
	heap.Fix(pq, item.Index)
}
