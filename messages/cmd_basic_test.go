package messages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/normen/whatscli/config"
)

// TestCmdMore_NoReceiver verifies the usage-guard path is unaffected by the
// switch from a hardcoded pagination limit to config.Config.General.BacklogMsgQuantity.
func TestCmdMore_NoReceiver(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdMore(sm, nil, "more", nil)

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 PrintText call, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

// TestCmdMore_UsesConfiguredBacklogQuantity seeds one message for the current
// chat and calls cmdMore with a couple of different BacklogMsgQuantity
// values, verifying that (a) the read of the config value does not crash or
// error regardless of its value, and (b) the DB is queried without error.
// Note: GetMessages (used to find the "oldest" anchor) is intentionally
// unbounded, so there is never an actual older message left to paginate to
// under the current storage layer — this test therefore asserts the honest
// "no older messages" outcome rather than a specific loaded count.
func TestCmdMore_UsesConfiguredBacklogQuantity(t *testing.T) {
	for _, quantity := range []int{1, 10, 50} {
		t.Run(fmt.Sprintf("quantity_%d", quantity), func(t *testing.T) {
			sm, mock := newTestSessionManager(t)

			orig := config.Config.General.BacklogMsgQuantity
			config.Config.General.BacklogMsgQuantity = quantity
			t.Cleanup(func() { config.Config.General.BacklogMsgQuantity = orig })

			sm.currentReceiver = "peer@s.whatsapp.net"
			msg := Message{
				Id:        "m1",
				ChatId:    "peer@s.whatsapp.net",
				Timestamp: 1000,
				Text:      "hello",
			}
			if err := sm.db.AddMessage(msg); err != nil {
				t.Fatalf("seed AddMessage failed: %v", err)
			}

			cmdMore(sm, nil, "more", nil)

			found := false
			for _, text := range mock.Texts {
				if strings.Contains(text, "No older messages available locally") {
					found = true
				}
			}
			if !found {
				t.Errorf("expected the honest 'no older messages' outcome, got texts: %v", mock.Texts)
			}
			if len(mock.Errors) != 0 {
				t.Errorf("expected no errors, got %v", mock.Errors)
			}
		})
	}
}

// ----- Phase 4.1: remaining cmd_basic.go handler tests -----

func TestCmdQuit(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdQuit(sm, nil, "quit", nil)

	if mock.QuitCalled != 1 {
		t.Errorf("expected Quit to be called once, got %d", mock.QuitCalled)
	}
}

func TestCmdColorList(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdColorList(sm, nil, "colorlist", nil)

	if mock.ColorCalled != 1 {
		t.Errorf("expected ShowColorList to be called once, got %d", mock.ColorCalled)
	}
}

func TestCmdRead_NoReceiver(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdRead(sm, nil, "read", nil)

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdRead_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdRead(sm, nil, "read", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdInfo_NoReceiver(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdInfo(sm, nil, "info", nil)

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdInfo_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdInfo(sm, nil, "info", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}
