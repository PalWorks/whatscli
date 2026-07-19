package messages

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB creates an in-memory MessageDatabase for testing.
func newTestDB(t *testing.T) *MessageDatabase {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatalf("InitWithDB failed: %v", err)
	}
	return md
}

// addTestMsg is a helper that calls AddMessage and fails the test on error.
func addTestMsg(t *testing.T, md *MessageDatabase, msg Message) {
	t.Helper()
	if err := md.AddMessage(msg); err != nil {
		t.Fatalf("AddMessage failed for %s: %v", msg.Id, err)
	}
}

func TestInit_CreatesTables(t *testing.T) {
	md := newTestDB(t)

	// Verify conversations table exists by querying it
	_, err := md.db.Exec("SELECT jid, name, last_msg_time, preview, unread, is_pinned, is_archived FROM conversations LIMIT 1")
	if err != nil {
		t.Errorf("conversations table not created: %v", err)
	}

	// Verify messages table exists
	_, err = md.db.Exec("SELECT id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text FROM messages LIMIT 1")
	if err != nil {
		t.Errorf("messages table not created: %v", err)
	}

	// Verify index exists
	row := md.db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_messages_chat_id'")
	var indexName string
	if err := row.Scan(&indexName); err != nil {
		t.Errorf("idx_messages_chat_id index not created: %v", err)
	}
}

func TestClose(t *testing.T) {
	md := newTestDB(t)

	if err := md.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	// After close, queries should fail
	_, err := md.db.Query("SELECT 1")
	if err == nil {
		t.Error("expected error after Close(), got nil")
	}
}

func TestClose_NilDB(t *testing.T) {
	md := &MessageDatabase{} // db is nil
	if err := md.Close(); err != nil {
		t.Fatalf("Close() on nil db should not error, got: %v", err)
	}
}

func TestAddMessage_AndGetMessages(t *testing.T) {
	md := newTestDB(t)

	chatID := "123@s.whatsapp.net"
	msgs := []Message{
		{Id: "m1", ChatId: chatID, ContactId: "c1", ContactName: "Alice", ContactShort: "A", Timestamp: 1000, FromMe: false, Text: "Hello"},
		{Id: "m2", ChatId: chatID, ContactId: "c1", ContactName: "Alice", ContactShort: "A", Timestamp: 2000, FromMe: false, Text: "World"},
		{Id: "m3", ChatId: chatID, ContactId: "me", ContactName: "Me", ContactShort: "M", Timestamp: 1500, FromMe: true, Text: "Hi there"},
	}

	for _, msg := range msgs {
		addTestMsg(t, md, msg)
	}

	result, err := md.GetMessages(chatID)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// Verify ordering: should be by timestamp ASC → m1(1000), m3(1500), m2(2000)
	expectedOrder := []string{"m1", "m3", "m2"}
	for i, expected := range expectedOrder {
		if result[i].Id != expected {
			t.Errorf("index %d: expected ID %s, got %s", i, expected, result[i].Id)
		}
	}

	// Verify message content roundtrip
	if result[0].Text != "Hello" || result[0].ContactName != "Alice" {
		t.Errorf("message content mismatch: got text=%q name=%q", result[0].Text, result[0].ContactName)
	}
}

func TestAddMessage_DuplicateIgnored(t *testing.T) {
	md := newTestDB(t)

	msg := Message{Id: "dup1", ChatId: "chat1", ContactId: "c1", Timestamp: 1000, Text: "original"}

	addTestMsg(t, md, msg)

	// Insert duplicate with different text — should be ignored (INSERT OR IGNORE)
	msg.Text = "duplicate"
	_ = md.AddMessage(msg) // no error expected for IGNORE

	result, err := md.GetMessages("chat1")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message after duplicate insert, got %d", len(result))
	}
	if result[0].Text != "original" {
		t.Errorf("expected original text, got %q", result[0].Text)
	}
}

func TestUpsertConversation_InsertAndUpdate(t *testing.T) {
	md := newTestDB(t)

	conv := Conversation{
		JID:         "123@s.whatsapp.net",
		Name:        "Alice",
		LastMsgTime: 1000,
		Preview:     "Hello",
		Unread:      2,
		IsPinned:    false,
		IsArchived:  false,
	}

	// Insert
	if err := md.UpsertConversation(conv); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	convs, err := md.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convs))
	}
	if convs[0].Name != "Alice" || convs[0].Preview != "Hello" || convs[0].IsArchived != false {
		t.Errorf("insert mismatch: name=%q preview=%q archived=%v", convs[0].Name, convs[0].Preview, convs[0].IsArchived)
	}

	// Update — also toggle IsArchived to true
	conv.Name = "Alice Updated"
	conv.Preview = "Bye"
	conv.LastMsgTime = 2000
	conv.IsArchived = true
	if err := md.UpsertConversation(conv); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	convs, err = md.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations after update failed: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected 1 conversation after upsert, got %d", len(convs))
	}
	if convs[0].Name != "Alice Updated" || convs[0].Preview != "Bye" || convs[0].LastMsgTime != 2000 || convs[0].IsArchived != true {
		t.Errorf("update mismatch: name=%q preview=%q time=%d archived=%v", convs[0].Name, convs[0].Preview, convs[0].LastMsgTime, convs[0].IsArchived)
	}
}

func TestGetConversations_Multiple(t *testing.T) {
	md := newTestDB(t)

	convos := []Conversation{
		{JID: "a@s.whatsapp.net", Name: "Alice", LastMsgTime: 100},
		{JID: "b@s.whatsapp.net", Name: "Bob", LastMsgTime: 200},
		{JID: "c@g.us", Name: "Group C", LastMsgTime: 300},
	}
	for _, c := range convos {
		if err := md.UpsertConversation(c); err != nil {
			t.Fatalf("UpsertConversation failed for %s: %v", c.JID, err)
		}
	}

	result, err := md.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(result))
	}
}

func TestGetMessages_EmptyChat(t *testing.T) {
	md := newTestDB(t)

	result, err := md.GetMessages("nonexistent@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestGetMessages_SpecialChars(t *testing.T) {
	md := newTestDB(t)

	chatID := "special@s.whatsapp.net"
	msg := Message{
		Id:          "special1",
		ChatId:      chatID,
		ContactId:   "c1",
		ContactName: "O'Brien",
		Timestamp:   1000,
		Text:        "Hello 🌍! \"Quotes\" & 'apostrophes' and\nnewlines",
	}

	addTestMsg(t, md, msg)

	result, err := md.GetMessages(chatID)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Text != msg.Text {
		t.Errorf("special chars text mismatch:\nexpected: %q\ngot:      %q", msg.Text, result[0].Text)
	}
	if result[0].ContactName != "O'Brien" {
		t.Errorf("contact name mismatch: expected O'Brien, got %q", result[0].ContactName)
	}
}

// --- Tests for SearchMessages ---

func TestSearchMessages_InChat(t *testing.T) {
	md := newTestDB(t)
	chat := "chat1@s.whatsapp.net"
	addTestMsg(t, md, Message{Id: "s1", ChatId: chat, ContactId: "c1", Timestamp: 1000, Text: "Hello world"})
	addTestMsg(t, md, Message{Id: "s2", ChatId: chat, ContactId: "c1", Timestamp: 2000, Text: "Goodbye world"})
	addTestMsg(t, md, Message{Id: "s3", ChatId: chat, ContactId: "c1", Timestamp: 3000, Text: "Hello again"})
	addTestMsg(t, md, Message{Id: "s4", ChatId: "other@s.whatsapp.net", ContactId: "c2", Timestamp: 4000, Text: "Hello from other chat"})

	results, err := md.SearchMessages(chat, "Hello", 50)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results scoped to chat, got %d", len(results))
	}
	// Newest first
	if results[0].Id != "s3" || results[1].Id != "s1" {
		t.Errorf("unexpected order: [%s, %s]", results[0].Id, results[1].Id)
	}
}

func TestSearchMessages_AllChats(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "a1", ChatId: "c1@s.whatsapp.net", ContactId: "x", Timestamp: 1000, Text: "Project Alpha update"})
	addTestMsg(t, md, Message{Id: "a2", ChatId: "c2@s.whatsapp.net", ContactId: "y", Timestamp: 2000, Text: "Alpha version released"})
	addTestMsg(t, md, Message{Id: "a3", ChatId: "c3@g.us", ContactId: "z", Timestamp: 3000, Text: "No match here"})

	results, err := md.SearchMessages("", "Alpha", 50)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 cross-chat results, got %d", len(results))
	}
}

func TestSearchMessages_NoResults(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "n1", ChatId: "c1@s.whatsapp.net", ContactId: "x", Timestamp: 1000, Text: "Nothing interesting"})

	results, err := md.SearchMessages("c1@s.whatsapp.net", "xyznonexistent", 50)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchMessages_EmptyKeyword(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "e1", ChatId: "c1@s.whatsapp.net", ContactId: "x", Timestamp: 1000, Text: "Something"})

	results, err := md.SearchMessages("c1@s.whatsapp.net", "", 50)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty keyword should return 0 results, got %d", len(results))
	}
}

func TestSearchMessages_Limit(t *testing.T) {
	md := newTestDB(t)
	chat := "c@s.whatsapp.net"
	for i := 0; i < 10; i++ {
		addTestMsg(t, md, Message{Id: fmt.Sprintf("lim%d", i), ChatId: chat, ContactId: "x", Timestamp: uint64(1000 + i), Text: "match"})
	}

	results, err := md.SearchMessages(chat, "match", 3)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

// --- Tests for GetMessagesPaginated ---

func TestGetMessagesPaginated_Basic(t *testing.T) {
	md := newTestDB(t)
	chat := "chat@s.whatsapp.net"
	for i := 1; i <= 5; i++ {
		addTestMsg(t, md, Message{Id: fmt.Sprintf("p%d", i), ChatId: chat, ContactId: "c1", Timestamp: uint64(i * 1000), Text: fmt.Sprintf("msg %d", i)})
	}

	// Get messages before timestamp 4000 (should get p1, p2, p3)
	results, err := md.GetMessagesPaginated(chat, 4000, 50)
	if err != nil {
		t.Fatalf("GetMessagesPaginated error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 messages before ts=4000, got %d", len(results))
	}
	// Newest first (DESC order)
	if results[0].Id != "p3" || results[1].Id != "p2" || results[2].Id != "p1" {
		t.Errorf("unexpected order: [%s, %s, %s]", results[0].Id, results[1].Id, results[2].Id)
	}
}

func TestGetMessagesPaginated_Limit(t *testing.T) {
	md := newTestDB(t)
	chat := "chat@s.whatsapp.net"
	for i := 1; i <= 10; i++ {
		addTestMsg(t, md, Message{Id: fmt.Sprintf("pl%d", i), ChatId: chat, ContactId: "c1", Timestamp: uint64(i * 100), Text: "msg"})
	}

	results, err := md.GetMessagesPaginated(chat, 800, 3)
	if err != nil {
		t.Fatalf("GetMessagesPaginated error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 messages with limit=3, got %d", len(results))
	}
}

func TestGetMessagesPaginated_NoOlderMessages(t *testing.T) {
	md := newTestDB(t)
	chat := "chat@s.whatsapp.net"
	addTestMsg(t, md, Message{Id: "only1", ChatId: chat, ContactId: "c1", Timestamp: 5000, Text: "only message"})

	// Before this message's timestamp — nothing exists
	results, err := md.GetMessagesPaginated(chat, 5000, 50)
	if err != nil {
		t.Fatalf("GetMessagesPaginated error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 older messages, got %d", len(results))
	}
}

// ----- Audit fix regression tests (T-1) -----

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`c\d`, `c\\d`},
		{`%_\`, `\%\_\\`},
	}
	for _, tt := range tests {
		got := escapeLike(tt.input)
		if got != tt.want {
			t.Errorf("escapeLike(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSearchMessages_LikeMetacharsEscaped(t *testing.T) {
	md := newTestDB(t)
	chat := "chat@s.whatsapp.net"

	// Insert a message with literal % and _ in the text
	addTestMsg(t, md, Message{Id: "m1", ChatId: chat, ContactId: "c1", Timestamp: 1000, Text: "100% works"})
	addTestMsg(t, md, Message{Id: "m2", ChatId: chat, ContactId: "c1", Timestamp: 2000, Text: "foo_bar"})
	addTestMsg(t, md, Message{Id: "m3", ChatId: chat, ContactId: "c1", Timestamp: 3000, Text: "nothing special"})

	// Searching for "%" should only match the message containing literal %
	results, err := md.SearchMessages(chat, "%", 50)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 1 || results[0].Id != "m1" {
		t.Errorf("search for '%%': expected [m1], got %v", msgIDs(results))
	}

	// Searching for "_" should only match the message containing literal _
	results, err = md.SearchMessages(chat, "_", 50)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(results) != 1 || results[0].Id != "m2" {
		t.Errorf("search for '_': expected [m2], got %v", msgIDs(results))
	}
}

func TestInit_CreatesCompositeIndex(t *testing.T) {
	md := newTestDB(t)
	row := md.db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_messages_chat_ts'")
	var name string
	if err := row.Scan(&name); err != nil {
		t.Errorf("composite index idx_messages_chat_ts not found: %v", err)
	}
}

// msgIDs is a test helper that extracts message IDs for readable error output.
func msgIDs(msgs []Message) []string {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.Id
	}
	return ids
}

// ----- Phase 2.1: media column migration tests -----

func TestAddMessage_MediaFieldsRoundTrip(t *testing.T) {
	md := newTestDB(t)

	msg := Message{
		Id:            "media1",
		ChatId:        "chat@s.whatsapp.net",
		ContactId:     "c1",
		Timestamp:     1000,
		Text:          "[IMAGE] a caption",
		MediaType:     "image",
		Mimetype:      "image/jpeg",
		MediaKey:      []byte{0x01, 0x02, 0x03, 0xFF},
		DirectPath:    "/v/t62.7118-24/abc",
		FileSHA256:    []byte{0xAA, 0xBB},
		FileEncSHA256: []byte{0xCC, 0xDD},
		FileLength:    123456,
		FileName:      "",
	}
	addTestMsg(t, md, msg)

	result, err := md.GetMessages(msg.ChatId)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	got := result[0]
	if got.MediaType != "image" {
		t.Errorf("MediaType: expected %q, got %q", "image", got.MediaType)
	}
	if got.Mimetype != "image/jpeg" {
		t.Errorf("Mimetype: expected %q, got %q", "image/jpeg", got.Mimetype)
	}
	if string(got.MediaKey) != string(msg.MediaKey) {
		t.Errorf("MediaKey mismatch: expected %v, got %v", msg.MediaKey, got.MediaKey)
	}
	if got.DirectPath != msg.DirectPath {
		t.Errorf("DirectPath: expected %q, got %q", msg.DirectPath, got.DirectPath)
	}
	if string(got.FileSHA256) != string(msg.FileSHA256) {
		t.Errorf("FileSHA256 mismatch: expected %v, got %v", msg.FileSHA256, got.FileSHA256)
	}
	if string(got.FileEncSHA256) != string(msg.FileEncSHA256) {
		t.Errorf("FileEncSHA256 mismatch: expected %v, got %v", msg.FileEncSHA256, got.FileEncSHA256)
	}
	if got.FileLength != msg.FileLength {
		t.Errorf("FileLength: expected %d, got %d", msg.FileLength, got.FileLength)
	}
	if got.Deleted {
		t.Error("expected Deleted=false for a freshly inserted message")
	}
}

func TestAddMessage_NonMediaMessage_MediaFieldsEmpty(t *testing.T) {
	md := newTestDB(t)

	addTestMsg(t, md, Message{Id: "plain1", ChatId: "chat@s.whatsapp.net", ContactId: "c1", Timestamp: 1000, Text: "just text"})

	result, err := md.GetMessages("chat@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	got := result[0]
	if got.MediaType != "" || got.Mimetype != "" || got.DirectPath != "" || got.FileName != "" {
		t.Errorf("expected all media text fields empty for a plain message, got %+v", got)
	}
	if got.MediaKey != nil || got.FileSHA256 != nil || got.FileEncSHA256 != nil {
		t.Errorf("expected all media blob fields nil for a plain message, got %+v", got)
	}
	if got.FileLength != 0 {
		t.Errorf("expected FileLength=0 for a plain message, got %d", got.FileLength)
	}
}

// TestInitWithDB_UpgradesPreMigrationSchema verifies that a database created
// with the pre-Phase-2 schema (no media columns) is upgraded in place by a
// second InitWithDB call, and that rows inserted before the upgrade still
// read back correctly with empty/zero media fields.
func TestInitWithDB_UpgradesPreMigrationSchema(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	// Recreate the pre-Phase-2 schema by hand (no media/deleted columns).
	_, err = db.Exec(`
	CREATE TABLE conversations (
		jid TEXT PRIMARY KEY,
		name TEXT,
		last_msg_time INTEGER,
		preview TEXT,
		unread INTEGER,
		is_pinned BOOLEAN,
		is_archived BOOLEAN DEFAULT 0
	);
	CREATE TABLE messages (
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
	CREATE INDEX idx_messages_chat_id ON messages(chat_id);
	CREATE INDEX idx_messages_chat_ts ON messages(chat_id, timestamp);
	`)
	if err != nil {
		t.Fatalf("failed to create legacy schema: %v", err)
	}

	// Insert a "legacy" row directly, bypassing AddMessage (which now
	// expects the new columns).
	_, err = db.Exec(`INSERT INTO messages (id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text)
		VALUES ('legacy1', 'chat@s.whatsapp.net', 'c1', 'Alice', 'A', 500, 0, 0, 'a legacy message')`)
	if err != nil {
		t.Fatalf("failed to insert legacy row: %v", err)
	}

	// Now run the real migration path against this pre-existing schema.
	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatalf("InitWithDB failed to upgrade legacy schema: %v", err)
	}

	// The legacy row must still read back correctly, with zero-value media fields.
	result, err := md.GetMessages("chat@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages on upgraded schema failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 legacy message after upgrade, got %d", len(result))
	}
	legacy := result[0]
	if legacy.Text != "a legacy message" || legacy.ContactName != "Alice" {
		t.Errorf("legacy row content corrupted after upgrade: %+v", legacy)
	}
	if legacy.MediaType != "" || legacy.Mimetype != "" {
		t.Errorf("expected empty media text fields on legacy row, got MediaType=%q Mimetype=%q", legacy.MediaType, legacy.Mimetype)
	}
	if legacy.MediaKey != nil {
		t.Errorf("expected nil MediaKey on legacy row, got %v", legacy.MediaKey)
	}
	if legacy.Deleted {
		t.Error("expected Deleted=false on legacy row")
	}

	// A new message inserted through the normal path must also work post-upgrade.
	addTestMsg(t, md, Message{Id: "new1", ChatId: "chat@s.whatsapp.net", ContactId: "c1", Timestamp: 600, Text: "new message", MediaType: "document", FileName: "report.pdf"})
	result, err = md.GetMessages("chat@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages after post-upgrade insert failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages after post-upgrade insert, got %d", len(result))
	}
}

func TestGetMessageByID_Found(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "byid1", ChatId: "chat@s.whatsapp.net", ContactId: "c1", Timestamp: 1000, Text: "hello", MediaType: "image"})

	msg, err := md.GetMessageByID("byid1")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if msg.Id != "byid1" || msg.Text != "hello" || msg.MediaType != "image" {
		t.Errorf("unexpected message: %+v", msg)
	}
}

func TestGetMessageByID_NotFound(t *testing.T) {
	md := newTestDB(t)

	_, err := md.GetMessageByID("does-not-exist")
	if err == nil {
		t.Error("expected an error for a nonexistent message ID, got nil")
	}
}

func TestMarkMessageDeleted(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "del1", ChatId: "chat@s.whatsapp.net", ContactId: "c1", Timestamp: 1000, Text: "to be revoked"})

	if err := md.MarkMessageDeleted("del1"); err != nil {
		t.Fatalf("MarkMessageDeleted error: %v", err)
	}

	msg, err := md.GetMessageByID("del1")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if !msg.Deleted {
		t.Error("expected Deleted=true after MarkMessageDeleted")
	}
	if msg.Text != "[DELETED]" {
		t.Errorf("expected text to be replaced with [DELETED], got %q", msg.Text)
	}
}

func TestMarkMessageDeleted_NonexistentID_NoError(t *testing.T) {
	md := newTestDB(t)

	// UPDATE against a nonexistent id affects 0 rows but is not itself an error.
	if err := md.MarkMessageDeleted("does-not-exist"); err != nil {
		t.Errorf("expected no error marking a nonexistent message deleted, got %v", err)
	}
}

// ----- Phase 5.1: reply threading (QuotedID/QuotedPreview) -----

func TestAddMessage_QuotedFieldsRoundTrip(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "orig1", ChatId: "c@s.whatsapp.net", ContactId: "c1", Timestamp: 1000, Text: "original"})
	addTestMsg(t, md, Message{
		Id: "reply1", ChatId: "c@s.whatsapp.net", ContactId: "c1", Timestamp: 1001, Text: "a reply",
		QuotedID: "orig1", QuotedPreview: "original",
	})

	msg, err := md.GetMessageByID("reply1")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if msg.QuotedID != "orig1" {
		t.Errorf("QuotedID: expected %q, got %q", "orig1", msg.QuotedID)
	}
	if msg.QuotedPreview != "original" {
		t.Errorf("QuotedPreview: expected %q, got %q", "original", msg.QuotedPreview)
	}
}

func TestAddMessage_NonReplyMessage_QuotedFieldsEmpty(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "plain2", ChatId: "c@s.whatsapp.net", ContactId: "c1", Timestamp: 1000, Text: "just text"})

	msg, err := md.GetMessageByID("plain2")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if msg.QuotedID != "" || msg.QuotedPreview != "" {
		t.Errorf("expected empty quote fields on a non-reply message, got QuotedID=%q QuotedPreview=%q", msg.QuotedID, msg.QuotedPreview)
	}
}

func TestInitWithDB_UpgradesSchemaMissingQuoteColumns(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	// Recreate the pre-5.1 schema: has the Phase 2/3 media/deleted columns
	// but not quoted_id/quoted_preview, and no reactions table at all.
	_, err = db.Exec(`
	CREATE TABLE conversations (
		jid TEXT PRIMARY KEY, name TEXT, last_msg_time INTEGER, preview TEXT,
		unread INTEGER, is_pinned BOOLEAN, is_archived BOOLEAN DEFAULT 0
	);
	CREATE TABLE messages (
		id TEXT PRIMARY KEY, chat_id TEXT, contact_id TEXT, contact_name TEXT,
		contact_short TEXT, timestamp INTEGER, from_me BOOLEAN, forwarded BOOLEAN, text TEXT,
		media_type TEXT DEFAULT '', mimetype TEXT DEFAULT '', media_key BLOB,
		direct_path TEXT DEFAULT '', file_sha256 BLOB, file_enc_sha256 BLOB,
		file_length INTEGER DEFAULT 0, file_name TEXT DEFAULT '', deleted BOOLEAN DEFAULT 0
	);
	`)
	if err != nil {
		t.Fatalf("failed to create pre-5.1 schema: %v", err)
	}
	_, err = db.Exec(`INSERT INTO messages (id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text)
		VALUES ('legacy2', 'chat@s.whatsapp.net', 'c1', 'Bob', 'B', 500, 0, 0, 'pre-reply-feature message')`)
	if err != nil {
		t.Fatalf("failed to insert legacy row: %v", err)
	}

	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatalf("InitWithDB failed to upgrade pre-5.1 schema: %v", err)
	}

	legacy, err := md.GetMessageByID("legacy2")
	if err != nil {
		t.Fatalf("GetMessageByID on upgraded schema failed: %v", err)
	}
	if legacy.Text != "pre-reply-feature message" {
		t.Errorf("legacy row content corrupted after upgrade: %+v", legacy)
	}
	if legacy.QuotedID != "" || legacy.QuotedPreview != "" {
		t.Errorf("expected empty quote fields on legacy row, got QuotedID=%q QuotedPreview=%q", legacy.QuotedID, legacy.QuotedPreview)
	}

	// The reactions table must now exist and be usable after the upgrade.
	if err := md.SetReaction("legacy2", "sender1@s.whatsapp.net", "👍"); err != nil {
		t.Errorf("expected reactions table to be usable after upgrade, SetReaction failed: %v", err)
	}
}

// ----- Phase 5.2: reactions -----

func TestSetReaction_AddThenRoundTrip(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "r1", ChatId: "c@s.whatsapp.net", Text: "hi"})

	if err := md.SetReaction("r1", "alice@s.whatsapp.net", "👍"); err != nil {
		t.Fatalf("SetReaction error: %v", err)
	}

	msg, err := md.GetMessageByID("r1")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if len(msg.Reactions) != 1 || msg.Reactions[0].Emoji != "👍" || msg.Reactions[0].Count != 1 {
		t.Errorf("expected one 👍 reaction, got %+v", msg.Reactions)
	}
}

func TestSetReaction_SameSenderReplacesReaction(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "r2", ChatId: "c@s.whatsapp.net", Text: "hi"})

	if err := md.SetReaction("r2", "alice@s.whatsapp.net", "👍"); err != nil {
		t.Fatalf("SetReaction error: %v", err)
	}
	if err := md.SetReaction("r2", "alice@s.whatsapp.net", "😂"); err != nil {
		t.Fatalf("SetReaction (replace) error: %v", err)
	}

	msg, err := md.GetMessageByID("r2")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if len(msg.Reactions) != 1 || msg.Reactions[0].Emoji != "😂" {
		t.Errorf("expected only the replacement 😂 reaction, got %+v", msg.Reactions)
	}
}

func TestSetReaction_EmptyEmojiRemoves(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "r3", ChatId: "c@s.whatsapp.net", Text: "hi"})

	if err := md.SetReaction("r3", "alice@s.whatsapp.net", "👍"); err != nil {
		t.Fatalf("SetReaction error: %v", err)
	}
	if err := md.SetReaction("r3", "alice@s.whatsapp.net", ""); err != nil {
		t.Fatalf("SetReaction (remove) error: %v", err)
	}

	msg, err := md.GetMessageByID("r3")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if len(msg.Reactions) != 0 {
		t.Errorf("expected reactions to be empty after removal, got %+v", msg.Reactions)
	}
}

func TestSetReaction_RemovingNonexistentReaction_NoError(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "r4", ChatId: "c@s.whatsapp.net", Text: "hi"})

	if err := md.SetReaction("r4", "alice@s.whatsapp.net", ""); err != nil {
		t.Errorf("expected no error removing a reaction that was never set, got %v", err)
	}
}

func TestGetReactionSummaries_AggregatesAcrossSenders(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "r5", ChatId: "c@s.whatsapp.net", Text: "hi"})

	for _, sender := range []string{"alice@s.whatsapp.net", "bob@s.whatsapp.net", "carol@s.whatsapp.net"} {
		if err := md.SetReaction("r5", sender, "👍"); err != nil {
			t.Fatalf("SetReaction(%s) error: %v", sender, err)
		}
	}
	if err := md.SetReaction("r5", "dave@s.whatsapp.net", "❤️"); err != nil {
		t.Fatalf("SetReaction error: %v", err)
	}

	summaries, err := md.GetReactionSummaries([]string{"r5"})
	if err != nil {
		t.Fatalf("GetReactionSummaries error: %v", err)
	}
	got := summaries["r5"]
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct emoji, got %+v", got)
	}
	// Ordered by emoji ascending: "❤️" sorts before "👍".
	if got[0].Emoji != "❤️" || got[0].Count != 1 {
		t.Errorf("expected {❤️ 1} first, got %+v", got[0])
	}
	if got[1].Emoji != "👍" || got[1].Count != 3 {
		t.Errorf("expected {👍 3} second, got %+v", got[1])
	}
}

func TestGetReactionSummaries_EmptyInput(t *testing.T) {
	md := newTestDB(t)
	summaries, err := md.GetReactionSummaries(nil)
	if err != nil {
		t.Fatalf("GetReactionSummaries(nil) error: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected an empty map for no message IDs, got %+v", summaries)
	}
}

func TestGetMessages_AttachesReactionsToMultipleMessages(t *testing.T) {
	md := newTestDB(t)
	addTestMsg(t, md, Message{Id: "r6", ChatId: "c@s.whatsapp.net", Timestamp: 1, Text: "first"})
	addTestMsg(t, md, Message{Id: "r7", ChatId: "c@s.whatsapp.net", Timestamp: 2, Text: "second"})
	if err := md.SetReaction("r6", "alice@s.whatsapp.net", "👍"); err != nil {
		t.Fatalf("SetReaction error: %v", err)
	}

	msgs, err := md.GetMessages("c@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if len(msgs[0].Reactions) != 1 || msgs[0].Reactions[0].Emoji != "👍" {
		t.Errorf("expected r6 to carry the 👍 reaction, got %+v", msgs[0].Reactions)
	}
	if len(msgs[1].Reactions) != 0 {
		t.Errorf("expected r7 to have no reactions, got %+v", msgs[1].Reactions)
	}
}
