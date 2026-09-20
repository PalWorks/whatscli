package messages

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.mau.fi/whatsmeow"
)

func cmdLogin(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	// login blocks for the whole pairing flow (a QR scan is user-paced and
	// can take minutes), so it must not run on the command loop (plan H1.5).
	// The connecting guard inside login prevents overlapping attempts.
	go func() {
		err := sm.login()
		switch {
		case err == nil:
			sm.uiHandler.PrintText("Successfully connected to WhatsApp")
		case errors.Is(err, errAlreadyConnecting):
			sm.uiHandler.PrintError(err)
		default:
			sm.uiHandler.PrintError(fmt.Errorf("WhatsApp connection failed: %v", err))
			sm.uiHandler.PrintText("Try using /reset to completely reset the connection")
		}
	}()
}

func cmdDisconnect(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintError(sm.disconnect())
}

func cmdLogout(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintError(sm.logout())
}

func cmdReset(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	// Fully reset everything — must use sm.client directly under write lock
	// (not the captured client parameter) because we need to nil it out.
	sm.mu.Lock()
	if sm.connecting {
		// A backgrounded /connect (possibly mid-QR-pairing) is still using
		// the client and session store this command is about to destroy
		// (audit F5) — resetting under it would delete session.db while the
		// pairing goroutine keeps writing keys into the unlinked file.
		sm.mu.Unlock()
		sm.uiHandler.PrintError(errors.New("a connection attempt is in progress — wait for it to finish before /reset"))
		return
	}
	// Hold the connecting guard for the whole reset so a reconnect-driven
	// login() cannot start between here and the file removal below and race
	// on session.db (master audit F5). Cleared at the very end.
	sm.connecting = true
	defer func() {
		sm.mu.Lock()
		sm.connecting = false
		sm.mu.Unlock()
	}()

	// Capture client + container under the lock, then do the disconnect and
	// the Store.Delete DB write OUTSIDE the lock — holding sm.mu across a DB
	// write violates the project's locking rule (master audit F9).
	localClient := sm.client
	sm.client = nil
	container := sm.container
	sm.container = nil
	sm.mu.Unlock()

	if localClient != nil {
		// Unconditional Disconnect disarms whatsmeow's auto-reconnect even on
		// an already-disconnected client (master audit F1).
		localClient.Disconnect()

		if localClient.Store != nil {
			if err := localClient.Store.Delete(context.Background()); err != nil {
				sm.uiHandler.PrintText("Warning: Couldn't remove session: " + err.Error())
			}
		}
	}

	// Close the store container BEFORE removing its database file (plan
	// H2.1) — previously the handle leaked and the file was unlinked while
	// still open.
	if container != nil {
		if err := container.Close(); err != nil {
			sm.uiHandler.PrintText("Warning: Couldn't close session store: " + err.Error())
		}
	}

	// Remove the WhatsApp session DB file
	dbPath := sessionFilePath() + ".db"
	err := os.Remove(dbPath)
	if err != nil && !os.IsNotExist(err) {
		sm.uiHandler.PrintText("Warning: Couldn't remove session database: " + err.Error())
	}

	// Also remove the metadata DB (messages, conversations) — audit R-4.
	sm.db.Close()
	metaPath := sessionFilePath() + "_meta.db"
	err = os.Remove(metaPath)
	if err != nil && !os.IsNotExist(err) {
		sm.uiHandler.PrintText("Warning: Couldn't remove metadata database: " + err.Error())
	}

	// Re-init the meta DB and clear in-memory state
	if initErr := sm.db.Init(); initErr != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to re-initialize metadata DB: %v", initErr))
	}
	sm.mu.Lock()
	sm.convByJID = make(map[string]*Conversation)
	sm.priorityQueue = make(PriorityQueue, 0)
	sm.loggedOut = false // Clear so auto-reconnect works on next /connect
	sm.mu.Unlock()

	sm.uiHandler.PrintText("Session reset. Use /connect to reconnect with a new QR code.")
	sm.StatusChannel <- StatusMsg{false, nil}
}
