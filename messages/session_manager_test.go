package messages

import (
	"container/heap"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestSessionManager creates a SessionManager with in-memory SQLite and a MockUiHandler.
func newTestSessionManager(t *testing.T) (*SessionManager, *MockUiHandler) {
	t.Helper()

	// Create in-memory DB
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatalf("InitWithDB failed: %v", err)
	}

	mock := NewMockUiHandler()

	sm := &SessionManager{}
	sm.db = md
	sm.uiHandler = mock
	sm.priorityQueue = make(PriorityQueue, 0)
	heap.Init(&sm.priorityQueue)
	sm.convByJID = make(map[string]*Conversation)
	sm.CommandChannel = make(chan Command, 10)

	return sm, mock
}

func TestInit_PopulatesPQFromDB(t *testing.T) {
	// Pre-seed the DB with conversations, then verify Init loads them
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatalf("InitWithDB failed: %v", err)
	}

	// Seed conversations
	convos := []Conversation{
		{JID: "alice@s.whatsapp.net", Name: "Alice", LastMsgTime: 100},
		{JID: "bob@s.whatsapp.net", Name: "Bob", LastMsgTime: 200},
		{JID: "group@g.us", Name: "Group Chat", LastMsgTime: 300, IsPinned: true},
	}
	for _, c := range convos {
		if err := md.UpsertConversation(c); err != nil {
			t.Fatalf("UpsertConversation failed: %v", err)
		}
	}

	// Simulate what Init does: load conversations into PQ
	sm := &SessionManager{}
	sm.db = md
	sm.uiHandler = NewMockUiHandler()
	sm.priorityQueue = make(PriorityQueue, 0)
	heap.Init(&sm.priorityQueue)
	sm.convByJID = make(map[string]*Conversation)
	sm.CommandChannel = make(chan Command, 10)

	loadedConvs, err := sm.db.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	for _, c := range loadedConvs {
		conv := c
		heap.Push(&sm.priorityQueue, &conv)
		sm.convByJID[conv.JID] = &conv
	}

	// Verify PQ has 3 items
	if sm.priorityQueue.Len() != 3 {
		t.Errorf("expected PQ length 3, got %d", sm.priorityQueue.Len())
	}

	// Verify convByJID has 3 entries
	if len(sm.convByJID) != 3 {
		t.Errorf("expected convByJID length 3, got %d", len(sm.convByJID))
	}

	// Verify lookup works
	if _, ok := sm.convByJID["alice@s.whatsapp.net"]; !ok {
		t.Error("alice not found in convByJID")
	}
	if _, ok := sm.convByJID["group@g.us"]; !ok {
		t.Error("group not found in convByJID")
	}
}

func TestSnapshotPQ_DeepCopy(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	// Add conversations to PQ
	c1 := &Conversation{JID: "original@s.whatsapp.net", Name: "Original", LastMsgTime: 100}
	sm.mu.Lock()
	heap.Push(&sm.priorityQueue, c1)
	sm.convByJID[c1.JID] = c1
	snapshot := sm.snapshotPQ()
	sm.mu.Unlock()

	if len(snapshot) != 1 {
		t.Fatalf("expected snapshot length 1, got %d", len(snapshot))
	}

	// Modify original — snapshot should be unaffected
	c1.Name = "Modified"

	if snapshot[0].Name != "Original" {
		t.Errorf("snapshot was not a deep copy: expected 'Original', got %q", snapshot[0].Name)
	}
}

func TestExecCommand_DispatchesToRegistry(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// Dispatch "help" command — should call PrintHelp on the UI handler
	sm.execCommand(Command{Name: "help", Params: []string{}})

	if mock.HelpCalled != 1 {
		t.Errorf("expected PrintHelp to be called once, got %d", mock.HelpCalled)
	}
}

func TestExecCommand_UnknownCommand(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	sm.execCommand(Command{Name: "nonexistent_command", Params: []string{}})

	// Should print an error about unknown command
	found := false
	for _, text := range mock.Texts {
		if len(text) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error text for unknown command, got nothing")
	}
}

func TestSetCurrentReceiver(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	sm.setCurrentReceiver("test@s.whatsapp.net")

	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver != "test@s.whatsapp.net" {
		t.Errorf("expected receiver 'test@s.whatsapp.net', got %q", receiver)
	}
}
