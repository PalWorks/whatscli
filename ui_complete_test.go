// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/PalWorks/whatscli/messages"
)

func TestCommandHintsCoverRegistry(t *testing.T) {
	hints := map[string]bool{}
	for _, h := range commandHints {
		if hints[h.name] {
			t.Errorf("duplicate hint %q", h.name)
		}
		hints[h.name] = true
	}
	registered := map[string]bool{}
	for _, name := range messages.CommandNames() {
		registered[name] = true
		if !hints[name] {
			t.Errorf("command %q has no hint", name)
		}
	}
	for name := range hints {
		if !registered[name] {
			t.Errorf("hint %q is not a registered command", name)
		}
	}
}

func entryNames(entries []string) []string {
	var names []string
	for _, e := range entries {
		names = append(names, firstWord(e, "/"))
	}
	return names
}

func TestCommandEntries(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"/lo", "login logout"},
		{"/LO", "login logout"},
		{"/re", "read remove removeadmin reset revoke"},
		{"/read", ""}, // fully typed: no list, Enter runs it
		{"/remove", "remove removeadmin"},
		{"/send ", ""}, // arguments started
		{"hello", ""},  // not a command
		{"/zz", ""},    // no match
		{"/", strings.Join(entryNames(commandEntries("/", "/")), " ")},
	}
	for _, tt := range tests {
		got := strings.Join(entryNames(commandEntries(tt.text, "/")), " ")
		if got != tt.want {
			t.Errorf("commandEntries(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
	if n := len(commandEntries("/", "/")); n != len(commandHints) {
		t.Errorf("bare prefix lists %d commands, want %d", n, len(commandHints))
	}
}

func TestCommandEntriesShowUsageAndDescription(t *testing.T) {
	e := commandEntries("/sendi", "/")
	if len(e) != 1 || !strings.Contains(e[0], "/sendimage <path>") || !strings.Contains(e[0], "Send image") {
		t.Errorf("entry %q", e)
	}
}

func TestCompletedText(t *testing.T) {
	if got := completedText("/logout                      Log out and delete data", "/"); got != "/logout" {
		t.Errorf("no-argument command: %q", got)
	}
	if got := completedText("/sendimage <path>            Send image", "/"); got != "/sendimage " {
		t.Errorf("command with arguments: %q", got)
	}
}

func TestCompletingCommand(t *testing.T) {
	for text, want := range map[string]bool{"/": true, "/lo": true, "/send x": false, "hi": false, "": false} {
		if got := completingCommand(text, "/"); got != want {
			t.Errorf("completingCommand(%q) = %v", text, got)
		}
	}
	if completingCommand("/lo", "") {
		t.Error("empty prefix must never complete")
	}
}

func TestCommandEcho(t *testing.T) {
	now := time.Date(2026, 10, 31, 21, 28, 40, 0, time.UTC)
	got := commandEcho("/logout [x]", now)
	if !strings.Contains(got, "(31-10-26 21:28:40) > /logout") {
		t.Errorf("echo %q", got)
	}
	if strings.Contains(got, "[x]") {
		t.Errorf("tview tags in the command must be escaped: %q", got)
	}
}

func TestChatQuery(t *testing.T) {
	tests := []struct {
		text, want string
		ok         bool
	}{
		{"/select ali", "ali", true},
		{"/SELECT Ali ", "Ali", true},
		{"/select ", "", true},
		{"/select", "", false},
		{"/send ali", "", false},
		{"select ali", "", false},
	}
	for _, tt := range tests {
		got, ok := chatQuery(tt.text, "/")
		if got != tt.want || ok != tt.ok {
			t.Errorf("chatQuery(%q) = %q,%v; want %q,%v", tt.text, got, ok, tt.want, tt.ok)
		}
	}
}

func TestMatchChats(t *testing.T) {
	chats := []*messages.Conversation{
		{JID: "status@broadcast", Name: "status"},
		{JID: "919000000001@s.whatsapp.net", Name: "Alagu"},
		{JID: "120363@g.us", Name: "Alagu Family"},
		{JID: "919000000002@s.whatsapp.net", Name: "Bob"},
		{JID: "919000000003@s.whatsapp.net", Name: ""},
	}
	names := func(cs []*messages.Conversation) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.JID)
		}
		return strings.Join(out, " ")
	}
	if got := names(matchChats(chats, "ALA", 10)); got != "919000000001@s.whatsapp.net 120363@g.us" {
		t.Errorf("name match: %s", got)
	}
	if got := names(matchChats(chats, "0003", 10)); got != "919000000003@s.whatsapp.net" {
		t.Errorf("number match: %s", got)
	}
	if got := len(matchChats(chats, "", 10)); got != 4 {
		t.Errorf("empty query: %d chats, want 4 (no status)", got)
	}
	if got := len(matchChats(chats, "", 2)); got != 2 {
		t.Errorf("limit: %d", got)
	}
	if got := matchChats(chats, "zzz", 10); len(got) != 0 {
		t.Errorf("no match: %v", got)
	}
}

func TestChatEntry(t *testing.T) {
	e := chatEntry(&messages.Conversation{JID: "1@g.us", Name: "Family [x]"})
	if !strings.Contains(e, "(group)") || strings.Contains(e, "[x]") {
		t.Errorf("entry %q", e)
	}
	long := chatEntry(&messages.Conversation{JID: "1@s.whatsapp.net", Name: strings.Repeat("n", 50)})
	if !strings.Contains(long, "…") {
		t.Errorf("long name not shortened: %q", long)
	}
}
