package messages

import (
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestCanonicalChatForSource(t *testing.T) {
	mapping := fakeLIDs{"111": "alice"}
	tests := []struct {
		name    string
		mapping lidMapper
		src     types.MessageSource
		want    string
	}{
		{"PN chat unchanged", nil, types.MessageSource{Chat: pnJID("bob"), Sender: pnJID("bob")}, "bob@s.whatsapp.net"},
		{"group unchanged", mapping, types.MessageSource{Chat: grpJID("g"), Sender: lidJID("111")}, "g@g.us"},
		{"LID chat, no mapping, kept", nil, types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")}, "111@lid"},
		{"LID chat, store mapping", mapping, types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")}, "alice@s.whatsapp.net"},
		{"LID chat, SenderAlt PN", nil,
			types.MessageSource{Chat: lidJID("111"), Sender: types.JID{User: "111", Device: 2, Server: types.HiddenUserServer},
				SenderAlt: types.JID{User: "alice", Device: 2, Server: types.DefaultUserServer}}, "alice@s.whatsapp.net"},
		{"own message to LID chat, RecipientAlt PN", nil,
			types.MessageSource{Chat: lidJID("111"), Sender: pnJID("me"), IsFromMe: true, RecipientAlt: pnJID("alice")}, "alice@s.whatsapp.net"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, _ := newTestSessionManager(t)
			sm.lidMapperOverride = tt.mapping
			if got := sm.canonicalChatForSource(tt.src).String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSameUser(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	tests := []struct {
		a, b types.JID
		want bool
	}{
		{pnJID("alice"), pnJID("alice"), true},
		{pnJID("alice"), types.JID{User: "alice", Device: 4, Server: types.DefaultUserServer}, true},
		{pnJID("alice"), lidJID("111"), true},
		{lidJID("111"), pnJID("alice"), true},
		{pnJID("alice"), pnJID("bob"), false},
		{lidJID("222"), pnJID("alice"), false},
		{types.EmptyJID, types.EmptyJID, false},
	}
	for _, tt := range tests {
		if got := sm.sameUser(tt.a, tt.b); got != tt.want {
			t.Errorf("sameUser(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// historyConv builds a history sync conversation with one incoming text message.
func historyConv(id, pnJID, msgID, text string) *waHistorySync.Conversation {
	return &waHistorySync.Conversation{
		ID:    proto.String(id),
		PnJID: optString(pnJID),
		Name:  proto.String("Alice"),
		Messages: []*waHistorySync.HistorySyncMsg{{
			Message: &waWeb.WebMessageInfo{
				Key:              &waCommon.MessageKey{RemoteJID: proto.String(id), FromMe: proto.Bool(false), ID: proto.String(msgID)},
				MessageTimestamp: proto.Uint64(500),
				Message:          &waE2E.Message{Conversation: proto.String(text)},
			},
		}},
	}
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// A zero whatsmeow.Client parses incoming (not from-me) web messages without
// any connection, which is all storeHistorySync needs here.
var testParser webMessageParser = (&whatsmeow.Client{}).ParseWebMessage

func TestStoreHistorySync_LIDConversations(t *testing.T) {
	tests := []struct {
		name     string
		mapping  lidMapper
		conv     *waHistorySync.Conversation
		wantChat string
	}{
		{"LID conversation with PnJID", nil, historyConv("111@lid", "alice@s.whatsapp.net", "H1", "hi"), "alice@s.whatsapp.net"},
		{"LID conversation, store mapping", fakeLIDs{"111": "alice"}, historyConv("111@lid", "", "H1", "hi"), "alice@s.whatsapp.net"},
		{"LID conversation, no mapping kept", nil, historyConv("111@lid", "", "H1", "hi"), "111@lid"},
		{"PN conversation unchanged", fakeLIDs{"111": "alice"}, historyConv("bob@s.whatsapp.net", "", "H1", "hi"), "bob@s.whatsapp.net"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, _ := newTestSessionManager(t)
			sm.lidMapperOverride = tt.mapping
			sm.storeHistorySync(&waHistorySync.HistorySync{
				SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
				Conversations: []*waHistorySync.Conversation{tt.conv},
			}, testParser)
			msg, found, err := sm.db.GetMessage("H1")
			if err != nil || !found {
				t.Fatalf("history message not stored: found=%v err=%v", found, err)
			}
			if msg.ChatId != tt.wantChat {
				t.Errorf("stored under %q, want %q", msg.ChatId, tt.wantChat)
			}
			if sm.convByJID[tt.wantChat] == nil {
				t.Errorf("conversation %q not created", tt.wantChat)
			}
			if len(sm.convByJID) != 1 {
				t.Errorf("expected exactly one conversation, got %d", len(sm.convByJID))
			}
		})
	}
}

func TestStoreHistorySync_LiveAndHistoryAgree(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	eh := &eventHandler{sm: sm}
	storeText(eh, "LIVE1", types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")}, "live")
	sm.storeHistorySync(&waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{historyConv("111@lid", "", "OLD1", "old")},
	}, testParser)
	n, err := sm.db.CountMessages("alice@s.whatsapp.net")
	if err != nil || n != 2 {
		t.Fatalf("expected live and history in one chat, n=%d err=%v", n, err)
	}
	if len(sm.convByJID) != 1 {
		t.Errorf("one person shows as %d chats", len(sm.convByJID))
	}
}

func TestStoreHistorySync_SkipsStatusBroadcast(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.storeHistorySync(&waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{historyConv("status@broadcast", "", "S1", "x")},
	}, testParser)
	if _, found, _ := sm.db.GetMessage("S1"); found {
		t.Error("status broadcast stored")
	}
}

func TestBacklogTarget(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	if got, moved := sm.backlogTarget("111@lid"); !moved || got != "alice@s.whatsapp.net" {
		t.Errorf("backlogTarget(111@lid) = %q,%v", got, moved)
	}
	if _, moved := sm.backlogTarget("222@lid"); moved {
		t.Error("unmapped LID chat must stay the backlog target")
	}
	if _, moved := sm.backlogTarget("g@g.us"); moved {
		t.Error("group must stay the backlog target")
	}
	if _, moved := sm.backlogTarget("bob@s.whatsapp.net"); moved {
		t.Error("PN chat must stay the backlog target")
	}
}
