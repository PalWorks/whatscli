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

	chatJID, err := types.ParseJID(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID: %v", err))
		return
	}

	// Get recent messages to collect IDs for read receipt
	msgs, err := sm.db.GetMessages(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
		return
	}
	if len(msgs) == 0 {
		sm.uiHandler.PrintText("No messages to mark as read")
		return
	}

	// Collect unread message IDs (up to last 50 messages from others)
	var ids []types.MessageID
	var lastSender types.JID
	for i := len(msgs) - 1; i >= 0 && len(ids) < 50; i-- {
		if !msgs[i].FromMe {
			sender, _ := types.ParseJID(msgs[i].ContactId)
			if len(ids) == 0 {
				lastSender = sender
			}
			// MarkRead requires all IDs to be from the same sender
			if sender == lastSender {
				ids = append(ids, msgs[i].Id)
			}
		}
	}

	if len(ids) == 0 {
		sm.uiHandler.PrintText("No unread messages from others")
		return
	}

	err = client.MarkRead(context.Background(), ids, time.Now(), chatJID, lastSender)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to mark as read: %v", err))
		return
	}

	// Reset unread counter — copy under lock, DB write outside (audit R-2).
	var convCopy *Conversation
	sm.mu.Lock()
	if conv := sm.convByJID[receiver]; conv != nil {
		conv.Unread = 0
		c := *conv
		convCopy = &c
	}
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()

	if convCopy != nil {
		if err := sm.db.UpsertConversation(*convCopy); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
		}
	}

	sm.uiHandler.UpdateChatList(safeList)
	sm.uiHandler.PrintText(fmt.Sprintf("Marked %d messages as read", len(ids)))
}

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
		// Group chat info
		groupInfo, err := client.GetGroupInfo(context.Background(), jid)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to get group info: %v", err))
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

// cmdMore loads older messages for the current chat (pagination).
func cmdMore(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	// Get current messages to find the oldest timestamp
	currentMsgs, err := sm.db.GetMessages(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
		return
	}
	if len(currentMsgs) == 0 {
		sm.uiHandler.PrintText("No messages in this chat")
		return
	}

	// Find the oldest timestamp in the current view
	oldestTimestamp := currentMsgs[0].Timestamp
	for _, m := range currentMsgs {
		if m.Timestamp < oldestTimestamp {
			oldestTimestamp = m.Timestamp
		}
	}

	// Load 50 older messages
	olderMsgs, err := sm.db.GetMessagesPaginated(receiver, oldestTimestamp, 50)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load older messages: %v", err))
		return
	}
	if len(olderMsgs) == 0 {
		sm.uiHandler.PrintText("No older messages available locally. Try /backlog to request from server.")
		return
	}

	// Reverse to chronological order (DB returns newest-first)
	for i, j := 0, len(olderMsgs)-1; i < j; i, j = i+1, j-1 {
		olderMsgs[i], olderMsgs[j] = olderMsgs[j], olderMsgs[i]
	}

	// Combine: older messages + current messages → full screen refresh
	combined := append(olderMsgs, currentMsgs...)
	sm.uiHandler.NewScreen(combined)
	sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d older messages", len(olderMsgs)))
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
