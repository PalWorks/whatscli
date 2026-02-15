package messages

// event_handler.go — extracted from session_manager.go (audit A2-A3).
// Contains the WhatsApp event handler, message extraction, and contact
// name resolution logic.

import (
	"container/heap"
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type eventHandler struct {
	sm *SessionManager
}

func (eh *eventHandler) Handle(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		eh.handleMessage(v)
	case *events.Connected:
		eh.sm.mu.Lock()
		eh.sm.reconnecting = false
		eh.sm.loggedOut = false
		eh.sm.mu.Unlock()
		eh.sm.StatusChannel <- StatusMsg{true, nil}
	case *events.Disconnected:
		eh.sm.StatusChannel <- StatusMsg{false, nil}
		// Attempt auto-reconnect unless logged out or already reconnecting
		eh.sm.mu.Lock()
		shouldReconnect := !eh.sm.loggedOut && !eh.sm.reconnecting && eh.sm.started
		if shouldReconnect {
			eh.sm.reconnecting = true
		}
		eh.sm.mu.Unlock()
		if shouldReconnect {
			go eh.sm.scheduleReconnect()
		}
	case *events.LoggedOut:
		eh.sm.mu.Lock()
		eh.sm.loggedOut = true
		eh.sm.mu.Unlock()
		eh.sm.StatusChannel <- StatusMsg{false, nil}
		reasonText := fmt.Sprintf("%v", v.Reason)
		eh.sm.uiHandler.PrintText("Logged out: " + reasonText)
	case *events.HistorySync:
		// Reload chats when history sync occurs
		eh.sm.uiHandler.PrintText("Receiving history sync...")
		go eh.sm.loadRecentChats()
	}
}

// extractMessageContent extracts display text and chat-list preview from a
// whatsmeow Message proto. This is a pure function with no side effects,
// making it easy to test.
func extractMessageContent(msg *waE2E.Message) (text, preview string) {
	if msg == nil {
		return "", ""
	}

	// 1. Extended text (replies, link previews, formatted text)
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		t := ext.GetText()
		if t != "" {
			return t, truncatePreview(t)
		}
	}

	// 2. Image
	if img := msg.GetImageMessage(); img != nil {
		c := img.GetCaption()
		if c != "" {
			return "[IMAGE] " + c, "[IMAGE] " + c
		}
		return "[IMAGE]", "[IMAGE]"
	}

	// 3. Video / GIF
	if vid := msg.GetVideoMessage(); vid != nil {
		tag := "[VIDEO]"
		if vid.GetGifPlayback() {
			tag = "[GIF]"
		}
		c := vid.GetCaption()
		if c != "" {
			return tag + " " + c, tag + " " + c
		}
		return tag, tag
	}

	// 4. Audio / Voice note
	if aud := msg.GetAudioMessage(); aud != nil {
		if aud.GetPTT() {
			secs := aud.GetSeconds()
			if secs > 0 {
				t := fmt.Sprintf("[VOICE NOTE] %ds", secs)
				return t, t
			}
			return "[VOICE NOTE]", "[VOICE NOTE]"
		}
		secs := aud.GetSeconds()
		if secs > 0 {
			t := fmt.Sprintf("[AUDIO] %ds", secs)
			return t, t
		}
		return "[AUDIO]", "[AUDIO]"
	}

	// 5. Document
	if doc := msg.GetDocumentMessage(); doc != nil {
		name := doc.GetFileName()
		if name == "" {
			name = doc.GetTitle()
		}
		if name != "" {
			return "[DOCUMENT] " + name, "[DOCUMENT] " + name
		}
		return "[DOCUMENT]", "[DOCUMENT]"
	}

	// 6. Sticker
	if msg.GetStickerMessage() != nil {
		return "[STICKER]", "[STICKER]"
	}

	// 7. Contact
	if con := msg.GetContactMessage(); con != nil {
		name := con.GetDisplayName()
		if name != "" {
			return "[CONTACT] " + name, "[CONTACT] " + name
		}
		return "[CONTACT]", "[CONTACT]"
	}

	// 8. Location
	if loc := msg.GetLocationMessage(); loc != nil {
		name := loc.GetName()
		if name == "" {
			name = loc.GetAddress()
		}
		if name != "" {
			t := fmt.Sprintf("[LOCATION] %s (%.4f, %.4f)", name, loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
			return t, "[LOCATION] " + name
		}
		t := fmt.Sprintf("[LOCATION] (%.4f, %.4f)", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		return t, t
	}

	// 9. Reaction
	if react := msg.GetReactionMessage(); react != nil {
		emoji := react.GetText()
		if emoji != "" {
			return "[REACTION] " + emoji, "[REACTION] " + emoji
		}
		return "[REACTION]", "[REACTION]"
	}

	// 10. Plain text (fallback — must come last)
	if t := msg.GetConversation(); t != "" {
		return t, truncatePreview(t)
	}

	return "", ""
}

// truncatePreview shortens text for the chat list preview column.
func truncatePreview(s string) string {
	const maxLen = 80
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}

// processIncomingMessage handles all common steps for any received message:
// build Message struct → persist to DB → update PQ → UI update → notification.
func (eh *eventHandler) processIncomingMessage(evt *events.Message, text, preview string) {
	if text == "" {
		return // nothing to process
	}

	chatJID := evt.Info.Chat.String()
	timestamp := uint64(evt.Info.Timestamp.Unix())

	msg := Message{
		Id:           evt.Info.ID,
		ChatId:       chatJID,
		FromMe:       evt.Info.IsFromMe,
		Timestamp:    timestamp,
		Text:         text,
		ContactId:    evt.Info.Sender.String(),
		ContactName:  eh.getContactName(evt.Info.Sender),
		ContactShort: eh.getContactShort(evt.Info.Sender),
	}

	// Persist message
	if err := eh.sm.db.AddMessage(msg); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("failed to save message: %v", err))
	}

	// Resolve chat name BEFORE acquiring the write lock. getChatName()
	// internally calls getClient() which acquires mu.RLock — calling it
	// under mu.Lock would deadlock.
	chatName := eh.sm.getChatName(evt.Info.Chat)

	// Update priority queue and conversation
	eh.sm.mu.Lock()
	conv := eh.sm.convByJID[chatJID]
	var toUpsert Conversation
	if conv != nil {
		conv.LastMsgTime = int64(timestamp)
		conv.Preview = preview
		if !evt.Info.IsFromMe {
			conv.Unread++
		}
		eh.sm.priorityQueue.Update(conv, conv.LastMsgTime, conv.IsPinned)
		toUpsert = *conv
	} else {
		unread := uint16(0)
		if !evt.Info.IsFromMe {
			unread = 1
		}
		newConv := &Conversation{
			JID:         chatJID,
			Name:        chatName,
			LastMsgTime: int64(timestamp),
			Preview:     preview,
			Unread:      unread,
			IsPinned:    false,
		}
		heap.Push(&eh.sm.priorityQueue, newConv)
		eh.sm.convByJID[chatJID] = newConv
		toUpsert = *newConv
	}
	safeList := eh.sm.snapshotPQ()
	eh.sm.mu.Unlock()

	// DB write outside lock
	if err := eh.sm.db.UpsertConversation(toUpsert); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
	}

	// UI: show in current chat or send notification
	eh.sm.mu.RLock()
	isCurrent := chatJID == eh.sm.currentReceiver
	eh.sm.mu.RUnlock()

	if isCurrent {
		eh.sm.uiHandler.NewMessage(msg)
	} else if !evt.Info.IsFromMe {
		if timestamp > uint64(time.Now().Unix()-30) {
			senderName := eh.getContactShort(evt.Info.Sender)
			if senderName == "" {
				senderName = "New Message"
			}
			if err := notify(senderName, text); err != nil {
				eh.sm.uiHandler.PrintError(err)
			}
		}
	}

	// Update chat list ordering
	eh.sm.uiHandler.UpdateChatList(safeList)
}

// Handle incoming messages — dispatches to extractMessageContent then processIncomingMessage.
func (eh *eventHandler) handleMessage(evt *events.Message) {
	text, preview := extractMessageContent(evt.Message)
	eh.processIncomingMessage(evt, text, preview)
}

// Helper to get contact name
func (eh *eventHandler) getContactName(jid types.JID) string {
	if client := eh.sm.getClient(); client != nil && client.Store != nil {
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found && contact.FullName != "" {
			return contact.FullName
		}
	}

	// Fallback to JID User (Phone Number)
	return jid.User
}

// Helper to get short contact name
func (eh *eventHandler) getContactShort(jid types.JID) string {
	if client := eh.sm.getClient(); client != nil && client.Store != nil {
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.Found && contact.PushName != "" {
			return contact.PushName
		}
	}

	// Fallback to JID User (Phone Number)
	return jid.User
}
