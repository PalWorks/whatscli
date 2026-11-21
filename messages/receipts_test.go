// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"database/sql"
	"testing"

	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func receipt(t types.ReceiptType, fromMe bool, ids ...string) *events.Receipt {
	evt := &events.Receipt{Type: t}
	evt.IsFromMe = fromMe
	for _, id := range ids {
		evt.MessageIDs = append(evt.MessageIDs, types.MessageID(id))
	}
	return evt
}

func storedStatus(t *testing.T, sm *SessionManager, id string) int {
	t.Helper()
	m, found, err := sm.db.GetMessage(id)
	if err != nil || !found {
		t.Fatalf("message %s: found=%v err=%v", id, found, err)
	}
	return m.Status
}

func TestReceiptsRaiseOwnMessageStatusOnly(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	chat := "111@s.whatsapp.net"
	for _, m := range []Message{
		{Id: "mine", ChatId: chat, FromMe: true, Text: "hi", Status: StatusSent},
		{Id: "theirs", ChatId: chat, Text: "hello"},
	} {
		if _, err := sm.db.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	sm.setCurrentReceiver(chat)

	sm.handleReceipt(receipt(types.ReceiptTypeDelivered, false, "mine", "theirs"))
	if got := storedStatus(t, sm, "mine"); got != StatusDelivered {
		t.Fatalf("after delivery: %d", got)
	}
	if got := storedStatus(t, sm, "theirs"); got != StatusUnknown {
		t.Fatalf("a received message got a status: %d", got)
	}
	if len(ui.Screens) == 0 {
		t.Fatal("open chat not redrawn after a status change")
	}

	sm.handleReceipt(receipt(types.ReceiptTypeRead, false, "mine"))
	if got := storedStatus(t, sm, "mine"); got != StatusRead {
		t.Fatalf("after read: %d", got)
	}
	screens := len(ui.Screens)
	sm.handleReceipt(receipt(types.ReceiptTypeDelivered, false, "mine")) // late delivery receipt
	if got := storedStatus(t, sm, "mine"); got != StatusRead {
		t.Fatalf("status went down to %d", got)
	}
	if len(ui.Screens) != screens {
		t.Error("redrawn although nothing changed")
	}

	// Our own devices' receipts and unrelated types change nothing.
	if _, err := sm.db.AddMessage(Message{Id: "mine2", ChatId: chat, FromMe: true, Text: "x", Status: StatusSent}); err != nil {
		t.Fatal(err)
	}
	sm.handleReceipt(receipt(types.ReceiptTypeReadSelf, true, "mine2"))
	sm.handleReceipt(receipt(types.ReceiptTypeRetry, false, "mine2"))
	if got := storedStatus(t, sm, "mine2"); got != StatusSent {
		t.Fatalf("status changed by an unrelated receipt: %d", got)
	}
}

// A receipt that arrives before the sent message is stored is applied when
// it is stored.
func TestEarlyReceiptAppliedWhenStored(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.handleReceipt(receipt(types.ReceiptTypeDelivered, false, "new"))
	msg := Message{Id: "new", ChatId: "111@s.whatsapp.net", FromMe: true, Text: "hi", Status: StatusSent}
	sm.withEarlyStatus(&msg)
	if msg.Status != StatusDelivered {
		t.Fatalf("status = %d, want delivered", msg.Status)
	}
	again := Message{Id: "new", FromMe: true, Status: StatusSent}
	sm.withEarlyStatus(&again)
	if again.Status != StatusSent {
		t.Fatal("an early receipt must be used once")
	}
}

func TestEarlyReceiptsBounded(t *testing.T) {
	var e earlyReceipts
	for i := 0; i < earlyReceiptLimit+10; i++ {
		e.put(string(rune('a'+i%26))+string(rune(i)), StatusRead)
	}
	if len(e.status) != earlyReceiptLimit || len(e.order) != earlyReceiptLimit {
		t.Fatalf("kept %d/%d, want %d", len(e.status), len(e.order), earlyReceiptLimit)
	}
}

func TestHistoryStatus(t *testing.T) {
	for in, want := range map[waWeb.WebMessageInfo_Status]int{
		waWeb.WebMessageInfo_ERROR:        StatusUnknown,
		waWeb.WebMessageInfo_PENDING:      StatusUnknown,
		waWeb.WebMessageInfo_SERVER_ACK:   StatusSent,
		waWeb.WebMessageInfo_DELIVERY_ACK: StatusDelivered,
		waWeb.WebMessageInfo_READ:         StatusRead,
		waWeb.WebMessageInfo_PLAYED:       StatusRead,
	} {
		if got := historyStatus(in); got != want {
			t.Errorf("historyStatus(%v) = %d, want %d", in, got, want)
		}
	}
}

// A database from before the status column gains it, and its rows read as
// StatusUnknown.
func TestStatusColumnMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE messages (id TEXT PRIMARY KEY, chat_id TEXT, contact_id TEXT, contact_name TEXT,
		contact_short TEXT, timestamp INTEGER, from_me BOOLEAN, forwarded BOOLEAN, text TEXT);
		INSERT INTO messages VALUES ('old', 'c', '', '', '', 1, 1, 0, 'hi');`); err != nil {
		t.Fatal(err)
	}
	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatal(err)
	}
	m, found, err := md.GetMessage("old")
	if err != nil || !found || m.Status != StatusUnknown {
		t.Fatalf("old row: %+v found=%v err=%v", m, found, err)
	}
	if err := md.InitWithDB(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
}
