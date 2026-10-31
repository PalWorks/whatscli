// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"bytes"
	"testing"
	"time"

	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// oldStyleRow is a document message as stored before the media columns
// existed: text only, empty media metadata.
func oldStyleRow(id, chat string) Message {
	return Message{Id: id, ChatId: chat, ContactId: "a@s.whatsapp.net", Timestamp: 100, Text: "[DOCUMENT] report.pdf"}
}

func TestAddMessage_StoreResult(t *testing.T) {
	md := newTestDB(t)
	msg := Message{Id: "R1", ChatId: "c", Timestamp: 1, Text: "first"}

	res, err := md.AddMessage(msg)
	if err != nil || res != StoreInserted || !res.Changed() {
		t.Fatalf("new row: res=%v err=%v", res, err)
	}
	// Same ID again, text only: nothing to fill, first write wins.
	msg.Text = "second"
	res, err = md.AddMessage(msg)
	if err != nil || res != StoreUnchanged || res.Changed() {
		t.Fatalf("duplicate: res=%v err=%v", res, err)
	}
	if got, _, _ := md.GetMessage("R1"); got.Text != "first" {
		t.Errorf("text overwritten: %q", got.Text)
	}
}

func TestAddMessage_FillsMediaOnlyWhenEmpty(t *testing.T) {
	md := newTestDB(t)
	if _, err := md.AddMessage(oldStyleRow("F1", "c@s.whatsapp.net")); err != nil {
		t.Fatal(err)
	}

	withMedia := docMsg(t, "F1", "report.pdf")
	withMedia.Text = "[DOCUMENT] changed text"
	withMedia.Timestamp = 999
	res, err := md.AddMessage(withMedia)
	if err != nil || res != StoreMediaFilled {
		t.Fatalf("fill: res=%v err=%v", res, err)
	}
	got, _, _ := md.GetMessage("F1")
	if got.Text != "[DOCUMENT] report.pdf" || got.Timestamp != 100 {
		t.Errorf("fill changed text or timestamp: %+v", got)
	}
	if got.MediaKind != MediaKindDocument || got.FileName != "report.pdf" || !bytes.Equal(got.MediaProto, withMedia.MediaProto) {
		t.Errorf("media not filled: kind=%q name=%q", got.MediaKind, got.FileName)
	}
	if _, err := downloadableFromMessage(got); err != nil {
		t.Errorf("filled row not downloadable: %v", err)
	}

	// A second write with other media never replaces what is stored.
	other := docMsg(t, "F1", "other.pdf")
	res, err = md.AddMessage(other)
	if err != nil || res != StoreUnchanged {
		t.Fatalf("refill: res=%v err=%v", res, err)
	}
	if got, _, _ := md.GetMessage("F1"); got.FileName != "report.pdf" {
		t.Errorf("stored media replaced: %q", got.FileName)
	}
}

func TestAddMessage_RevokedRowUntouched(t *testing.T) {
	md := newTestDB(t)
	if _, err := md.AddMessage(oldStyleRow("V1", "c@s.whatsapp.net")); err != nil {
		t.Fatal(err)
	}
	if ok, err := md.MarkMessageRevoked("c@s.whatsapp.net", "V1"); err != nil || !ok {
		t.Fatalf("revoke: ok=%v err=%v", ok, err)
	}
	res, err := md.AddMessage(docMsg(t, "V1", "report.pdf"))
	if err != nil || res != StoreUnchanged {
		t.Fatalf("revoked row: res=%v err=%v", res, err)
	}
	got, _, _ := md.GetMessage("V1")
	if got.Text != RevokedText || got.MediaKind != "" || len(got.MediaProto) != 0 {
		t.Errorf("revoked row changed: %+v", got)
	}
}

func TestAddMessage_FillScopedToChat(t *testing.T) {
	md := newTestDB(t)
	if _, err := md.AddMessage(oldStyleRow("X1", "other@s.whatsapp.net")); err != nil {
		t.Fatal(err)
	}
	// docMsg uses chat c@s.whatsapp.net: same ID in another chat must not fill.
	res, err := md.AddMessage(docMsg(t, "X1", "evil.pdf"))
	if err != nil || res != StoreUnchanged {
		t.Fatalf("cross-chat fill: res=%v err=%v", res, err)
	}
	if got, _, _ := md.GetMessage("X1"); got.MediaKind != "" {
		t.Errorf("row in another chat gained media: %+v", got)
	}
}

func TestStoreHistorySync_RedrawsOnlyWhenChanged(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.currentReceiver = "bob@s.whatsapp.net"
	sync := func() {
		sm.storeHistorySync(&waHistorySync.HistorySync{
			SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
			Conversations: []*waHistorySync.Conversation{historyConv("bob@s.whatsapp.net", "", "H1", "hi")},
		}, testParser)
	}
	sync()
	if len(mock.Screens) != 1 {
		t.Fatalf("first chunk: %d redraws, want 1", len(mock.Screens))
	}
	sync()
	if len(mock.Screens) != 1 {
		t.Errorf("repeat chunk redrew the open chat: %d redraws", len(mock.Screens))
	}
}

func TestLiveMessage_DuplicateAndFill(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}
	chat := types.NewJID("bob", types.DefaultUserServer)
	src := types.MessageSource{Chat: chat, Sender: chat}
	sm.currentReceiver = chat.String()

	storeText(eh, "L1", src, "hello")
	storeText(eh, "L1", src, "hello")
	if len(mock.Messages) != 1 {
		t.Errorf("duplicate delivery shown %d times", len(mock.Messages))
	}
	if len(mock.Screens) != 0 {
		t.Errorf("duplicate delivery redrew: %d", len(mock.Screens))
	}
	if c := sm.convByJID[chat.String()]; c == nil || c.Unread != 1 {
		t.Errorf("duplicate delivery changed unread: %+v", c)
	}

	// A row stored without media, then the same ID with media: redraw once,
	// do not append it again.
	if _, err := sm.db.AddMessage(oldStyleRow("L2", chat.String())); err != nil {
		t.Fatal(err)
	}
	wa := sampleDocMessage()
	eh.handleMessage(&events.Message{
		Info:    types.MessageInfo{ID: "L2", MessageSource: src, Timestamp: time.Unix(100, 0)},
		Message: wa,
	})
	if len(mock.Messages) != 1 || len(mock.Screens) != 1 {
		t.Errorf("media fill: messages=%d screens=%d, want 1 and 1", len(mock.Messages), len(mock.Screens))
	}
	if got, _, _ := sm.db.GetMessage("L2"); got.MediaKind != MediaKindDocument {
		t.Errorf("live fill did not store media: %+v", got)
	}
}
