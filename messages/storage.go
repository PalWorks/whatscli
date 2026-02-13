package messages

import (
	"fmt"
	"encoding/gob"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// MessageDatabase stores messages and contact data
type MessageDatabase struct {
	messages      map[string][]Message
	messagesById  map[string]Message
	chats         map[string]Chat
	contacts      map[string]Contact
	contactLock   sync.RWMutex
	chatLock      sync.RWMutex
	messageLock   sync.RWMutex
	saveLock      sync.Mutex
	dirty         bool
	dirtyLock     sync.RWMutex
}

// Data dump struct for gob encoding
type storageDump struct {
	Messages     map[string][]Message
	MessagesById map[string]Message
	Chats        map[string]Chat
	Contacts     map[string]Contact
}

// Initializes the message database
func (md *MessageDatabase) Init() {
	md.messages = make(map[string][]Message)
	md.messagesById = make(map[string]Message)
	md.chats = make(map[string]Chat)
	md.contacts = make(map[string]Contact)
}

// Checks if database needs saving
func (md *MessageDatabase) IsDirty() bool {
	md.dirtyLock.RLock()
	defer md.dirtyLock.RUnlock()
	return md.dirty
}

// Marks database as clean
func (md *MessageDatabase) MarkClean() {
	md.dirtyLock.Lock()
	defer md.dirtyLock.Unlock()
	md.dirty = false
}

// Marks database as dirty
func (md *MessageDatabase) MarkDirty() {
	md.dirtyLock.Lock()
	defer md.dirtyLock.Unlock()
	md.dirty = true
}

// Save persists the database to a file
func (md *MessageDatabase) Save(filePath string) error {
	md.saveLock.Lock()
	defer md.saveLock.Unlock()

	md.messageLock.RLock()
	defer md.messageLock.RUnlock()
	md.chatLock.RLock()
	defer md.chatLock.RUnlock()
	md.contactLock.RLock()
	defer md.contactLock.RUnlock()

	data := storageDump{
		Messages:     md.messages,
		MessagesById: md.messagesById,
		Chats:        md.chats,
		Contacts:     md.contacts,
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(data)
	if err == nil {
		md.MarkClean()
	}
	return err
}

// Load restores the database from a file
func (md *MessageDatabase) Load(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var data storageDump
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return err
	}

	md.messageLock.Lock()
	defer md.messageLock.Unlock()
	md.chatLock.Lock()
	defer md.chatLock.Unlock()
	md.contactLock.Lock()
	defer md.contactLock.Unlock()

	if data.Messages != nil {
		md.messages = data.Messages
	}
	if data.MessagesById != nil {
		md.messagesById = data.MessagesById
	}
	if data.Chats != nil {
		md.chats = data.Chats
	}
	if data.Contacts != nil {
		md.contacts = data.Contacts
	}

	return nil
}

// Adds a message to the database
func (md *MessageDatabase) AddMessage(msg Message) bool {
	md.messageLock.Lock()
	defer md.messageLock.Unlock()
	
	// Check if we already have this message
	if _, ok := md.messagesById[msg.Id]; ok {
		return false
	}
	
	// Add to message ID lookup
	md.messagesById[msg.Id] = msg
	
	// Create or get message array for chat
	msgs, ok := md.messages[msg.ChatId]
	if !ok {
		msgs = []Message{}
	}
	
	// Add message to chat
	msgs = append(msgs, msg)
	
	// Sort by timestamp
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Timestamp > msgs[j].Timestamp
	})
	
	// Store updated messages
	md.messages[msg.ChatId] = msgs
	
	// Update or create chat
	md.updateChatFromMessage(msg)
	
	md.MarkDirty()
	return true
}

// Update or create a chat based on a message
func (md *MessageDatabase) updateChatFromMessage(msg Message) {
	md.chatLock.Lock()
	defer md.chatLock.Unlock()
	
	chat, exists := md.chats[msg.ChatId]
	if !exists {
		// Create new chat
		isGroup := strings.Contains(msg.ChatId, GROUPSUFFIX)
		chat = Chat{
			Id:          msg.ChatId,
			IsGroup:     isGroup,
			Name:        msg.ContactName,
			Unread:      0,
			LastMessage: int64(msg.Timestamp),
		}
	} else {
		// Update last message time
		chat.LastMessage = int64(msg.Timestamp)
	}
	
	// If this is a new message and not from us, increment unread
	timeNow := time.Now().Unix()
	if int64(msg.Timestamp) > timeNow-30 && !msg.FromMe {
		chat.Unread++
	}
	
	md.chats[msg.ChatId] = chat
	
	// Add contact if we don't have it yet
	if msg.ContactId != "" {
		md.contactLock.Lock()
		defer md.contactLock.Unlock()
		
		_, exists := md.contacts[msg.ContactId]
		if !exists {
			contact := Contact{
				Id:    msg.ContactId,
				Name:  msg.ContactName,
				Short: msg.ContactShort,
			}
			md.contacts[msg.ContactId] = contact
		}
	}
}

// Add chat to database
func (md *MessageDatabase) AddChat(chat Chat) {
	md.chatLock.Lock()
	defer md.chatLock.Unlock()
	md.chats[chat.Id] = chat
	md.dirtyLock.Lock()
	md.dirty = true
	md.dirtyLock.Unlock()
}

// Add contact to database
func (md *MessageDatabase) AddContact(contact Contact) {
	md.contactLock.Lock()
	defer md.contactLock.Unlock()
	md.contacts[contact.Id] = contact
	md.dirtyLock.Lock()
	md.dirty = true
	md.dirtyLock.Unlock()
}

// NewUnreadChat marks a chat as having unread messages
func (md *MessageDatabase) NewUnreadChat(chatId string) {
	md.chatLock.Lock()
	defer md.chatLock.Unlock()
	if chat, ok := md.chats[chatId]; ok {
		chat.Unread++
		md.chats[chatId] = chat
		md.MarkDirty()
	}
}

// get sorted chat ids
func (md *MessageDatabase) GetChatIds() []Chat {
	md.chatLock.RLock()
	defer md.chatLock.RUnlock()
	
	allChats := []Chat{}
	
	// Build a single slice with all chats
	for _, val := range md.chats {
		allChats = append(allChats, val)
	}
	
	// Sort all chats by LastMessage timestamp (most recent first)
	sort.Slice(allChats, func(i, j int) bool {
		return allChats[i].LastMessage > allChats[j].LastMessage
	})
	
	return allChats
}

// get all messages for a chat id
func (md *MessageDatabase) GetMessages(wid string) []Message {
	md.messageLock.RLock()
	defer md.messageLock.RUnlock()
	
	// Return empty array if no messages
	msgs, ok := md.messages[wid]
	if !ok {
		return []Message{}
	}
	
	return msgs
}

// get info for message
func (md *MessageDatabase) GetMessageInfo(wid string) string {
	md.messageLock.RLock()
	defer md.messageLock.RUnlock()
	
	msg, ok := md.messagesById[wid]
	if !ok {
		return "Message not found"
	}
	
	name := md.GetIdName(msg.ContactId)
	short := md.GetIdShort(msg.ContactId)
	
	var direction string
	if msg.FromMe {
		direction = "→"
	} else {
		direction = "←"
	}
	
	timeStr := time.Unix(int64(msg.Timestamp), 0).Format(time.RFC1123)
	
	info := fmt.Sprintf("ID: %s\nFrom: %s (%s) %s\nTime: %s\nChat: %s", 
		msg.Id, name, short, direction, timeStr, msg.ChatId)
	
	return info
}

// get contact id name
func (md *MessageDatabase) GetIdName(wid string) string {
	if wid == "" {
		return "Unknown"
	}
	
	md.contactLock.RLock()
	defer md.contactLock.RUnlock()
	
	if cont, ok := md.contacts[wid]; ok {
		if cont.Name != "" {
			return cont.Name
		}
	}
	
	return wid
}

// get contact id short name
func (md *MessageDatabase) GetIdShort(wid string) string {
	if wid == "" {
		return "Unknown"
	}
	
	md.contactLock.RLock()
	defer md.contactLock.RUnlock()
	
	if cont, ok := md.contacts[wid]; ok {
		if cont.Short != "" {
			return cont.Short
		}
	}
	
	return wid
}
