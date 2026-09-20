package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/normen/whatscli/config"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// sessionFilePath returns the base path for the WhatsApp session files
// (session.db, session_meta.db). A package-level function variable rather
// than a direct call (plan H5.1) so tests can point it at a temp directory:
// config.GetSessionFilePath resolves through adrg/xdg, whose package init()
// reads XDG_CONFIG_HOME before any t.Setenv can take effect, making the
// real path impossible to sandbox otherwise. Production code never
// reassigns this.
var sessionFilePath = config.GetSessionFilePath

// SessionManager deals with the connection and receives commands from the UI
// it updates the UI accordingly
type SessionManager struct {
	db              *MessageDatabase
	currentReceiver string // currently selected chat for message handling
	uiHandler       UiMessageHandler
	client          *whatsmeow.Client
	container       *sqlstore.Container
	StatusChannel   chan StatusMsg
	CommandChannel  chan Command
	statusInfo      SessionStatus
	started         bool
	reconnecting    bool // guards against duplicate reconnect goroutines
	connecting      bool // set while a login attempt (incl. QR pairing) is in progress
	loggedOut       bool // set on explicit logout to suppress auto-reconnect
	stop            chan struct{}
	eventHandler    *eventHandler
	mu              sync.RWMutex
	priorityQueue   PriorityQueue
	convByJID       map[string]*Conversation
	debug           bool // T-2: enable verbose WhatsApp protocol logging

	// logFilePath is an explicit --logfile override; empty means use
	// defaultDebugLogPath(). Only consulted when debug is true.
	logFilePath  string
	debugLogFile *os.File
	debugLogger  waLog.Logger

	// syncWaiters holds one channel per goroutine currently waiting for a
	// history sync to complete (see addSyncWaiter/notifySyncWaiters).
	// Guarded by mu. Replaces the old single buffered historySyncDone
	// channel, which concurrent /backlog waiters raced for: one waiter
	// consumed the only signal and the others sat out their full fallback
	// timeout (plan H3.1).
	syncWaiters []chan struct{}

	// Phase 5.7 voice recording state, guarded by mu like every other field
	// here. recordCancel is non-nil exactly while a recording is in
	// progress; cmdStopRecording and Shutdown both use it to end the
	// recorder's process early.
	recording    bool
	recordCancel context.CancelFunc
}

// initialize the SessionManager
func (sm *SessionManager) Init(handler UiMessageHandler) error {
	sm.db = &MessageDatabase{}
	if err := sm.db.Init(); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}

	// Initialize Priority Queue and lookup map
	sm.priorityQueue = make(PriorityQueue, 0)
	heap.Init(&sm.priorityQueue)
	sm.convByJID = make(map[string]*Conversation)

	// Load conversations from SQLite into PriorityQueue
	convs, err := sm.db.GetConversations()
	if err == nil {
		for _, c := range convs {
			conv := c
			heap.Push(&sm.priorityQueue, &conv)
			sm.convByJID[conv.JID] = &conv
		}
	}

	// When debug logging is on, open the log file and route every
	// PrintError/PrintText call through it in addition to the UI, giving a
	// post-mortem log for connection failures (Phase 4.2). Debug logging is
	// best-effort: a failure to open the file falls back to the plain
	// handler rather than failing session startup over it.
	finalHandler := handler
	if sm.debug {
		f, err := openDebugLog(sm.logFilePath)
		if err != nil {
			handler.PrintError(fmt.Errorf("failed to open debug log: %v", err))
		} else {
			sm.debugLogFile = f
			sm.debugLogger = newFileLogger("whatscli", f)
			finalHandler = &loggingUiHandler{UiMessageHandler: handler, logger: sm.debugLogger}
		}
	}

	sm.uiHandler = finalHandler
	sm.StatusChannel = make(chan StatusMsg, 10)
	sm.CommandChannel = make(chan Command, 10)
	sm.eventHandler = &eventHandler{sm: sm}
	return nil
}

// starts the receiver and message handling go routine
func (sm *SessionManager) StartManager() error {
	if sm.started {
		return errors.New("session manager running, send commands to control")
	}
	sm.started = true
	sm.stop = make(chan struct{})
	go sm.runManager()
	return nil
}

func (sm *SessionManager) runManager() error {
	// Startup login runs in the background through the same guarded login()
	// path as /connect and auto-reconnect (plan H1.5), so no two attempts
	// can ever overlap. A failed startup connection no longer prevents the
	// command loop from starting: the user can still /reset or /connect.
	go func() {
		if err := sm.login(); err != nil {
			sm.uiHandler.PrintError(err)
		}
	}()

	for {
		select {
		case <-sm.stop:
			sm.uiHandler.PrintText("closing the receiver")
			if client := sm.getClient(); client != nil {
				client.Disconnect()
			}
			return nil
		case command := <-sm.CommandChannel:
			sm.execCommand(command)
		case statusMsg := <-sm.StatusChannel:
			prevStatus := sm.statusInfo.Connected
			if statusMsg.err != nil {
				sm.uiHandler.PrintError(statusMsg.err)
			} else {
				sm.statusInfo.Connected = statusMsg.connected
			}

			if client := sm.getClient(); client != nil {
				sm.statusInfo.Connected = client.IsConnected()
			} else {
				sm.statusInfo.Connected = false
			}

			sm.uiHandler.SetStatus(sm.statusInfo)
			if prevStatus != sm.statusInfo.Connected {
				if sm.statusInfo.Connected {
					sm.uiHandler.PrintText("connected")
				} else {
					sm.uiHandler.PrintText("disconnected")
				}
			}
		}
	}
}

// addSyncWaiter registers interest in the next completed history sync and
// returns a channel that notifySyncWaiters will close, plus a cancel func
// that deregisters the waiter. Register BEFORE triggering the action whose
// sync response you want to observe, so a fast response can never slip
// through unobserved; call cancel (e.g. via defer) once done waiting so an
// abandoned waiter — timeout, error, or shutdown — does not linger in the
// registry until some later, unrelated sync (master audit F6). cancel is
// idempotent and safe after the channel was already closed by notify (it
// simply finds nothing to remove).
func (sm *SessionManager) addSyncWaiter() (<-chan struct{}, func()) {
	ch := make(chan struct{})
	sm.mu.Lock()
	sm.syncWaiters = append(sm.syncWaiters, ch)
	sm.mu.Unlock()
	cancel := func() {
		sm.mu.Lock()
		defer sm.mu.Unlock()
		for i, c := range sm.syncWaiters {
			if c == ch {
				sm.syncWaiters = append(sm.syncWaiters[:i], sm.syncWaiters[i+1:]...)
				return
			}
		}
	}
	return ch, cancel
}

// notifySyncWaiters wakes every registered sync waiter exactly once (by
// closing its channel) and clears the registry. Closing happens outside the
// lock; close on a channel never blocks, but there is no reason to hold mu
// while doing it.
func (sm *SessionManager) notifySyncWaiters() {
	sm.mu.Lock()
	waiters := sm.syncWaiters
	sm.syncWaiters = nil
	sm.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
}

// Shutdown performs a clean shutdown, saving data and disconnecting
func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	if sm.started {
		close(sm.stop)
		sm.started = false
	}
	// Stop an in-progress recording rather than leaving the recorder process
	// running as an orphan after the app exits.
	if sm.recordCancel != nil {
		sm.recordCancel()
	}
	sm.mu.Unlock()

	// Disconnect is handled by the runManager loop via channel close,
	// but we can ensure it here too just in case
	if client := sm.getClient(); client != nil && client.IsConnected() {
		client.Disconnect()
	}

	// Close the whatsmeow session store container (plan H2.1) — after the
	// disconnect above, nothing uses its SQLite handle anymore.
	sm.mu.Lock()
	container := sm.container
	sm.container = nil
	sm.mu.Unlock()
	if container != nil {
		if err := container.Close(); err != nil {
			fmt.Printf("warning: failed to close session store: %v\n", err)
		}
	}

	// Close the metadata database to release file handles and flush WAL
	if sm.db != nil {
		if err := sm.db.Close(); err != nil {
			fmt.Printf("warning: failed to close metadata db: %v\n", err)
		}
	}

	// Close the debug log file, if one was opened
	if sm.debugLogFile != nil {
		if err := sm.debugLogFile.Close(); err != nil {
			fmt.Printf("warning: failed to close debug log: %v\n", err)
		}
	}
}

// executes a command via the command registry
func (sm *SessionManager) execCommand(command Command) {
	client := sm.getClient()

	handler, ok := commandRegistry[command.Name]
	if !ok {
		sm.uiHandler.PrintText("[" + config.Config.Colors.Negative + "]Unknown command: [-]" + command.Name)
		return
	}
	handler(sm, client, command.Name, command.Params)
}

// helper for built-in command help
func (sm *SessionManager) printCommandUsage(command string, usage string) {
	sm.uiHandler.PrintText("[" + config.Config.Colors.Negative + "]Usage:[-] " + command + " " + usage)
}

// check if parameters for command are okay
func checkParam(arr []string, length int) bool {
	if arr == nil || len(arr) < length {
		return false
	}
	return true
}

var (
	notifyMu       sync.Mutex
	lastNotifyTime time.Time
)

// notify will send a notification via beeep if EnableNotification is true. If
// UseTerminalBell is true it will send a terminal bell instead. Repeated
// notifications are throttled to at most one per NotificationTimeout seconds,
// so a burst of incoming messages does not spam the desktop notifier.
func notify(title, message string) error {
	if !config.Config.General.EnableNotifications {
		return nil
	}

	timeout := time.Duration(config.Config.General.NotificationTimeout) * time.Second
	notifyMu.Lock()
	if time.Since(lastNotifyTime) < timeout {
		notifyMu.Unlock()
		return nil
	}
	lastNotifyTime = time.Now()
	notifyMu.Unlock()

	if config.Config.General.UseTerminalBell {
		_, err := fmt.Printf("\a")
		return err
	}
	return beeep.Notify(title, message, "")
}
