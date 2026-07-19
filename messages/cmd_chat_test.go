package messages

import (
	"testing"
	"time"
)

func TestCmdSend_NotEnoughParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSend(sm, nil, "send", []string{"onlyonearg"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdSend_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSend(sm, nil, "send", []string{"peer@s.whatsapp.net", "hello", "world"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
	if mock.Errors[0].Error() != "not connected to WhatsApp" {
		t.Errorf("expected 'not connected to WhatsApp', got %q", mock.Errors[0].Error())
	}
}

// ----- Phase 5.6: Status is read-only -----

func TestCmdSend_RefusesStatusBroadcast(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// The status guard fires before the connectivity check, so this is
	// reported even with a nil client.
	cmdSend(sm, nil, "send", []string{STATUSSUFFIX, "hello", "world"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "cannot send messages to Status (view only)" {
		t.Fatalf("expected the status-is-read-only error, got %v", mock.Errors)
	}
}

func TestCmdSelect_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSelect(sm, nil, "select", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdSelect_SetsReceiverAndRefreshesScreen(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSelect(sm, nil, "select", []string{"peer@s.whatsapp.net"})

	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()
	if receiver != "peer@s.whatsapp.net" {
		t.Errorf("expected currentReceiver to be set, got %q", receiver)
	}
	if len(mock.Screens) != 1 {
		t.Errorf("expected NewScreen to be called once, got %d", len(mock.Screens))
	}
}

func TestCmdBacklog_NoReceiver(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdBacklog(sm, nil, "backlog", nil)

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdBacklog_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdBacklog(sm, nil, "backlog", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

// TestCmdSyncGroups_NilClient exercises cmdSyncGroups' only client-independent
// path. Unlike other command handlers, the nil-client check here lives
// *inside* the always-spawned goroutine (audit finding R-3: the client is
// re-fetched inside the goroutine rather than checked synchronously before
// it), so this test must synchronize with that goroutine via MockUiHandler's
// embedded mutex rather than reading its fields directly.
func TestCmdSyncGroups_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSyncGroups(sm, nil, "sync-groups", nil)

	deadline := time.Now().Add(2 * time.Second)
	for {
		mock.Lock()
		errCount := len(mock.Errors)
		mock.Unlock()
		if errCount > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for cmdSyncGroups' goroutine to report 'not connected'")
		}
		time.Sleep(5 * time.Millisecond)
	}

	mock.Lock()
	defer mock.Unlock()
	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}
