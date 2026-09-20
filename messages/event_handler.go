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
		go eh.sm.processHistorySync(v.Data)
	case *events.MarkChatAsRead:
		// Emitted when the whole chat is marked read/unread from another
		// device (e.g. reading it on the phone). Only clear the local
		// unread badge on a "read" action, not "unread".
		if v.Action.GetRead() {
			eh.sm.clearUnread(v.JID.String())
		}
	case *events.Receipt:
		eh.handleReceipt(v)
	}
}

// handleReceipt persists per-message delivery state (plan H4) and refreshes
// the message view if the receipt's chat is currently displayed. Runs on
// whatsmeow's event goroutine, like every other case in Handle. Only
// delivered/read receipts map to a display state; sender/retry/played
// receipts are ignored. events.MarkChatAsRead separately covers the
// conversation-level unread badge.
func (eh *eventHandler) handleReceipt(evt *events.Receipt) {
	var status int
	switch evt.Type {
	case types.ReceiptTypeDelivered:
		status = ReceiptDelivered
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		status = ReceiptRead
	default:
		return
	}

	if len(evt.MessageIDs) == 0 {
		return
	}
	if err := eh.sm.db.SetReceiptStatus(evt.MessageIDs, status); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("failed to store receipt: %v", err))
		return
	}

	chat := evt.Chat.String()
	eh.sm.mu.RLock()
	isCurrent := eh.sm.currentReceiver == chat
	eh.sm.mu.RUnlock()
	if isCurrent {
		eh.sm.uiHandler.NewScreen(eh.sm.getMessages(chat))
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

// mediaInfo carries the media-specific fields extracted from a message
// proto, used to populate Message.MediaType/Mimetype/MediaKey/etc. so a
// later /download can call client.Download without re-parsing the proto.
type mediaInfo struct {
	mediaType     string
	mimetype      string
	mediaKey      []byte
	directPath    string
	fileSHA256    []byte
	fileEncSHA256 []byte
	fileLength    uint64
	fileName      string
}

// extractMediaInfo extracts media metadata from a whatsmeow Message proto.
// The second return value is false for messages that carry no media
// attachment (plain text, contact, location, reaction, etc.). This is a
// pure function with no side effects, making it easy to test.
func extractMediaInfo(msg *waE2E.Message) (mediaInfo, bool) {
	if msg == nil {
		return mediaInfo{}, false
	}

	if img := msg.GetImageMessage(); img != nil {
		return mediaInfo{
			mediaType:     "image",
			mimetype:      img.GetMimetype(),
			mediaKey:      img.GetMediaKey(),
			directPath:    img.GetDirectPath(),
			fileSHA256:    img.GetFileSHA256(),
			fileEncSHA256: img.GetFileEncSHA256(),
			fileLength:    img.GetFileLength(),
		}, true
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return mediaInfo{
			mediaType:     "video",
			mimetype:      vid.GetMimetype(),
			mediaKey:      vid.GetMediaKey(),
			directPath:    vid.GetDirectPath(),
			fileSHA256:    vid.GetFileSHA256(),
			fileEncSHA256: vid.GetFileEncSHA256(),
			fileLength:    vid.GetFileLength(),
		}, true
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return mediaInfo{
			mediaType:     "audio",
			mimetype:      aud.GetMimetype(),
			mediaKey:      aud.GetMediaKey(),
			directPath:    aud.GetDirectPath(),
			fileSHA256:    aud.GetFileSHA256(),
			fileEncSHA256: aud.GetFileEncSHA256(),
			fileLength:    aud.GetFileLength(),
		}, true
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		name := doc.GetFileName()
		if name == "" {
			name = doc.GetTitle()
		}
		return mediaInfo{
			mediaType:     "document",
			mimetype:      doc.GetMimetype(),
			mediaKey:      doc.GetMediaKey(),
			directPath:    doc.GetDirectPath(),
			fileSHA256:    doc.GetFileSHA256(),
			fileEncSHA256: doc.GetFileEncSHA256(),
			fileLength:    doc.GetFileLength(),
			fileName:      name,
		}, true
	}
	if sticker := msg.GetStickerMessage(); sticker != nil {
		return mediaInfo{
			mediaType:     "sticker",
			mimetype:      sticker.GetMimetype(),
			mediaKey:      sticker.GetMediaKey(),
			directPath:    sticker.GetDirectPath(),
			fileSHA256:    sticker.GetFileSHA256(),
			fileEncSHA256: sticker.GetFileEncSHA256(),
			fileLength:    sticker.GetFileLength(),
		}, true
	}

	return mediaInfo{}, false
}

// extractForwarded reports whether msg was forwarded, by checking the
// ContextInfo of whichever sub-message is present (nested GetContextInfo/
// GetIsForwarded calls are nil-safe protobuf getters, so this never panics
// even when a message has no context info). Checked in the same priority
// order as extractMessageContent.
func extractForwarded(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo().GetIsForwarded()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetContextInfo().GetIsForwarded()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo().GetIsForwarded()
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo().GetIsForwarded()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo().GetIsForwarded()
	}
	if sticker := msg.GetStickerMessage(); sticker != nil {
		return sticker.GetContextInfo().GetIsForwarded()
	}
	// Contact and location messages (plan H3.2): exactly the remaining types
	// extractMessageContent renders — parity rule, no more, no fewer.
	if con := msg.GetContactMessage(); con != nil {
		return con.GetContextInfo().GetIsForwarded()
	}
	if loc := msg.GetLocationMessage(); loc != nil {
		return loc.GetContextInfo().GetIsForwarded()
	}
	return false
}

// extractQuotedID returns the message ID this message is replying to, read
// from the ContextInfo of whichever sub-message is present, or "" if it isn't
// a reply. Checked in the same priority order as extractMessageContent.
func extractQuotedID(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo().GetStanzaID()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetContextInfo().GetStanzaID()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo().GetStanzaID()
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo().GetStanzaID()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo().GetStanzaID()
	}
	if sticker := msg.GetStickerMessage(); sticker != nil {
		return sticker.GetContextInfo().GetStanzaID()
	}
	return ""
}

// extractEmbeddedQuotePreview returns a short preview of the quoted message
// content embedded directly in this message's ContextInfo (senders commonly
// include it so other clients can render the reply without a lookup of their
// own). Returns "" if there is no embedded quote — the caller is expected to
// fall back to a local DB lookup by the ID from extractQuotedID in that case.
func extractEmbeddedQuotePreview(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	var quoted *waE2E.Message
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		quoted = ext.GetContextInfo().GetQuotedMessage()
	} else if img := msg.GetImageMessage(); img != nil {
		quoted = img.GetContextInfo().GetQuotedMessage()
	} else if vid := msg.GetVideoMessage(); vid != nil {
		quoted = vid.GetContextInfo().GetQuotedMessage()
	} else if aud := msg.GetAudioMessage(); aud != nil {
		quoted = aud.GetContextInfo().GetQuotedMessage()
	} else if doc := msg.GetDocumentMessage(); doc != nil {
		quoted = doc.GetContextInfo().GetQuotedMessage()
	} else if sticker := msg.GetStickerMessage(); sticker != nil {
		quoted = sticker.GetContextInfo().GetQuotedMessage()
	}
	if quoted == nil {
		return ""
	}
	_, preview := extractMessageContent(quoted)
	return preview
}

// resolveQuote determines the QuotedID/QuotedPreview pair for a received
// message: if it's a reply, prefer a preview derived from our own local copy
// of the quoted message (accurate, reflects e.g. a since-edited or deleted
// state), falling back to whatever preview the sender embedded, and finally a
// generic placeholder if neither is available (the quoted message is neither
// stored locally nor embedded — e.g. it predates this session and history
// hasn't synced it yet).
func resolveQuote(db *MessageDatabase, msg *waE2E.Message) (quotedID, quotedPreview string) {
	quotedID = extractQuotedID(msg)
	if quotedID == "" {
		return "", ""
	}
	if local, err := db.GetMessageByID(quotedID); err == nil {
		return quotedID, truncatePreview(local.Text)
	}
	if embedded := extractEmbeddedQuotePreview(msg); embedded != "" {
		return quotedID, embedded
	}
	return quotedID, "[message]"
}

// processIncomingMessage handles all common steps for any received message:
// build Message struct → persist to DB → update PQ → UI update → notification.
func (eh *eventHandler) processIncomingMessage(evt *events.Message, text, preview string) {
	if text == "" {
		return // nothing to process
	}

	chatJID := evt.Info.Chat.String()
	timestamp := uint64(evt.Info.Timestamp.Unix())
	media, _ := extractMediaInfo(evt.Message)
	quotedID, quotedPreview := resolveQuote(eh.sm.db, evt.Message)

	msg := Message{
		Id:            evt.Info.ID,
		ChatId:        chatJID,
		FromMe:        evt.Info.IsFromMe,
		Timestamp:     timestamp,
		Text:          text,
		ContactId:     evt.Info.Sender.String(),
		ContactName:   eh.getContactName(evt.Info.Sender),
		ContactShort:  eh.getContactShort(evt.Info.Sender),
		MediaType:     media.mediaType,
		Mimetype:      media.mimetype,
		MediaKey:      media.mediaKey,
		DirectPath:    media.directPath,
		FileSHA256:    media.fileSHA256,
		FileEncSHA256: media.fileEncSHA256,
		FileLength:    media.fileLength,
		FileName:      media.fileName,
		Forwarded:     extractForwarded(evt.Message),
		QuotedID:      quotedID,
		QuotedPreview: quotedPreview,
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
// Reactions, revokes, and edits are intercepted first since they mutate
// existing state (a reactions row, or an existing stored message) rather
// than adding a new message of their own.
func (eh *eventHandler) handleMessage(evt *events.Message) {
	if react := evt.Message.GetReactionMessage(); react != nil {
		eh.handleReaction(evt, react)
		return
	}

	if proto := evt.Message.GetProtocolMessage(); proto != nil {
		switch proto.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			eh.handleRevoke(proto)
			return
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			eh.handleEdit(proto)
			return
		}
	}

	text, preview := extractMessageContent(evt.Message)
	eh.processIncomingMessage(evt, text, preview)
}

// handleReaction records a reaction (or its removal, signaled by an empty
// emoji) against the target message and refreshes the screen if that
// message's chat is currently displayed. Reactions are stored in their own
// table rather than as standalone messages (Phase 5.2) — this is why they are
// intercepted here rather than falling through to processIncomingMessage.
func (eh *eventHandler) handleReaction(evt *events.Message, react *waE2E.ReactionMessage) {
	targetID := react.GetKey().GetID()
	if targetID == "" {
		return
	}

	sender := evt.Info.Sender.String()
	if err := eh.sm.db.SetReaction(targetID, sender, react.GetText()); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("failed to store reaction: %v", err))
		return
	}

	eh.sm.mu.RLock()
	isCurrent := eh.sm.currentReceiver == evt.Info.Chat.String()
	eh.sm.mu.RUnlock()
	if isCurrent {
		eh.sm.uiHandler.NewScreen(eh.sm.getMessages(evt.Info.Chat.String()))
	}
}

// handleRevoke marks the target message deleted when a revoke protocol
// message is received (from any participant, not just the local user — this
// mirrors what the server/other client already did), and refreshes the
// screen if that message's chat is currently displayed.
func (eh *eventHandler) handleRevoke(proto *waE2E.ProtocolMessage) {
	targetID := proto.GetKey().GetID()
	if targetID == "" {
		return
	}

	msg, err := eh.sm.db.GetMessageByID(targetID)
	if err != nil {
		// Message not locally known (e.g. it arrived before this session
		// started, or history hasn't synced it yet) — nothing to update.
		return
	}

	if err := eh.sm.db.MarkMessageDeleted(targetID); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("failed to mark message deleted: %v", err))
		return
	}

	eh.sm.mu.RLock()
	isCurrent := eh.sm.currentReceiver == msg.ChatId
	eh.sm.mu.RUnlock()
	if isCurrent {
		eh.sm.uiHandler.NewScreen(eh.sm.getMessages(msg.ChatId))
	}
}

// handleEdit updates the stored text for a message when an edit protocol
// message is received, appending an "(edited)" marker so the change is
// visible in history.
func (eh *eventHandler) handleEdit(proto *waE2E.ProtocolMessage) {
	targetID := proto.GetKey().GetID()
	edited := proto.GetEditedMessage()
	if targetID == "" || edited == nil {
		return
	}

	msg, err := eh.sm.db.GetMessageByID(targetID)
	if err != nil {
		return
	}

	newText, _ := extractMessageContent(edited)
	if newText == "" {
		return
	}

	if err := eh.sm.db.UpdateMessageText(targetID, newText+" (edited)"); err != nil {
		eh.sm.uiHandler.PrintError(fmt.Errorf("failed to update edited message: %v", err))
		return
	}

	eh.sm.mu.RLock()
	isCurrent := eh.sm.currentReceiver == msg.ChatId
	eh.sm.mu.RUnlock()
	if isCurrent {
		eh.sm.uiHandler.NewScreen(eh.sm.getMessages(msg.ChatId))
	}
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
