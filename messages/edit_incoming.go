// SPDX-License-Identifier: AGPL-3.0-only

package messages

// edit_incoming.go: message edits. WhatsApp sends an edit as a MESSAGE_EDIT
// protocol message carrying the original message's ID and the new content.
// An edit is applied only in the original message's chat and only when it
// comes from the original sender (our own devices for our messages), the same
// rules as a revoke by the sender. A revoked message is never edited back.

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// EditedMark is appended to the text of an edited message.
const EditedMark = "(edited)"

// handleIncomingEdit checks and applies an edit of message targetID.
func (sm *SessionManager) handleIncomingEdit(src types.MessageSource, targetID string, edited *waE2E.Message) {
	target, found, err := sm.db.GetMessage(targetID)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !found || !sm.editAllowed(src, target) {
		return
	}
	text, _ := extractMessageContent(edited)
	if text == "" {
		return
	}
	updated, err := sm.db.UpdateMessageText(target.ChatId, target.Id, text+" "+EditedMark)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if updated {
		if chat := sm.refreshTarget(target.ChatId); chat != "" {
			sm.uiHandler.NewScreen(sm.getMessages(chat))
		}
	}
}

// editAllowed reports whether src may edit the stored target: same chat, and
// the editor is the target's sender. Never call it under sm.mu.
func (sm *SessionManager) editAllowed(src types.MessageSource, target Message) bool {
	if !sm.revokeChatMatches(src, target.ChatId) {
		return false
	}
	if src.IsFromMe {
		return target.FromMe
	}
	if target.FromMe {
		return false
	}
	stored, err := types.ParseJID(target.ContactId)
	return err == nil && (sm.sameUser(src.Sender, stored) || sm.sameUser(src.SenderAlt, stored))
}
