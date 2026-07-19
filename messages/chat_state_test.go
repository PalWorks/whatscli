package messages

import (
	"container/heap"
	"testing"
)

func TestClearUnread_ResetsCounterAndPersists(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	conv := &Conversation{JID: "peer@s.whatsapp.net", Name: "Peer", LastMsgTime: 1000, Unread: 5}
	sm.mu.Lock()
	heap.Push(&sm.priorityQueue, conv)
	sm.convByJID[conv.JID] = conv
	sm.mu.Unlock()
	if err := sm.db.UpsertConversation(*conv); err != nil {
		t.Fatalf("seed UpsertConversation failed: %v", err)
	}

	sm.clearUnread("peer@s.whatsapp.net")

	sm.mu.RLock()
	got := sm.convByJID["peer@s.whatsapp.net"].Unread
	sm.mu.RUnlock()
	if got != 0 {
		t.Errorf("expected in-memory Unread=0, got %d", got)
	}

	stored, err := sm.db.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	if len(stored) != 1 || stored[0].Unread != 0 {
		t.Errorf("expected persisted Unread=0, got %+v", stored)
	}

	if len(mock.ChatLists) == 0 {
		t.Error("expected UpdateChatList to be called")
	}
}

func TestClearUnread_UnknownJID_NoOp(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// Must not panic and must still refresh the (empty) chat list.
	sm.clearUnread("does-not-exist@s.whatsapp.net")

	if len(mock.ChatLists) == 0 {
		t.Error("expected UpdateChatList to still be called for an unknown JID")
	}
}

func TestClearUnread_AlreadyZero_SkipsDBWrite(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	conv := &Conversation{JID: "peer2@s.whatsapp.net", Name: "Peer2", LastMsgTime: 1000, Unread: 0}
	sm.mu.Lock()
	heap.Push(&sm.priorityQueue, conv)
	sm.convByJID[conv.JID] = conv
	sm.mu.Unlock()
	// Deliberately not persisted to the DB, to prove clearUnread does not
	// attempt an UpsertConversation (and thus does not error) when Unread
	// is already zero.

	sm.clearUnread("peer2@s.whatsapp.net")

	stored, err := sm.db.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	if len(stored) != 0 {
		t.Errorf("expected no DB row written for an already-zero conversation, got %+v", stored)
	}
}
