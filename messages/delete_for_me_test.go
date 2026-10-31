// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"container/heap"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// seedChat stores a chat with messages and lists it in the priority queue.
func seedChat(t *testing.T, sm *SessionManager, jid string, msgs ...Message) {
	t.Helper()
	last := msgs[len(msgs)-1]
	conv := &Conversation{JID: jid, Name: "n", LastMsgTime: int64(last.Timestamp), Preview: last.Text}
	for _, m := range msgs {
		m.ChatId = jid
		if _, err := sm.db.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	heap.Push(&sm.priorityQueue, conv)
	sm.convByJID[jid] = conv
	if err := sm.db.UpsertConversation(*conv); err != nil {
		t.Fatal(err)
	}
}

func deleteForMe(chat types.JID, id string) *events.DeleteForMe {
	return &events.DeleteForMe{ChatJID: chat, MessageID: id, Timestamp: time.Unix(1000, 0)}
}

func TestDeleteForMe_RemovesMessage(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	seedChat(t, sm, "bob@s.whatsapp.net",
		Message{Id: "A", Timestamp: 10, Text: "one"},
		Message{Id: "B", Timestamp: 20, Text: "two"})
	(&eventHandler{sm: sm}).Handle(deleteForMe(pnJID("bob"), "A"))

	if _, found, _ := sm.db.GetMessage("A"); found {
		t.Error("deleted message still stored")
	}
	if _, found, _ := sm.db.GetMessage("B"); !found {
		t.Error("other message was removed")
	}
	if got := sm.convByJID["bob@s.whatsapp.net"]; got.Preview != "two" || got.LastMsgTime != 20 {
		t.Errorf("preview changed for an older message delete: %+v", got)
	}
}

func TestDeleteForMe_LatestUpdatesPreviewAndRedraws(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	seedChat(t, sm, "bob@s.whatsapp.net",
		Message{Id: "A", Timestamp: 10, Text: "one"},
		Message{Id: "B", Timestamp: 20, Text: "two"})
	sm.currentReceiver = "bob@s.whatsapp.net"
	sm.handleDeleteForMe(deleteForMe(pnJID("bob"), "B"))

	conv := sm.convByJID["bob@s.whatsapp.net"]
	if conv.Preview != "one" || conv.LastMsgTime != 10 {
		t.Errorf("conv = %+v, want preview one, time 10", conv)
	}
	convs, _ := sm.db.GetConversations()
	if len(convs) != 1 || convs[0].Preview != "one" || convs[0].LastMsgTime != 10 {
		t.Errorf("stored conv = %+v", convs)
	}
	if len(mock.ChatLists) == 0 {
		t.Error("chat list not refreshed")
	}
	if len(mock.Screens) != 1 || len(mock.Screens[0]) != 1 || mock.Screens[0][0].Id != "A" {
		t.Errorf("open chat not redrawn correctly: %+v", mock.Screens)
	}
}

func TestDeleteForMe_NotCurrentChatNoRedraw(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	seedChat(t, sm, "bob@s.whatsapp.net", Message{Id: "A", Timestamp: 10, Text: "one"})
	sm.currentReceiver = "other@s.whatsapp.net"
	sm.handleDeleteForMe(deleteForMe(pnJID("bob"), "A"))
	if len(mock.Screens) != 0 {
		t.Error("redrew a chat that is not open")
	}
}

func TestDeleteForMe_LastMessageKeepsChat(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	seedChat(t, sm, "bob@s.whatsapp.net", Message{Id: "A", Timestamp: 10, Text: "one"})
	sm.handleDeleteForMe(deleteForMe(pnJID("bob"), "A"))
	conv := sm.convByJID["bob@s.whatsapp.net"]
	if conv == nil || conv.Preview != emptyChatPreview || conv.LastMsgTime != 10 {
		t.Errorf("conv = %+v, want kept with empty preview", conv)
	}
}

func TestDeleteForMe_LIDChatMapsToStoredPN(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	seedChat(t, sm, "alice@s.whatsapp.net",
		Message{Id: "A", Timestamp: 10, Text: "one"},
		Message{Id: "B", Timestamp: 20, Text: "two"})
	sm.handleDeleteForMe(deleteForMe(lidJID("111"), "B"))
	if _, found, _ := sm.db.GetMessage("B"); found {
		t.Error("message not deleted through the LID form of the chat")
	}
	if got := sm.convByJID["alice@s.whatsapp.net"].Preview; got != "one" {
		t.Errorf("preview = %q, want one", got)
	}
}

func TestDeleteForMe_WrongChatOrUnknownIgnored(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	seedChat(t, sm, "bob@s.whatsapp.net", Message{Id: "A", Timestamp: 10, Text: "one"})
	sm.handleDeleteForMe(deleteForMe(pnJID("carol"), "A"))           // other chat
	sm.handleDeleteForMe(deleteForMe(pnJID("bob"), "NOPE"))          // unknown id
	sm.handleDeleteForMe(deleteForMe(grpJID("g"), "A"))              // group vs DM
	sm.handleDeleteForMe(&events.DeleteForMe{ChatJID: pnJID("bob")}) // no id
	if _, found, _ := sm.db.GetMessage("A"); !found {
		t.Error("message deleted by an event for another chat")
	}
	if len(mock.Errors) != 0 {
		t.Errorf("unexpected errors: %v", mock.Errors)
	}
}
