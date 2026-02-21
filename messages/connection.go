package messages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/qrcode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// SetDebug enables or disables verbose WhatsApp protocol logging (T-2).
func (sm *SessionManager) SetDebug(on bool) { sm.debug = on }

// getConnection creates or returns the existing WhatsApp client.
func (sm *SessionManager) getConnection() (*whatsmeow.Client, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.client == nil {
		// Create database store for WhatsApp
		dbPath := config.GetSessionFilePath() + ".db"
		// T-2: Use verbose logger when debug mode is enabled.
		var logger waLog.Logger = waLog.Noop
		if sm.debug {
			logger = waLog.Stdout("CLIENT", "DEBUG", true)
		}
		container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", logger)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %v", err)
		}

		// Get device store
		deviceStore, err := container.GetFirstDevice(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to get device: %v", err)
		}

		// Create client
		client := whatsmeow.NewClient(deviceStore, logger)

		// Set event handler
		client.AddEventHandler(sm.eventHandler.Handle)

		sm.client = client
		sm.container = container
	}

	return sm.client, nil
}

// getClient safely returns the current client pointer under a read lock.
func (sm *SessionManager) getClient() *whatsmeow.Client {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.client
}

// login logs in the user. It tries to see if a session already exists. If not,
// tries to create a new one using qr scanned on the terminal.
func (sm *SessionManager) login() error {
	// Clear any existing connection for retry
	sm.mu.Lock()
	sm.client = nil
	sm.mu.Unlock()

	client, err := sm.getConnection()
	if err != nil {
		return fmt.Errorf("failed to create WhatsApp connection: %v", err)
	}

	if client == nil {
		return errors.New("could not establish WhatsApp connection")
	}

	// Try to log in
	return sm.loginWithConnection(client)
}

// loginWithConnection logs in the user using a provided connection. It tries to
// see if a session already exists. If not, tries to create a new one using qr
// scanned on the terminal.
func (sm *SessionManager) loginWithConnection(client *whatsmeow.Client) error {
	sm.uiHandler.PrintText("connecting..")

	// Ensure connection is clean before starting
	if client.IsConnected() {
		client.Disconnect()
		sm.StatusChannel <- StatusMsg{false, nil}
		// Small pause to ensure disconnection completes
		time.Sleep(500 * time.Millisecond)
	}

	// Check if we need to pair
	if client.Store.ID == nil {
		// Need to pair with QR code
		return sm.loginWithQRCode(client)
	}

	// We have credentials, try connecting
	err := client.Connect()
	if err != nil {
		// If we get authentication errors, we may need to re-pair
		if errors.Is(err, whatsmeow.ErrNotConnected) ||
			errors.Is(err, whatsmeow.ErrNotLoggedIn) {
			sm.uiHandler.PrintText("Session expired, need to scan QR code again")

			// Clear the device from the store
			err := client.Store.Delete(context.Background())
			if err != nil {
				return fmt.Errorf("failed to clear expired session: %v", err)
			}

			// Recreate the client — set nil under lock, then call
			// getConnection which acquires its own lock
			sm.mu.Lock()
			sm.client = nil
			sm.mu.Unlock()

			client, err = sm.getConnection()
			if err != nil {
				return fmt.Errorf("failed to create new connection: %v", err)
			}

			// Try pairing
			return sm.loginWithQRCode(client)
		}

		return fmt.Errorf("connection failed: %v", err)
	}

	sm.uiHandler.PrintText("Session restored successfully")
	sm.StatusChannel <- StatusMsg{true, nil}

	// Load existing chats after successful connection
	go sm.loadRecentChats()

	return nil
}

// loginWithQRCode handles the QR code pairing flow.
func (sm *SessionManager) loginWithQRCode(client *whatsmeow.Client) error {
	sm.uiHandler.PrintText("Please scan the QR code with your phone")

	// Request QR code
	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get QR channel: %v", err)
	}
	err = client.Connect()
	if err != nil {
		return fmt.Errorf("error connecting to WhatsApp: %v", err)
	}

	qrCount := 0
	currentQR := ""
	qrTimeout := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case evt := <-qrChan:
			if evt.Event == "code" {
				qrCount++
				if qrCount > 6 {
					sm.uiHandler.PrintText("QR code attempt limit reached. Login timed out.")
					sm.StatusChannel <- StatusMsg{false, errors.New("login timed out")}
					return errors.New("QR code limit reached")
				}

				// Convert to ASCII QR code and print
				terminal := qrcode.New()
				qrStr := terminal.Get(evt.Code)

				currentQR = string(*qrStr)
				qrTimeout = 20 // Reset countdown to 20s as requested

				// Atomic Update
				sm.uiHandler.UpdateQR(currentQR, qrCount, qrTimeout)

			} else if evt.Event == "success" {
				sm.uiHandler.PrintText("Successfully logged in!")
				sm.StatusChannel <- StatusMsg{true, nil}

				// Load existing chats after successful login
				go sm.loadRecentChats()

				return nil
			} else if evt.Event == "timeout" {
				sm.uiHandler.PrintText("QR code timeout. Waiting for new code...")
			} else if evt.Event == "error" {
				return fmt.Errorf("QR code event error")
			}
		case <-ticker.C:
			if qrTimeout > 0 && currentQR != "" {
				qrTimeout--
				sm.uiHandler.UpdateQR(currentQR, qrCount, qrTimeout)
			} else if qrTimeout == 0 && currentQR != "" {
				// Prevent negative countdown, just show waiting
				qrTimeout = -1
				sm.uiHandler.UpdateQR(currentQR, qrCount, 0) // Show 0s
			}
		}
	}
}

// disconnect disconnects the WhatsApp session.
func (sm *SessionManager) disconnect() error {
	if client := sm.getClient(); client != nil && client.IsConnected() {
		client.Disconnect()
		sm.StatusChannel <- StatusMsg{false, nil}
	}
	return nil
}

// scheduleReconnect attempts to re-establish the WhatsApp connection using
// exponential backoff: 2s → 4s → 8s → 16s → 30s, max 5 attempts.
func (sm *SessionManager) scheduleReconnect() {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second
	const maxAttempts = 5

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check if we should still reconnect
		sm.mu.RLock()
		abort := sm.loggedOut || !sm.started
		sm.mu.RUnlock()
		if abort {
			sm.mu.Lock()
			sm.reconnecting = false
			sm.mu.Unlock()
			return
		}

		sm.uiHandler.PrintText(fmt.Sprintf("Reconnecting in %v... (attempt %d/%d)", backoff, attempt, maxAttempts))

		// Wait with backoff — check stop channel to allow clean shutdown
		select {
		case <-sm.stop:
			sm.mu.Lock()
			sm.reconnecting = false
			sm.mu.Unlock()
			return
		case <-time.After(backoff):
		}

		// Re-check after wait — Shutdown() may have run during the backoff (audit R-5).
		sm.mu.RLock()
		abortAfterWait := sm.loggedOut || !sm.started
		sm.mu.RUnlock()
		if abortAfterWait {
			sm.mu.Lock()
			sm.reconnecting = false
			sm.mu.Unlock()
			return
		}

		if err := sm.login(); err != nil {
			sm.uiHandler.PrintText(fmt.Sprintf("Reconnect attempt %d failed: %v", attempt, err))
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Success — reconnecting flag will be cleared by Connected event
		return
	}

	sm.mu.Lock()
	sm.reconnecting = false
	sm.mu.Unlock()
	sm.uiHandler.PrintError(errors.New("auto-reconnect failed after 5 attempts — use /connect to retry"))
}

// logout logs out the user, deletes session file.
func (sm *SessionManager) logout() error {
	sm.mu.Lock()
	sm.loggedOut = true // suppress auto-reconnect
	if sm.client == nil {
		sm.mu.Unlock()
		sm.StatusChannel <- StatusMsg{false, nil}
		sm.uiHandler.PrintText("Already logged out")
		return nil
	}

	// Capture and clear client under lock
	client := sm.client
	sm.client = nil
	sm.mu.Unlock()

	// Disconnect + delete session outside lock (avoids blocking other goroutines)
	if client.IsConnected() {
		client.Disconnect()
	}

	if client.Store != nil {
		err := client.Store.Delete(context.Background())
		if err != nil {
			sm.uiHandler.PrintText("Warning: Couldn't properly remove session: " + err.Error())
		}
	}

	sm.uiHandler.PrintText("Successfully logged out")
	sm.StatusChannel <- StatusMsg{false, nil}
	return nil
}
