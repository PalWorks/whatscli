package messages

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// MessageDatabase stores messages and contact data in SQLite
type MessageDatabase struct {
	db *sql.DB
}

// Init initializes the message database with a file-based SQLite connection.
func (md *MessageDatabase) Init() error {
	dbPath := sessionFilePath() + "_meta.db"
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
	// SQLite ignores the ALTER if the column already exists when using this pattern.
	md.db.Exec(`ALTER TABLE conversations ADD COLUMN is_archived BOOLEAN DEFAULT 0`)

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

	// Backward-compatible migrations: add media and deleted-message columns
	// for existing databases. SQLite errors on ALTER TABLE ADD COLUMN when
	// the column already exists; that error is intentionally discarded here
	// (same pattern as is_archived above), so this is safe to run every time
	// InitWithDB is called. Existing rows are backfilled with each column's
	// DEFAULT value by SQLite itself (the BLOB columns have no default and
	// so become NULL, which scans into a nil []byte without error).
	md.db.Exec(`ALTER TABLE messages ADD COLUMN media_type TEXT DEFAULT ''`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN mimetype TEXT DEFAULT ''`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN media_key BLOB`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN direct_path TEXT DEFAULT ''`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN file_sha256 BLOB`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN file_enc_sha256 BLOB`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN file_length INTEGER DEFAULT 0`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN file_name TEXT DEFAULT ''`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN deleted BOOLEAN DEFAULT 0`)

	// Phase 5.1: reply threading — the ID and a short display preview of the
	// message being replied to. Both empty for a non-reply message.
	md.db.Exec(`ALTER TABLE messages ADD COLUMN quoted_id TEXT DEFAULT ''`)
	md.db.Exec(`ALTER TABLE messages ADD COLUMN quoted_preview TEXT DEFAULT ''`)

	// Phase H4 (read markers): delivery state for own messages, driven by
	// live receipt events. 0=none/sent, 1=delivered, 2=read.
	md.db.Exec(`ALTER TABLE messages ADD COLUMN receipt_status INTEGER DEFAULT 0`)

	// Phase 5.2: reactions get their own table rather than being stored as
	// standalone messages. PRIMARY KEY(message_id, sender) means a second
	// reaction from the same sender on the same message replaces the first,
	// matching WhatsApp's one-reaction-per-person-per-message semantics.
	_, err = md.db.Exec(`
	CREATE TABLE IF NOT EXISTS reactions (
		message_id TEXT NOT NULL,
		sender TEXT NOT NULL,
		emoji TEXT NOT NULL,
		PRIMARY KEY (message_id, sender)
	);
	CREATE INDEX IF NOT EXISTS idx_reactions_message_id ON reactions(message_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to create reactions table: %w", err)
	}

	return nil
}

// messageColumns is the shared column list for every SELECT against the
// messages table, kept in one place so GetMessages, SearchMessages, and
// GetMessagesPaginated stay in sync with collectMessages' Scan order.
// Reactions are deliberately not part of this list: they live in a separate
// table and are attached to Message.Reactions by attachReactions after the
// row scan, not scanned as a column.
const messageColumns = "id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text, " +
	"media_type, mimetype, media_key, direct_path, file_sha256, file_enc_sha256, file_length, file_name, deleted, " +
	"quoted_id, quoted_preview, receipt_status"

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
	(`+messageColumns+`)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		msg.Id, msg.ChatId, msg.ContactId, msg.ContactName,
		msg.ContactShort, msg.Timestamp, msg.FromMe, msg.Forwarded, msg.Text,
		msg.MediaType, msg.Mimetype, msg.MediaKey, msg.DirectPath,
		msg.FileSHA256, msg.FileEncSHA256, msg.FileLength, msg.FileName, msg.Deleted,
		msg.QuotedID, msg.QuotedPreview, msg.ReceiptStatus,
	)
	return err
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
	rows, err := md.db.Query("SELECT "+messageColumns+" FROM messages WHERE chat_id = ? ORDER BY timestamp ASC", chatId)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	msgs, err := collectMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := md.attachReactions(msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// SetReaction records or removes a reaction on a message. An empty emoji
// removes the sender's existing reaction — this is how WhatsApp itself
// signals "reaction removed" (an empty-text ReactionMessage). A non-empty
// emoji replaces any prior reaction from the same sender on this message,
// matching one-reaction-per-person-per-message semantics.
func (md *MessageDatabase) SetReaction(messageID, sender, emoji string) error {
	if md.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if emoji == "" {
		_, err := md.db.Exec(`DELETE FROM reactions WHERE message_id = ? AND sender = ?`, messageID, sender)
		return err
	}
	_, err := md.db.Exec(`INSERT OR REPLACE INTO reactions (message_id, sender, emoji) VALUES (?, ?, ?)`, messageID, sender, emoji)
	return err
}

// GetReactionSummaries aggregates reactions for the given message IDs into a
// map keyed by message ID, each value ordered by emoji ascending for
// deterministic rendering. Returns an empty map without querying the database
// if messageIDs is empty.
func (md *MessageDatabase) GetReactionSummaries(messageIDs []string) (map[string][]ReactionSummary, error) {
	result := make(map[string][]ReactionSummary)
	if len(messageIDs) == 0 {
		return result, nil
	}
	if md.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	placeholders := make([]string, len(messageIDs))
	args := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "SELECT message_id, emoji, COUNT(*) FROM reactions WHERE message_id IN (" +
		strings.Join(placeholders, ",") + ") GROUP BY message_id, emoji ORDER BY message_id, emoji ASC"

	rows, err := md.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query reaction summaries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var messageID, emoji string
		var count int
		if err := rows.Scan(&messageID, &emoji, &count); err != nil {
			return nil, fmt.Errorf("failed to scan reaction summary row: %w", err)
		}
		result[messageID] = append(result[messageID], ReactionSummary{Emoji: emoji, Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reaction summary rows: %w", err)
	}
	return result, nil
}

// attachReactions populates Reactions on each message in msgs (mutating in
// place) via a single batched query keyed by message ID, keeping every read
// path free of N+1 queries. A no-op for an empty slice.
func (md *MessageDatabase) attachReactions(msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.Id
	}
	summaries, err := md.GetReactionSummaries(ids)
	if err != nil {
		return fmt.Errorf("failed to load reactions: %w", err)
	}
	for i := range msgs {
		msgs[i].Reactions = summaries[msgs[i].Id]
	}
	return nil
}

// GetMessageByID retrieves a single message by its primary key.
// Returns an error if no message with that ID exists.
func (md *MessageDatabase) GetMessageByID(id string) (Message, error) {
	if md.db == nil {
		return Message{}, fmt.Errorf("database not initialized")
	}
	row := md.db.QueryRow("SELECT "+messageColumns+" FROM messages WHERE id = ?", id)

	var msg Message
	var ts int64
	err := row.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text,
		&msg.MediaType, &msg.Mimetype, &msg.MediaKey, &msg.DirectPath, &msg.FileSHA256, &msg.FileEncSHA256, &msg.FileLength, &msg.FileName, &msg.Deleted,
		&msg.QuotedID, &msg.QuotedPreview, &msg.ReceiptStatus)
	if err != nil {
		return Message{}, fmt.Errorf("failed to get message %q: %w", id, err)
	}
	msg.Timestamp = uint64(ts)
	msgs := []Message{msg}
	if err := md.attachReactions(msgs); err != nil {
		return Message{}, err
	}
	return msgs[0], nil
}

// SetReceiptStatus raises receipt_status to status for every given message
// ID. Monotonic by construction (MAX(receipt_status, ?)): receipts can arrive
// out of order (Android sends newest-first, iOS oldest-first, and a
// "delivered" receipt can trail a "read" one on a flaky link), so a lower
// state never overwrites a higher one. IDs not present in the table are
// silently ignored — receipts routinely reference messages sent before this
// session's history begins.
func (md *MessageDatabase) SetReceiptStatus(ids []string, status int) error {
	if md.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, status)
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := md.db.Exec(`UPDATE messages SET receipt_status = MAX(receipt_status, ?) WHERE id IN (`+placeholders+`)`, args...)
	return err
}

// MarkMessageDeleted marks a message as revoked/deleted: the stored text is
// replaced with a placeholder and Deleted is set, but the row (and its
// metadata) is kept for audit purposes rather than removed.
func (md *MessageDatabase) MarkMessageDeleted(id string) error {
	if md.db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := md.db.Exec(`UPDATE messages SET text = '[DELETED]', deleted = 1 WHERE id = ?`, id)
	return err
}

// UpdateMessageText replaces the stored text for a message. Used when an
// edit protocol message is received for a message already in the DB.
func (md *MessageDatabase) UpdateMessageText(id, text string) error {
	if md.db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := md.db.Exec(`UPDATE messages SET text = ? WHERE id = ?`, text, id)
	return err
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
		rows, err = md.db.Query(
			"SELECT "+messageColumns+`
			FROM messages
			WHERE chat_id = ? AND text LIKE ? ESCAPE '\'
			ORDER BY timestamp DESC LIMIT ?`, chatId, likePattern, limit)
	} else {
		rows, err = md.db.Query(
			"SELECT "+messageColumns+`
			FROM messages
			WHERE text LIKE ? ESCAPE '\'
			ORDER BY timestamp DESC LIMIT ?`, likePattern, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	msgs, err := collectMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := md.attachReactions(msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// GetMessagesPaginated returns up to `limit` messages older than `beforeTimestamp` for a chat,
// ordered newest-first (so caller can reverse for display).
func (md *MessageDatabase) GetMessagesPaginated(chatId string, beforeTimestamp uint64, limit int) ([]Message, error) {
	if md.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := md.db.Query(
		"SELECT "+messageColumns+`
		FROM messages
		WHERE chat_id = ? AND timestamp < ?
		ORDER BY timestamp DESC LIMIT ?`, chatId, beforeTimestamp, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query paginated messages: %w", err)
	}
	defer rows.Close()

	msgs, err := collectMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := md.attachReactions(msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// collectMessages iterates over rows and scans each into a Message (audit CQ-2).
func collectMessages(rows *sql.Rows) ([]Message, error) {
	msgs := make([]Message, 0)
	for rows.Next() {
		var msg Message
		var ts int64
		if err := rows.Scan(&msg.Id, &msg.ChatId, &msg.ContactId, &msg.ContactName, &msg.ContactShort, &ts, &msg.FromMe, &msg.Forwarded, &msg.Text,
			&msg.MediaType, &msg.Mimetype, &msg.MediaKey, &msg.DirectPath, &msg.FileSHA256, &msg.FileEncSHA256, &msg.FileLength, &msg.FileName, &msg.Deleted,
			&msg.QuotedID, &msg.QuotedPreview, &msg.ReceiptStatus); err != nil {
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
