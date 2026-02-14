package messages

import (
	"database/sql"
	"encoding/gob"
	"fmt"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/normen/whatscli/config"
)

// MessageDatabase stores messages and contact data
type MessageDatabase struct {
	db *sql.DB

	// Legacy maps provided for backward compatibility during refactor phases
	// In the future, these should be removed or replaced by DB access
	messages     map[string][]Message
	messagesById map[string]Message
	chats        map[string]Chat // Deprecated: use conversations table

	// Locks for legacy maps
	chatLock    sync.RWMutex
	messageLock sync.RWMutex
	saveLock    sync.Mutex
	dirty       bool
	dirtyLock   sync.RWMutex
}

type storageDump struct {
	Messages     map[string][]Message
	MessagesById map[string]Message
	Chats        map[string]Chat
}

// Initializes the message database with SQLite
func (md *MessageDatabase) Init() {
	// Initialize legacy maps for now to prevent nil pointer panics in existing code
	md.messages = make(map[string][]Message)
	md.messagesById = make(map[string]Message)
	md.chats = make(map[string]Chat)

	// Initialize SQLite
	dbPath := config.GetSessionFilePath() + "_meta.db"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to open metadata db: %v", err))
	}
	md.db = db

	// Create conversations table
	query := `
	CREATE TABLE IF NOT EXISTS conversations (
		jid TEXT PRIMARY KEY,
		name TEXT,
		last_msg_time INTEGER,
		preview TEXT,
		unread INTEGER,
		is_pinned BOOLEAN
	);
	`
	_, err = md.db.Exec(query)
	if err != nil {
		panic(fmt.Sprintf("Failed to create conversations table: %v", err))
	}

	// Create messages table (Phase 5)
	queryMsgs := `
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		chat_id TEXT,
		contact_id TEXT,
		contact_name TEXT,
		contact_short TEXT,
		timestamp INTEGER,
		from_me BOOLEAN,
		forwarded BOOLEAN,
		text TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
	`
	_, err = md.db.Exec(queryMsgs)
	if err != nil {
		panic(fmt.Sprintf("Failed to create messages table: %v", err))
	}
}

// Checks if database needs saving (Legacy support)
func (md *MessageDatabase) IsDirty() bool {
	md.dirtyLock.RLock()
	defer md.dirtyLock.RUnlock()
	return md.dirty
}

// Marks database as clean (Legacy support)
func (md *MessageDatabase) MarkClean() {
	md.dirtyLock.Lock()
	defer md.dirtyLock.Unlock()
	md.dirty = false
}

// Marks database as dirty (Legacy support)
func (md *MessageDatabase) MarkDirty() {
	md.dirtyLock.Lock()
	defer md.dirtyLock.Unlock()
	md.dirty = true
}

// Save persists the database to a file (Legacy support - Gob for maps)
func (md *MessageDatabase) Save(filePath string) error {
	md.saveLock.Lock()
	defer md.saveLock.Unlock()

	md.messageLock.RLock()
	defer md.messageLock.RUnlock()
	md.chatLock.RLock()
	defer md.chatLock.RUnlock()

	data := storageDump{
		Messages:     md.messages,
		MessagesById: md.messagesById,
		Chats:        md.chats,
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

// Load restores the database from a file (Legacy support - Gob for maps)
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

	// Phase 6: DO NOT load messages into memory.
	// We rely on SQLite now.
	// if data.Messages != nil {
	// 	md.messages = data.Messages
	// }
	// if data.MessagesById != nil {
	// 	md.messagesById = data.MessagesById
	// }

	if data.Chats != nil {
		md.chats = data.Chats
	}

	return nil
}

// --- Message Persistence (Phase 5) ---

// Migration function to move messages from memory to SQLite
func (md *MessageDatabase) MigrateToSQLite() {
	// 1. Migrate Chats/Conversations
	// If conversations table is empty, try to populate from legacy md.chats
	rowC := md.db.QueryRow("SELECT COUNT(*) FROM conversations")
	var countC int
	errC := rowC.Scan(&countC)
	if errC == nil && countC == 0 && len(md.chats) > 0 {
		// fmt.Println("Migrating chats to conversations table...")
		tx, _ := md.db.Begin()
		stmt, _ := tx.Prepare(`INSERT OR IGNORE INTO conversations (jid, name, last_msg_time, preview, unread, is_pinned) VALUES (?, ?, ?, ?, ?, ?)`)
		for _, chat := range md.chats {
			_, _ = stmt.Exec(chat.Id, chat.Name, 0, "", chat.Unread, false)
		}
		tx.Commit()
		stmt.Close()
	}

	// 2. Migrate Messages
	// Check if already populated
	row := md.db.QueryRow("SELECT COUNT(*) FROM messages")
	var count int
	err := row.Scan(&count)
	if err == nil && count > 0 {
		return // Already populated
	}

	// fmt.Println("Migrating messages to SQLite...")

	// If md.messages is empty (because Load skipped it), we need to read the Gob file again just for migration
	sourceMessages := md.messages
	if len(sourceMessages) == 0 {
		filePath := config.GetSessionFilePath() + ".gob"
		file, err := os.Open(filePath)
		if err == nil {
			defer file.Close()
			var data storageDump
			decoder := gob.NewDecoder(file)
			if err := decoder.Decode(&data); err == nil {
				sourceMessages = data.Messages
			}
		}
	}

	if len(sourceMessages) == 0 {
		return // Nothing to migrate
	}

	md.messageLock.RLock()
	defer md.messageLock.RUnlock()

	count = 0
	tx, err := md.db.Begin()
	if err != nil {
		// uiHandler.PrintError(fmt.Sprintf("Migration failed to start transaction: %v", err))
		return
	}

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO messages 
		(id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		// fmt.Printf("Migration failed to prepare statement: %v\n", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	for _, msgs := range sourceMessages {
		for _, msg := range msgs {
			_, err := stmt.Exec(
				msg.Id, msg.ChatId, msg.ContactId, msg.ContactName,
				msg.ContactShort, msg.Timestamp, msg.FromMe, msg.Forwarded, msg.Text,
			)
			if err != nil {
				// fmt.Printf("Failed to migrate message %s: %v\n", msg.Id, err)
			} else {
				count++
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		// uiHandler.PrintError(fmt.Sprintf("Migration failed to commit: %v", err))
	} else {
		// fmt.Printf("Migrated %d messages to SQLite\n", count)
	}
}

// AddMessageToDB persists a message to SQLite (unchanged)
func (md *MessageDatabase) AddMessageToDB(msg Message) error {
	query := `
	INSERT OR IGNORE INTO messages 
	(id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text) 
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := md.db.Exec(query,
		msg.Id, msg.ChatId, msg.ContactId, msg.ContactName,
		msg.ContactShort, msg.Timestamp, msg.FromMe, msg.Forwarded, msg.Text,
	)
	return err
}

// --- Legacy Methods (Kept for compatibility until Phase 3/6) ---

// Adds a message to the database
func (md *MessageDatabase) AddMessage(msg Message) bool {
	// Phase 6: Only write to SQLite
	err := md.AddMessageToDB(msg)
	if err != nil {
		fmt.Printf("Error adding message to DB: %v\n", err)
		// We return true anyway as failure here shouldn't crash caller if DB is momentarily quirky,
		// but ideally we bubble error. Legacy signature returns bool.
		return false
	}
	return true
}

// UpsertConversation updates or inserts a conversation
func (md *MessageDatabase) UpsertConversation(c Conversation) error {
	query := `
	INSERT INTO conversations (jid, name, last_msg_time, preview, unread, is_pinned)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(jid) DO UPDATE SET
		name=excluded.name,
		last_msg_time=excluded.last_msg_time,
		preview=excluded.preview,
		unread=excluded.unread,
		is_pinned=excluded.is_pinned;
	`
	_, err := md.db.Exec(query, c.JID, c.Name, c.LastMsgTime, c.Preview, c.Unread, c.IsPinned)
	return err
}

// GetConversations retrieves all conversations from the DB
func (md *MessageDatabase) GetConversations() ([]Conversation, error) {
	rows, err := md.db.Query("SELECT jid, name, last_msg_time, preview, unread, is_pinned FROM conversations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMsgTime, &c.Preview, &c.Unread, &c.IsPinned); err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	return convs, nil
}

// --- Legacy Methods (Kept for compatibility until Phase 3/6) ---

// Add chat to database (Legacy)
func (md *MessageDatabase) AddChat(chat Chat) {
	md.chatLock.Lock()
	defer md.chatLock.Unlock()
	md.chats[chat.Id] = chat
	md.MarkDirty()
}

// NewUnreadChat marks a chat as having unread messages (Legacy)
func (md *MessageDatabase) NewUnreadChat(chatId string) {
	md.chatLock.Lock()
	defer md.chatLock.Unlock()
	if chat, ok := md.chats[chatId]; ok {
		chat.Unread++
		md.chats[chatId] = chat
		md.MarkDirty()
	}
}

// get sorted chat ids (Legacy)
func (md *MessageDatabase) GetChatIds() []Chat {
	md.chatLock.RLock()
	defer md.chatLock.RUnlock()

	allChats := []Chat{}
	for _, val := range md.chats {
		allChats = append(allChats, val)
	}
	return allChats
}

// get all messages for a chat id (Legacy + Phase 5)
func (md *MessageDatabase) GetMessages(wid string) []Message {
	// Try to get from SQLite first (Phase 5/6)
	return md.GetMessagesFromDB(wid)
}

func (md *MessageDatabase) GetMessagesFromDB(chatId string) []Message {
	if md.db == nil {
		return []Message{}
	}
	rows, err := md.db.Query(`SELECT id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text FROM messages WHERE chat_id = ? ORDER BY timestamp ASC`, chatId)
	if err != nil {
		// Table might not exist yet or other error
		return []Message{}
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var msg Message
		var ts int64
		err := rows.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text)
		if err != nil {
			continue
		}
		msg.Timestamp = uint64(ts)
		msgs = append(msgs, msg)
	}
	return msgs
}

// get info for message (Legacy)
func (md *MessageDatabase) GetMessageInfo(wid string) string {
	// Simplified stub for now
	return "Info not available in legacy transition"
}
