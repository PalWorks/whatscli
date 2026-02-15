package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/qrcode"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// SessionManager deals with the connection and receives commands from the UI
// it updates the UI accordingly
type SessionManager struct {
	db              *MessageDatabase
	currentReceiver string // currently selected chat for message handling
	uiHandler       UiMessageHandler
	client          *whatsmeow.Client
	container       *sqlstore.Container
	BatteryChannel  chan BatteryMsg
	StatusChannel   chan StatusMsg
	CommandChannel  chan Command
	ChatChannel     chan Chat
	TextChannel     chan *waProto.Message
	statusInfo      SessionStatus
	lastSent        time.Time
	started         bool
	reconnecting    bool // guards against duplicate reconnect goroutines
	loggedOut       bool // set on explicit logout to suppress auto-reconnect
	stop            chan struct{}
	eventHandler    *eventHandler
	mu              sync.RWMutex
	priorityQueue   PriorityQueue
	convByJID       map[string]*Conversation
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

	sm.uiHandler = handler
	sm.BatteryChannel = make(chan BatteryMsg, 10)
	sm.StatusChannel = make(chan StatusMsg, 10)
	sm.CommandChannel = make(chan Command, 10)
	sm.ChatChannel = make(chan Chat, 10)
	sm.TextChannel = make(chan *waProto.Message, 10)
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
	client, err := sm.getConnection()
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to create WhatsApp connection: %v", err))
		return err
	}

	if client == nil {
		sm.uiHandler.PrintError(errors.New("could not establish WhatsApp connection"))
		return errors.New("could not establish WhatsApp connection")
	}

	// Run login asynchronously to allow command processing
	go func() {
		err = sm.loginWithConnection(client)
		if err != nil {
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
		case batteryMsg := <-sm.BatteryChannel:
			sm.statusInfo.BatteryLoading = batteryMsg.loading
			sm.statusInfo.BatteryPowersave = batteryMsg.powersave
			sm.statusInfo.BatteryCharge = batteryMsg.charge
			sm.uiHandler.SetStatus(sm.statusInfo)
		case statusMsg := <-sm.StatusChannel:
			prevStatus := sm.statusInfo.Connected
			if statusMsg.err != nil {
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

// set the currently selected chat
func (sm *SessionManager) setCurrentReceiver(id string) {
	sm.mu.Lock()
	sm.currentReceiver = id
	sm.mu.Unlock()
	screen := sm.getMessages(id)
	sm.uiHandler.NewScreen(screen)
}

// get the next message id to select (highlighted + offset)
func (sm *SessionManager) getConnection() (*whatsmeow.Client, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.client == nil {
		// Create database store for WhatsApp
		dbPath := config.GetSessionFilePath() + ".db"
		container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", waLog.Noop)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %v", err)
		}

		// Get device store
		deviceStore, err := container.GetFirstDevice(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to get device: %v", err)
		}

		// Create client
		client := whatsmeow.NewClient(deviceStore, waLog.Noop)

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

// snapshotPQ returns a deep copy of the priority queue. Caller must hold sm.mu.
func (sm *SessionManager) snapshotPQ() []*Conversation {
	safeList := make([]*Conversation, len(sm.priorityQueue))
	for i, item := range sm.priorityQueue {
		c := *item
		safeList[i] = &c
	}
	return safeList
}

// login logs in the user. It tries to see if a session already exists. If not, tries to create a
// new one using qr scanned on the terminal.
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

// loginWithConnection logs in the user using a provided connection. It tries to see if a session already exists. If not, tries to create a
// new one using qr scanned on the terminal.
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

// Helper method to login with QR code
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

// loadRecentChats fetches recent chats from WhatsApp and adds them to the database
func (sm *SessionManager) loadRecentChats() {
	sm.uiHandler.PrintText("Loading chats...")

	// Capture client pointer safely
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}

	// Try to get all chats through the whatsmeow API
	if client.Store != nil && client.Store.Contacts != nil {
		contacts, err := client.Store.Contacts.GetAllContacts(context.Background())
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to load contacts for chat list: %v", err))
			return
		}

		// Process each contact as a potential chat
		chatCount := 0

		// Map to track what we've processed to avoid duplicates in PQ if reloading
		processed := make(map[string]bool)

		for jid, contact := range contacts {
			if !contact.Found {
				continue
			}

			// Skip non-chat JIDs
			if jid.Server != "s.whatsapp.net" && jid.Server != "g.us" {
				continue
			}

			jidStr := jid.String()
			processed[jidStr] = true

			// Determine name
			var name string
			isGroup := jid.Server == "g.us"
			if isGroup {
				groupInfo, err := client.GetGroupInfo(context.Background(), jid)
				if err == nil && groupInfo.Name != "" {
					name = groupInfo.Name
				} else {
					name = "Group: " + jid.User
				}
			} else {
				name = contact.FullName
				if name == "" {
					name = contact.PushName
				}
				if name == "" {
					name = jid.User
				}
			}

			// Create/Update Conversation struct
			// In a real sync, we might want to check the actual last message time.
			// For now, if it's a new import, use current time.
			// If it exists in DB, we should probably prefer the DB's time unless we have new info.

			// Update PQ under lock, collect DB write for outside
			sm.mu.Lock()
			existingConv := sm.convByJID[jidStr]

			var toUpsert *Conversation
			if existingConv != nil {
				if name != "" && existingConv.Name != name {
					existingConv.Name = name
					c := *existingConv
					toUpsert = &c
				}
			} else {
				newConv := &Conversation{
					JID:         jidStr,
					Name:        name,
					LastMsgTime: 0,
					Preview:     "New chat",
					Unread:      0,
					IsPinned:    false,
				}
				heap.Push(&sm.priorityQueue, newConv)
				sm.convByJID[newConv.JID] = newConv
				c := *newConv
				toUpsert = &c
			}
			sm.mu.Unlock()

			if toUpsert != nil {
				if err := sm.db.UpsertConversation(*toUpsert); err != nil {
					sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
				}
			}

			chatCount++
		}

		sm.mu.Lock()
		safeList := sm.snapshotPQ()
		sm.mu.Unlock()
		sm.uiHandler.UpdateChatList(safeList)

		sm.uiHandler.PrintText("Chats loaded.")
	} else {
		sm.uiHandler.PrintError(errors.New("failed to access contacts store"))
	}
}

// loadRecentMessages loads the most recent messages for a chat
func (sm *SessionManager) loadRecentMessages(chatJID string) {
	if client := sm.getClient(); client == nil || !client.IsConnected() {
		return
	}

	// For now, message history retrieval is limited in whatsmeow
	// Messages will be populated as they're sent and received
	// silenced: sm.uiHandler.PrintText(fmt.Sprintf("Message history for %s will be populated as you communicate", chatJID))

	// If this is the currently selected chat, update the UI
	if chatJID == sm.currentReceiver {
		screen := sm.getMessages(chatJID)
		sm.uiHandler.NewScreen(screen)
	}
}

// Helper functions to handle nil pointers safely
func stringOrEmpty(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func boolOrFalse(ptr *bool) bool {
	if ptr == nil {
		return false
	}
	return *ptr
}

func uint64OrZero(ptr *uint64) uint64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}

// getChatName returns the best display name for a chat
func (sm *SessionManager) getChatName(jid types.JID) string {
	client := sm.getClient()
	if client == nil {
		return jid.User
	}

	// For groups, use the group name if available
	if jid.Server == "g.us" {
		groupInfo, err := client.GetGroupInfo(context.Background(), jid)
		if err == nil && groupInfo.Name != "" {
			return groupInfo.Name
		}
		return "Group Chat"
	}

	// For individual chats, try to get the contact name
	if client.Store != nil {
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found {
			if contact.FullName != "" {
				return contact.FullName
			}
			if contact.PushName != "" {
				return contact.PushName
			}
		}
	}

	// Fallback to JID User (Phone Number)
	return jid.User
}

// disconnects the session
func (sm *SessionManager) disconnect() error {
	if client := sm.getClient(); client != nil && client.IsConnected() {
		client.Disconnect()
		sm.StatusChannel <- StatusMsg{false, nil}
	}
	return nil
}

// Shutdown performs a clean shutdown, saving data and disconnecting
func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	if sm.started {
		close(sm.stop)
		sm.started = false
	}
	sm.mu.Unlock()

	// Disconnect is handled by the runManager loop via channel close,
	// but we can ensure it here too just in case
	if client := sm.getClient(); client != nil && client.IsConnected() {
		client.Disconnect()
	}

	// Close the metadata database to release file handles and flush WAL
	if sm.db != nil {
		if err := sm.db.Close(); err != nil {
			fmt.Printf("warning: failed to close metadata db: %v\n", err)
		}
	}
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

// logout logs out the user, deletes session file
func (sm *SessionManager) logout() error {
	sm.mu.Lock()
	sm.loggedOut = true // suppress auto-reconnect
	if sm.client == nil {
		sm.StatusChannel <- StatusMsg{false, nil}
		sm.uiHandler.PrintText("Already logged out")
		sm.mu.Unlock()
		return nil
	}

	if sm.client.IsConnected() {
		sm.client.Disconnect()
	}

	// Delete device from store
	if sm.client.Store != nil {
		err := sm.client.Store.Delete(context.Background())
		if err != nil {
			sm.uiHandler.PrintText("Warning: Couldn't properly remove session: " + err.Error())
		}
	}

	// Reset client
	sm.client = nil
	sm.mu.Unlock()

	sm.uiHandler.PrintText("Successfully logged out")
	sm.StatusChannel <- StatusMsg{false, nil}
	return nil
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

// get all messages for one chat id
func (sm *SessionManager) getMessages(wid string) []Message {
	msgs, err := sm.db.GetMessages(wid)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
		return []Message{}
	}
	return msgs
}

// sends text to whatsapp id
func (sm *SessionManager) sendText(wid string, text string) {
	sm.mu.RLock()
	client := sm.client
	sm.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}

	// Parse JID
	receiver, err := types.ParseJID(wid)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
		return
	}

	// Create message
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}

	// Send message
	sm.lastSent = time.Now()
	resp, err := client.SendMessage(context.Background(), receiver, msg)

	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to send message: %v", err))
	} else {
		// Create a Message struct to save to the database
		newMsg := Message{
			Id:           resp.ID,
			ChatId:       wid,
			FromMe:       true,
			Timestamp:    uint64(time.Now().Unix()),
			Text:         text,
			ContactId:    client.Store.ID.String(),
			ContactName:  "Me",
			ContactShort: "Me",
		}

		if err := sm.db.AddMessage(newMsg); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to save sent message: %v", err))
		}

		sm.mu.RLock()
		isCurrent := sm.currentReceiver == wid
		sm.mu.RUnlock()

		if isCurrent {
			sm.uiHandler.NewMessage(newMsg)
		}
	}
}

// notify will send a notification via beeep if EnableNotification is true. If
// UseTerminalBell is true it will send a terminal bell instead.
func notify(title, message string) error {
	if !config.Config.General.EnableNotifications {
		return nil
	} else if config.Config.General.UseTerminalBell {
		_, err := fmt.Printf("\a")
		return err
	}
	return beeep.Notify(title, message, "")
}

// Event handler for whatsmeow events
type eventHandler struct {
	sm *SessionManager
}

func (eh *eventHandler) Handle(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		eh.handleMessage(v)
	case *events.Connected:
		eh.sm.mu.Lock()
		eh.sm.reconnecting = false
		eh.sm.loggedOut = false
		eh.sm.mu.Unlock()
		eh.sm.StatusChannel <- StatusMsg{true, nil}
	case *events.Disconnected:
		eh.sm.StatusChannel <- StatusMsg{false, nil}
		// Attempt auto-reconnect unless logged out or already reconnecting
		eh.sm.mu.Lock()
		shouldReconnect := !eh.sm.loggedOut && !eh.sm.reconnecting && eh.sm.started
		if shouldReconnect {
			eh.sm.reconnecting = true
		}
		eh.sm.mu.Unlock()
		if shouldReconnect {
			go eh.sm.scheduleReconnect()
		}
	case *events.LoggedOut:
		eh.sm.mu.Lock()
		eh.sm.loggedOut = true
		eh.sm.mu.Unlock()
		eh.sm.StatusChannel <- StatusMsg{false, nil}
		reasonText := fmt.Sprintf("%v", v.Reason)
		eh.sm.uiHandler.PrintText("Logged out: " + reasonText)
	case *events.HistorySync:
		// Reload chats when history sync occurs
		eh.sm.uiHandler.PrintText("Receiving history sync...")
		go eh.sm.loadRecentChats()
	}
}

// extractMessageContent extracts display text and chat-list preview from a
// whatsmeow Message proto. This is a pure function with no side effects,
// making it easy to test.
func extractMessageContent(msg *waE2E.Message) (text, preview string) {
	if msg == nil {
		return "", ""
	}

	// 1. Extended text (replies, link previews, formatted text)
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		t := ext.GetText()
		if t != "" {
			return t, truncatePreview(t)
		}
	}

	// 2. Image
	if img := msg.GetImageMessage(); img != nil {
		c := img.GetCaption()
		if c != "" {
			return "[IMAGE] " + c, "[IMAGE] " + c
		}
		return "[IMAGE]", "[IMAGE]"
	}

	// 3. Video / GIF
	if vid := msg.GetVideoMessage(); vid != nil {
		tag := "[VIDEO]"
		if vid.GetGifPlayback() {
			tag = "[GIF]"
		}
		c := vid.GetCaption()
		if c != "" {
			return tag + " " + c, tag + " " + c
		}
		return tag, tag
	}

	// 4. Audio / Voice note
	if aud := msg.GetAudioMessage(); aud != nil {
		if aud.GetPTT() {
			secs := aud.GetSeconds()
			if secs > 0 {
				t := fmt.Sprintf("[VOICE NOTE] %ds", secs)
				return t, t
			}
			return "[VOICE NOTE]", "[VOICE NOTE]"
		}
		secs := aud.GetSeconds()
		if secs > 0 {
			t := fmt.Sprintf("[AUDIO] %ds", secs)
			return t, t
		}
		return "[AUDIO]", "[AUDIO]"
	}

	// 5. Document
	if doc := msg.GetDocumentMessage(); doc != nil {
		name := doc.GetFileName()
		if name == "" {
			name = doc.GetTitle()
		}
		if name != "" {
			return "[DOCUMENT] " + name, "[DOCUMENT] " + name
		}
		return "[DOCUMENT]", "[DOCUMENT]"
	}

	// 6. Sticker
	if msg.GetStickerMessage() != nil {
		return "[STICKER]", "[STICKER]"
	}

	// 7. Contact
	if con := msg.GetContactMessage(); con != nil {
		name := con.GetDisplayName()
		if name != "" {
			return "[CONTACT] " + name, "[CONTACT] " + name
		}
		return "[CONTACT]", "[CONTACT]"
	}

	// 8. Location
	if loc := msg.GetLocationMessage(); loc != nil {
		name := loc.GetName()
		if name == "" {
			name = loc.GetAddress()
		}
		if name != "" {
			t := fmt.Sprintf("[LOCATION] %s (%.4f, %.4f)", name, loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
			return t, "[LOCATION] " + name
		}
		t := fmt.Sprintf("[LOCATION] (%.4f, %.4f)", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		return t, t
	}

	// 9. Reaction
	if react := msg.GetReactionMessage(); react != nil {
		emoji := react.GetText()
		if emoji != "" {
			return "[REACTION] " + emoji, "[REACTION] " + emoji
		}
		return "[REACTION]", "[REACTION]"
	}

	// 10. Plain text (fallback — must come last)
	if t := msg.GetConversation(); t != "" {
		return t, truncatePreview(t)
	}

	return "", ""
}

// truncatePreview shortens text for the chat list preview column.
func truncatePreview(s string) string {
	const maxLen = 80
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}

// processIncomingMessage handles all common steps for any received message:
// build Message struct → persist to DB → update PQ → UI update → notification.
func (eh *eventHandler) processIncomingMessage(evt *events.Message, text, preview string) {
	if text == "" {
		return // nothing to process
	}

	chatJID := evt.Info.Chat.String()
	timestamp := uint64(evt.Info.Timestamp.Unix())

	msg := Message{
		Id:           evt.Info.ID,
		ChatId:       chatJID,
		FromMe:       evt.Info.IsFromMe,
		Timestamp:    timestamp,
		Text:         text,
		ContactId:    evt.Info.Sender.String(),
		ContactName:  eh.getContactName(evt.Info.Sender),
		ContactShort: eh.getContactShort(evt.Info.Sender),
	}

	// Persist message
	if err := eh.sm.db.AddMessage(msg); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("failed to save message: %v", err))
	}

	// Update priority queue and conversation
	eh.sm.mu.Lock()
	conv := eh.sm.convByJID[chatJID]
	var toUpsert Conversation
	if conv != nil {
		conv.LastMsgTime = int64(timestamp)
		conv.Preview = preview
		if !evt.Info.IsFromMe {
			conv.Unread++
		}
		eh.sm.priorityQueue.Update(conv, conv.LastMsgTime, conv.IsPinned)
		toUpsert = *conv
	} else {
		unread := uint16(0)
		if !evt.Info.IsFromMe {
			unread = 1
		}
		newConv := &Conversation{
			JID:         chatJID,
			Name:        eh.sm.getChatName(evt.Info.Chat),
			LastMsgTime: int64(timestamp),
			Preview:     preview,
			Unread:      unread,
			IsPinned:    false,
		}
		heap.Push(&eh.sm.priorityQueue, newConv)
		eh.sm.convByJID[chatJID] = newConv
		toUpsert = *newConv
	}
	safeList := eh.sm.snapshotPQ()
	eh.sm.mu.Unlock()

	// DB write outside lock
	if err := eh.sm.db.UpsertConversation(toUpsert); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
	}

	// UI: show in current chat or send notification
	eh.sm.mu.RLock()
	isCurrent := chatJID == eh.sm.currentReceiver
	eh.sm.mu.RUnlock()

	if isCurrent {
		eh.sm.uiHandler.NewMessage(msg)
	} else if !evt.Info.IsFromMe {
		if timestamp > uint64(time.Now().Unix()-30) {
			senderName := eh.getContactShort(evt.Info.Sender)
			if senderName == "" {
				senderName = "New Message"
			}
			if err := notify(senderName, text); err != nil {
				eh.sm.uiHandler.PrintError(err)
			}
		}
	}

	// Update chat list ordering
	eh.sm.uiHandler.UpdateChatList(safeList)
}

// Handle incoming messages — dispatches to extractMessageContent then processIncomingMessage.
func (eh *eventHandler) handleMessage(evt *events.Message) {
	text, preview := extractMessageContent(evt.Message)
	eh.processIncomingMessage(evt, text, preview)
}

// Helper to get contact name
func (eh *eventHandler) getContactName(jid types.JID) string {
	if client := eh.sm.getClient(); client != nil && client.Store != nil {
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found && contact.FullName != "" {
			return contact.FullName
		}
	}

	// Fallback to JID User (Phone Number)
	return jid.User
}

// Helper to get short contact name
func (eh *eventHandler) getContactShort(jid types.JID) string {
	if client := eh.sm.getClient(); client != nil && client.Store != nil {
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found && contact.PushName != "" {
			return contact.PushName
		}
	}

	// Fallback to JID User (Phone Number)
	return jid.User
}
