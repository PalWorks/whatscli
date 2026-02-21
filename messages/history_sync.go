package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

// loadRecentChats fetches recent chats from WhatsApp and adds them to the database
func (sm *SessionManager) loadRecentChats() {
	sm.uiHandler.PrintText("Loading chats...")

	// Capture client pointer safely
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}

	// Try to get all chats through the whatsmeow API
	if client.Store != nil && client.Store.Contacts != nil {
		contacts, err := client.Store.Contacts.GetAllContacts(context.Background())
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to load contacts for chat list: %v", err))
			return
		}

		// P-3: Pre-fetch all group names in a single batch call to avoid N+1 API calls.
		groupNames := make(map[string]string) // JID string → group name
		if groups, err := client.GetJoinedGroups(context.Background()); err == nil {
			for _, g := range groups {
				if g.Name != "" {
					groupNames[g.JID.String()] = g.Name
				}
			}
		}

		// Process each contact as a potential chat
		chatCount := 0

		// Map to track what we've processed to avoid duplicates in PQ if reloading
		processed := make(map[string]bool)

		for jid, contact := range contacts {
			if !contact.Found {
				continue
			}

			// Skip non-chat JIDs
			if jid.Server != "s.whatsapp.net" && jid.Server != "g.us" {
				continue
			}

			jidStr := jid.String()
			processed[jidStr] = true

			// Determine name
			var name string
			isGroup := jid.Server == "g.us"
			if isGroup {
				if gn, ok := groupNames[jidStr]; ok {
					name = gn
				} else {
					name = "Group: " + jid.User
				}
			} else {
				name = contact.FullName
				if name == "" {
					name = contact.PushName
				}
				if name == "" {
					name = jid.User
				}
			}

			// Create/Update Conversation struct
			// In a real sync, we might want to check the actual last message time.
			// For now, if it's a new import, use current time.
			// If it exists in DB, we should probably prefer the DB's time unless we have new info.

			// Update PQ under lock, collect DB write for outside
			sm.mu.Lock()
			existingConv := sm.convByJID[jidStr]

			var toUpsert *Conversation
			if existingConv != nil {
				if name != "" && existingConv.Name != name {
					existingConv.Name = name
					c := *existingConv
					toUpsert = &c
				}
			} else {
				newConv := &Conversation{
					JID:         jidStr,
					Name:        name,
					LastMsgTime: 0,
					Preview:     "New chat",
					Unread:      0,
					IsPinned:    false,
				}
				heap.Push(&sm.priorityQueue, newConv)
				sm.convByJID[newConv.JID] = newConv
				c := *newConv
				toUpsert = &c
			}
			sm.mu.Unlock()

			if toUpsert != nil {
				if err := sm.db.UpsertConversation(*toUpsert); err != nil {
					sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
				}
			}

			chatCount++
		}

		sm.mu.Lock()
		safeList := sm.snapshotPQ()
		sm.mu.Unlock()
		sm.uiHandler.UpdateChatList(safeList)

		sm.uiHandler.PrintText("Chats loaded.")
	} else {
		sm.uiHandler.PrintError(errors.New("failed to access contacts store"))
	}
}

// processHistorySync parses a HistorySync event to extract real conversation
// metadata (timestamps, unread counts, pinned status) and store historical
// messages in SQLite. This enables correct chat ordering on first login.
func (sm *SessionManager) processHistorySync(data *waHistorySync.HistorySync) {
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected — cannot process history sync"))
		return
	}

	conversations := data.GetConversations()
	if len(conversations) == 0 {
		sm.uiHandler.PrintText(fmt.Sprintf("History sync (%s): no conversations", data.GetSyncType()))
		return
	}

	sm.uiHandler.PrintText(fmt.Sprintf("Processing history sync (%s): %d conversations...",
		data.GetSyncType(), len(conversations)))

	var toUpsert []Conversation
	totalMessages := 0
	addMsgErrors := 0

	for _, conv := range conversations {
		chatJIDStr := conv.GetID()
		chatJID, err := types.ParseJID(chatJIDStr)
		if err != nil {
			continue
		}

		// Skip non-chat JIDs (status broadcasts, etc.)
		if chatJID.Server != "s.whatsapp.net" && chatJID.Server != "g.us" {
			continue
		}

		jidStr := chatJID.String()

		// --- Extract conversation metadata from proto ---
		lastMsgTimestamp := int64(conv.GetLastMsgTimestamp())
		if lastMsgTimestamp == 0 {
			lastMsgTimestamp = int64(conv.GetConversationTimestamp())
		}

		unreadCount := uint16(conv.GetUnreadCount())
		isPinned := conv.GetPinned() > 0
		isArchived := conv.GetArchived()

		// Determine chat name from history proto
		name := conv.GetName()
		if name == "" {
			name = conv.GetDisplayName()
		}

		// --- Process individual messages ---
		var latestPreview string
		var latestMsgTs int64

		for _, histMsg := range conv.GetMessages() {
			webMsg := histMsg.GetMessage()
			if webMsg == nil {
				continue
			}

			evt, err := client.ParseWebMessage(chatJID, webMsg)
			if err != nil {
				continue
			}

			text, preview := extractMessageContent(evt.Message)
			if text == "" {
				continue
			}

			msgTimestamp := uint64(evt.Info.Timestamp.Unix())

			// Resolve sender name: prefer PushName, fall back to JID
			senderName := evt.Info.PushName
			if senderName == "" {
				senderName = evt.Info.Sender.User
			}

			msg := Message{
				Id:           evt.Info.ID,
				ChatId:       jidStr,
				FromMe:       evt.Info.IsFromMe,
				Timestamp:    msgTimestamp,
				Text:         text,
				ContactId:    evt.Info.Sender.String(),
				ContactName:  senderName,
				ContactShort: senderName,
			}

			// AddMessage uses INSERT OR IGNORE, so duplicates are skipped
			if err := sm.db.AddMessage(msg); err != nil {
				addMsgErrors++
			}

			// Track the most recent message for the conversation preview
			if int64(msgTimestamp) > latestMsgTs {
				latestMsgTs = int64(msgTimestamp)
				latestPreview = preview
			}

			totalMessages++
		}

		// Use best available timestamp
		if latestMsgTs > lastMsgTimestamp {
			lastMsgTimestamp = latestMsgTs
		}

		if latestPreview == "" {
			latestPreview = "New chat"
		}

		// --- Update priority queue ---
		sm.mu.Lock()
		existingConv := sm.convByJID[jidStr]

		if existingConv != nil {
			updated := false

			// Only update timestamp/preview if history has newer data
			if lastMsgTimestamp > existingConv.LastMsgTime {
				existingConv.LastMsgTime = lastMsgTimestamp
				existingConv.Preview = latestPreview
				updated = true
			}

			// Fill in name if we only had a phone number
			if name != "" && (existingConv.Name == existingConv.JID || existingConv.Name == chatJID.User) {
				existingConv.Name = name
				updated = true
			}

			// Always adopt pinned status from authoritative source
			if isPinned != existingConv.IsPinned {
				existingConv.IsPinned = isPinned
				updated = true
			}

			// Prefer history unread count if it's higher
			if unreadCount > existingConv.Unread {
				existingConv.Unread = unreadCount
				updated = true
			}

			// Adopt archived status from authoritative source
			if isArchived != existingConv.IsArchived {
				existingConv.IsArchived = isArchived
				updated = true
			}

			if updated {
				sm.priorityQueue.Update(existingConv, existingConv.LastMsgTime, existingConv.IsPinned)
				c := *existingConv
				toUpsert = append(toUpsert, c)
			}
		} else {
			// New conversation — use history name or fall back to JID user
			if name == "" {
				name = chatJID.User
			}
			newConv := &Conversation{
				JID:         jidStr,
				Name:        name,
				LastMsgTime: lastMsgTimestamp,
				Preview:     latestPreview,
				Unread:      unreadCount,
				IsPinned:    isPinned,
				IsArchived:  isArchived,
			}
			heap.Push(&sm.priorityQueue, newConv)
			sm.convByJID[jidStr] = newConv
			toUpsert = append(toUpsert, *newConv)
		}
		sm.mu.Unlock()
	}

	// Persist all conversation updates (outside lock)
	for _, c := range toUpsert {
		if err := sm.db.UpsertConversation(c); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
		}
	}

	// Refresh chat list UI
	sm.mu.Lock()
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()
	sm.uiHandler.UpdateChatList(safeList)

	if addMsgErrors > 0 {
		sm.uiHandler.PrintError(fmt.Errorf("failed to store %d history messages", addMsgErrors))
	}

	sm.uiHandler.PrintText(fmt.Sprintf("History sync complete: %d conversations, %d messages stored.",
		len(toUpsert), totalMessages))
}

// loadRecentMessages loads the most recent messages for a chat
func (sm *SessionManager) loadRecentMessages(chatJID string) {
	if client := sm.getClient(); client == nil || !client.IsConnected() {
		return
	}

	// For now, message history retrieval is limited in whatsmeow
	// Messages will be populated as they're sent and received
	// silenced: sm.uiHandler.PrintText(fmt.Sprintf("Message history for %s will be populated as you communicate", chatJID))

	// If this is the currently selected chat, update the UI
	if chatJID == sm.currentReceiver {
		screen := sm.getMessages(chatJID)
		sm.uiHandler.NewScreen(screen)
	}
}
