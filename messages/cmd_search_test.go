package messages

import (
	"strings"
	"testing"
)

func TestCmdSearch_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSearch(sm, nil, "search", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdSearch_NoReceiver(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSearch(sm, nil, "search", []string{"hello"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdSearch_NoResults(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdSearch(sm, nil, "search", []string{"nonexistentterm"})

	if len(mock.Texts) != 1 || !strings.Contains(mock.Texts[0], "No messages found") {
		t.Fatalf("expected 'No messages found' message, got %v", mock.Texts)
	}
}

func TestCmdSearch_FindsResultsScopedToCurrentChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "m1", ChatId: "peer@s.whatsapp.net", ContactShort: "Alice", Timestamp: 1000, Text: "Hello world"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	if err := sm.db.AddMessage(Message{Id: "m2", ChatId: "other@s.whatsapp.net", ContactShort: "Bob", Timestamp: 2000, Text: "Hello from elsewhere"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdSearch(sm, nil, "search", []string{"Hello"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 PrintText call, got %d: %v", len(mock.Texts), mock.Texts)
	}
	if !strings.Contains(mock.Texts[0], "Alice") || strings.Contains(mock.Texts[0], "Bob") {
		t.Errorf("expected results scoped to the current chat (Alice) and not the other chat (Bob), got %q", mock.Texts[0])
	}
	if !strings.Contains(mock.Texts[0], "1 found") {
		t.Errorf("expected result count of 1, got %q", mock.Texts[0])
	}
}

func TestCmdSearch_MultiWordKeyword(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "m1", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "the quick brown fox"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdSearch(sm, nil, "search", []string{"quick", "brown"})

	if len(mock.Texts) != 1 || !strings.Contains(mock.Texts[0], "1 found") {
		t.Fatalf("expected the joined 'quick brown' keyword to match, got %v", mock.Texts)
	}
}

func TestCmdSearchContact_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSearchContact(sm, nil, "search-contact", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdSearchContact_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSearchContact(sm, nil, "search-contact", []string{"Alice"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}
