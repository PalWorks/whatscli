package messages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// minExportPassphraseLength is a minimal client-side sanity floor. age's
// scrypt KDF makes brute-forcing expensive per guess, but a trivially short
// passphrase (e.g. "1234") is still weak regardless of KDF cost.
const minExportPassphraseLength = 8

// exportPassphraseEnvVar names the environment variable /export reads its
// passphrase from, rather than accepting it as a command parameter. A typed
// command parameter would sit in plaintext in the visible input field (and
// potentially scrollback) for as long as it takes to type and submit; an
// env var set before launching whatscli is the more conventional way
// command-line tools accept secrets without echoing them to the screen.
const exportPassphraseEnvVar = "WHATSCLI_EXPORT_PASSPHRASE"

// cmdExport writes every stored message for a chat to path, encrypted with
// age (https://age-encryption.org) using a passphrase read from
// WHATSCLI_EXPORT_PASSPHRASE. The export is standard age-format ciphertext,
// so it can be decrypted with any age-compatible tool — including the
// official age CLI — without whatscli needing to implement decryption
// itself: `age -d -p exported.age > messages.json`.
//
// params[0] is the chat JID; the remaining params are joined to form the
// destination path (matching cmdMedia's convention for multi-word paths).
func cmdExport(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 2) {
		sm.printCommandUsage(cmdName, "<chat-jid> <output-path>")
		return
	}

	chatID := params[0]
	if _, err := types.ParseJID(chatID); err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID %q: %v", chatID, err))
		return
	}

	path := strings.Join(params[1:], " ")

	passphrase := os.Getenv(exportPassphraseEnvVar)
	if passphrase == "" {
		sm.uiHandler.PrintError(fmt.Errorf("set %s before running /export (see README)", exportPassphraseEnvVar))
		return
	}
	if len(passphrase) < minExportPassphraseLength {
		sm.uiHandler.PrintError(fmt.Errorf("%s must be at least %d characters", exportPassphraseEnvVar, minExportPassphraseLength))
		return
	}

	if _, err := os.Stat(path); err == nil {
		sm.uiHandler.PrintError(fmt.Errorf("%s already exists — choose a different path", path))
		return
	} else if !os.IsNotExist(err) {
		sm.uiHandler.PrintError(fmt.Errorf("failed to check destination path: %v", err))
		return
	}

	// The full-chat DB read, JSON marshaling and the scrypt KDF (deliberately
	// slow/memory-hard) all run off the command loop (plan H2.3) — a large
	// chat history is a different cost class than the single-row lookups
	// other commands do synchronously.
	go func() {
		msgs, err := sm.db.GetMessages(chatID)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
			return
		}

		if err := exportMessagesEncrypted(msgs, path, passphrase); err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		sm.uiHandler.PrintText(fmt.Sprintf(
			"Exported %d messages to %s (age-encrypted; decrypt with: age -d -p %s > messages.json)",
			len(msgs), path, path))
	}()
}

// exportMessagesEncrypted marshals msgs to JSON and writes them to path as
// age ciphertext under a passphrase-derived (scrypt) recipient, using age's
// own default work factor (calibrated to ~1s on typical hardware — a
// reasonable one-time cost for an explicit, infrequent archival action, and
// not something to weaken for speed).
func exportMessagesEncrypted(msgs []Message, path, passphrase string) error {
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("failed to derive encryption key: %w", err)
	}
	return encryptMessagesToPath(msgs, path, recipient)
}

// encryptMessagesToPath does the actual marshal+encrypt+write, parameterized
// on recipient so tests can substitute a cheap-to-compute one (see
// cmd_export_test.go) without changing exportMessagesEncrypted's real,
// security-calibrated default. path must not already exist (checked by the
// caller and enforced again here via O_EXCL); a failed write does not leave
// a partial file behind.
func encryptMessagesToPath(msgs []Message, path string, recipient age.Recipient) error {
	plaintext, err := json.MarshalIndent(msgs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode messages: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists — choose a different path", path)
		}
		return fmt.Errorf("failed to create %s: %w", path, err)
	}

	w, err := age.Encrypt(f, recipient)
	if err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("failed to start encryption: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("failed to write encrypted data: %w", err)
	}
	if err := w.Close(); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("failed to finalize encryption: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("failed to close %s: %w", path, err)
	}
	return nil
}
