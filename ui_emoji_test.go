// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestEmojiQuery(t *testing.T) {
	tests := []struct {
		text  string
		query string
		ok    bool
	}{
		{":sm", "sm", true},
		{"hello :thumbs", "thumbs", true},
		{"ok :+1", "+1", true},
		{":s", "", false},               // too short to be worth a list
		{"see you at 10:30", "", false}, // a time, not an emoji
		{"http://x", "", false},
		{"hi :smile ", "", false}, // finished word
		{"smile", "", false},
		{":)", "", false},
	}
	for _, tt := range tests {
		q, ok := emojiQuery(tt.text)
		if q != tt.query || ok != tt.ok {
			t.Errorf("emojiQuery(%q) = %q, %v; want %q, %v", tt.text, q, ok, tt.query, tt.ok)
		}
	}
}

func TestEmojiEntriesPrefixFirstAndLimited(t *testing.T) {
	e := emojiEntries("smil")
	if len(e) == 0 || len(e) > emojiLimit {
		t.Fatalf("got %d entries, want 1..%d", len(e), emojiLimit)
	}
	if !strings.Contains(e[0], " :smil") {
		t.Errorf("first entry %q should be a :smil... prefix match", e[0])
	}
	for _, entry := range e {
		if !strings.Contains(entry, "smil") {
			t.Errorf("entry %q does not match the query", entry)
		}
	}
	if got := emojiEntries("zzzznotanemoji"); len(got) != 0 {
		t.Errorf("no match should give no entries, got %q", got)
	}
}

func TestApplyEmoji(t *testing.T) {
	if got := applyEmoji("hello :smi", "😄 :smile:"); got != "hello 😄 " {
		t.Errorf("applyEmoji = %q", got)
	}
	if got := applyEmoji(":+", "👍 :+1:"); got != "👍 " {
		t.Errorf("applyEmoji at start = %q", got)
	}
}

// Typing ":smil" in a message shows emoji; Enter puts the chosen one in the
// message instead of sending it.
func TestTypingColonNameInsertsEmoji(t *testing.T) {
	app, input, _, screen := runInputApp(t)
	typeKeys(app, input, "hi :smile")
	if !waitFor(app, func() bool { return screenContains(screen, ":smile:") }) {
		t.Fatalf("emoji list not shown; screen:\n%s", screenText(screen))
	}
	typeKeys(app, input, tcell.KeyEnter)
	if !waitFor(app, func() bool { return input.GetText() == "hi 😄 " }) {
		t.Fatalf("input = %q, want %q", input.GetText(), "hi 😄 ")
	}
}
