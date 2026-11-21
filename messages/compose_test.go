// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestChatActivityText(t *testing.T) {
	for _, tt := range []struct {
		state  types.ChatPresence
		media  types.ChatPresenceMedia
		group  bool
		sender string
		want   string
	}{
		{types.ChatPresenceComposing, types.ChatPresenceMediaText, false, "", "typing…"},
		{types.ChatPresenceComposing, types.ChatPresenceMediaAudio, false, "", "recording audio…"},
		{types.ChatPresenceComposing, types.ChatPresenceMediaText, true, "Asha", "Asha is typing…"},
		{types.ChatPresencePaused, types.ChatPresenceMediaText, true, "Asha", ""},
	} {
		if got := chatActivityText(tt.state, tt.media, tt.group, tt.sender); got != tt.want {
			t.Errorf("%+v: got %q, want %q", tt, got, tt.want)
		}
	}
}

func TestPresenceText(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.Local)
	for _, tt := range []struct {
		unavailable bool
		seen        time.Time
		want        string
	}{
		{false, time.Time{}, "online"},
		{true, time.Time{}, ""},
		{true, now.Add(-time.Hour), "last seen today at 11:00"},
		{true, time.Date(2026, 11, 20, 22, 5, 0, 0, time.Local), "last seen yesterday at 22:05"},
		{true, time.Date(2026, 11, 2, 9, 30, 0, 0, time.Local), "last seen 2 Nov 09:30"},
	} {
		if got := presenceText(tt.unavailable, tt.seen, now); got != tt.want {
			t.Errorf("%+v: got %q, want %q", tt, got, tt.want)
		}
	}
}

func TestPresenceEventsReachUI(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	chat := types.NewJID("111", types.DefaultUserServer)
	evt := &events.ChatPresence{State: types.ChatPresenceComposing}
	evt.Chat, evt.Sender = chat, chat
	sm.handleChatPresence(evt)
	mine := &events.ChatPresence{State: types.ChatPresenceComposing}
	mine.Chat, mine.IsFromMe = chat, true
	sm.handleChatPresence(mine) // our own typing on another device: ignored
	if len(ui.Activity) != 1 || ui.Activity[0] != "111@s.whatsapp.net|typing…" {
		t.Fatalf("activity = %q", ui.Activity)
	}
	sm.handlePresence(&events.Presence{From: chat})
	if len(ui.Presence) != 1 || ui.Presence[0] != "111@s.whatsapp.net|online" {
		t.Fatalf("presence = %q", ui.Presence)
	}
}

// Offline, a message is kept and shown, marked as not sent, with a hint
// how to send it again.
func TestSendTextOfflineKeepsFailedMessage(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	chat := "111@s.whatsapp.net"
	sm.setCurrentReceiver(chat)
	sm.sendText(chat, "are you there?")
	if len(ui.Messages) != 1 {
		t.Fatalf("message not shown: %+v", ui.Messages)
	}
	m := ui.Messages[0]
	if m.Status != StatusFailed || m.Text != "are you there?" || m.Id == "" {
		t.Fatalf("shown %+v", m)
	}
	stored, found, err := sm.db.GetMessage(m.Id)
	if err != nil || !found || stored.Status != StatusFailed {
		t.Fatalf("stored %+v found=%v err=%v", stored, found, err)
	}
	if len(ui.Errors) == 0 || !strings.Contains(ui.Errors[0].Error(), "try again") {
		t.Fatalf("errors = %v", ui.Errors)
	}
	// A retry while still offline says so and leaves it failed.
	cmdRetry(sm, nil, "retry", []string{m.Id})
	if last := ui.Errors[len(ui.Errors)-1].Error(); !strings.Contains(last, "not connected") {
		t.Fatalf("retry offline: %q", last)
	}
}

func TestRetryOnlyFailedOwnTextMessages(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	for _, m := range []Message{
		{Id: "sent", ChatId: "c@s.whatsapp.net", FromMe: true, Text: "ok", Status: StatusSent},
		{Id: "theirs", ChatId: "c@s.whatsapp.net", Text: "hi"},
		{Id: "media", ChatId: "c@s.whatsapp.net", FromMe: true, Text: "[IMAGE]", MediaKind: "image", Status: StatusFailed},
	} {
		if _, err := sm.db.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"sent", "theirs", "missing", "media"} {
		n := len(ui.Errors)
		cmdRetry(sm, nil, "retry", []string{id})
		if len(ui.Errors) != n+1 {
			t.Errorf("%s: want an error", id)
		}
	}
}

func TestEditable(t *testing.T) {
	now := time.Now()
	ts := func(ago time.Duration) uint64 { return uint64(now.Add(-ago).Unix()) }
	ok := Message{FromMe: true, Text: "x", Timestamp: ts(time.Minute), Status: StatusSent}
	if err := Editable(ok, now); err != nil {
		t.Fatalf("recent own message: %v", err)
	}
	for name, m := range map[string]Message{
		"theirs":  {Text: "x", Timestamp: ts(time.Minute)},
		"media":   {FromMe: true, Text: "[IMAGE]", MediaKind: "image", Timestamp: ts(time.Minute)},
		"revoked": {FromMe: true, Text: RevokedText, Timestamp: ts(time.Minute)},
		"failed":  {FromMe: true, Text: "x", Status: StatusFailed, Timestamp: ts(time.Minute)},
		"too old": {FromMe: true, Text: "x", Timestamp: ts(EditWindow + time.Minute)},
	} {
		if Editable(m, now) == nil {
			t.Errorf("%s: should not be editable", name)
		}
	}
}

func TestCmdEditChecks(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	chat := "c@s.whatsapp.net"
	if _, err := sm.db.AddMessage(Message{Id: "m", ChatId: chat, FromMe: true, Text: "helo", Timestamp: uint64(time.Now().Unix()), Status: StatusSent}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		params []string
		want   string
	}{
		{[]string{chat, "m"}, "Usage:"},
		{[]string{"other@s.whatsapp.net", "m", "hello"}, "not found"},
		{[]string{chat, "m", "hello"}, "not connected"}, // valid, but no client
	} {
		cmdEdit(sm, nil, "edit", tt.params)
		got := ""
		if len(ui.Errors) > 0 {
			got = ui.Errors[len(ui.Errors)-1].Error()
		}
		if len(ui.Texts) > 0 && strings.Contains(ui.Texts[len(ui.Texts)-1], "Usage:") {
			got = ui.Texts[len(ui.Texts)-1]
		}
		if !strings.Contains(got, tt.want) {
			t.Errorf("%q: got %q, want %q", tt.params, got, tt.want)
		}
		ui.Texts, ui.Errors = nil, nil
	}
	if m, _, _ := sm.db.GetMessage("m"); m.Text != "helo" {
		t.Fatalf("text changed without a send: %q", m.Text)
	}
}
