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
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
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
	debug           bool // T-2: enable verbose WhatsApp protocol logging
}

// SetDebug enables or disables verbose WhatsApp protocol logging (T-2).
func (sm *SessionManager) SetDebug(on bool) { sm.debug = on }

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
	// Check if conversation has unread messages
	hasUnread := false
	if conv := sm.convByJID[id]; conv != nil && conv.Unread > 0 {
		hasUnread = true
	}
	sm.mu.Unlock()
	screen := sm.getMessages(id)
	sm.uiHandler.NewScreen(screen)

	// Auto-mark as read in background (like WhatsApp Web)
	if hasUnread {
		go sm.markChatAsRead(id)
	}
}

// markChatAsRead sends read receipts for the most recent incoming messages
// in the given chat and resets the unread counter in PQ and DB.
// Safe for background use — returns silently on any failure.
func (sm *SessionManager) markChatAsRead(jidStr string) {
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		return
	}

	chatJID, err := types.ParseJID(jidStr)
	if err != nil {
		return
	}

	// Load messages to collect IDs for read receipt
	msgs, err := sm.db.GetMessages(jidStr)
	if err != nil || len(msgs) == 0 {
		return
	}

	// Collect unread message IDs (up to last 50 messages from others)
	var ids []types.MessageID
	var lastSender types.JID
	for i := len(msgs) - 1; i >= 0 && len(ids) < 50; i-- {
		if !msgs[i].FromMe {
			sender, _ := types.ParseJID(msgs[i].ContactId)
			if len(ids) == 0 {
				lastSender = sender
			}
			// MarkRead requires all IDs to be from the same sender
			if sender == lastSender {
				ids = append(ids, msgs[i].Id)
			}
		}
	}

	if len(ids) == 0 {
		return
	}

	if err := client.MarkRead(context.Background(), ids, time.Now(), chatJID, lastSender); err != nil {
		return
	}

	// Reset unread counter — copy under lock, DB write outside.
	var convCopy *Conversation
	sm.mu.Lock()
	if conv := sm.convByJID[jidStr]; conv != nil {
		conv.Unread = 0
		c := *conv
		convCopy = &c
	}
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()

	if convCopy != nil {
		_ = sm.db.UpsertConversation(*convCopy)
	}

	sm.uiHandler.UpdateChatList(safeList)
}

// get the next message id to select (highlighted + offset)
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

		// P-3: Pre-fetch all group names in a single batch call to avoid N+1 API calls.
		groupNames := make(map[string]string) // JID string → group name
		if groups, err := client.GetJoinedGroups(context.Background()); err == nil {
			for _, g := range groups {
				if g.Name != "" {
					groupNames[g.JID.String()] = g.Name
				}
			}
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
				if gn, ok := groupNames[jidStr]; ok {
					name = gn
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

// processHistorySync parses a HistorySync event to extract real conversation
// metadata (timestamps, unread counts, pinned status) and store historical
// messages in SQLite. This enables correct chat ordering on first login.
func (sm *SessionManager) processHistorySync(data *waHistorySync.HistorySync) {
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected — cannot process history sync"))
		return
	}

	conversations := data.GetConversations()
	if len(conversations) == 0 {
		sm.uiHandler.PrintText(fmt.Sprintf("History sync (%s): no conversations", data.GetSyncType()))
		return
	}

	sm.uiHandler.PrintText(fmt.Sprintf("Processing history sync (%s): %d conversations...",
		data.GetSyncType(), len(conversations)))

	var toUpsert []Conversation
	totalMessages := 0
	addMsgErrors := 0

	for _, conv := range conversations {
		chatJIDStr := conv.GetID()
		chatJID, err := types.ParseJID(chatJIDStr)
		if err != nil {
			continue
		}

		// Skip non-chat JIDs (status broadcasts, etc.)
		if chatJID.Server != "s.whatsapp.net" && chatJID.Server != "g.us" {
			continue
		}

		jidStr := chatJID.String()

		// --- Extract conversation metadata from proto ---
		lastMsgTimestamp := int64(conv.GetLastMsgTimestamp())
		if lastMsgTimestamp == 0 {
			lastMsgTimestamp = int64(conv.GetConversationTimestamp())
		}

		unreadCount := uint16(conv.GetUnreadCount())
		isPinned := conv.GetPinned() > 0
		isArchived := conv.GetArchived()

		// Determine chat name from history proto
		name := conv.GetName()
		if name == "" {
			name = conv.GetDisplayName()
		}

		// --- Process individual messages ---
		var latestPreview string
		var latestMsgTs int64

		for _, histMsg := range conv.GetMessages() {
			webMsg := histMsg.GetMessage()
			if webMsg == nil {
				continue
			}

			evt, err := client.ParseWebMessage(chatJID, webMsg)
			if err != nil {
				continue
			}

			text, preview := extractMessageContent(evt.Message)
			if text == "" {
				continue
			}

			msgTimestamp := uint64(evt.Info.Timestamp.Unix())

			// Resolve sender name: prefer PushName, fall back to JID
			senderName := evt.Info.PushName
			if senderName == "" {
				senderName = evt.Info.Sender.User
			}

			msg := Message{
				Id:           evt.Info.ID,
				ChatId:       jidStr,
				FromMe:       evt.Info.IsFromMe,
				Timestamp:    msgTimestamp,
				Text:         text,
				ContactId:    evt.Info.Sender.String(),
				ContactName:  senderName,
				ContactShort: senderName,
			}

			// AddMessage uses INSERT OR IGNORE, so duplicates are skipped
			if err := sm.db.AddMessage(msg); err != nil {
				addMsgErrors++
			}

			// Track the most recent message for the conversation preview
			if int64(msgTimestamp) > latestMsgTs {
				latestMsgTs = int64(msgTimestamp)
				latestPreview = preview
			}

			totalMessages++
		}

		// Use best available timestamp
		if latestMsgTs > lastMsgTimestamp {
			lastMsgTimestamp = latestMsgTs
		}

		if latestPreview == "" {
			latestPreview = "New chat"
		}

		// --- Update priority queue ---
		sm.mu.Lock()
		existingConv := sm.convByJID[jidStr]

		if existingConv != nil {
			updated := false

			// Only update timestamp/preview if history has newer data
			if lastMsgTimestamp > existingConv.LastMsgTime {
				existingConv.LastMsgTime = lastMsgTimestamp
				existingConv.Preview = latestPreview
				updated = true
			}

			// Fill in name if we only had a phone number
			if name != "" && (existingConv.Name == existingConv.JID || existingConv.Name == chatJID.User) {
				existingConv.Name = name
				updated = true
			}

			// Always adopt pinned status from authoritative source
			if isPinned != existingConv.IsPinned {
				existingConv.IsPinned = isPinned
				updated = true
			}

			// Prefer history unread count if it's higher
			if unreadCount > existingConv.Unread {
				existingConv.Unread = unreadCount
				updated = true
			}

			// Adopt archived status from authoritative source
			if isArchived != existingConv.IsArchived {
				existingConv.IsArchived = isArchived
				updated = true
			}

			if updated {
				sm.priorityQueue.Update(existingConv, existingConv.LastMsgTime, existingConv.IsPinned)
				c := *existingConv
				toUpsert = append(toUpsert, c)
			}
		} else {
			// New conversation — use history name or fall back to JID user
			if name == "" {
				name = chatJID.User
			}
			newConv := &Conversation{
				JID:         jidStr,
				Name:        name,
				LastMsgTime: lastMsgTimestamp,
				Preview:     latestPreview,
				Unread:      unreadCount,
				IsPinned:    isPinned,
				IsArchived:  isArchived,
			}
			heap.Push(&sm.priorityQueue, newConv)
			sm.convByJID[jidStr] = newConv
			toUpsert = append(toUpsert, *newConv)
		}
		sm.mu.Unlock()
	}

	// Persist all conversation updates (outside lock)
	for _, c := range toUpsert {
		if err := sm.db.UpsertConversation(c); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
		}
	}

	// Refresh chat list UI
	sm.mu.Lock()
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()
	sm.uiHandler.UpdateChatList(safeList)

	if addMsgErrors > 0 {
		sm.uiHandler.PrintError(fmt.Errorf("failed to store %d history messages", addMsgErrors))
	}

	sm.uiHandler.PrintText(fmt.Sprintf("History sync complete: %d conversations, %d messages stored.",
		len(toUpsert), totalMessages))
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
