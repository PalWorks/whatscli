package messages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
	ContactChannel  chan Contact
	TextChannel     chan *waProto.Message
	statusInfo      SessionStatus
	lastSent        time.Time
	started         bool
	stop            chan struct{}
	eventHandler    *eventHandler
	mu              sync.RWMutex
}

// initialize the SessionManager
func (sm *SessionManager) Init(handler UiMessageHandler) {
	sm.db = &MessageDatabase{}
	sm.db.Init()
	// Load existing data
	sm.db.Load(config.GetSessionFilePath() + ".gob")
	sm.uiHandler = handler
	sm.BatteryChannel = make(chan BatteryMsg, 10)
	sm.StatusChannel = make(chan StatusMsg, 10)
	sm.CommandChannel = make(chan Command, 10)
	sm.ChatChannel = make(chan Chat, 10)
	sm.ContactChannel = make(chan Contact, 10)
	sm.TextChannel = make(chan *waProto.Message, 10)
	sm.eventHandler = &eventHandler{sm: sm}
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
	
	err = sm.loginWithConnection(client)
	if err != nil {
		sm.uiHandler.PrintError(err)
	}
	
	// Start auto-saver
	go sm.startAutoSaver()

	for {
		select {
		case <-sm.stop:
			sm.uiHandler.PrintText("closing the receiver")
			if sm.client != nil {
				sm.client.Disconnect()
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
			
			if sm.client != nil {
				sm.statusInfo.Connected = sm.client.IsConnected()
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

// login logs in the user. It tries to see if a session already exists. If not, tries to create a
// new one using qr scanned on the terminal.
func (sm *SessionManager) login() error {
	// Clear any existing connection for retry
	sm.client = nil
	
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
			
			// Recreate the client
			sm.mu.Lock()
			sm.client = nil
			client, err = sm.getConnection()
			sm.mu.Unlock()
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
				go sm.uiHandler.Clear() // Clear the QR to avoid confusion or just keep it?
				// Better to keep it but change the title? 
				// The UpdateQR function takes logic.
				// Let's just update the text manually for now.
				sm.uiHandler.UpdateQR(currentQR, qrCount, 0) // Show 0s
			}
		}
	}
}

// loadRecentChats fetches recent chats from WhatsApp and adds them to the database
func (sm *SessionManager) loadRecentChats() {
	if sm.client == nil || !sm.client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}
	

	
	// Load contacts first to ensure proper name display
	// sm.loadContacts() // Postpone until HistorySync

	
	// Capture client pointer safely
	sm.mu.RLock()
	client := sm.client
	sm.mu.RUnlock()

	if client == nil {
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
		for jid, contact := range contacts {
			if !contact.Found {
				continue
			}
			
			// Skip non-chat JIDs
			if jid.Server != "s.whatsapp.net" && jid.Server != "g.us" {
				continue
			}
			
			jidStr := jid.String()
			
			// Create a Chat object
			isGroup := jid.Server == "g.us"
			
			// Pick the best name available
			var name string
			if isGroup {
				// For groups, try to get the group info
				groupInfo, err := sm.client.GetGroupInfo(context.Background(), jid)
				if err == nil && groupInfo.Name != "" {
					name = groupInfo.Name
				} else {
					name = "Group: " + jid.User
				}
			} else {
				// For contacts, use the full name first, then pushname, then JID
				name = contact.FullName
				if name == "" {
					name = contact.PushName
				}
				if name == "" {
					name = jid.User
				}
			}
			
			chatObj := Chat{
				Id:          jidStr,
				IsGroup:     isGroup,
				Name:        name,
				Unread:      0, // We don't have unread info here
				LastMessage: time.Now().Unix(),
			}
			
			// Add to database
			sm.db.AddChat(chatObj)
			chatCount++
			
			// Load recent messages for this chat in the background
			go sm.loadRecentMessages(jidStr)
		}
		
		// Update UI with the new chat list
		sm.uiHandler.SetChats(sm.db.GetChatIds())
		
		sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d contacts as chats", chatCount))
	} else {
		sm.uiHandler.PrintError(errors.New("failed to access contacts store"))
	}
}

// loadContacts loads contacts from the WhatsApp address book
func (sm *SessionManager) loadContacts() {
	if sm.client == nil || sm.client.Store == nil {
		return
	}
	
	// Get all contacts from the store - GetAllContacts returns contacts and an error
	contactCount := 0
	contacts, err := sm.client.Store.Contacts.GetAllContacts(context.Background())
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load contacts: %v", err))
		return
	}
	
	for jid, contact := range contacts {
		if !contact.Found {
			continue
		}
		
		// Determine best name to use
		name := contact.FullName
		if name == "" {
			name = contact.PushName
		}
		if name == "" {
			name = jid.User
		}
		
		// Create Contact object
		contactObj := Contact{
			Id:    jid.String(),
			Name:  name,
			Short: contact.PushName,
		}
		
		// Add to database
		sm.db.AddContact(contactObj)
		contactCount++
	}
	
	if contactCount > 0 {
		sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d contacts", contactCount))
	}
}

// loadRecentMessages loads the most recent messages for a chat
func (sm *SessionManager) loadRecentMessages(chatJID string) {
	if sm.client == nil || !sm.client.IsConnected() {
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
	// For groups, use the group name if available
	if jid.Server == "g.us" {
		// Try to get group info from the store
		groupInfo, err := sm.client.GetGroupInfo(context.Background(), jid)
		if err == nil && groupInfo.Name != "" {
			return groupInfo.Name
		}
		return "Group Chat"
	}
	
	// For individual chats, try to get the contact name
	if sm.client != nil && sm.client.Store != nil {
		contact, err := sm.client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found {
			if contact.FullName != "" {
				return contact.FullName
			}
			if contact.PushName != "" {
				return contact.PushName
			}
		}
	}
	
	// Fallback to JID
	return sm.db.GetIdName(jid.String())
}

// disconnects the session
func (sm *SessionManager) disconnect() error {
	// Remove the GOB file
	gobPath := config.GetSessionFilePath() + ".gob"
	os.Remove(gobPath)

	if sm.client != nil && sm.client.IsConnected() {
		sm.client.Disconnect()
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
	if sm.client != nil && sm.client.IsConnected() {
		sm.client.Disconnect()
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

// executes a command
func (sm *SessionManager) execCommand(command Command) {
	cmd := command.Name
	switch cmd {
	default:
		sm.uiHandler.PrintText("[" + config.Config.Colors.Negative + "]Unknown command: [-]" + cmd)
	case "backlog":
		sm.mu.RLock()
		receiver := sm.currentReceiver
		sm.mu.RUnlock()

		if receiver != "" {
			// First approach: Try to use the direct conversation query method
			jid, err := types.ParseJID(receiver)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
				return
			}
			
			sm.uiHandler.PrintText("Retrieving message history...")
			
			// Get existing messages to compare later
			existingMessages := sm.db.GetMessages(receiver)
			
			// Find the ID of the oldest message we have - not used currently but could be in future
			var oldestTimestamp uint64 = ^uint64(0) // Maximum uint64 value
			for _, msg := range existingMessages {
				if msg.Timestamp < oldestTimestamp {
					oldestTimestamp = msg.Timestamp
				}
			}
			
			// Try multiple approaches:
			var messagesFetched bool
			
			// 1. First try direct message fetch
			if sm.client != nil && sm.client.IsConnected() {
				sm.uiHandler.PrintText("Attempting to fetch older messages...")
				
				// Try to send a simpler read receipt which sometimes triggers history sync
				receiptType := types.ReceiptTypeRead
				err := sm.client.MarkRead(context.Background(), []types.MessageID{}, time.Now(), jid, jid, receiptType)
				if err != nil {
					sm.uiHandler.PrintText(fmt.Sprintf("Note: Could not send read receipt: %v", err))
				}
				
				// Wait a bit
				time.Sleep(2 * time.Second)
				
				// Check if we got any new messages
				updatedMessages := sm.db.GetMessages(sm.currentReceiver)
				if len(updatedMessages) > len(existingMessages) {
					messagesFetched = true
					sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(updatedMessages)-len(existingMessages)))
				}
			}
			
			// 2. If that didn't work, try a presence update which can trigger history sync
			if !messagesFetched && sm.client != nil && sm.client.IsConnected() {
				sm.uiHandler.PrintText("Trying alternative method...")
				
				// Send chat presence - using ChatPresence constants from the types package
				err = sm.client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
				if err != nil {
					sm.uiHandler.PrintText(fmt.Sprintf("Note: Could not send chat presence: %v", err))
				}
				
				// Wait a bit longer for any messages to arrive
				time.Sleep(3 * time.Second)
				
				// Check if we got any new messages
				updatedMessages := sm.db.GetMessages(sm.currentReceiver)
				if len(updatedMessages) > len(existingMessages) {
					messagesFetched = true
					sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(updatedMessages)-len(existingMessages)))
				}
			}
			
			// 3. Last resort: try history sync notification
			if !messagesFetched && sm.client != nil && sm.client.IsConnected() {
				sm.uiHandler.PrintText("Trying final method...")
				
				// Create context with timeout
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				
				// Create a basic history sync notification
				historyMsg := &waProto.Message{
					ProtocolMessage: &waProto.ProtocolMessage{
						HistorySyncNotification: &waProto.HistorySyncNotification{
							ChunkOrder:    proto.Uint32(0),
							FileLength:    proto.Uint64(0),
							FileEncSHA256: []byte{},
						},
						Type: waProto.ProtocolMessage_HISTORY_SYNC_NOTIFICATION.Enum(),
					},
				}
				
				// Send it and ignore errors (it may not work)
				if sm.client != nil {
					sm.client.SendMessage(ctx, jid, historyMsg)
				}
				
				// Wait a bit longer for any messages to arrive
				time.Sleep(3 * time.Second)
				
				// Final check if we got any new messages
				finalMessages := sm.db.GetMessages(sm.currentReceiver)
				if len(finalMessages) > len(existingMessages) {
					sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(finalMessages)-len(existingMessages)))
				} else {
					sm.uiHandler.PrintText("No additional messages found. WhatsApp may limit history access.")
				}
			}
			

			
			// Show the updated message list
			screen := sm.getMessages(receiver)
			sm.uiHandler.NewScreen(screen)
		} else {
			sm.printCommandUsage("backlog", "-> only works in a chat")
		}
	case "login", "connect":
		err := sm.login()
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("WhatsApp connection failed: %v", err))
			sm.uiHandler.PrintText("Try using /reset to completely reset the connection")
		} else {
			sm.uiHandler.PrintText("Successfully connected to WhatsApp")
		}
	case "reset":
		// Fully reset everything
		sm.mu.Lock()
		client := sm.client
		if client != nil {
			if client.IsConnected() {
				client.Disconnect()
			}
			
			if client.Store != nil {
				err := client.Store.Delete(context.Background())
				if err != nil {
					sm.uiHandler.PrintText("Warning: Couldn't remove session: " + err.Error())
				}
			}
		}
		
		sm.client = nil
		sm.container = nil
		sm.mu.Unlock()
		
		// Remove the DB file
		dbPath := config.GetSessionFilePath() + ".db"
		err := os.Remove(dbPath)
		if err != nil && !os.IsNotExist(err) {
			sm.uiHandler.PrintText("Warning: Couldn't remove database file: " + err.Error())
		}
		
		sm.uiHandler.PrintText("Session reset. Use /connect to reconnect with a new QR code.")
		sm.StatusChannel <- StatusMsg{false, nil}
	case "disconnect":
		sm.uiHandler.PrintError(sm.disconnect())
	case "logout":
		sm.uiHandler.PrintError(sm.logout())
	case "send":
		if checkParam(command.Params, 2) {
			receiver := command.Params[0]
			textParams := command.Params[1:]
			text := strings.Join(textParams, " ")
			sm.sendText(receiver, text)
		} else {
			sm.printCommandUsage("send", "[chat-id[] [message text[]")
		}
	case "select":
		if checkParam(command.Params, 1) {
			sm.setCurrentReceiver(command.Params[0])
		} else {
			sm.printCommandUsage("select", "[chat-id[]")
		}
	case "read":
		sm.mu.RLock()
		receiver := sm.currentReceiver
		sm.mu.RUnlock()

		if receiver != "" {
			// TODO: Implement marking messages as read in whatsmeow
			sm.uiHandler.PrintText("Read command not implemented yet with the new backend")
		} else {
			sm.printCommandUsage("read", "-> only works in a chat")
		}
	case "info":
		if checkParam(command.Params, 1) {
			sm.uiHandler.PrintText(sm.db.GetMessageInfo(command.Params[0]))
		} else {
			sm.printCommandUsage("info", "[message-id[]")
		}
	case "colorlist":
		sm.uiHandler.ShowColorList()
	case "more":
		sm.uiHandler.PrintText("More command not implemented yet with the new backend")
	}
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
			Id:          resp.ID,
			ChatId:      wid,
			FromMe:      true,
			Timestamp:   uint64(time.Now().Unix()),
			Text:        text,
			ContactId:   client.Store.ID.String(),
			ContactName: "Me",
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
	
	// For simplicity, we'll only handle text messages for now
	if evt.Message.GetConversation() != "" {
		text := evt.Message.GetConversation()
		
		// Create a Message struct that our application can use
		msg := Message{
			Id:          evt.Info.ID,
			ChatId:      chatJID,
			FromMe:      evt.Info.IsFromMe,
			Timestamp:   timestamp,
			Text:        text,
			ContactId:   evt.Info.Sender.String(),
			ContactName: eh.getContactName(evt.Info.Sender),
			ContactShort: eh.getContactShort(evt.Info.Sender),
		}
		
		// Add to database
		// Add to database
		eh.sm.db.AddMessage(msg)
		
		// If this is for the current chat, update the UI
		eh.sm.mu.RLock()
		isCurrent := chatJID == eh.sm.currentReceiver
		eh.sm.mu.RUnlock()

		if isCurrent {
			eh.sm.uiHandler.NewMessage(msg)
		} else {
			// Notify if message is recent and not in focus
			if timestamp > uint64(time.Now().Unix()-30) && !evt.Info.IsFromMe {
				eh.sm.db.NewUnreadChat(chatJID)
				err := notify(eh.getContactShort(evt.Info.Sender), text)
				if err != nil {
					eh.sm.uiHandler.PrintError(err)
				}
			}
		}
	}
	
	// Handle media messages (images, documents, etc.)
	// This is a simplified version; we just create a text message with a media indicator
	if evt.Message.GetImageMessage() != nil {
		imgMsg := evt.Message.GetImageMessage()
		caption := imgMsg.GetCaption()
		if caption == "" {
			caption = "[IMAGE]"
		} else {
			caption = "[IMAGE] " + caption
		}
		
		msg := Message{
			Id:          evt.Info.ID,
			ChatId:      chatJID,
			FromMe:      evt.Info.IsFromMe,
			Timestamp:   timestamp,
			Text:        caption,
			ContactId:   evt.Info.Sender.String(),
			ContactName: eh.getContactName(evt.Info.Sender),
			ContactShort: eh.getContactShort(evt.Info.Sender),
		}
		
		eh.sm.db.AddMessage(msg)
		
		eh.sm.mu.RLock()
		isCurrent := chatJID == eh.sm.currentReceiver
		eh.sm.mu.RUnlock()

		if isCurrent {
			eh.sm.uiHandler.NewMessage(msg)
		}
	}
	
	// Make sure to update the chat list with new ordering
	eh.sm.uiHandler.SetChats(eh.sm.db.GetChatIds())
}

// Helper to get contact name
func (eh *eventHandler) getContactName(jid types.JID) string {
	if eh.sm.client != nil && eh.sm.client.Store != nil {
		contact, err := eh.sm.client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found && contact.FullName != "" {
			return contact.FullName
		}
	}
	
	// Fallback to the database
	return eh.sm.db.GetIdName(jid.String())
}

// Helper to get short contact name
func (eh *eventHandler) getContactShort(jid types.JID) string {
	if eh.sm.client != nil && eh.sm.client.Store != nil {
		contact, err := eh.sm.client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found && contact.PushName != "" {
			return contact.PushName
		}
	}
	
	// Fallback to the database
	return eh.sm.db.GetIdShort(jid.String())
}
