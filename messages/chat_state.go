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
	sm.mu.Lock()
	sm.currentReceiver = id
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

// getMessages retrieves all messages for one chat id.
func (sm *SessionManager) getMessages(wid string) []Message {
	msgs, err := sm.db.GetMessages(wid)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
		return []Message{}
	}
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
	sm.lastSent = time.Now()
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

		if err := sm.db.AddMessage(newMsg); err != nil {
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
