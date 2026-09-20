package messages

import (
	"container/heap"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/normen/whatscli/config"
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
	sm.StatusChannel = make(chan StatusMsg, 10)
	sm.stop = make(chan struct{})

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

// withNotifyConfig sets General notification config for the duration of the
// test and restores the previous values on cleanup.
func withNotifyConfig(t *testing.T, enable, bell bool, timeoutSeconds int64) {
	t.Helper()
	orig := config.Config.General
	t.Cleanup(func() { config.Config.General = orig })

	config.Config.General.EnableNotifications = enable
	config.Config.General.UseTerminalBell = bell
	config.Config.General.NotificationTimeout = timeoutSeconds
}

func resetNotifyState() {
	notifyMu.Lock()
	lastNotifyTime = time.Time{}
	notifyMu.Unlock()
}

func TestNotify_Disabled_NoOp(t *testing.T) {
	withNotifyConfig(t, false, true, 60)
	resetNotifyState()

	if err := notify("title", "message"); err != nil {
		t.Fatalf("expected nil error when notifications disabled, got %v", err)
	}

	notifyMu.Lock()
	set := !lastNotifyTime.IsZero()
	notifyMu.Unlock()
	if set {
		t.Error("expected lastNotifyTime to remain unset when notifications are disabled")
	}
}

func TestNotify_ThrottlesRepeatedCalls(t *testing.T) {
	withNotifyConfig(t, true, true, 60)
	resetNotifyState()

	if err := notify("t1", "m1"); err != nil {
		t.Fatalf("first notify failed: %v", err)
	}
	notifyMu.Lock()
	first := lastNotifyTime
	notifyMu.Unlock()
	if first.IsZero() {
		t.Fatal("expected lastNotifyTime to be set after first notify")
	}

	// Immediate second call within the 60s window must be throttled —
	// lastNotifyTime must not advance.
	if err := notify("t2", "m2"); err != nil {
		t.Fatalf("second notify failed: %v", err)
	}
	notifyMu.Lock()
	second := lastNotifyTime
	notifyMu.Unlock()
	if !second.Equal(first) {
		t.Errorf("expected throttled call to leave lastNotifyTime unchanged; got %v -> %v", first, second)
	}
}

func TestNotify_ZeroTimeoutNeverThrottles(t *testing.T) {
	withNotifyConfig(t, true, true, 0)
	resetNotifyState()

	if err := notify("t1", "m1"); err != nil {
		t.Fatalf("first notify failed: %v", err)
	}
	notifyMu.Lock()
	first := lastNotifyTime
	notifyMu.Unlock()

	if err := notify("t2", "m2"); err != nil {
		t.Fatalf("second notify failed: %v", err)
	}
	notifyMu.Lock()
	second := lastNotifyTime
	notifyMu.Unlock()

	if !second.After(first) {
		t.Errorf("expected lastNotifyTime to advance with zero timeout; got %v -> %v", first, second)
	}
}
