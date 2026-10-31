// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
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

		sm.importContacts(contacts, groupNames)

		sm.mu.Lock()
		safeList := sm.snapshotPQ()
		sm.mu.Unlock()
		sm.uiHandler.UpdateChatList(safeList)

		sm.uiHandler.PrintText("Chats loaded.")
		sm.mergeKnownLIDChats()
	} else {
		sm.uiHandler.PrintError(errors.New("failed to access contacts store"))
	}
}

// importContacts adds the contact list to the chat list. Only phone-number,
// LID and group contacts are chat JIDs. A LID contact is stored under its
// phone-number chat when the mapping is known (canonicalChatJID) and is kept
// as @lid otherwise. Only groups get a new empty chat here; other contacts
// without messages only refresh the name of a chat that already exists, so no
// empty rows appear (isEmptyChat, DeleteEmptyConversations).
func (sm *SessionManager) importContacts(contacts map[types.JID]types.ContactInfo, groupNames map[string]string) {
	for jid, contact := range contacts {
		if !contact.Found {
			continue
		}

		// Skip non-chat JIDs
		if !isHistoryChatServer(jid.Server) {
			continue
		}

		isGroup := jid.Server == types.GroupServer
		isLID := jid.Server == types.HiddenUserServer
		if isLID && contact.FullName == "" && contact.PushName == "" {
			// A bare LID number is no name and must not replace a real one.
			continue
		}

		// Resolve the chat JID before taking sm.mu (the LID lookup may call getClient).
		chatJID := sm.canonicalChatJID(jid)
		jidStr := chatJID.String()

		// Determine name
		var name string
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

		// Update PQ under lock, collect DB write for outside
		sm.mu.Lock()
		existingConv := sm.convByJID[jidStr]
		if existingConv == nil && isLID {
			// Chat still stored under the raw LID (mapping learned later).
			existingConv = sm.convByJID[jid.String()]
		}

		var toUpsert *Conversation
		if existingConv != nil {
			if name != "" && existingConv.Name != name {
				existingConv.Name = name
				c := *existingConv
				toUpsert = &c
			}
		} else if isGroup {
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
	sm.storeHistorySync(data, client.ParseWebMessage)
}

// webMessageParser is client.ParseWebMessage; a seam so tests can feed history
// sync data without a connected client.
type webMessageParser func(chatJID types.JID, webMsg *waWeb.WebMessageInfo) (*events.Message, error)

// storeHistorySync stores the conversations and messages of a history sync.
func (sm *SessionManager) storeHistorySync(data *waHistorySync.HistorySync, parseWebMessage webMessageParser) {
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
	mediaErrors := 0
	currentChatChanged := false
	sm.mu.RLock()
	currentChat := sm.currentReceiver
	sm.mu.RUnlock()

	for _, conv := range conversations {
		chatJIDStr := conv.GetID()
		chatJID, err := types.ParseJID(chatJIDStr)
		if err != nil {
			continue
		}

		// Skip non-chat JIDs (status broadcasts, etc.). @lid chats are kept
		// and stored under their phone-number JID when the mapping is known.
		if !isHistoryChatServer(chatJID.Server) {
			continue
		}
		storeJID := sm.historyChatJID(chatJID, conv.GetPnJID())
		jidStr := storeJID.String()

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

			evt, err := parseWebMessage(chatJID, webMsg)
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
			if err := attachMediaInfo(&msg, evt.Message); err != nil {
				mediaErrors++
			}

			// Known IDs keep their text; only missing media metadata is
			// filled. Redraw the open chat only when a row changed.
			if res, err := sm.db.AddMessage(msg); err != nil {
				addMsgErrors++
			} else if res.Changed() && jidStr == currentChat {
				currentChatChanged = true
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
			if name != "" && (existingConv.Name == existingConv.JID || existingConv.Name == chatJID.User || existingConv.Name == storeJID.User) {
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
		} else if isEmptyChat(storeJID, lastMsgTimestamp, unreadCount, isPinned) {
			// A contact WhatsApp lists without any message or activity: not a
			// chat yet (ROADMAP row 33). /search-contact still finds it.
		} else {
			// New conversation — use history name or fall back to JID user
			if name == "" {
				name = storeJID.User
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
	if mediaErrors > 0 {
		sm.uiHandler.PrintError(fmt.Errorf("failed to keep media metadata for %d history messages", mediaErrors))
	}

	// ON_DEMAND responses (from /backlog) usually arrive after the backlog
	// command stopped polling, so refresh the open chat here as well.
	if currentChatChanged {
		sm.mu.RLock()
		stillCurrent := sm.currentReceiver == currentChat
		sm.mu.RUnlock()
		if stillCurrent {
			sm.uiHandler.NewScreen(sm.getMessages(currentChat))
		}
	}

	sm.uiHandler.PrintText(fmt.Sprintf("History sync complete: %d conversations, %d messages stored.",
		len(toUpsert), totalMessages))

	// History sync also teaches whatsmeow LID mappings; merge chats that
	// were stored under a LID before (ROADMAP row 13).
	sm.mergeKnownLIDChats()
}

// isHistoryChatServer reports whether a history sync conversation on this JID
// server is a chat we store: phone-number DMs, LID DMs and groups.
func isHistoryChatServer(server string) bool {
	return server == types.DefaultUserServer || server == types.HiddenUserServer || server == types.GroupServer
}

// isEmptyChat reports whether a conversation is a 1:1 chat with no message,
// time, unread count or pin: a contact entry rather than a chat. Groups are
// always kept, because membership is meaningful without messages.
func isEmptyChat(jid types.JID, lastMsgTime int64, unread uint16, pinned bool) bool {
	return jid.Server != types.GroupServer && lastMsgTime == 0 && unread == 0 && !pinned
}

// historyChatJID returns the JID a history sync conversation is stored under.
// For an @lid conversation the phone-number JID carried in the conversation
// (pnJID) is used first, then the LID store; without either the @lid JID is
// kept. Other chats are returned unchanged.
func (sm *SessionManager) historyChatJID(chat types.JID, pnJID string) types.JID {
	if chat.Server != types.HiddenUserServer {
		return chat
	}
	if pnJID != "" {
		if pn, err := types.ParseJID(pnJID); err == nil && pn.Server == types.DefaultUserServer && pn.User != "" {
			return pn.ToNonAD()
		}
	}
	return sm.canonicalChatJID(chat)
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
