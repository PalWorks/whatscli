// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"google.golang.org/protobuf/proto"
)

func TestStoreHistorySync_SkipsEmptyChats(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.storeHistorySync(&waHistorySync.HistorySync{
		SyncType: waHistorySync.HistorySync_RECENT.Enum(),
		Conversations: []*waHistorySync.Conversation{
			{ID: proto.String("empty@s.whatsapp.net")},
			{ID: proto.String("111@lid")},
			{ID: proto.String("quiet@g.us"), Name: proto.String("Quiet group")},
			{ID: proto.String("dated@s.whatsapp.net"), ConversationTimestamp: proto.Uint64(400)},
			{ID: proto.String("unread@s.whatsapp.net"), UnreadCount: proto.Uint32(1)},
			{ID: proto.String("pinned@s.whatsapp.net"), Pinned: proto.Uint32(1)},
			historyConv("bob@s.whatsapp.net", "", "H1", "hi"),
		},
	}, testParser)

	for _, jid := range []string{"empty@s.whatsapp.net", "111@lid"} {
		if sm.convByJID[jid] != nil {
			t.Errorf("empty chat %s was created", jid)
		}
	}
	for _, jid := range []string{"quiet@g.us", "dated@s.whatsapp.net", "unread@s.whatsapp.net", "pinned@s.whatsapp.net", "bob@s.whatsapp.net"} {
		if sm.convByJID[jid] == nil {
			t.Errorf("chat %s missing", jid)
		}
	}
	convs, err := sm.db.GetConversations()
	if err != nil || len(convs) != 5 {
		t.Errorf("stored %d conversations, want 5 (err %v)", len(convs), err)
	}
}

func TestDeleteEmptyConversations(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	for _, c := range []Conversation{
		{JID: "empty@s.whatsapp.net", Preview: "New chat"},
		{JID: "111@lid", Name: "111", Preview: "New chat"},
		{JID: "quiet@g.us", Preview: "Group synced"},
		{JID: "dated@s.whatsapp.net", LastMsgTime: 400},
		{JID: "unread@s.whatsapp.net", Unread: 1},
		{JID: "pinned@s.whatsapp.net", IsPinned: true},
		{JID: "withmsg@s.whatsapp.net", Preview: "New chat"},
	} {
		if err := sm.db.UpsertConversation(c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sm.db.AddMessage(Message{Id: "M1", ChatId: "withmsg@s.whatsapp.net", Text: "x"}); err != nil {
		t.Fatal(err)
	}

	n, err := sm.db.DeleteEmptyConversations()
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, err %v; want 2", n, err)
	}
	convs, _ := sm.db.GetConversations()
	left := map[string]bool{}
	for _, c := range convs {
		left[c.JID] = true
	}
	for _, jid := range []string{"empty@s.whatsapp.net", "111@lid"} {
		if left[jid] {
			t.Errorf("%s not deleted", jid)
		}
	}
	if len(left) != 5 {
		t.Errorf("kept %v, want 5 rows", left)
	}
	if n, _ := sm.db.DeleteEmptyConversations(); n != 0 {
		t.Errorf("second run deleted %d, want 0", n)
	}
}
