// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_emoji.go: emoji suggestions in the input line. Typing ":" and at least
// two letters of a name (":smil") shows matching emoji in the drop-down;
// Enter or Tab puts the chosen one in place of the typed name.

import (
	"sort"
	"strings"
	"sync"

	"github.com/kyokomi/emoji/v2"
)

// emojiLimit caps the emoji drop-down.
const emojiLimit = 10

var (
	emojiOnce  sync.Once
	emojiNames []string          // ":name:" sorted shortest first, then by name
	emojiByKey map[string]string // ":name:" -> emoji (skin tone variants are not offered)
)

func loadEmoji() {
	emojiByKey = emoji.CodeMap()
	emojiNames = make([]string, 0, len(emojiByKey))
	for name := range emojiByKey {
		emojiNames = append(emojiNames, name)
	}
	sort.Slice(emojiNames, func(i, j int) bool {
		if len(emojiNames[i]) != len(emojiNames[j]) {
			return len(emojiNames[i]) < len(emojiNames[j])
		}
		return emojiNames[i] < emojiNames[j]
	})
}

// emojiQuery returns the name being typed when the last word of text is ":"
// followed by at least two name characters (letters, digits, _ + -).
func emojiQuery(text string) (string, bool) {
	word := text[strings.LastIndexAny(text, " \t")+1:]
	if !strings.HasPrefix(word, ":") || len(word) < 3 {
		return "", false
	}
	q := strings.ToLower(word[1:])
	for _, r := range q {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '+' || r == '-') {
			return "", false
		}
	}
	return q, true
}

// emojiEntries lists up to emojiLimit emoji whose name contains query, names
// starting with it first, one line per emoji: "<emoji> :<name>:".
func emojiEntries(query string) []string {
	emojiOnce.Do(loadEmoji)
	var prefix, contains []string
	seen := map[string]bool{}
	for _, name := range emojiNames {
		e := emojiByKey[name]
		// Only the default look of each emoji: no skin tone variants, and
		// one entry for an emoji listed with and without its style
		// marker (U+FE0F), such as ☝️ and ☝.
		key := strings.ReplaceAll(e, "\uFE0F", "")
		if seen[key] || hasSkinTone(e) {
			continue
		}
		switch {
		case strings.HasPrefix(name, ":"+query):
			prefix = append(prefix, e+" "+name)
		case strings.Contains(name, query):
			contains = append(contains, e+" "+name)
		default:
			continue
		}
		seen[key] = true
	}
	entries := append(prefix, contains...)
	if len(entries) > emojiLimit {
		entries = entries[:emojiLimit]
	}
	return entries
}

// hasSkinTone reports whether e contains an emoji skin tone modifier
// (U+1F3FB to U+1F3FF).
func hasSkinTone(e string) bool {
	return strings.ContainsFunc(e, func(r rune) bool { return r >= 0x1F3FB && r <= 0x1F3FF })
}

// applyEmoji replaces the ":name" being typed at the end of text with the
// emoji of the chosen drop-down entry, followed by a space.
func applyEmoji(text, entry string) string {
	e, _, _ := strings.Cut(entry, " ")
	return text[:strings.LastIndexAny(text, " \t")+1] + e + " "
}
