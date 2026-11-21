// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestAttachQuote(t *testing.T) {
	reply := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String("kuch nahi"),
		ContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String("Q1"),
			Participant:   proto.String("111@s.whatsapp.net"),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("kya hua?")},
		},
	}}
	var m Message
	attachQuote(&m, reply)
	if m.QuoteId != "Q1" || m.QuoteSender != "111@s.whatsapp.net" || m.QuoteText != "kya hua?" {
		t.Fatalf("got %+v", m)
	}
	img := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{ContextInfo: &waE2E.ContextInfo{
		StanzaID:      proto.String("Q2"),
		QuotedMessage: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("beach")}},
	}}}
	m = Message{}
	attachQuote(&m, img)
	if m.QuoteId != "Q2" || m.QuoteText != "[IMAGE] beach" {
		t.Fatalf("image reply: %+v", m)
	}
	m = Message{}
	attachQuote(&m, &waE2E.Message{Conversation: proto.String("plain")})
	if m.QuoteId != "" {
		t.Fatalf("plain message got a quote: %+v", m)
	}
}

func TestTextMessageAsReply(t *testing.T) {
	if m := textMessage("hi", "", "", nil); m.GetConversation() != "hi" || m.GetExtendedTextMessage() != nil {
		t.Fatalf("plain: %v", m)
	}
	m := textMessage("ok", "Q1", "111@s.whatsapp.net", &waE2E.Message{Conversation: proto.String("kya hua?")})
	ci := m.GetExtendedTextMessage().GetContextInfo()
	if m.GetExtendedTextMessage().GetText() != "ok" || ci.GetStanzaID() != "Q1" || ci.GetParticipant() != "111@s.whatsapp.net" ||
		ci.GetQuotedMessage().GetConversation() != "kya hua?" {
		t.Fatalf("reply: %v", m)
	}
}

func TestQuotedCopyDropsEditedMark(t *testing.T) {
	if got := quotedCopy(Message{Text: "helo (edited)"}).GetConversation(); got != "helo" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelQuote(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	chat := "111@s.whatsapp.net"
	for _, m := range []Message{
		{Id: "mine", ChatId: chat, FromMe: true, Text: "Q beta"},
		{Id: "theirs", ChatId: chat, ContactId: "222@s.whatsapp.net", ContactShort: "Asha", Text: "mera dimag"},
	} {
		if _, err := sm.db.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		msg      Message
		name     string
		filledIn string
	}{
		{Message{QuoteId: "mine"}, QuoteYou, "Q beta"},
		{Message{QuoteId: "theirs", QuoteText: "a changed copy"}, "+222", "mera dimag"}, // ours wins
		{Message{QuoteId: "gone", QuoteSender: "333@s.whatsapp.net", QuoteText: "old"}, "+333", "old"},
		{Message{}, "", ""},
	} {
		m := tt.msg
		sm.labelQuote(&m)
		if m.QuoteName != tt.name || m.QuoteText != tt.filledIn {
			t.Errorf("%+v: name %q text %q, want %q %q", tt.msg, m.QuoteName, m.QuoteText, tt.name, tt.filledIn)
		}
	}
}

// A reply sent offline is kept as failed with its quote, and its fields
// survive the database.
func TestCmdReplyStoresQuote(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	chat := "111@s.whatsapp.net"
	sm.setCurrentReceiver(chat)
	if _, err := sm.db.AddMessage(Message{Id: "orig", ChatId: chat, ContactId: "111@s.whatsapp.net", Text: "kya hua?"}); err != nil {
		t.Fatal(err)
	}
	cmdReply(sm, nil, "reply", []string{chat, "orig", "kuch", "nahi"})
	if len(ui.Messages) != 1 {
		t.Fatalf("reply not shown: %+v", ui.Messages)
	}
	shown := ui.Messages[0]
	if shown.Text != "kuch nahi" || shown.QuoteId != "orig" || shown.QuoteText != "kya hua?" || shown.QuoteName != "+111" || shown.Status != StatusFailed {
		t.Fatalf("shown %+v", shown)
	}
	stored, _, err := sm.db.GetMessage(shown.Id)
	if err != nil || stored.QuoteId != "orig" || stored.QuoteSender != "111@s.whatsapp.net" || stored.QuoteText != "kya hua?" {
		t.Fatalf("stored %+v err %v", stored, err)
	}
	if !strings.Contains(ui.Errors[0].Error(), "not sent") {
		t.Fatalf("errors %v", ui.Errors)
	}
}

func TestCmdReplyChecks(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	chat := "111@s.whatsapp.net"
	if _, err := sm.db.AddMessage(Message{Id: "del", ChatId: chat, Text: RevokedText}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		params []string
		want   string
	}{
		{[]string{chat, "del"}, "Usage:"},
		{[]string{chat, "missing", "hi"}, "not found"},
		{[]string{"222@s.whatsapp.net", "del", "hi"}, "not found"},
		{[]string{chat, "del", "hi"}, "deleted"},
	} {
		ui.Texts, ui.Errors = nil, nil
		cmdReply(sm, nil, "reply", tt.params)
		got := strings.Join(ui.Texts, " ")
		for _, e := range ui.Errors {
			got += " " + e.Error()
		}
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q: got %q, want %q", tt.params, got, tt.want)
		}
	}
	if len(ui.Messages) != 0 {
		t.Fatalf("a refused reply was sent: %+v", ui.Messages)
	}
}
