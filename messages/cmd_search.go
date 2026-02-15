package messages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
)

// cmdSearch searches messages for a keyword in the current chat.
func cmdSearch(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "[keyword]")
		return
	}

	keyword := strings.Join(params, " ")

	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	results, err := sm.db.SearchMessages(receiver, keyword, 50)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to search messages: %v", err))
		return
	}
	if len(results) == 0 {
		sm.uiHandler.PrintText(fmt.Sprintf("No messages found matching '%s'", keyword))
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("[::b]Search results for '%s' (%d found):[-:-:-]", keyword, len(results)))
	for _, msg := range results {
		ts := time.Unix(int64(msg.Timestamp), 0).Format("01/02 15:04")
		sender := msg.ContactShort
		if sender == "" {
			sender = msg.ContactName
		}
		if msg.FromMe {
			sender = "You"
		}

		// Truncate long messages
		text := msg.Text
		if len([]rune(text)) > 100 {
			text = string([]rune(text)[:100]) + "…"
		}

		lines = append(lines, fmt.Sprintf("  [%s] %s: %s", ts, sender, text))
	}

	sm.uiHandler.PrintText(strings.Join(lines, "\n"))
}

// cmdSearchContact searches contacts by name and displays matching JIDs.
func cmdSearchContact(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "[name]")
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	query := strings.ToLower(strings.Join(params, " "))

	contacts, err := client.Store.Contacts.GetAllContacts(context.Background())
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to get contacts: %v", err))
		return
	}

	type match struct {
		name string
		jid  string
	}

	var matches []match
	for jid, info := range contacts {
		// Search across all name fields
		names := []string{info.FullName, info.PushName, info.BusinessName, info.FirstName}
		for _, n := range names {
			if n != "" && strings.Contains(strings.ToLower(n), query) {
				displayName := info.FullName
				if displayName == "" {
					displayName = info.PushName
				}
				if displayName == "" {
					displayName = info.BusinessName
				}
				matches = append(matches, match{name: displayName, jid: jid.String()})
				break // only add once per contact
			}
		}
	}

	if len(matches) == 0 {
		sm.uiHandler.PrintText(fmt.Sprintf("No contacts found matching '%s'", strings.Join(params, " ")))
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("[::b]Contacts matching '%s' (%d found):[-:-:-]", strings.Join(params, " "), len(matches)))
	for _, m := range matches {
		lines = append(lines, fmt.Sprintf("  %s — %s", m.name, m.jid))
	}
	lines = append(lines, "")
	lines = append(lines, "Use /select <jid> to open a chat")

	sm.uiHandler.PrintText(strings.Join(lines, "\n"))
}
