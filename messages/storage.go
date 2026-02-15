package messages

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/normen/whatscli/config"
)

// MessageDatabase stores messages and contact data in SQLite
type MessageDatabase struct {
	db *sql.DB
}

// Init initializes the message database with a file-based SQLite connection.
func (md *MessageDatabase) Init() error {
	dbPath := config.GetSessionFilePath() + "_meta.db"
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to open metadata db: %w", err)
	}
	return md.InitWithDB(db)
}

// InitWithDB initializes the database schema using the provided *sql.DB.
// This allows tests to inject an in-memory SQLite instance.
func (md *MessageDatabase) InitWithDB(db *sql.DB) error {
	md.db = db

	_, err := md.db.Exec(`
	CREATE TABLE IF NOT EXISTS conversations (
		jid TEXT PRIMARY KEY,
		name TEXT,
		last_msg_time INTEGER,
		preview TEXT,
		unread INTEGER,
		is_pinned BOOLEAN
	);
	`)
	if err != nil {
		return fmt.Errorf("failed to create conversations table: %w", err)
	}

	_, err = md.db.Exec(`
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
	CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON messages(chat_id, timestamp);
	`)
	if err != nil {
		return fmt.Errorf("failed to create messages table: %w", err)
	}

	return nil
}

// escapeLike escapes the SQL LIKE metacharacters (%, _, \) so they are
// treated as literal characters in a LIKE ? ESCAPE '\' clause.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// Close closes the underlying SQLite connection.
// Safe to call on nil or already-closed databases.
func (md *MessageDatabase) Close() error {
	if md.db != nil {
		return md.db.Close()
	}
	return nil
}

// AddMessage persists a message to SQLite.
// Returns nil on success, or an error if the insert fails.
func (md *MessageDatabase) AddMessage(msg Message) error {
	if md.db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := md.db.Exec(`
	INSERT OR IGNORE INTO messages
	(id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		msg.Id, msg.ChatId, msg.ContactId, msg.ContactName,
		msg.ContactShort, msg.Timestamp, msg.FromMe, msg.Forwarded, msg.Text,
	)
	return err
}

// UpsertConversation updates or inserts a conversation
func (md *MessageDatabase) UpsertConversation(c Conversation) error {
	_, err := md.db.Exec(`
	INSERT INTO conversations (jid, name, last_msg_time, preview, unread, is_pinned)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(jid) DO UPDATE SET
		name=excluded.name,
		last_msg_time=excluded.last_msg_time,
		preview=excluded.preview,
		unread=excluded.unread,
		is_pinned=excluded.is_pinned;
	`, c.JID, c.Name, c.LastMsgTime, c.Preview, c.Unread, c.IsPinned)
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

// GetMessages retrieves all messages for a chat.
// Returns the messages and any error encountered during the query.
func (md *MessageDatabase) GetMessages(chatId string) ([]Message, error) {
	if md.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := md.db.Query(`SELECT id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text FROM messages WHERE chat_id = ? ORDER BY timestamp ASC`, chatId)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	msgs := make([]Message, 0)
	for rows.Next() {
		var msg Message
		var ts int64
		if err := rows.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text); err != nil {
			return msgs, fmt.Errorf("failed to scan message row: %w", err)
		}
		msg.Timestamp = uint64(ts)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// SearchMessages searches for messages containing the keyword.
// If chatId is non-empty, results are scoped to that chat; otherwise all chats are searched.
// Results are returned newest-first, capped at limit.
func (md *MessageDatabase) SearchMessages(chatId, keyword string, limit int) ([]Message, error) {
	if md.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if keyword == "" {
		return []Message{}, nil
	}
	likePattern := "%" + escapeLike(keyword) + "%"

	var rows *sql.Rows
	var err error
	if chatId != "" {
		rows, err = md.db.Query(`
			SELECT id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text
			FROM messages
			WHERE chat_id = ? AND text LIKE ? ESCAPE '\'
			ORDER BY timestamp DESC LIMIT ?`, chatId, likePattern, limit)
	} else {
		rows, err = md.db.Query(`
			SELECT id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text
			FROM messages
			WHERE text LIKE ? ESCAPE '\'
			ORDER BY timestamp DESC LIMIT ?`, likePattern, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	msgs := make([]Message, 0)
	for rows.Next() {
		var msg Message
		var ts int64
		if err := rows.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text); err != nil {
			return msgs, fmt.Errorf("failed to scan search result row: %w", err)
		}
		msg.Timestamp = uint64(ts)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// GetMessagesPaginated returns up to `limit` messages older than `beforeTimestamp` for a chat,
// ordered newest-first (so caller can reverse for display).
func (md *MessageDatabase) GetMessagesPaginated(chatId string, beforeTimestamp uint64, limit int) ([]Message, error) {
	if md.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := md.db.Query(`
		SELECT id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text
		FROM messages
		WHERE chat_id = ? AND timestamp < ?
		ORDER BY timestamp DESC LIMIT ?`, chatId, beforeTimestamp, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query paginated messages: %w", err)
	}
	defer rows.Close()

	msgs := make([]Message, 0)
	for rows.Next() {
		var msg Message
		var ts int64
		if err := rows.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text); err != nil {
			return msgs, fmt.Errorf("failed to scan paginated row: %w", err)
		}
		msg.Timestamp = uint64(ts)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}
