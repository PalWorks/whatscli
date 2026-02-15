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
