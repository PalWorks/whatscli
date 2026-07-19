package messages

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

// ----- Phase 5.8: cmdExport guards -----

func TestCmdExport_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdExport(sm, nil, "export", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdExport_MissingPath(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdExport(sm, nil, "export", []string{"peer@s.whatsapp.net"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdExport_InvalidChatJID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdExport(sm, nil, "export", []string{"user.notanumber@s.whatsapp.net", filepath.Join(t.TempDir(), "out.age")})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdExport_MissingPassphraseEnvVar(t *testing.T) {
	t.Setenv(exportPassphraseEnvVar, "")
	sm, mock := newTestSessionManager(t)

	cmdExport(sm, nil, "export", []string{"peer@s.whatsapp.net", filepath.Join(t.TempDir(), "out.age")})

	if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), exportPassphraseEnvVar) {
		t.Fatalf("expected an error mentioning %s, got %v", exportPassphraseEnvVar, mock.Errors)
	}
}

func TestCmdExport_PassphraseTooShort(t *testing.T) {
	t.Setenv(exportPassphraseEnvVar, "short")
	sm, mock := newTestSessionManager(t)

	cmdExport(sm, nil, "export", []string{"peer@s.whatsapp.net", filepath.Join(t.TempDir(), "out.age")})

	if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), "at least") {
		t.Fatalf("expected a too-short passphrase error, got %v", mock.Errors)
	}
}

func TestCmdExport_RefusesToOverwriteExistingFile(t *testing.T) {
	t.Setenv(exportPassphraseEnvVar, "a-fine-long-passphrase")
	sm, mock := newTestSessionManager(t)

	path := filepath.Join(t.TempDir(), "out.age")
	if err := os.WriteFile(path, []byte("existing content"), 0o600); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}

	cmdExport(sm, nil, "export", []string{"peer@s.whatsapp.net", path})

	if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), "already exists") {
		t.Fatalf("expected an already-exists error, got %v", mock.Errors)
	}

	// The existing file must be untouched.
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing content" {
		t.Errorf("expected the existing file to be left untouched, got %q (err=%v)", data, err)
	}
}

func TestCmdExport_MultiWordPathJoined(t *testing.T) {
	// Proves params[1:] are joined with spaces into one path (matching
	// cmdMedia's convention), without paying for a real (slow-by-design)
	// scrypt derivation: pre-creating a file at the expected joined path and
	// relying on the synchronous "already exists" guard — which runs before
	// any encryption — confirms cmdExport computed the same path.
	t.Setenv(exportPassphraseEnvVar, "a-fine-long-passphrase")
	sm, mock := newTestSessionManager(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "my export.age")
	if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}

	cmdExport(sm, nil, "export", []string{"peer@s.whatsapp.net", filepath.Join(dir, "my"), "export.age"})

	if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), "already exists") {
		t.Fatalf("expected an already-exists error for the joined path %q, got %v", path, mock.Errors)
	}
}

// fastScryptRecipient builds a ScryptRecipient with age's minimum work
// factor, so tests that need genuinely-encrypted content don't each pay
// the library's real ~1s-by-design KDF cost (see
// TestCmdExport_HappyPath_EndToEnd for the one test that deliberately does
// pay it, against the real exportMessagesEncrypted entry point). Decryption
// of the resulting file is correspondingly fast: the work factor travels in
// the ciphertext's own header, so a plain age.NewScryptIdentity needs no
// special setup to read it back.
func fastScryptRecipient(t *testing.T, passphrase string) *age.ScryptRecipient {
	t.Helper()
	r, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		t.Fatalf("NewScryptRecipient error: %v", err)
	}
	r.SetWorkFactor(4)
	return r
}

// ----- Phase 5.8: encryptMessagesToPath + full encrypt/decrypt round trip -----

func TestExportMessagesEncrypted_RoundTrip(t *testing.T) {
	msgs := []Message{
		{Id: "m1", ChatId: "c@s.whatsapp.net", ContactShort: "Alice", Text: "hello there", Timestamp: 1000, FromMe: false},
		{Id: "m2", ChatId: "c@s.whatsapp.net", ContactShort: "Me", Text: "hi Alice!", Timestamp: 1001, FromMe: true},
	}
	path := filepath.Join(t.TempDir(), "export.age")
	passphrase := "correct horse battery staple"

	if err := encryptMessagesToPath(msgs, path, fastScryptRecipient(t, passphrase)); err != nil {
		t.Fatalf("encryptMessagesToPath error: %v", err)
	}

	// The file must not contain the plaintext message text anywhere.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if strings.Contains(string(raw), "hello there") {
		t.Error("expected the exported file to NOT contain plaintext message text")
	}

	// Decrypting with the real age library (not whatscli's own code) proves
	// the file is genuine, standard age ciphertext, decryptable by any
	// age-compatible tool — not just internally self-consistent.
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		t.Fatalf("NewScryptIdentity error: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer f.Close()
	plaintextReader, err := age.Decrypt(f, identity)
	if err != nil {
		t.Fatalf("age.Decrypt error: %v", err)
	}
	plaintext, err := io.ReadAll(plaintextReader)
	if err != nil {
		t.Fatalf("failed to read decrypted plaintext: %v", err)
	}

	var got []Message
	if err := json.Unmarshal(plaintext, &got); err != nil {
		t.Fatalf("failed to unmarshal decrypted JSON: %v", err)
	}
	if len(got) != 2 || got[0].Text != "hello there" || got[1].Text != "hi Alice!" {
		t.Errorf("decrypted messages don't match the originals: %+v", got)
	}
}

func TestExportMessagesEncrypted_WrongPassphraseFailsToDecrypt(t *testing.T) {
	msgs := []Message{{Id: "m1", ChatId: "c@s.whatsapp.net", Text: "secret"}}
	path := filepath.Join(t.TempDir(), "export.age")

	if err := encryptMessagesToPath(msgs, path, fastScryptRecipient(t, "the-real-passphrase")); err != nil {
		t.Fatalf("encryptMessagesToPath error: %v", err)
	}

	identity, err := age.NewScryptIdentity("a-completely-wrong-passphrase")
	if err != nil {
		t.Fatalf("NewScryptIdentity error: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer f.Close()
	if _, err := age.Decrypt(f, identity); err == nil {
		t.Error("expected decryption with the wrong passphrase to fail")
	}
}

func TestExportMessagesEncrypted_EmptyMessageList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.age")
	if err := encryptMessagesToPath(nil, path, fastScryptRecipient(t, "a-fine-long-passphrase")); err != nil {
		t.Fatalf("expected exporting an empty chat to succeed, got: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected an output file even for zero messages: %v", err)
	}
}

func TestExportMessagesEncrypted_RefusesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.age")
	if err := os.WriteFile(path, []byte("preexisting"), 0o600); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}

	// This test intentionally still goes through the real
	// exportMessagesEncrypted (not encryptMessagesToPath directly): the
	// O_EXCL check fires before any KDF work runs, so it's fast regardless
	// of work factor, and using the real entry point exercises
	// NewScryptRecipient's own error path too.
	err := exportMessagesEncrypted([]Message{{Id: "m1"}}, path, "a-fine-long-passphrase")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an already-exists error, got %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "preexisting" {
		t.Error("expected the existing file to be left untouched")
	}
}

// TestCmdExport_HappyPath_EndToEnd is the one test that goes through the
// real, registered cmdExport command with its actual (deliberately slow —
// ~1s by age's own design) scrypt work factor, rather than a fast test-only
// recipient. This is intentionally the only such test in this file: it
// exists once to give real confidence in the full success path (background
// goroutine → real encryption → PrintText), while every other test below
// uses a low work factor via encryptMessagesToPath directly to stay fast.
func TestCmdExport_HappyPath_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow real-scrypt end-to-end test in -short mode")
	}
	t.Setenv(exportPassphraseEnvVar, "a-fine-long-passphrase")
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "e1", ChatId: "peer@s.whatsapp.net", Text: "exported message"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "out.age")
	cmdExport(sm, nil, "export", []string{"peer@s.whatsapp.net", path})

	waitForCmdExportCompletion(t, mock)

	if len(mock.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", mock.Errors)
	}
	if len(mock.Texts) != 1 || !strings.Contains(mock.Texts[0], "Exported 1 messages") {
		t.Errorf("expected a success message mentioning the export, got %v", mock.Texts)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected the output file to exist: %v", err)
	}
}

// waitForCmdExportCompletion polls the mock handler until it has recorded
// either an error or a success text from cmdExport's background goroutine,
// or fails the test after a bounded timeout. The timeout is generous (age's
// own default work factor is calibrated to ~1s on typical hardware, but
// this is the one test that pays that cost for real, and shared/throttled
// CPUs can take substantially longer).
func waitForCmdExportCompletion(t *testing.T, mock *MockUiHandler) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		mock.Lock()
		done := len(mock.Errors) > 0 || len(mock.Texts) > 0
		mock.Unlock()
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for cmdExport's background goroutine to finish")
}
