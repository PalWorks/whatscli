package messages

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

// Note: the dial/QR-pairing path of cmdLogin/cmdConnect is intentionally not
// covered here — past the connecting guard, sm.login() creates a real
// whatsmeow client and attempts a real network connection (or QR pairing) to
// WhatsApp's servers, and whatsmeow.Client is a concrete struct that cannot
// be faked. The guard branch added by plan H1.5 IS covered below
// (TestLogin_RefusesConcurrentAttempt, TestCmdLogin_RefusesWhileConnecting):
// it exits before any client or network activity, so it is hermetic.
//
// cmdReset used to be untestable because it deleted real files at
// config.GetSessionFilePath(), which resolves through the adrg/xdg package
// whose init() reads XDG_CONFIG_HOME before any t.Setenv can take effect.
// Plan H5.1 added the sessionFilePath seam (session_manager.go) precisely
// so the tests below can redirect every session-file path into a t.TempDir.
// NEVER call cmdReset in a test without overriding sessionFilePath first —
// it would operate against the user's real ~/.config/whatscli/.

// withSessionFileStub redirects sessionFilePath into a temp directory for
// the duration of the test and returns the base path (".db" and "_meta.db"
// are appended by the code under test).
func withSessionFileStub(t *testing.T) string {
	t.Helper()
	base := filepath.Join(t.TempDir(), "session")
	orig := sessionFilePath
	sessionFilePath = func() string { return base }
	t.Cleanup(func() { sessionFilePath = orig })
	return base
}

func TestCmdReset_DeletesSessionAndReinitializes(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	base := withSessionFileStub(t)

	// Pre-create fake session files where cmdReset expects them.
	for _, f := range []string{base + ".db", base + "_meta.db"} {
		if err := os.WriteFile(f, []byte("fake session data"), 0o600); err != nil {
			t.Fatalf("failed to create fixture %s: %v", f, err)
		}
	}

	// Seed in-memory state that the reset must clear.
	sm.mu.Lock()
	sm.convByJID["x@s.whatsapp.net"] = &Conversation{JID: "x@s.whatsapp.net", Name: "X"}
	sm.loggedOut = true
	sm.mu.Unlock()

	cmdReset(sm, nil, "reset", nil)

	// The WhatsApp session DB is gone. The meta DB was deleted and then
	// immediately re-created empty by the sm.db.Init() re-initialization.
	if _, err := os.Stat(base + ".db"); !os.IsNotExist(err) {
		t.Errorf("expected %s.db to be deleted, stat err = %v", base, err)
	}
	convs, err := sm.db.GetConversations()
	if err != nil {
		t.Fatalf("re-initialized meta DB is not usable: %v", err)
	}
	if len(convs) != 0 {
		t.Errorf("re-initialized meta DB should be empty, has %d conversations", len(convs))
	}

	sm.mu.RLock()
	convCount := len(sm.convByJID)
	pqLen := sm.priorityQueue.Len()
	loggedOut := sm.loggedOut
	sm.mu.RUnlock()
	if convCount != 0 || pqLen != 0 {
		t.Errorf("in-memory state not cleared: convByJID=%d, pq=%d", convCount, pqLen)
	}
	if loggedOut {
		t.Error("loggedOut should be cleared so the next /connect can auto-reconnect")
	}

	found := false
	for _, text := range mock.Texts {
		if strings.Contains(text, "Session reset") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Session reset' confirmation, got texts: %v", mock.Texts)
	}
}

func TestCmdReset_RefusesWhileConnecting(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	base := withSessionFileStub(t)

	fixture := base + ".db"
	if err := os.WriteFile(fixture, []byte("fake session data"), 0o600); err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}

	sm.mu.Lock()
	sm.connecting = true
	sm.mu.Unlock()

	cmdReset(sm, nil, "reset", nil)

	if len(mock.Errors) == 0 {
		t.Fatal("expected an error refusing reset during a connection attempt")
	}
	// Nothing was destroyed under the in-progress attempt.
	if _, err := os.Stat(fixture); err != nil {
		t.Errorf("session file must be untouched while connecting, stat err = %v", err)
	}
}

func TestLogin_RefusesConcurrentAttempt(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	// Simulate an in-progress attempt (e.g. a user mid-QR-scan). The guard
	// must fire before login touches the client, the session store, or the
	// network — that is what makes this test hermetic.
	sm.mu.Lock()
	sm.connecting = true
	sm.mu.Unlock()

	if err := sm.login(); !errors.Is(err, errAlreadyConnecting) {
		t.Fatalf("expected errAlreadyConnecting, got %v", err)
	}

	// The colliding attempt must not have cleared the flag owned by the
	// first attempt.
	sm.mu.RLock()
	stillConnecting := sm.connecting
	sm.mu.RUnlock()
	if !stillConnecting {
		t.Error("colliding login cleared the connecting flag owned by the in-progress attempt")
	}
}

func TestCmdLogin_RefusesWhileConnecting(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	sm.mu.Lock()
	sm.connecting = true
	sm.mu.Unlock()

	// cmdLogin backgrounds the login attempt (plan H1.5), so poll for the
	// guard error to surface via the UI handler.
	cmdLogin(sm, nil, "login", nil)

	deadline := time.After(5 * time.Second)
	for {
		mock.Lock()
		var found bool
		for _, err := range mock.Errors {
			if errors.Is(err, errAlreadyConnecting) {
				found = true
			}
		}
		mock.Unlock()
		if found {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for errAlreadyConnecting from backgrounded cmdLogin")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestOwnJID covers the nil-safety guard that keeps a backgrounded send from
// panicking when a concurrent logout/reset nils Store.ID mid-send (master
// audit F3).
func TestOwnJID(t *testing.T) {
	if got := ownJID(nil); got != "" {
		t.Errorf("ownJID(nil) = %q, want empty", got)
	}
	if got := ownJID(&whatsmeow.Client{}); got != "" {
		t.Errorf("ownJID with nil Store = %q, want empty", got)
	}
	if got := ownJID(&whatsmeow.Client{Store: &store.Device{}}); got != "" {
		t.Errorf("ownJID with nil Store.ID = %q, want empty", got)
	}
	jid := types.NewJID("12345", types.DefaultUserServer)
	client := &whatsmeow.Client{Store: &store.Device{ID: &jid}}
	if got := ownJID(client); got != jid.String() {
		t.Errorf("ownJID with valid ID = %q, want %q", got, jid.String())
	}
}

// TestLogout_RefusesWhileConnecting covers the shared connecting-guard that
// stops /logout from tearing down a live pairing attempt's client and store
// (master audit F4).
func TestLogout_RefusesWhileConnecting(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	sm.mu.Lock()
	sm.connecting = true
	sm.mu.Unlock()

	if err := sm.logout(); !errors.Is(err, errAlreadyConnecting) {
		t.Fatalf("expected errAlreadyConnecting, got %v", err)
	}

	// The refusal must not have set loggedOut (that happens only past the
	// guard) nor cleared the flag owned by the in-progress attempt.
	sm.mu.RLock()
	loggedOut := sm.loggedOut
	stillConnecting := sm.connecting
	sm.mu.RUnlock()
	if loggedOut {
		t.Error("refused logout must not set loggedOut")
	}
	if !stillConnecting {
		t.Error("refused logout cleared the connecting flag owned by the in-progress attempt")
	}
}

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
