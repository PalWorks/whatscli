package messages

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

// ----- Phase 3.4: deterministic backlog completion tests -----

// TestProcessHistorySync_SignalsCompletionEvenOnEarlyReturn verifies that
// the historySyncDone completion signal fires regardless of which guard
// causes processHistorySync to return early (here: no client set, so it
// hits the "not connected" guard). This is what lets cmdBacklog's wait
// resolve promptly instead of blocking for its full fallback timeout.
func TestProcessHistorySync_SignalsCompletionEvenOnEarlyReturn(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	// No client set: sm.getClient() returns nil, so processHistorySync
	// hits its "not connected" guard and returns immediately.

	sm.processHistorySync(&waHistorySync.HistorySync{})

	select {
	case <-sm.historySyncDone:
		// signaled, as expected
	default:
		t.Fatal("expected historySyncDone to be signaled even on early return")
	}
}

// TestHistorySyncDone_UnblocksWaitingGoroutine exercises the channel
// mechanics cmdBacklog relies on directly: historySyncDone is buffered
// (size 1), so a signal sent before a waiting goroutine even starts its
// select is not lost — the goroutine unblocks as soon as it runs.
func TestHistorySyncDone_UnblocksWaitingGoroutine(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	select {
	case sm.historySyncDone <- struct{}{}:
	default:
		t.Fatal("failed to send on historySyncDone (buffer should have room)")
	}

	unblocked := make(chan struct{})
	go func() {
		select {
		case <-sm.historySyncDone:
		case <-time.After(5 * time.Second):
			t.Error("timed out waiting for historySyncDone")
		}
		close(unblocked)
	}()

	select {
	case <-unblocked:
		// unblocked promptly, as expected
	case <-time.After(1 * time.Second):
		t.Fatal("waiting goroutine did not unblock promptly after signal")
	}
}

// TestHistorySyncDone_NonBlockingSendNeverPanics verifies the send-with-
// default pattern used by processHistorySync's defer never blocks or
// panics even when the buffer is already full (no listener has drained it).
func TestHistorySyncDone_NonBlockingSendNeverPanics(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	for i := 0; i < 3; i++ {
		done := make(chan struct{})
		go func() {
			select {
			case sm.historySyncDone <- struct{}{}:
			default:
			}
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatalf("non-blocking send #%d did not return promptly", i)
		}
	}
}

// ----- Phase 5.6: status@broadcast is no longer skipped -----

func TestIsSyncableChatJID_Contact(t *testing.T) {
	jid, err := types.ParseJID("123@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ParseJID error: %v", err)
	}
	if !isSyncableChatJID(jid) {
		t.Error("expected a contact JID to be syncable")
	}
}

func TestIsSyncableChatJID_Group(t *testing.T) {
	jid, err := types.ParseJID("123456@g.us")
	if err != nil {
		t.Fatalf("ParseJID error: %v", err)
	}
	if !isSyncableChatJID(jid) {
		t.Error("expected a group JID to be syncable")
	}
}

func TestIsSyncableChatJID_StatusBroadcast(t *testing.T) {
	if !isSyncableChatJID(types.StatusBroadcastJID) {
		t.Error("expected status@broadcast to be syncable (Phase 5.6: viewed read-only, no longer skipped)")
	}
}

func TestIsSyncableChatJID_OtherBroadcastSkipped(t *testing.T) {
	// A broadcast list JID that is NOT the status broadcast (e.g. a
	// newsletter or ordinary broadcast list) must still be skipped — only
	// status@broadcast is special-cased.
	jid := types.NewJID("1234567890", "broadcast")
	if isSyncableChatJID(jid) {
		t.Error("expected a non-status broadcast JID to remain skipped")
	}
}
