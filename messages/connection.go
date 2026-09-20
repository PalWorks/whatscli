package messages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/normen/whatscli/qrcode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// SetDebug enables or disables verbose WhatsApp protocol logging (T-2).
func (sm *SessionManager) SetDebug(on bool) { sm.debug = on }

// SetLogFile overrides the debug log file path (see defaultDebugLogPath).
// Only meaningful when SetDebug(true) is also set; must be called before
// Init().
func (sm *SessionManager) SetLogFile(path string) { sm.logFilePath = path }

// getConnection creates or returns the existing WhatsApp client.
func (sm *SessionManager) getConnection() (*whatsmeow.Client, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.client == nil {
		// Create database store for WhatsApp
		dbPath := sessionFilePath() + ".db"
		// T-2: Use verbose logger when debug mode is enabled, writing to
		// the same debug log file as everything else logged via Init()
		// (Phase 4.2). Falls back to stdout if the file logger wasn't set
		// up (e.g. it failed to open — see Init()).
		var logger waLog.Logger = waLog.Noop
		if sm.debug {
			if sm.debugLogger != nil {
				logger = sm.debugLogger.Sub("CLIENT")
			} else {
				logger = waLog.Stdout("CLIENT", "DEBUG", true)
			}
		}
		container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", logger)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %v", err)
		}

		// Get device store
		deviceStore, err := container.GetFirstDevice(context.Background())
		if err != nil {
			container.Close()
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

// ownJID returns the local user's JID string, or "" if the client is not
// currently logged in. Store.ID is a *types.JID that whatsmeow sets to nil on
// Store.Delete (logout/reset/expired-session re-pair), so a backgrounded send
// that dereferences it directly can panic the process if a teardown lands
// mid-send (master audit F3). Every read of the own JID for message
// construction goes through this guard.
func ownJID(client *whatsmeow.Client) string {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return ""
	}
	return client.Store.ID.String()
}

// errAlreadyConnecting is returned by login when another connection attempt
// (startup, manual /connect, or auto-reconnect) is already in progress. The
// QR pairing flow can legitimately run for minutes, so overlapping attempts
// are a real possibility once /connect runs off the command loop (plan H1.5).
var errAlreadyConnecting = errors.New("connection attempt already in progress")

// login logs in the user. It tries to see if a session already exists. If not,
// tries to create a new one using qr scanned on the terminal. Every login
// path (startup, /connect, auto-reconnect) goes through this function, so the
// connecting guard below is the single point that prevents overlapping
// attempts.
func (sm *SessionManager) login() error {
	// Guard against concurrent attempts, and take ownership of any existing
	// connection for teardown, in one critical section.
	sm.mu.Lock()
	if sm.connecting {
		sm.mu.Unlock()
		return errAlreadyConnecting
	}
	sm.connecting = true
	oldClient := sm.client
	oldContainer := sm.container
	sm.client = nil
	sm.container = nil
	sm.mu.Unlock()
	defer func() {
		sm.mu.Lock()
		sm.connecting = false
		sm.mu.Unlock()
	}()

	// Tear down what the previous session left behind (plan H2.1, audit F4):
	// disconnect the old client so its websocket and registered event
	// handler don't outlive it (a manual Disconnect does not emit
	// events.Disconnected, so this cannot trigger a reconnect storm), then
	// close the old store container to release its SQLite handle —
	// previously both leaked on every reconnect. Disconnect is called even
	// on a currently-disconnected client: whatsmeow's own EnableAutoReconnect
	// (on by default) keeps a pending internal reconnect armed until
	// Disconnect marks the disconnect as expected — skipping it lets the
	// dropped client resurrect itself later as a duplicate (master audit F1).
	if oldClient != nil {
		oldClient.Disconnect()
	}
	if oldContainer != nil {
		if err := oldContainer.Close(); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to close previous session store: %v", err))
		}
	}

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
			// getConnection which acquires its own lock. The container this
			// login created minutes ago must be closed too, or it leaks on
			// every expired-session re-pair (master audit F2).
			sm.mu.Lock()
			sm.client = nil
			oldContainer := sm.container
			sm.container = nil
			sm.mu.Unlock()
			if oldContainer != nil {
				if err := oldContainer.Close(); err != nil {
					sm.uiHandler.PrintError(fmt.Errorf("failed to close expired session store: %v", err))
				}
			}

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
		case evt, ok := <-qrChan:
			if !ok {
				// whatsmeow closes the QR channel after any terminal event.
				// If none of the branches below returned first, pairing is
				// over without a login — returning (instead of reading zero
				// values from the closed channel forever) is what releases
				// the connecting guard (audit F1).
				sm.StatusChannel <- StatusMsg{false, nil}
				return errors.New("QR pairing ended without login — use /connect to try again")
			}
			switch evt.Event {
			case "code":
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

			case "success":
				sm.uiHandler.PrintText("Successfully logged in!")
				sm.StatusChannel <- StatusMsg{true, nil}

				// Load existing chats after successful login
				go sm.loadRecentChats()

				return nil

			case "timeout":
				// Terminal per whatsmeow's QRChannelTimeout: the server
				// disconnected the socket before pairing completed. New
				// codes within the validity window arrive as repeated
				// "code" events, never as "timeout" (audit F1).
				sm.StatusChannel <- StatusMsg{false, errors.New("QR login timed out")}
				return errors.New("QR login timed out — use /connect to try again")

			default:
				// "error" plus the err-* family (client outdated, scanned
				// without multidevice, unexpected state) — all terminal.
				if evt.Error != nil {
					return fmt.Errorf("QR pairing failed (%s): %v", evt.Event, evt.Error)
				}
				return fmt.Errorf("QR pairing failed: %s", evt.Event)
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

		// A manual /connect may have succeeded during the backoff wait (both
		// paths run off the command loop since plan H1.5) — don't stomp a
		// live connection with another login. A login() that collides with a
		// still-in-progress manual attempt returns errAlreadyConnecting and
		// simply counts as a failed attempt below, retrying after backoff.
		if c := sm.getClient(); c != nil && c.IsConnected() {
			sm.mu.Lock()
			sm.reconnecting = false
			sm.mu.Unlock()
			return
		}

		if err := sm.login(); err != nil {
			if errors.Is(err, errAlreadyConnecting) {
				// A manual /connect owns the connection process — stand down
				// instead of burning the retry budget against the guard
				// (audit F6). If that attempt fails, the user retries or the
				// next Disconnected event starts a fresh reconnect cycle.
				sm.mu.Lock()
				sm.reconnecting = false
				sm.mu.Unlock()
				return
			}
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
	// A backgrounded /connect (possibly mid-QR-pairing) owns the client and
	// container this would tear down — refuse rather than delete a
	// half-paired device's store out from under it (master audit F4). The
	// connecting flag is the single point of mutual exclusion shared with
	// login(), cmdReset, and startup.
	if sm.connecting {
		sm.mu.Unlock()
		return errAlreadyConnecting
	}
	sm.loggedOut = true // suppress auto-reconnect
	if sm.client == nil {
		sm.mu.Unlock()
		sm.StatusChannel <- StatusMsg{false, nil}
		sm.uiHandler.PrintText("Already logged out")
		return nil
	}

	// Hold the connecting guard for the whole teardown so no login can start
	// against the client/container we're about to destroy (master audit F4).
	sm.connecting = true
	defer func() {
		sm.mu.Lock()
		sm.connecting = false
		sm.mu.Unlock()
	}()

	// Capture and clear client + container under lock
	client := sm.client
	sm.client = nil
	container := sm.container
	sm.container = nil
	sm.mu.Unlock()

	// Disconnect + delete session outside lock (avoids blocking other
	// goroutines). Disconnect is unconditional so whatsmeow's own
	// auto-reconnect is disarmed even on an already-disconnected client
	// (master audit F1).
	client.Disconnect()

	if client.Store != nil {
		err := client.Store.Delete(context.Background())
		if err != nil {
			sm.uiHandler.PrintText("Warning: Couldn't properly remove session: " + err.Error())
		}
	}

	// Close the store container AFTER Store.Delete (which needs its DB
	// handle) — plan H2.1: this handle previously leaked on every logout.
	if container != nil {
		if err := container.Close(); err != nil {
			sm.uiHandler.PrintText("Warning: Couldn't close session store: " + err.Error())
		}
	}

	sm.uiHandler.PrintText("Successfully logged out")
	sm.StatusChannel <- StatusMsg{false, nil}
	return nil
}
