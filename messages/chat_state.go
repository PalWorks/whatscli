// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"context"
	"errors"
	"fmt"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// setCurrentReceiver sets the currently selected chat and refreshes the
// message view. Automatically marks the chat as read in the background.
func (sm *SessionManager) setCurrentReceiver(id string) {
	anchor := sm.openViewAnchor(id) // DB read, outside sm.mu
	sm.mu.Lock()
	sm.currentReceiver = id
	sm.viewAnchor = anchor
	// Check if conversation has unread messages
	hasUnread := false
	if conv := sm.convByJID[id]; conv != nil && conv.Unread > 0 {
		hasUnread = true
	}
	sm.mu.Unlock()
	screen := sm.getMessages(id)
	sm.uiHandler.NewScreen(screen)

	// Auto-mark as read in background (like WhatsApp Web)
	if hasUnread {
		go sm.markChatAsRead(id)
	}
}

// showStatus prints status (and errs) for an action that may also have
// changed the open chat. With refreshChat set, the chat is redrawn and the
// status printed after it in one UI update, so the redraw cannot wipe it
// (ROADMAP row 10); otherwise errs and status are just printed.
func (sm *SessionManager) showStatus(refreshChat, status string, errs ...error) {
	if refreshChat != "" {
		sm.uiHandler.NewScreenWithStatus(sm.getMessages(refreshChat), status, errs...)
		return
	}
	for _, err := range errs {
		if err != nil {
			sm.uiHandler.PrintError(err)
		}
	}
	if status != "" {
		sm.uiHandler.PrintText(status)
	}
}

// refreshTarget returns chat if it is still the open chat, else "", so a
// background job only redraws the chat the user is looking at.
func (sm *SessionManager) refreshTarget(chat string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.currentReceiver == chat {
		return chat
	}
	return ""
}

// markChatAsRead sends read receipts for the most recent incoming messages
// in the given chat and resets the unread counter in PQ and DB.
// Safe for background use — returns silently on any failure.
func (sm *SessionManager) markChatAsRead(jidStr string) {
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		return
	}

	chatJID, err := types.ParseJID(jidStr)
	if err != nil {
		return
	}

	// Load messages to collect IDs for read receipt
	msgs, err := sm.db.GetMessages(jidStr)
	if err != nil || len(msgs) == 0 {
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
		return
	}

	if err := client.MarkRead(context.Background(), ids, time.Now(), chatJID, lastSender); err != nil {
		return
	}

	// Reset unread counter — copy under lock, DB write outside.
	var convCopy *Conversation
	sm.mu.Lock()
	if conv := sm.convByJID[jidStr]; conv != nil {
		conv.Unread = 0
		c := *conv
		convCopy = &c
	}
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()

	if convCopy != nil {
		_ = sm.db.UpsertConversation(*convCopy)
	}

	sm.uiHandler.UpdateChatList(safeList)
}

// snapshotPQ returns a deep copy of the priority queue. Caller must hold sm.mu.
func (sm *SessionManager) snapshotPQ() []*Conversation {
	safeList := make([]*Conversation, len(sm.priorityQueue))
	for i, item := range sm.priorityQueue {
		c := *item
		safeList[i] = &c
	}
	return safeList
}

// getChatName returns the best display name for a chat.
func (sm *SessionManager) getChatName(jid types.JID) string {
	client := sm.getClient()
	if client == nil {
		return jid.User
	}

	// For groups, use the group name if available
	if jid.Server == "g.us" {
		groupInfo, err := client.GetGroupInfo(context.Background(), jid)
		if err == nil && groupInfo.Name != "" {
			return groupInfo.Name
		}
		return "Group Chat"
	}

	// For individual chats, try to get the contact name
	if client.Store != nil {
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found {
			if contact.FullName != "" {
				return contact.FullName
			}
			if contact.PushName != "" {
				return contact.PushName
			}
		}
	}

	return jid.User
}

// getMessages returns the messages to show for wid: from the view anchor
// for the open chat, the newest page for any other chat.
func (sm *SessionManager) getMessages(wid string) []Message {
	sm.mu.RLock()
	open, anchor := sm.currentReceiver == wid, sm.viewAnchor
	sm.mu.RUnlock()
	if !open {
		anchor = sm.openViewAnchor(wid)
	}
	msgs, err := sm.db.GetMessagesSince(wid, anchor)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
		return []Message{}
	}
	sm.labelSenders(msgs) // saved name, else phone number (display_names.go)
	return msgs
}

// sendText sends a text message to a WhatsApp JID.
func (sm *SessionManager) sendText(wid string, text string) {
	sm.mu.RLock()
	client := sm.client
	sm.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}

	// Parse JID
	receiver, err := types.ParseJID(wid)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
		return
	}

	// Create message
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}

	// Send message
	resp, err := client.SendMessage(context.Background(), receiver, msg)

	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to send message: %v", err))
	} else {
		// Create a Message struct to save to the database
		newMsg := Message{
			Id:           resp.ID,
			ChatId:       wid,
			FromMe:       true,
			Timestamp:    uint64(time.Now().Unix()),
			Text:         text,
			ContactId:    client.Store.ID.String(),
			ContactName:  "Me",
			ContactShort: "Me",
		}

		if _, err := sm.db.AddMessage(newMsg); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to save sent message: %v", err))
		}

		sm.mu.RLock()
		isCurrent := sm.currentReceiver == wid
		sm.mu.RUnlock()

		if isCurrent {
			sm.uiHandler.NewMessage(newMsg)
		}
	}
}
