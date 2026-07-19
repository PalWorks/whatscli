package messages

import (
	"testing"
)

// Note: cmdLogin/cmdConnect is intentionally not covered here. It has no
// guardable early-exit path — it unconditionally calls sm.login(), which
// creates a real whatsmeow client and attempts a real network connection
// (or QR pairing) to WhatsApp's servers. There is no way to exercise it in
// a hermetic unit test without either hitting the network or building out
// a mock whatsmeow.Client (whatsmeow.Client is a concrete struct, not an
// interface, so it cannot be faked).
//
// cmdReset is also intentionally not covered here, for a more subtle
// reason discovered while writing this test: it deletes real files at
// config.GetSessionFilePath(), which resolves through the adrg/xdg
// package. That package resolves XDG_CONFIG_HOME (and every other base
// directory) in its own package-level init(), which runs once at test
// binary startup — before any test function's t.Setenv("XDG_CONFIG_HOME",
// ...) call can take effect. A t.Setenv-based sandbox for this test looks
// correct but is a no-op: cmdReset would still operate against the real
// ~/.config/whatscli/, potentially deleting or overwriting the user's
// actual session and message history. Testing this safely would require
// refactoring config.GetSessionFilePath()/cmdReset to accept an injectable
// base path, which is out of scope for adding test coverage to existing
// code. Do not add a test that calls cmdReset without first making that
// change — verify with `ls ~/.config/whatscli/` before and after any such
// test, in an isolated environment.

func TestCmdDisconnect_NoClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// sm.client is nil in a test SessionManager, so disconnect() takes its
	// no-op branch and returns a nil error.
	cmdDisconnect(sm, nil, "disconnect", nil)

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 PrintError call, got %d", len(mock.Errors))
	}
	if mock.Errors[0] != nil {
		t.Errorf("expected a nil error (nothing to disconnect), got %v", mock.Errors[0])
	}
}

func TestCmdLogout_AlreadyLoggedOut(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// sm.client is nil, so logout() takes its "already logged out" branch.
	cmdLogout(sm, nil, "logout", nil)

	found := false
	for _, text := range mock.Texts {
		if text == "Already logged out" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Already logged out' message, got texts: %v", mock.Texts)
	}

	sm.mu.RLock()
	loggedOut := sm.loggedOut
	sm.mu.RUnlock()
	if !loggedOut {
		t.Error("expected loggedOut to be set to true")
	}
}
