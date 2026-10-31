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
	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
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

// commandListOpen reports whether the input shows the command or chat
// drop-down.
func commandListOpen() bool {
	return len(inputEntries(ctx.TextInput.GetText())) > 0
}

// inputEntries is the drop-down for the input text: chats after
// "<prefix>select ", else matching commands. It also records chatChoices.
func inputEntries(text string) []string {
	chatChoices = nil
	if q, ok := chatQuery(text, commandPrefix()); ok {
		chatChoices = matchChats(ctx.AllChats, q, chatSearchLimit)
		entries := make([]string, len(chatChoices))
		for i, c := range chatChoices {
			entries[i] = chatEntry(c)
		}
		return entries
	}
	return commandEntries(text, commandPrefix())
}

// setupCommandCompletion attaches the drop-down to the input field.
func setupCommandCompletion(input *tview.InputField) {
	input.SetAutocompleteFunc(inputEntries)
	input.SetAutocompletedFunc(func(text string, index, source int) bool {
		if source == tview.AutocompletedNavigate {
			return false // keep the list open while moving through it
		}
		if _, ok := chatQuery(input.GetText(), commandPrefix()); ok {
			if index >= 0 && index < len(chatChoices) {
				SetDisplayedChat(chatFromConversation(chatChoices[index]))
			}
			input.SetText("")
			return true
		}
		input.SetText(completedText(text, commandPrefix()))
		if _, ok := chatQuery(input.GetText(), commandPrefix()); ok {
			// Offer chats right away. Queued: this callback runs while the
			// input field holds its drop-down lock.
			go ctx.App.QueueUpdateDraw(func() { input.Autocomplete() })
		}
		return true
	})
}

func commandPrefix() string { return config.Config.General.CmdPrefix }

// chatSearchLimit caps the chat drop-down.
const chatSearchLimit = 10

// chatChoices are the chats behind the current chat drop-down, in order.
var chatChoices []*messages.Conversation

// chatQuery returns the name typed after "<prefix>select " and whether the
// input is in that form.
func chatQuery(text, prefix string) (string, bool) {
	head := prefix + "select "
	if prefix == "" || len(text) < len(head) || !strings.EqualFold(text[:len(head)], head) {
		return "", false
	}
	return strings.TrimSpace(text[len(head):]), true
}

// matchChats returns up to limit chats whose name or number contains query
// (case-insensitive), in the order given (the chat list is newest first). An
// empty query matches every chat. The status broadcast is never offered.
func matchChats(chats []*messages.Conversation, query string, limit int) []*messages.Conversation {
	q := strings.ToLower(query)
	var out []*messages.Conversation
	for _, c := range chats {
		if c.JID == messages.STATUSSUFFIX {
			continue
		}
		user, _, _ := strings.Cut(c.JID, "@")
		if q == "" || strings.Contains(strings.ToLower(c.Name), q) || strings.Contains(user, q) {
			out = append(out, c)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

// chatEntry is one line of the chat drop-down: name, a group marker, time.
func chatEntry(c *messages.Conversation) string {
	name := c.Name
	if name == "" {
		name = c.JID
	}
	if strings.HasSuffix(c.JID, messages.GROUPSUFFIX) {
		name += " (group)"
	}
	if r := []rune(name); len(r) > 36 {
		name = string(r[:35]) + "…"
	}
	return tview.Escape(fmt.Sprintf("%-36s %s", name, formatRelativeTime(c.LastMsgTime)))
}

// chatFromConversation is the UI chat for a chat list entry.
func chatFromConversation(c *messages.Conversation) messages.Chat {
	return messages.Chat{
		Id:          c.JID,
		Name:        c.Name,
		Unread:      int(c.Unread),
		LastMessage: c.LastMsgTime,
		IsGroup:     strings.HasSuffix(c.JID, messages.GROUPSUFFIX),
	}
}

// openChatByName opens the newest chat matching name, or says none matched.
// It reports whether the input was a name (not a chat ID).
func openChatByName(param string) bool {
	if strings.Contains(param, "@") {
		return false
	}
	if m := matchChats(ctx.AllChats, param, 1); len(m) == 1 {
		SetDisplayedChat(chatFromConversation(m[0]))
	} else {
		PrintText("No chat matches \"" + tview.Escape(param) + "\"")
	}
	return true
}

// handleFindChat starts a chat search on the input line.
func handleFindChat(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.TextInput)
	ctx.TextInput.SetText(commandPrefix() + "select ")
	ctx.TextInput.Autocomplete()
	return nil
}
