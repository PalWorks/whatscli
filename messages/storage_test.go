package messages

import (
	"database/sql"
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

func TestInit_CreatesTables(t *testing.T) {
	md := newTestDB(t)

	// Verify conversations table exists by querying it
	_, err := md.db.Exec("SELECT jid, name, last_msg_time, preview, unread, is_pinned FROM conversations LIMIT 1")
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

func TestAddMessage_AndGetMessages(t *testing.T) {
	md := newTestDB(t)

	chatID := "123@s.whatsapp.net"
	msgs := []Message{
		{Id: "m1", ChatId: chatID, ContactId: "c1", ContactName: "Alice", ContactShort: "A", Timestamp: 1000, FromMe: false, Text: "Hello"},
		{Id: "m2", ChatId: chatID, ContactId: "c1", ContactName: "Alice", ContactShort: "A", Timestamp: 2000, FromMe: false, Text: "World"},
		{Id: "m3", ChatId: chatID, ContactId: "me", ContactName: "Me", ContactShort: "M", Timestamp: 1500, FromMe: true, Text: "Hi there"},
	}

	for _, msg := range msgs {
		if !md.AddMessage(msg) {
			t.Fatalf("AddMessage failed for %s", msg.Id)
		}
	}

	result := md.GetMessages(chatID)
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

	if !md.AddMessage(msg) {
		t.Fatal("first AddMessage failed")
	}

	// Insert duplicate with different text — should be ignored (INSERT OR IGNORE)
	msg.Text = "duplicate"
	md.AddMessage(msg) // may return false, that's fine

	result := md.GetMessages("chat1")
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
	if convs[0].Name != "Alice" || convs[0].Preview != "Hello" {
		t.Errorf("insert mismatch: name=%q preview=%q", convs[0].Name, convs[0].Preview)
	}

	// Update
	conv.Name = "Alice Updated"
	conv.Preview = "Bye"
	conv.LastMsgTime = 2000
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
	if convs[0].Name != "Alice Updated" || convs[0].Preview != "Bye" || convs[0].LastMsgTime != 2000 {
		t.Errorf("update mismatch: name=%q preview=%q time=%d", convs[0].Name, convs[0].Preview, convs[0].LastMsgTime)
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

	result := md.GetMessages("nonexistent@s.whatsapp.net")
	if result == nil {
		t.Error("expected empty slice, got nil")
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

	if !md.AddMessage(msg) {
		t.Fatal("AddMessage with special chars failed")
	}

	result := md.GetMessages(chatID)
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
