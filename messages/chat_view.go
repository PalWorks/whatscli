// SPDX-License-Identifier: AGPL-3.0-only

package messages

// chat_view.go: the open chat shows its newest messages, not the whole
// history (ROADMAP row 34). The view is "every stored message at or after
// viewAnchor"; opening a chat sets the anchor to the oldest of the newest
// chatPageSize messages, /more moves it back by morePageSize, and older
// messages stored by a history sync or backlog lower it so they appear. New
// messages are newer than the anchor, so they always show.

import (
	"fmt"
	"math"
)

const (
	chatPageSize = 200 // messages shown when a chat is opened
	morePageSize = 200 // older messages added by /more
)

// pageAnchor returns the timestamp of the n-th stored message older than
// before, so that showing messages at or after it adds n older ones. It
// returns 0 (show everything) when fewer than n older messages exist.
func (sm *SessionManager) pageAnchor(chat string, before uint64, n int) (uint64, error) {
	return sm.db.NthTimestampBefore(chat, before, n)
}

// openViewAnchor is the anchor for a freshly opened chat.
func (sm *SessionManager) openViewAnchor(chat string) uint64 {
	anchor, err := sm.pageAnchor(chat, math.MaxInt64, chatPageSize)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to page messages: %v", err))
		return 0
	}
	return anchor
}

// lowerViewAnchor makes messages from ts onwards visible if chat is open.
func (sm *SessionManager) lowerViewAnchor(chat string, ts uint64) {
	sm.mu.Lock()
	if sm.currentReceiver == chat && ts < sm.viewAnchor {
		sm.viewAnchor = ts
	}
	sm.mu.Unlock()
}
