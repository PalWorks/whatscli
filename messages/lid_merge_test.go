// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"container/heap"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// seedConv stores c in SQLite and in the session manager's chat list.
func seedConv(t *testing.T, sm *SessionManager, c Conversation) {
	t.Helper()
	if err := sm.db.UpsertConversation(c); err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}
	conv := c
	heap.Push(&sm.priorityQueue, &conv)
	sm.convByJID[conv.JID] = &conv
}

func seedMsg(t *testing.T, sm *SessionManager, id, chat string, ts uint64) {
	t.Helper()
	if err := sm.db.AddMessage(Message{Id: id, ChatId: chat, Timestamp: ts, Text: id, ContactId: "111:3@lid"}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
}

func chatIDOf(t *testing.T, sm *SessionManager, id string) string {
	t.Helper()
	m, found, err := sm.db.GetMessage(id)
	if err != nil || !found {
		t.Fatalf("GetMessage(%s): found=%v err=%v", id, found, err)
	}
	return m.ChatId
}

func TestMergeKnownLIDChats_MergesIntoExistingChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	seedConv(t, sm, Conversation{JID: "111@lid", Name: "111", LastMsgTime: 300, Preview: "from lid", Unread: 2, IsPinned: true, IsArchived: true})
	seedConv(t, sm, Conversation{JID: "alice@s.whatsapp.net", Name: "Alice", LastMsgTime: 200, Preview: "from pn", Unread: 1})
	seedMsg(t, sm, "L1", "111@lid", 300)
	seedMsg(t, sm, "P1", "alice@s.whatsapp.net", 200)
	// The same message ID stored first under the phone number: INSERT OR IGNORE
	// keeps the first row, so the @lid copy never exists.
	seedMsg(t, sm, "DUP", "alice@s.whatsapp.net", 250)
	seedMsg(t, sm, "DUP", "111@lid", 250)

	sm.mergeKnownLIDChats()

	for _, id := range []string{"L1", "P1", "DUP"} {
		if got := chatIDOf(t, sm, id); got != "alice@s.whatsapp.net" {
			t.Errorf("message %s chat = %q", id, got)
		}
	}
	if n, _ := sm.db.CountMessages("alice@s.whatsapp.net"); n != 3 {
		t.Errorf("merged chat has %d messages, want 3", n)
	}
	m, _, _ := sm.db.GetMessage("L1")
	if m.ContactId != "111:3@lid" {
		t.Errorf("ContactId changed to %q; it must stay as sent", m.ContactId)
	}
	if _, ok := sm.convByJID["111@lid"]; ok {
		t.Error("@lid chat still in chat list")
	}
	if len(sm.priorityQueue) != 1 {
		t.Errorf("chat list has %d entries, want 1", len(sm.priorityQueue))
	}
	got := *sm.convByJID["alice@s.whatsapp.net"]
	got.Index = 0
	want := Conversation{JID: "alice@s.whatsapp.net", Name: "Alice", LastMsgTime: 300, Preview: "from lid", Unread: 3, IsPinned: true, IsArchived: false}
	if got != want {
		t.Errorf("merged conversation = %+v, want %+v", got, want)
	}
	convs, err := sm.db.GetConversations()
	if err != nil || len(convs) != 1 || convs[0].JID != want.JID || convs[0].Unread != 3 || !convs[0].IsPinned || convs[0].IsArchived {
		t.Errorf("stored conversations = %+v (err %v)", convs, err)
	}
	if len(mock.ChatLists) == 0 {
		t.Error("chat list not refreshed")
	}
}

func TestMergeKnownLIDChats_NoPhoneChatYet(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	seedConv(t, sm, Conversation{JID: "111@lid", Name: "111", LastMsgTime: 300, IsArchived: true})
	seedMsg(t, sm, "L1", "111@lid", 300)

	sm.mergeKnownLIDChats()

	conv := sm.convByJID["alice@s.whatsapp.net"]
	if conv == nil {
		t.Fatal("phone-number chat not created")
	}
	if conv.Name != "alice" || !conv.IsArchived || conv.LastMsgTime != 300 {
		t.Errorf("conversation = %+v", *conv)
	}
	if got := chatIDOf(t, sm, "L1"); got != "alice@s.whatsapp.net" {
		t.Errorf("message chat = %q", got)
	}
}

func TestMergeKnownLIDChats_OpenChatMoves(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	seedConv(t, sm, Conversation{JID: "111@lid", Name: "Alice", LastMsgTime: 300})
	seedMsg(t, sm, "L1", "111@lid", 300)
	sm.currentReceiver = "111@lid"

	sm.mergeKnownLIDChats()

	if sm.currentReceiver != "alice@s.whatsapp.net" {
		t.Errorf("currentReceiver = %q", sm.currentReceiver)
	}
	if len(mock.Screens) == 0 || len(mock.Screens[len(mock.Screens)-1]) != 1 {
		t.Errorf("open chat not redrawn with its messages: %+v", mock.Screens)
	}
}

func TestMergeKnownLIDChats_UnknownMappingNoChange(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{}
	seedConv(t, sm, Conversation{JID: "111@lid", Name: "111", LastMsgTime: 300})
	seedMsg(t, sm, "L1", "111@lid", 300)

	sm.mergeKnownLIDChats()

	if _, ok := sm.convByJID["111@lid"]; !ok {
		t.Error("@lid chat removed without a mapping")
	}
	if got := chatIDOf(t, sm, "L1"); got != "111@lid" {
		t.Errorf("message moved to %q", got)
	}
	if len(mock.Texts) != 0 || len(mock.ChatLists) != 0 {
		t.Errorf("unexpected UI output: texts=%v lists=%d", mock.Texts, len(mock.ChatLists))
	}
}

func TestMergedConversation_Names(t *testing.T) {
	pn := types.JID{User: "919000", Server: types.DefaultUserServer}
	tests := []struct {
		lidName string
		pnConv  *Conversation
		want    string
	}{
		{"111", nil, "919000"},
		{"Alice L", nil, "Alice L"},
		{"Alice L", &Conversation{Name: "Alice P"}, "Alice P"},
		{"Alice L", &Conversation{Name: "919"}, "Alice L"},
		{"111", &Conversation{Name: "919"}, "919"},
	}
	for _, tt := range tests {
		got := mergedConversation(pn, &Conversation{JID: "111@lid", Name: tt.lidName}, tt.pnConv)
		if got.Name != tt.want {
			t.Errorf("lid %q pn %+v: name %q, want %q", tt.lidName, tt.pnConv, got.Name, tt.want)
		}
	}
}
