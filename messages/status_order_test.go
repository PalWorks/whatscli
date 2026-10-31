// SPDX-License-Identifier: AGPL-3.0-only

package messages

// Tests for ROADMAP row 10: a status printed after an action that also
// redraws the open chat must arrive in the same UI call as the redraw
// (NewScreenWithStatus), never as a separate PrintText that the redraw,
// queued from another goroutine, could wipe.

import (
	"errors"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
)

func noTextContaining(t *testing.T, mock *MockUiHandler, sub string) {
	t.Helper()
	for _, txt := range mock.Texts {
		if strings.Contains(txt, sub) {
			t.Errorf("%q printed separately from the redraw: %v", sub, mock.Texts)
		}
	}
}

func TestApplyRevoke_StatusWithScreen(t *testing.T) {
	tests := []struct {
		name       string
		open       string
		wantScreen bool
	}{
		{"open chat redrawn with status", "c@s.whatsapp.net", true},
		{"other chat open, status printed", "other@s.whatsapp.net", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, mock := newTestSessionManager(t)
			sm.currentReceiver = tt.open
			addTestMsg(t, sm.db, Message{Id: "MINE", ChatId: "c@s.whatsapp.net", FromMe: true, Text: "hi"})

			sm.applyRevoke("c@s.whatsapp.net", "MINE", "revoked: MINE")

			if tt.wantScreen {
				if len(mock.Screens) != 1 || len(mock.ScreenTexts) != 1 || mock.ScreenTexts[0] != "revoked: MINE" {
					t.Fatalf("screens=%d screenTexts=%v", len(mock.Screens), mock.ScreenTexts)
				}
				noTextContaining(t, mock, "revoked")
				return
			}
			if len(mock.Screens) != 0 || len(mock.Texts) != 1 || mock.Texts[0] != "revoked: MINE" {
				t.Fatalf("screens=%d texts=%v", len(mock.Screens), mock.Texts)
			}
		})
	}
}

func TestApplyRevoke_IncomingHasNoStatus(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.currentReceiver = "c@s.whatsapp.net"
	addTestMsg(t, sm.db, Message{Id: "THEIRS", ChatId: "c@s.whatsapp.net", Text: "hi"})

	sm.applyRevoke("c@s.whatsapp.net", "THEIRS", "")

	if len(mock.Screens) != 1 || len(mock.Texts) != 0 {
		t.Fatalf("screens=%d texts=%v", len(mock.Screens), mock.Texts)
	}
}

func TestShowStatus(t *testing.T) {
	boom := errors.New("boom")

	sm, mock := newTestSessionManager(t)
	sm.showStatus("c@s.whatsapp.net", "done", boom, nil)
	if len(mock.Screens) != 1 || len(mock.ScreenTexts) != 1 || mock.ScreenTexts[0] != "done" ||
		len(mock.Errors) != 1 || len(mock.Texts) != 0 {
		t.Fatalf("with refresh: screens=%d screenTexts=%v errors=%v texts=%v",
			len(mock.Screens), mock.ScreenTexts, mock.Errors, mock.Texts)
	}

	sm, mock = newTestSessionManager(t)
	sm.showStatus("", "done", boom, nil)
	if len(mock.Screens) != 0 || len(mock.Errors) != 1 || len(mock.Texts) != 1 || mock.Texts[0] != "done" {
		t.Fatalf("without refresh: screens=%d errors=%v texts=%v", len(mock.Screens), mock.Errors, mock.Texts)
	}
}

func TestRefreshTarget(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.currentReceiver = "a@s.whatsapp.net"
	if got := sm.refreshTarget("a@s.whatsapp.net"); got != "a@s.whatsapp.net" {
		t.Errorf("open chat: got %q", got)
	}
	if got := sm.refreshTarget("b@s.whatsapp.net"); got != "" {
		t.Errorf("other chat: got %q", got)
	}
}

func TestBacklogStatus(t *testing.T) {
	if got := backlogStatus(3, 8); got != "Loaded 5 additional messages" {
		t.Errorf("got %q", got)
	}
	if got := backlogStatus(3, 3); !strings.Contains(got, "none arrived yet") {
		t.Errorf("got %q", got)
	}
}

func TestStoreHistorySync_StatusWithScreen(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.currentReceiver = "bob@s.whatsapp.net"

	sm.storeHistorySync(&waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{historyConv("bob@s.whatsapp.net", "", "H1", "hi")},
	}, testParser)

	if len(mock.Screens) != 1 || len(mock.ScreenTexts) != 1 ||
		!strings.Contains(mock.ScreenTexts[0], "History sync complete") {
		t.Fatalf("open chat must be redrawn with the status: screens=%d screenTexts=%v",
			len(mock.Screens), mock.ScreenTexts)
	}
	noTextContaining(t, mock, "History sync complete")
}

func TestStoreHistorySync_StatusWithoutOpenChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.currentReceiver = "other@s.whatsapp.net"

	sm.storeHistorySync(&waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{historyConv("bob@s.whatsapp.net", "", "H1", "hi")},
	}, testParser)

	if len(mock.Screens) != 0 {
		t.Fatalf("a chat that is not open must not be redrawn: %d screens", len(mock.Screens))
	}
	found := false
	for _, txt := range mock.Texts {
		found = found || strings.Contains(txt, "History sync complete")
	}
	if !found {
		t.Fatalf("status not printed: %v", mock.Texts)
	}
}
