// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// cmdRead marks the current chat as read by sending read receipts.
func cmdRead(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	sm.markChatAsRead(receiver)
	sm.uiHandler.PrintText("Marked chat as read")
}

// groupInfoTimeout bounds the /info group request.
const groupInfoTimeout = 15 * time.Second

// cmdInfo shows information about the current chat.
func cmdInfo(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	jid, err := types.ParseJID(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
		return
	}

	if jid.Server == types.GroupServer {
		// Group chat info is a network request: run it off the command loop
		// under a timeout so a slow server never blocks other commands.
		go sm.printGroupInfo(client, jid)
	} else {
		// Direct message contact info
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to get contact: %v", err))
			return
		}

		var lines []string
		name := contact.FullName
		if name == "" {
			name = contact.PushName
		}
		if name == "" {
			name = "Unknown"
		}
		lines = append(lines, fmt.Sprintf("[::b]Contact: %s[-:-:-]", name))
		lines = append(lines, fmt.Sprintf("  JID: %s", jid.String()))
		if contact.FullName != "" {
			lines = append(lines, fmt.Sprintf("  Full Name: %s", contact.FullName))
		}
		if contact.PushName != "" {
			lines = append(lines, fmt.Sprintf("  Push Name: %s", contact.PushName))
		}
		if contact.BusinessName != "" {
			lines = append(lines, fmt.Sprintf("  Business: %s", contact.BusinessName))
		}

		// Message count from local DB
		msgs, msgErr := sm.db.GetMessages(receiver)
		if msgErr != nil {
			lines = append(lines, fmt.Sprintf("  Messages stored: (error: %v)", msgErr))
		} else {
			lines = append(lines, fmt.Sprintf("  Messages stored: %d", len(msgs)))
		}

		sm.uiHandler.PrintText(strings.Join(lines, "\n"))
	}
}

// printGroupInfo fetches and prints group info for /info.
func (sm *SessionManager) printGroupInfo(client *whatsmeow.Client, jid types.JID) {
	ctx, cancel := context.WithTimeout(sm.rootContext(), groupInfoTimeout)
	defer cancel()
	groupInfo, err := client.GetGroupInfo(ctx, jid)
	if err != nil {
		if sm.rootContext().Err() == nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to get group info: %v", err))
		}
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("[::b]Group: %s[-:-:-]", groupInfo.Name))
	lines = append(lines, fmt.Sprintf("  JID: %s", jid.String()))
	if groupInfo.Topic != "" {
		lines = append(lines, fmt.Sprintf("  Topic: %s", groupInfo.Topic))
	}
	lines = append(lines, fmt.Sprintf("  Participants: %d", len(groupInfo.Participants)))
	if groupInfo.OwnerJID != (types.JID{}) {
		lines = append(lines, fmt.Sprintf("  Owner: %s", groupInfo.OwnerJID.String()))
	}
	if !groupInfo.GroupCreated.IsZero() {
		lines = append(lines, fmt.Sprintf("  Created: %s", groupInfo.GroupCreated.Format("2006-01-02 15:04")))
	}
	sm.uiHandler.PrintText(strings.Join(lines, "\n"))
}

// cmdMore shows older stored messages in the open chat (ROADMAP row 34).
func cmdMore(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver, anchor := sm.currentReceiver, sm.viewAnchor
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	total, err := sm.db.CountMessages(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to count messages: %v", err))
		return
	}
	if anchor == 0 {
		sm.uiHandler.PrintText(fmt.Sprintf("All %d stored messages are shown. Use /backlog to request older ones from WhatsApp.", total))
		return
	}

	newAnchor, err := sm.pageAnchor(receiver, anchor, morePageSize)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	sm.mu.Lock()
	if sm.currentReceiver != receiver {
		sm.mu.Unlock()
		return // the user switched chats meanwhile
	}
	sm.viewAnchor = newAnchor
	sm.mu.Unlock()

	msgs := sm.getMessages(receiver)
	sm.uiHandler.NewScreenWithStatus(msgs, fmt.Sprintf("Showing %d of %d stored messages", len(msgs), total))
}

// cmdHelp shows help text.
func cmdHelp(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintHelp()
}

// cmdQuit exits the application.
func cmdQuit(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.Quit()
}

// cmdColorList shows available colors.
func cmdColorList(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.ShowColorList()
}
