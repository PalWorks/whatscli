// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_complete.go: typing help on the input line. Commands typed with the
// prefix are echoed into the message view with a timestamp, and while the
// command name is being typed a drop-down lists the matching commands; Tab or
// Enter picks one.

import (
	"fmt"
	"strings"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/rivo/tview"
)

// commandHint describes one command for the drop-down.
type commandHint struct {
	name string // without the prefix
	args string // argument placeholder, empty when the command takes none
	desc string
}

// commandHints lists every command in messages.CommandNames (aliases
// included); TestCommandHintsCoverRegistry keeps the two in sync.
var commandHints = []commandHint{
	{"add", "<userid>", "Add to group"},
	{"admin", "<userid>", "Promote to admin"},
	{"backlog", "", "Request older messages from the phone"},
	{"colorlist", "", "Show colour names"},
	{"connect", "", "(Re)connect"},
	{"create", "<nums> <subject>", "Create group"},
	{"demote", "<userid>", "Remove admin"},
	{"disconnect", "", "Disconnect"},
	{"download", "<msg-id>", "Download media"},
	{"help", "", "Help"},
	{"info", "", "Chat info"},
	{"leave", "", "Leave group"},
	{"login", "", "(Re)connect"},
	{"logout", "", "Log out and delete data"},
	{"more", "", "Show older messages"},
	{"open", "<msg-id>", "Download and open"},
	{"promote", "<userid>", "Promote to admin"},
	{"quit", "", "Exit"},
	{"read", "", "Mark chat read"},
	{"remove", "<userid>", "Remove from group"},
	{"removeadmin", "<userid>", "Remove admin"},
	{"reset", "", "Delete session and messages"},
	{"revoke", "<msg-id>", "Delete own message for everyone"},
	{"search", "<keyword>", "Search messages"},
	{"search-contact", "<name>", "Find contacts"},
	{"select", "<chat-id>", "Open chat"},
	{"send", "<chat-id> <text>", "Send text to a chat"},
	{"sendaudio", "<path>", "Send audio"},
	{"sendimage", "<path>", "Send image"},
	{"sendvideo", "<path>", "Send video"},
	{"show", "<msg-id>", "Show image"},
	{"subject", "<subject>", "Set group subject"},
	{"sync-groups", "", "Refresh group list"},
	{"upload", "<path>", "Send file"},
	{"url", "<msg-id>", "Open link"},
}

// completingCommand reports whether text is a command name still being typed:
// the prefix and no space yet.
func completingCommand(text, prefix string) bool {
	return prefix != "" && strings.HasPrefix(text, prefix) && !strings.ContainsAny(text, " \t")
}

// commandEntries returns the drop-down lines for text, or nil when text is
// not a command name being typed or nothing matches.
func commandEntries(text, prefix string) []string {
	if !completingCommand(text, prefix) {
		return nil
	}
	typed := strings.ToLower(strings.TrimPrefix(text, prefix))
	var entries []string
	for _, h := range commandHints {
		if !strings.HasPrefix(h.name, typed) {
			continue
		}
		usage := prefix + h.name
		if h.args != "" {
			usage += " " + h.args
		}
		entries = append(entries, tview.Escape(fmt.Sprintf("%-28s %s", usage, h.desc)))
	}
	if len(entries) == 1 && strings.EqualFold(typed, firstWord(entries[0], prefix)) {
		return nil // fully typed already; Enter runs it
	}
	return entries
}

// firstWord returns the command name of a drop-down line, without prefix.
func firstWord(entry, prefix string) string {
	f := strings.Fields(entry)
	if len(f) == 0 {
		return ""
	}
	return strings.TrimPrefix(f[0], prefix)
}

// completedText is the input text after choosing entry: the command, plus a
// space when it takes arguments.
func completedText(entry, prefix string) string {
	name := firstWord(entry, prefix)
	for _, h := range commandHints {
		if h.name == name && h.args != "" {
			return prefix + name + " "
		}
	}
	return prefix + name
}

// commandEcho is the line printed when a command is entered.
func commandEcho(cmd string, now time.Time) string {
	return fmt.Sprintf("[::d](%s) > %s[::-]", now.Format("02-01-06 15:04:05"), tview.Escape(cmd))
}

// commandListOpen reports whether the input shows the command drop-down.
func commandListOpen() bool {
	return len(commandEntries(ctx.TextInput.GetText(), commandPrefix())) > 0
}

// setupCommandCompletion attaches the drop-down to the input field.
func setupCommandCompletion(input *tview.InputField) {
	input.SetAutocompleteFunc(func(text string) []string {
		return commandEntries(text, commandPrefix())
	})
	input.SetAutocompletedFunc(func(text string, index, source int) bool {
		if source == tview.AutocompletedNavigate {
			return false // keep the list open while moving through it
		}
		input.SetText(completedText(text, commandPrefix()))
		return true
	})
}

func commandPrefix() string { return config.Config.General.CmdPrefix }
