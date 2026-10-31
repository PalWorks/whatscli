// SPDX-License-Identifier: AGPL-3.0-only

package messages

// delete_for_me.go: "Delete for me" done on the phone. WhatsApp delivers it as
// app state (whatsmeow events.DeleteForMe), not as a message. It is the user's
// own local delete, so the stored row is removed (unlike a delete-for-everyone
// revoke, which leaves a placeholder). whatsmeow fetches app state after
// connecting and emits the event for incremental patches; events replayed by a
// full sync are suppressed unless Client.EmitAppStateEventsOnFullSync is set,
// which is deliberately left off.

import (
	"fmt"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// emptyChatPreview is the chat list preview of a chat with no stored message.
const emptyChatPreview = "New chat"

// handleDeleteForMe removes the stored message of a DeleteForMe event.
func (sm *SessionManager) handleDeleteForMe(evt *events.DeleteForMe) {
	if evt.MessageID == "" {
		return
	}
	target, found, err := sm.db.GetMessage(evt.MessageID)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !found {
		return
	}
	// The chat may be @lid in the event and phone-number in storage or the
	// reverse; the message must belong to that chat in either form.
	src := types.MessageSource{Chat: evt.ChatJID, IsFromMe: evt.IsFromMe}
	if !sm.revokeChatMatches(src, target.ChatId) {
		return
	}
	deleted, err := sm.db.DeleteMessage(target.ChatId, target.Id)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !deleted {
		return
	}
	sm.refreshPreviewAfterDelete(target)

	sm.mu.RLock()
	isCurrent := sm.currentReceiver == target.ChatId
	sm.mu.RUnlock()
	if isCurrent {
		sm.uiHandler.NewScreen(sm.getMessages(target.ChatId))
	}
}

// refreshPreviewAfterDelete points the chat's last message and preview at the
// newest remaining message when the deleted one was the latest. DB reads and
// writes happen outside sm.mu.
func (sm *SessionManager) refreshPreviewAfterDelete(deleted Message) {
	latest, found, err := sm.db.GetLatestMessage(deleted.ChatId)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}

	sm.mu.Lock()
	conv := sm.convByJID[deleted.ChatId]
	if conv == nil || int64(deleted.Timestamp) < conv.LastMsgTime {
		sm.mu.Unlock()
		return // chat not listed, or an older message was deleted
	}
	if found {
		conv.LastMsgTime = int64(latest.Timestamp)
		conv.Preview = truncatePreview(latest.Text)
	} else {
		conv.Preview = emptyChatPreview // keep the time: the chat itself stays
	}
	sm.priorityQueue.Update(conv, conv.LastMsgTime, conv.IsPinned)
	toUpsert := *conv
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()

	if err := sm.db.UpsertConversation(toUpsert); err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
	}
	sm.uiHandler.UpdateChatList(safeList)
}
