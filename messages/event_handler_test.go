package messages

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// ----- Phase 3.2: handleRevoke tests -----

func TestHandleRevoke_MarksMessageDeleted(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target1", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("target1")},
	}
	eh.handleRevoke(pm)

	msg, err := sm.db.GetMessageByID("target1")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if !msg.Deleted {
		t.Error("expected Deleted=true after handleRevoke")
	}
	if msg.Text != "[DELETED]" {
		t.Errorf("expected text '[DELETED]', got %q", msg.Text)
	}
	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors, got %v", mock.Errors)
	}
}

func TestHandleRevoke_UnknownMessageID_NoOp(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("does-not-exist")},
	}
	// Must not panic and must not report an error for a message we never stored.
	eh.handleRevoke(pm)

	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors for an unknown message ID, got %v", mock.Errors)
	}
}

func TestHandleRevoke_EmptyKeyID_NoOp(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{},
	}
	eh.handleRevoke(pm)

	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors, got %v", mock.Errors)
	}
}

func TestHandleRevoke_RefreshesScreenIfCurrentChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target2", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("target2")},
	}
	eh.handleRevoke(pm)

	if len(mock.Screens) == 0 {
		t.Error("expected NewScreen to be called since the revoked message's chat is currently displayed")
	}
}

func TestHandleRevoke_NoScreenRefreshIfNotCurrentChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target3", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	sm.mu.Lock()
	sm.currentReceiver = "someone-else@s.whatsapp.net"
	sm.mu.Unlock()

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("target3")},
	}
	eh.handleRevoke(pm)

	if len(mock.Screens) != 0 {
		t.Error("expected no NewScreen call since a different chat is displayed")
	}
}

// ----- Phase 3.2: handleEdit tests -----

func TestHandleEdit_UpdatesMessageText(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "edit1", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "original text"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("edit1")},
		EditedMessage: &waE2E.Message{
			Conversation: proto.String("corrected text"),
		},
	}
	eh.handleEdit(pm)

	msg, err := sm.db.GetMessageByID("edit1")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if !strings.Contains(msg.Text, "corrected text") {
		t.Errorf("expected updated text to contain 'corrected text', got %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "(edited)") {
		t.Errorf("expected updated text to be marked '(edited)', got %q", msg.Text)
	}
	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors, got %v", mock.Errors)
	}
}

func TestHandleEdit_UnknownMessageID_NoOp(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	pm := &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto.String("does-not-exist")},
		EditedMessage: &waE2E.Message{Conversation: proto.String("x")},
	}
	eh.handleEdit(pm)

	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors for an unknown message ID, got %v", mock.Errors)
	}
}

func TestHandleEdit_NilEditedMessage_NoOp(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "edit2", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "original"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	pm := &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("edit2")},
		// EditedMessage deliberately nil
	}
	eh.handleEdit(pm)

	msg, err := sm.db.GetMessageByID("edit2")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if msg.Text != "original" {
		t.Errorf("expected text to remain unchanged, got %q", msg.Text)
	}
	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors, got %v", mock.Errors)
	}
}

// ----- Phase 3.2: handleMessage dispatch tests -----

func TestHandleMessage_RevokeDoesNotCreateNewMessage(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target4", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	evt := &events.Message{
		Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type: waE2E.ProtocolMessage_REVOKE.Enum(),
				Key:  &waCommon.MessageKey{ID: proto.String("target4")},
			},
		},
	}
	eh.handleMessage(evt)

	msgs, err := sm.db.GetMessages("peer@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected the revoke to update the existing message, not add a new one; got %d messages", len(msgs))
	}
	if !msgs[0].Deleted {
		t.Error("expected the existing message to be marked deleted")
	}
}

// ----- Phase 5.2: reaction interception tests -----

func reactionEvent(chat, sender, targetID, emoji string) *events.Message {
	chatJID := types.NewJID("", "")
	if chat != "" {
		chatJID, _ = types.ParseJID(chat)
	}
	senderJID := types.NewJID("", "")
	if sender != "" {
		senderJID, _ = types.ParseJID(sender)
	}
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chatJID, Sender: senderJID},
		},
		Message: &waE2E.Message{
			ReactionMessage: &waE2E.ReactionMessage{
				Key:  &waCommon.MessageKey{ID: proto.String(targetID)},
				Text: proto.String(emoji),
			},
		},
	}
}

func TestHandleReaction_RecordsReaction(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target5", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target5", "👍"))

	msg, err := sm.db.GetMessageByID("target5")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if len(msg.Reactions) != 1 || msg.Reactions[0].Emoji != "👍" || msg.Reactions[0].Count != 1 {
		t.Errorf("expected one 👍 reaction, got %+v", msg.Reactions)
	}
	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors, got %v", mock.Errors)
	}
}

func TestHandleMessage_ReactionDoesNotCreateNewMessage(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target6", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target6", "❤️"))

	msgs, err := sm.db.GetMessages("peer@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected the reaction to attach to the existing message, not add a new one; got %d messages", len(msgs))
	}
}

func TestHandleReaction_EmptyTargetID_NoOp(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "", "👍"))

	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors for an empty target ID, got %v", mock.Errors)
	}
}

func TestHandleReaction_EmptyEmojiRemovesExistingReaction(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target7", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target7", "👍"))
	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target7", ""))

	msg, err := sm.db.GetMessageByID("target7")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if len(msg.Reactions) != 0 {
		t.Errorf("expected the reaction to be removed, got %+v", msg.Reactions)
	}
}

func TestHandleReaction_SecondReactionFromSameSenderReplacesFirst(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}

	if err := sm.db.AddMessage(Message{Id: "target8", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target8", "👍"))
	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target8", "😂"))

	msg, err := sm.db.GetMessageByID("target8")
	if err != nil {
		t.Fatalf("GetMessageByID error: %v", err)
	}
	if len(msg.Reactions) != 1 || msg.Reactions[0].Emoji != "😂" {
		t.Errorf("expected only the replacement 😂 reaction, got %+v", msg.Reactions)
	}
}

func TestHandleReaction_RefreshesScreenIfCurrentChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	if err := sm.db.AddMessage(Message{Id: "target9", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target9", "👍"))

	if len(mock.Screens) == 0 {
		t.Error("expected NewScreen to be called since the reacted-to message's chat is currently displayed")
	}
}

func TestHandleReaction_NoScreenRefreshIfNotCurrentChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}
	sm.mu.Lock()
	sm.currentReceiver = "someone-else@s.whatsapp.net"
	sm.mu.Unlock()

	if err := sm.db.AddMessage(Message{Id: "target10", ChatId: "peer@s.whatsapp.net", Timestamp: 1000, Text: "hello"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	eh.handleMessage(reactionEvent("peer@s.whatsapp.net", "reactor@s.whatsapp.net", "target10", "👍"))

	if len(mock.Screens) != 0 {
		t.Error("expected no NewScreen call for a chat that isn't currently displayed")
	}
}
