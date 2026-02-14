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
	"github.com/normen/whatscli/qrcode"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
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
	// Load existing data
	sm.db.Load(config.GetSessionFilePath() + ".gob")

	// Phase 5: Ensure messages are migrated to SQLite
	// This performs a one-time migration of loaded Gob data to SQLite
	sm.db.MigrateToSQLite()

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

	// Start auto-saver
	go sm.startAutoSaver()

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

// Background routine to save database periodically
func (sm *SessionManager) startAutoSaver() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stop:
			return
		case <-ticker.C:
			if sm.db != nil && sm.db.IsDirty() {
				err := sm.db.Save(config.GetSessionFilePath() + ".gob")
				if err != nil {
					sm.uiHandler.PrintError(fmt.Errorf("auto-save failed: %v", err))
				}
			}
		}
	}
}

// set the currently selected chat
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

			// LOCKING for PQ access
			sm.mu.Lock()
			
			existingConv := sm.convByJID[jidStr]

			var lastMsgTime int64
			var isPinned bool
			var unread uint16

			if existingConv != nil {
				lastMsgTime = existingConv.LastMsgTime
				isPinned = existingConv.IsPinned
				unread = existingConv.Unread
				// Update name if changed
				if name != "" {
					existingConv.Name = name
					// Update in DB
					sm.db.UpsertConversation(*existingConv)
				}
			} else {
				lastMsgTime = 0 // Default for new import: 0 means no messages, so bottom of list
				unread = 0
				isPinned = false

				newConv := &Conversation{
					JID:         jidStr,
					Name:        name,
					LastMsgTime: lastMsgTime,
					Preview:     "New chat", // Placeholder
					Unread:      unread,
					IsPinned:    isPinned,
				}

				// Persist to SQLite
				err := sm.db.UpsertConversation(*newConv)
				if err != nil {
					sm.uiHandler.PrintError(fmt.Errorf("failed to upsert conversation %s: %v", jidStr, err))
				}

				// Add to Priority Queue and index
				heap.Push(&sm.priorityQueue, newConv)
				sm.convByJID[newConv.JID] = newConv
			}
			
			sm.mu.Unlock()
			// END LOCKING

			// Legacy support: Add to old maps for now (until Phase 6)
			chatObj := Chat{
				Id:          jidStr,
				IsGroup:     isGroup,
				Name:        name,
				Unread:      int(unread),
				LastMessage: lastMsgTime,
			}
			sm.db.AddChat(chatObj)

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
	// Remove the GOB file
	gobPath := config.GetSessionFilePath() + ".gob"
	os.Remove(gobPath)

	if client := sm.getClient(); client != nil && client.IsConnected() {
		client.Disconnect()
		sm.StatusChannel <- StatusMsg{false, nil}
	}
	// Save database on disconnect
	if sm.db != nil {
		sm.db.Save(config.GetSessionFilePath() + ".gob")
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

	if sm.db != nil {
		sm.db.Save(config.GetSessionFilePath() + ".gob")
	}
	// Disconnect is handled by the runManager loop via channel close,
	// but we can ensure it here too just in case
	if client := sm.getClient(); client != nil && client.IsConnected() {
		client.Disconnect()
	}
}

// logout logs out the user, deletes session file
func (sm *SessionManager) logout() error {
	sm.mu.Lock()
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

	// Remove the GOB file
	gobPath := config.GetSessionFilePath() + ".gob"
	os.Remove(gobPath)

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
	msgs := sm.db.GetMessages(wid)
	ids := []Message{}
	for _, msg := range msgs {
		ids = append(ids, msg)
	}
	return ids
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

		sm.db.AddMessage(newMsg)

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
		eh.sm.StatusChannel <- StatusMsg{true, nil}
	case *events.Disconnected:
		eh.sm.StatusChannel <- StatusMsg{false, nil}
	case *events.LoggedOut:
		eh.sm.StatusChannel <- StatusMsg{false, nil}
		reasonText := fmt.Sprintf("%v", v.Reason)
		eh.sm.uiHandler.PrintText("Logged out: " + reasonText)
	case *events.HistorySync:
		// Reload chats when history sync occurs
		eh.sm.uiHandler.PrintText("Receiving history sync...")
		go eh.sm.loadRecentChats()
	}
}

// Handle incoming messages
func (eh *eventHandler) handleMessage(evt *events.Message) {
	chatJID := evt.Info.Chat.String()
	timestamp := uint64(evt.Info.Timestamp.Unix())
	shouldUpdateList := false

	// Common logic to update PQ
	updatePQ := func(preview string) []*Conversation {
		eh.sm.mu.Lock()
		defer eh.sm.mu.Unlock()

		conv := eh.sm.convByJID[chatJID]

		if conv != nil {
			conv.LastMsgTime = int64(timestamp)
			conv.Preview = preview
			if !evt.Info.IsFromMe {
				conv.Unread++
			}
			eh.sm.priorityQueue.Update(conv, conv.LastMsgTime, conv.IsPinned)
			eh.sm.db.UpsertConversation(*conv)
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
			eh.sm.db.UpsertConversation(*newConv)
		}
		shouldUpdateList = true
		return eh.sm.snapshotPQ()
	}

	// 1. Text Messages
	if evt.Message.GetConversation() != "" {
		text := evt.Message.GetConversation()

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

		// Legacy DB
		eh.sm.db.AddMessage(msg)

		// Update PQ
		safeList := updatePQ(text)

		// UI Update for Chat
		eh.sm.mu.RLock()
		isCurrent := chatJID == eh.sm.currentReceiver
		eh.sm.mu.RUnlock()

		if isCurrent {
			eh.sm.uiHandler.NewMessage(msg)
		} else if !evt.Info.IsFromMe {
			if timestamp > uint64(time.Now().Unix()-30) {
				eh.sm.db.NewUnreadChat(chatJID)
				senderName := eh.getContactShort(evt.Info.Sender)
				if senderName == "" {
					senderName = "New Message"
				}
				err := notify(senderName, text)
				if err != nil {
					eh.sm.uiHandler.PrintError(err)
				}
			}
		}
		
		if shouldUpdateList {
			eh.sm.uiHandler.UpdateChatList(safeList)
		}
	} else if evt.Message.GetImageMessage() != nil {
		// 2. Image Messages
		imgMsg := evt.Message.GetImageMessage()
		caption := imgMsg.GetCaption()
		if caption == "" {
			caption = "[IMAGE]"
		} else {
			caption = "[IMAGE] " + caption
		}

		msg := Message{
			Id:           evt.Info.ID,
			ChatId:       chatJID,
			FromMe:       evt.Info.IsFromMe,
			Timestamp:    timestamp,
			Text:         caption,
			ContactId:    evt.Info.Sender.String(),
			ContactName:  eh.getContactName(evt.Info.Sender),
			ContactShort: eh.getContactShort(evt.Info.Sender),
		}

		eh.sm.db.AddMessage(msg)

		// Update PQ
		safeList := updatePQ(caption)

		eh.sm.mu.RLock()
		isCurrent := chatJID == eh.sm.currentReceiver
		eh.sm.mu.RUnlock()

		if isCurrent {
			eh.sm.uiHandler.NewMessage(msg)
		}
		
		if shouldUpdateList {
			eh.sm.uiHandler.UpdateChatList(safeList)
		}
	}

	// Make sure to update the chat list with new ordering
	// logic moved inside specific message handlers to use safeList

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
