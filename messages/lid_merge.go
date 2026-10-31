// SPDX-License-Identifier: AGPL-3.0-only

package messages

// lid_merge.go: R1 (ROADMAP row 13). A chat stored under its @lid JID before
// the LID to phone-number mapping was known shows up as a second chat once new
// messages arrive under the phone-number JID. When the mapping becomes known,
// the @lid chat is merged into the phone-number chat: its messages move over,
// the two conversation rows become one, and the in-memory chat list follows.
// Message sender IDs (ContactId) are left as WhatsApp sent them, because they
// are sent back as the participant in read receipts and backlog anchors.

import (
	"container/heap"
	"fmt"
	"math"

	"go.mau.fi/whatsmeow/types"
)

// MergeChat moves every message of fromChat to conv.JID and replaces the
// fromChat conversation row with conv, in one transaction. Message IDs are
// the primary key, so a message can never exist in both chats.
func (md *MessageDatabase) MergeChat(fromChat string, conv Conversation) error {
	if md.db == nil {
		return fmt.Errorf("database not initialized")
	}
	tx, err := md.db.Begin()
	if err != nil {
		return fmt.Errorf("begin chat merge: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE messages SET chat_id = ? WHERE chat_id = ?`, conv.JID, fromChat); err != nil {
		return fmt.Errorf("move messages: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM conversations WHERE jid = ?`, fromChat); err != nil {
		return fmt.Errorf("delete merged conversation: %w", err)
	}
	if _, err := tx.Exec(`
	INSERT INTO conversations (jid, name, last_msg_time, preview, unread, is_pinned, is_archived)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(jid) DO UPDATE SET
		name=excluded.name,
		last_msg_time=excluded.last_msg_time,
		preview=excluded.preview,
		unread=excluded.unread,
		is_pinned=excluded.is_pinned,
		is_archived=excluded.is_archived;
	`, conv.JID, conv.Name, conv.LastMsgTime, conv.Preview, conv.Unread, conv.IsPinned, conv.IsArchived); err != nil {
		return fmt.Errorf("write merged conversation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chat merge: %w", err)
	}
	return nil
}

// mergedConversation combines the @lid conversation lid with the existing
// phone-number conversation pn (nil when there is none) under pnJID.
// A real name beats a bare number, the newer message wins the preview, unread
// counts add up, the chat is pinned if either was, and archived only if every
// existing side was (a merge never hides a chat).
func mergedConversation(pnJID types.JID, lid, pn *Conversation) Conversation {
	out := *lid
	out.JID = pnJID.String()
	out.Index = 0
	if isBareNumberName(out.Name) {
		out.Name = pnJID.User
	}
	if pn == nil {
		return out
	}
	if !isBareNumberName(pn.Name) || isBareNumberName(out.Name) {
		out.Name = pn.Name
	}
	if pn.LastMsgTime >= lid.LastMsgTime {
		out.LastMsgTime = pn.LastMsgTime
		out.Preview = pn.Preview
	}
	out.Unread = uint16(min(int(pn.Unread)+int(lid.Unread), math.MaxUint16))
	out.IsPinned = pn.IsPinned || lid.IsPinned
	out.IsArchived = pn.IsArchived && lid.IsArchived
	return out
}

// isBareNumberName reports whether a chat name is empty or only digits, which
// is what history sync stores when it knows no name.
func isBareNumberName(name string) bool {
	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// mergeKnownLIDChats merges every stored @lid chat whose phone-number JID is
// now known. It is safe to call repeatedly; chats without a mapping stay.
// It returns the number of chats merged and, if the open chat was merged,
// the chat it moved to. The caller redraws that chat together with its own
// status (mergedStatus), so the redraw cannot wipe it (ROADMAP row 10).
func (sm *SessionManager) mergeKnownLIDChats() (merged int, movedOpenChat string) {
	if sm.lidMap() == nil {
		return 0, ""
	}
	sm.mu.RLock()
	var lids []string
	for jid := range sm.convByJID {
		if parsed, err := types.ParseJID(jid); err == nil && parsed.Server == types.HiddenUserServer {
			lids = append(lids, jid)
		}
	}
	sm.mu.RUnlock()

	for _, lidStr := range lids {
		lid, _ := types.ParseJID(lidStr)
		pn := sm.canonicalUserJID(lid) // LID store lookup; never under sm.mu
		if pn.Server != types.DefaultUserServer {
			continue
		}
		ok, wasOpen := sm.mergeLIDChat(lidStr, pn)
		if ok {
			merged++
		}
		if wasOpen {
			movedOpenChat = pn.String()
		}
	}
	if merged == 0 {
		return 0, ""
	}
	sm.mu.Lock()
	safeList := sm.snapshotPQ()
	sm.mu.Unlock()
	sm.uiHandler.UpdateChatList(safeList)
	return merged, movedOpenChat
}

// mergedStatus appends the result of mergeKnownLIDChats to status.
func mergedStatus(status string, merged int) string {
	if merged == 0 {
		return status
	}
	return status + fmt.Sprintf("\nMerged %d chat(s) stored under a LID into their phone-number chat.", merged)
}

// mergeLIDChat merges the conversation lidStr into pn in SQLite and in the
// chat list, and moves the open chat over if it was the merged one (wasOpen
// reports that, so the caller can redraw it). The DB
// write happens outside sm.mu; the in-memory state is merged afterwards from
// its then-current values.
func (sm *SessionManager) mergeLIDChat(lidStr string, pn types.JID) (merged, wasOpen bool) {
	pnStr := pn.String()
	sm.mu.RLock()
	lidConv := sm.convByJID[lidStr]
	var conv Conversation
	if lidConv != nil {
		conv = mergedConversation(pn, lidConv, sm.convByJID[pnStr])
	}
	sm.mu.RUnlock()
	if lidConv == nil {
		return false, false
	}

	if err := sm.db.MergeChat(lidStr, conv); err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("merge chat %s into %s: %w", lidStr, pnStr, err))
		return false, false
	}

	sm.mu.Lock()
	lidConv = sm.convByJID[lidStr]
	if lidConv == nil {
		sm.mu.Unlock()
		return false, false
	}
	pnConv := sm.convByJID[pnStr]
	final := mergedConversation(pn, lidConv, pnConv)
	heap.Remove(&sm.priorityQueue, lidConv.Index)
	delete(sm.convByJID, lidStr)
	if pnConv == nil {
		pnConv = &Conversation{}
		*pnConv = final
		heap.Push(&sm.priorityQueue, pnConv)
		sm.convByJID[pnStr] = pnConv
	} else {
		idx := pnConv.Index
		*pnConv = final
		pnConv.Index = idx
		heap.Fix(&sm.priorityQueue, idx)
	}
	wasOpen = sm.currentReceiver == lidStr
	if wasOpen {
		sm.currentReceiver = pnStr
	}
	sm.mu.Unlock()

	// Events between the two locks may have changed either row; store the
	// final state (a no-op when nothing changed).
	if final != conv {
		if err := sm.db.UpsertConversation(final); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %w", err))
		}
	}
	return true, wasOpen
}
