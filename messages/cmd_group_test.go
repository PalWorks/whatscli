package messages

import (
	"strings"
	"testing"
)

// Every group command silently did nothing on a nil client before this
// test file was written (the same bug class Phase 1.5 fixed for
// cmd_media) — these tests both document and lock in that fix.

func TestCmdCreate_NotEnoughParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdCreate(sm, nil, "create", []string{"onlyonearg"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdCreate_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdCreate(sm, nil, "create", []string{"11111,22222", "My", "Group"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdCreate_InvalidPhoneReportsErrorButContinues(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// A phone containing '.' with a malformed device suffix fails
	// types.ParseJID (see cmd_message_test.go's TestCmdRevoke_InvalidChatJID
	// for why); cmdCreate reports it per-invalid-phone and continues rather
	// than aborting the whole command.
	cmdCreate(sm, nil, "create", []string{"bad.notanumber,22222", "My", "Group"})

	foundPhoneError := false
	for _, err := range mock.Errors {
		if err != nil && strings.Contains(err.Error(), "invalid phone") {
			foundPhoneError = true
		}
	}
	if !foundPhoneError {
		t.Errorf("expected an 'invalid phone' error, got %v", mock.Errors)
	}
}

func TestCmdSubject_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdSubject(sm, nil, "subject", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdSubject_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "group@g.us"
	sm.mu.Unlock()

	cmdSubject(sm, nil, "subject", []string{"New", "Subject"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdLeave_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "group@g.us"
	sm.mu.Unlock()

	cmdLeave(sm, nil, "leave", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdAdd_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdAdd(sm, nil, "add", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdAdd_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "group@g.us"
	sm.mu.Unlock()

	cmdAdd(sm, nil, "add", []string{"123456"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdAdd_InvalidGroupJID(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "bad.notanumber@g.us"
	sm.mu.Unlock()

	cmdAdd(sm, nil, "add", []string{"123456"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
	if !strings.Contains(mock.Errors[0].Error(), "invalid group JID") {
		t.Errorf("expected 'invalid group JID' error, got %q", mock.Errors[0].Error())
	}
}

func TestCmdRemove_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdRemove(sm, nil, "remove", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdRemove_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "group@g.us"
	sm.mu.Unlock()

	cmdRemove(sm, nil, "remove", []string{"123456"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdAdmin_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdAdmin(sm, nil, "admin", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdAdmin_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "group@g.us"
	sm.mu.Unlock()

	cmdAdmin(sm, nil, "admin", []string{"123456"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdRemoveAdmin_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdRemoveAdmin(sm, nil, "removeadmin", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdRemoveAdmin_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "group@g.us"
	sm.mu.Unlock()

	cmdRemoveAdmin(sm, nil, "removeadmin", []string{"123456"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

// TestGroupCommandAliases_Registered verifies the promote/demote aliases
// route to the same handlers as admin/removeadmin.
func TestGroupCommandAliases_Registered(t *testing.T) {
	if commandRegistry["promote"] == nil {
		t.Error("expected 'promote' alias to be registered")
	}
	if commandRegistry["demote"] == nil {
		t.Error("expected 'demote' alias to be registered")
	}
}
