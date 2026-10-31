// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
)

const pageChat = "alice@s.whatsapp.net"

// seedPagedChat stores n messages M1..Mn with timestamps 1..n.
func seedPagedChat(t *testing.T, sm *SessionManager, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		seedMsg(t, sm, fmt.Sprintf("M%d", i), pageChat, uint64(i))
	}
}

func lastScreen(t *testing.T, mock *MockUiHandler) []Message {
	t.Helper()
	if len(mock.Screens) == 0 {
		t.Fatal("no screen drawn")
	}
	return mock.Screens[len(mock.Screens)-1]
}

func TestOpenChat_ShowsNewestPage(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	seedPagedChat(t, sm, chatPageSize+50)

	sm.setCurrentReceiver(pageChat)

	screen := lastScreen(t, mock)
	if len(screen) != chatPageSize {
		t.Fatalf("shown %d messages, want %d", len(screen), chatPageSize)
	}
	if screen[0].Id != "M51" || screen[len(screen)-1].Id != fmt.Sprintf("M%d", chatPageSize+50) {
		t.Errorf("window %s..%s, want the newest page", screen[0].Id, screen[len(screen)-1].Id)
	}
}

func TestOpenChat_ShortChatShowsAll(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	seedPagedChat(t, sm, 30)

	sm.setCurrentReceiver(pageChat)

	if n := len(lastScreen(t, mock)); n != 30 {
		t.Errorf("shown %d messages, want 30", n)
	}
	if sm.viewAnchor != 0 {
		t.Errorf("anchor %d, want 0 (everything shown)", sm.viewAnchor)
	}
}

func TestCmdMore_PagesOlderThenReportsAllShown(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	total := chatPageSize + morePageSize + 10
	seedPagedChat(t, sm, total)
	sm.setCurrentReceiver(pageChat)

	cmdMore(sm, nil, "more", nil)
	if n := len(lastScreen(t, mock)); n != chatPageSize+morePageSize {
		t.Fatalf("after one /more shown %d, want %d", n, chatPageSize+morePageSize)
	}
	want := fmt.Sprintf("Showing %d of %d stored messages", chatPageSize+morePageSize, total)
	if got := mock.ScreenTexts[len(mock.ScreenTexts)-1]; got != want {
		t.Errorf("status %q, want %q", got, want)
	}

	cmdMore(sm, nil, "more", nil)
	if n := len(lastScreen(t, mock)); n != total {
		t.Fatalf("after two /more shown %d, want %d", n, total)
	}

	screens := len(mock.Screens)
	cmdMore(sm, nil, "more", nil)
	if len(mock.Screens) != screens {
		t.Error("redrew although everything was shown")
	}
	if got := mock.Texts[len(mock.Texts)-1]; !strings.HasPrefix(got, fmt.Sprintf("All %d stored messages are shown", total)) {
		t.Errorf("text %q", got)
	}
}

func TestCmdMore_NoChat(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	cmdMore(sm, nil, "more", nil)
	if len(mock.Screens) != 0 {
		t.Error("redrew without an open chat")
	}
}

func TestNewMessagesStayVisibleAfterOpen(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	seedPagedChat(t, sm, chatPageSize+5)
	sm.setCurrentReceiver(pageChat)

	seedMsg(t, sm, "NEW", pageChat, 10_000)
	screen := sm.getMessages(pageChat)
	if len(screen) != chatPageSize+1 || screen[len(screen)-1].Id != "NEW" {
		t.Errorf("new message not appended to the window (%d shown)", len(screen))
	}
}

func TestHistorySync_OlderMessagesInOpenChatBecomeVisible(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	// Timestamps 1001.. so the history message (500) is older than all.
	for i := 1; i <= chatPageSize+5; i++ {
		seedMsg(t, sm, fmt.Sprintf("M%d", i), pageChat, uint64(1000+i))
	}
	sm.setCurrentReceiver(pageChat)

	sm.storeHistorySync(&waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{historyConv(pageChat, "", "OLD1", "old")},
	}, testParser)

	screen := lastScreen(t, mock)
	if screen[0].Id != "OLD1" {
		t.Errorf("oldest shown %s, want the backlog message OLD1", screen[0].Id)
	}
	if len(screen) != chatPageSize+6 {
		t.Errorf("shown %d, want all %d", len(screen), chatPageSize+6)
	}
}

func TestNthTimestampBefore(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	seedPagedChat(t, sm, 10)
	tests := []struct {
		before uint64
		n      int
		want   uint64
	}{
		{100, 3, 8},
		{8, 3, 5},
		{100, 10, 1},
		{100, 11, 0},
		{3, 5, 0},
	}
	for _, tt := range tests {
		got, err := sm.db.NthTimestampBefore(pageChat, tt.before, tt.n)
		if err != nil || got != tt.want {
			t.Errorf("NthTimestampBefore(%d, %d) = %d, %v; want %d", tt.before, tt.n, got, err, tt.want)
		}
	}
}
