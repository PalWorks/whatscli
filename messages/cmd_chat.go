package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func cmdSend(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 2) {
		receiver := params[0]
		textParams := params[1:]
		text := strings.Join(textParams, " ")
		sm.sendText(receiver, text)
	} else {
		sm.printCommandUsage(cmdName, "[chat-id] [message text]")
	}
}

func cmdSelect(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		sm.setCurrentReceiver(params[0])
	} else {
		sm.printCommandUsage(cmdName, "[chat-id]")
	}
}

func cmdBacklog(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver != "" {
		// First approach: Try to use the direct conversation query method
		jid, err := types.ParseJID(receiver)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
			return
		}

		sm.uiHandler.PrintText("Retrieving message history...")

		// Get existing messages to compare later
		existingMessages := sm.db.GetMessages(receiver)

		// Find the ID of the oldest message we have - not used currently but could be in future
		var oldestTimestamp uint64 = ^uint64(0) // Maximum uint64 value
		for _, msg := range existingMessages {
			if msg.Timestamp < oldestTimestamp {
				oldestTimestamp = msg.Timestamp
			}
		}

		// Try multiple approaches:
		var messagesFetched bool

		// 1. First try direct message fetch
		if client != nil && client.IsConnected() {
			sm.uiHandler.PrintText("Attempting to fetch older messages...")

			// Try to send a simpler read receipt which sometimes triggers history sync
			receiptType := types.ReceiptTypeRead
			err := client.MarkRead(context.Background(), []types.MessageID{}, time.Now(), jid, jid, receiptType)
			if err != nil {
				sm.uiHandler.PrintText(fmt.Sprintf("Note: Could not send read receipt: %v", err))
			}

			// Wait a bit
			time.Sleep(2 * time.Second)

			// Check if we got any new messages
			updatedMessages := sm.db.GetMessages(sm.currentReceiver)
			if len(updatedMessages) > len(existingMessages) {
				messagesFetched = true
				sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(updatedMessages)-len(existingMessages)))
			}
		}

		// 2. If that didn't work, try a presence update which can trigger history sync
		if !messagesFetched && client != nil && client.IsConnected() {
			sm.uiHandler.PrintText("Trying alternative method...")

			// Send chat presence - using ChatPresence constants from the types package
			err = client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
			if err != nil {
				sm.uiHandler.PrintText(fmt.Sprintf("Note: Could not send chat presence: %v", err))
			}

			// Wait a bit longer for any messages to arrive
			time.Sleep(3 * time.Second)

			// Check if we got any new messages
			updatedMessages := sm.db.GetMessages(sm.currentReceiver)
			if len(updatedMessages) > len(existingMessages) {
				messagesFetched = true
				sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(updatedMessages)-len(existingMessages)))
			}
		}

		// 3. Proper method: Use BuildHistorySyncRequest
		if !messagesFetched && client != nil && client.IsConnected() {
			sm.uiHandler.PrintText("Requesting history sync from WhatsApp servers...")

			var anchor *types.MessageInfo
			msgs := sm.db.GetMessages(sm.currentReceiver)
			if len(msgs) > 0 {
				oldest := msgs[0]
				chatJID, _ := types.ParseJID(oldest.ChatId)
				senderJID, _ := types.ParseJID(oldest.ContactId)

				// Construct anchor message info
				anchor = &types.MessageInfo{
					ID: oldest.Id,
					MessageSource: types.MessageSource{
						Chat:     chatJID,
						Sender:   senderJID,
						IsFromMe: oldest.FromMe,
					},
					Timestamp: time.Unix(int64(oldest.Timestamp), 0),
				}
				sm.uiHandler.PrintText(fmt.Sprintf("Requesting 50 messages before: %s (%s)", oldest.Id, time.Unix(int64(oldest.Timestamp), 0).Format(time.RFC822)))
			} else {
				sm.uiHandler.PrintText("Requesting latest 50 messages...")
			}

			// BuildHistorySyncRequest takes (message *types.MessageInfo, limit int) and returns *waProto.Message
			req := client.BuildHistorySyncRequest(anchor, 50)

			// Send to self (primary device)
			target := *client.Store.ID
			target.Device = 0

			resp, err := client.SendMessage(context.Background(), target, req)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to send history sync request: %v", err))
			} else {
				sm.uiHandler.PrintText("History sync request sent (ID: " + resp.ID + "). Waiting for server response...")
				// History sync comes as an async event, so we just wait a bit to see if DB updates
				time.Sleep(5 * time.Second)

				// Check if we got any new messages
				finalMessages := sm.db.GetMessages(sm.currentReceiver)
				if len(finalMessages) > len(existingMessages) {
					sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(finalMessages)-len(existingMessages)))
				} else {
					sm.uiHandler.PrintText("No immediate history received. It may arrive in the background.")
				}
			}
		}

		// Show the updated message list
		screen := sm.getMessages(receiver)
		sm.uiHandler.NewScreen(screen)
	} else {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
	}
}

func cmdSyncGroups(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintText("Fetching joined groups from WhatsApp servers...")
	go func() {
		if client == nil {
			sm.uiHandler.PrintError(errors.New("not connected"))
			return
		}
		groups, err := client.GetJoinedGroups(context.Background())
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to fetch groups: %v", err))
			return
		}

		count := 0
		for _, group := range groups {
			// Convert to Conversation/Chat format and save
			jid := group.JID.String()
			name := group.Name
			if name == "" {
				name = "Unknown Group"
			}

			// Check if exists/Update logic similar to loadRecentChats
			sm.mu.Lock()
			var existingConv *Conversation
			for _, item := range sm.priorityQueue {
				if item.JID == jid {
					existingConv = item
					break
				}
			}

			if existingConv != nil {
				if existingConv.Name != name {
					existingConv.Name = name
					sm.db.UpsertConversation(*existingConv)
				}
			} else {
				newConv := &Conversation{
					JID:         jid,
					Name:        name,
					LastMsgTime: 0, // No messages yet
					Preview:     "Group synced",
					Unread:      0,
					IsPinned:    false,
				}
				heap.Push(&sm.priorityQueue, newConv)
				sm.db.UpsertConversation(*newConv)
			}
			sm.mu.Unlock()

			// Legacy DB support
			chatObj := Chat{
				Id:          jid,
				IsGroup:     true,
				Name:        name,
				Unread:      0,
				LastMessage: 0,
			}
			sm.db.AddChat(chatObj)
			count++
		}

		// Create safe copy of PQ for UI update
		sm.mu.Lock()
		safeList := make([]*Conversation, len(sm.priorityQueue))
		for i, item := range sm.priorityQueue {
			copiedItem := new(Conversation)
			*copiedItem = *item
			safeList[i] = copiedItem
		}
		sm.mu.Unlock()

		sm.uiHandler.UpdateChatList(safeList)
		sm.uiHandler.PrintText(fmt.Sprintf("Synced %d groups.", count))
	}()
}
