// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/PalWorks/whatscli/config"
	_ "github.com/mattn/go-sqlite3"
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

	// Backward-compatible migration: add is_archived column for existing databases.
	if err := md.addColumnIfMissing("conversations", "is_archived", "BOOLEAN DEFAULT 0"); err != nil {
		return err
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

	// Backward-compatible migration: media metadata columns (Phase 2a).
	// Existing rows receive the defaults, so old databases keep working and
	// pre-migration messages simply report "not a downloadable message".
	mediaColumns := []struct{ name, def string }{
		{"media_kind", "TEXT NOT NULL DEFAULT ''"},
		{"mime_type", "TEXT NOT NULL DEFAULT ''"},
		{"file_name", "TEXT NOT NULL DEFAULT ''"},
		{"media_proto", "BLOB"},
	}
	for _, col := range mediaColumns {
		if err := md.addColumnIfMissing("messages", col.name, col.def); err != nil {
			return err
		}
	}

	return nil
}

// addColumnIfMissing runs ALTER TABLE ... ADD COLUMN and treats SQLite's
// "duplicate column name" error as success, so the migration is idempotent.
// Any other error is returned.
func (md *MessageDatabase) addColumnIfMissing(table, column, definition string) error {
	_, err := md.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return fmt.Errorf("failed to add column %s.%s: %w", table, column, err)
	}
	return nil
}

// messageColumns is the column list shared by every message SELECT so that
// collectMessages can scan rows uniformly.
const messageColumns = `id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text,
	COALESCE(media_kind, ''), COALESCE(mime_type, ''), COALESCE(file_name, ''), media_proto`

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
	(id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text,
	 media_kind, mime_type, file_name, media_proto)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		msg.Id, msg.ChatId, msg.ContactId, msg.ContactName,
		msg.ContactShort, msg.Timestamp, msg.FromMe, msg.Forwarded, msg.Text,
		msg.MediaKind, msg.MimeType, msg.FileName, msg.MediaProto,
	)
	return err
}

// GetMessage returns a single message by ID. found is false when no row exists.
func (md *MessageDatabase) GetMessage(id string) (msg Message, found bool, err error) {
	if md.db == nil {
		return Message{}, false, fmt.Errorf("database not initialized")
	}
	rows, err := md.db.Query(`SELECT `+messageColumns+` FROM messages WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return Message{}, false, fmt.Errorf("failed to query message: %w", err)
	}
	defer rows.Close()
	msgs, err := collectMessages(rows)
	if err != nil {
		return Message{}, false, err
	}
	if len(msgs) == 0 {
		return Message{}, false, nil
	}
	return msgs[0], true, nil
}

// GetOldestMessage returns the oldest stored message of a chat, used as the
// anchor for on-demand history sync requests.
func (md *MessageDatabase) GetOldestMessage(chatId string) (msg Message, found bool, err error) {
	if md.db == nil {
		return Message{}, false, fmt.Errorf("database not initialized")
	}
	rows, err := md.db.Query(`SELECT `+messageColumns+` FROM messages WHERE chat_id = ? ORDER BY timestamp ASC LIMIT 1`, chatId)
	if err != nil {
		return Message{}, false, fmt.Errorf("failed to query oldest message: %w", err)
	}
	defer rows.Close()
	msgs, err := collectMessages(rows)
	if err != nil {
		return Message{}, false, err
	}
	if len(msgs) == 0 {
		return Message{}, false, nil
	}
	return msgs[0], true, nil
}

// CountMessages returns the number of stored messages for a chat.
func (md *MessageDatabase) CountMessages(chatId string) (int, error) {
	if md.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var n int
	if err := md.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE chat_id = ?`, chatId).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}
	return n, nil
}

// RevokedText replaces the text of a message deleted for everyone.
const RevokedText = "[message revoked]"

// MarkMessageRevoked replaces a message's text and clears its media metadata.
// The row must match both the message ID and the chat, so a revoke can never
// reach a message in another chat. Returns true when a row was updated.
func (md *MessageDatabase) MarkMessageRevoked(chatID, id string) (bool, error) {
	if md.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	res, err := md.db.Exec(`UPDATE messages SET text = ?, media_kind = '', mime_type = '', file_name = '', media_proto = NULL WHERE id = ? AND chat_id = ?`, RevokedText, id, chatID)
	if err != nil {
		return false, fmt.Errorf("failed to mark message revoked: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read revoke result: %w", err)
	}
	return n > 0, nil
}

// UpsertConversation updates or inserts a conversation
func (md *MessageDatabase) UpsertConversation(c Conversation) error {
	_, err := md.db.Exec(`
	INSERT INTO conversations (jid, name, last_msg_time, preview, unread, is_pinned, is_archived)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(jid) DO UPDATE SET
		name=excluded.name,
		last_msg_time=excluded.last_msg_time,
		preview=excluded.preview,
		unread=excluded.unread,
		is_pinned=excluded.is_pinned,
		is_archived=excluded.is_archived;
	`, c.JID, c.Name, c.LastMsgTime, c.Preview, c.Unread, c.IsPinned, c.IsArchived)
	return err
}

// DeleteEmptyConversations removes 1:1 chat rows that have no stored message,
// no time, no unread count and no pin: contact entries that older versions
// stored as "New chat" (ROADMAP row 33). Groups are kept. Idempotent.
func (md *MessageDatabase) DeleteEmptyConversations() (int64, error) {
	if md.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	res, err := md.db.Exec(`
	DELETE FROM conversations
	WHERE jid NOT LIKE '%@g.us'
	  AND COALESCE(last_msg_time, 0) = 0
	  AND COALESCE(unread, 0) = 0
	  AND NOT COALESCE(is_pinned, 0)
	  AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.chat_id = conversations.jid)`)
	if err != nil {
		return 0, fmt.Errorf("delete empty conversations: %w", err)
	}
	return res.RowsAffected()
}

// GetConversations retrieves all conversations from the DB
func (md *MessageDatabase) GetConversations() ([]Conversation, error) {
	rows, err := md.db.Query("SELECT jid, name, last_msg_time, preview, unread, is_pinned, is_archived FROM conversations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMsgTime, &c.Preview, &c.Unread, &c.IsPinned, &c.IsArchived); err != nil {
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
	rows, err := md.db.Query(`SELECT `+messageColumns+` FROM messages WHERE chat_id = ? ORDER BY timestamp ASC`, chatId)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	return collectMessages(rows)
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
			SELECT `+messageColumns+`
			FROM messages
			WHERE chat_id = ? AND text LIKE ? ESCAPE '\'
			ORDER BY timestamp DESC LIMIT ?`, chatId, likePattern, limit)
	} else {
		rows, err = md.db.Query(`
			SELECT `+messageColumns+`
			FROM messages
			WHERE text LIKE ? ESCAPE '\'
			ORDER BY timestamp DESC LIMIT ?`, likePattern, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	return collectMessages(rows)
}

// GetMessagesPaginated returns up to `limit` messages older than `beforeTimestamp` for a chat,
// ordered newest-first (so caller can reverse for display).
func (md *MessageDatabase) GetMessagesPaginated(chatId string, beforeTimestamp uint64, limit int) ([]Message, error) {
	if md.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := md.db.Query(`
		SELECT `+messageColumns+`
		FROM messages
		WHERE chat_id = ? AND timestamp < ?
		ORDER BY timestamp DESC LIMIT ?`, chatId, beforeTimestamp, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query paginated messages: %w", err)
	}
	defer rows.Close()

	return collectMessages(rows)
}

// collectMessages iterates over rows and scans each into a Message (audit CQ-2).
func collectMessages(rows *sql.Rows) ([]Message, error) {
	msgs := make([]Message, 0)
	for rows.Next() {
		var msg Message
		var ts int64
		if err := rows.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text, &msg.MediaKind, &msg.MimeType, &msg.FileName, &msg.MediaProto); err != nil {
			return msgs, fmt.Errorf("failed to scan message row: %w", err)
		}
		msg.Timestamp = uint64(ts)
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return msgs, fmt.Errorf("error iterating message rows: %w", err)
	}
	return msgs, nil
}
