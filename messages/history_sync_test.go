package messages

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

// ----- Phase 3.4 / H3.1: deterministic backlog completion tests -----

// TestProcessHistorySync_NotifiesWaitersEvenOnEarlyReturn verifies that
// registered sync waiters are woken regardless of which guard causes
// processHistorySync to return early (here: no client set, so it hits the
// "not connected" guard). This is what lets cmdBacklog's wait resolve
// promptly instead of blocking for its full fallback timeout.
func TestProcessHistorySync_NotifiesWaitersEvenOnEarlyReturn(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	waiter, _ := sm.addSyncWaiter()

	// No client set: sm.getClient() returns nil, so processHistorySync
	// hits its "not connected" guard and returns immediately.
	sm.processHistorySync(&waHistorySync.HistorySync{})

	select {
	case <-waiter:
		// woken, as expected
	default:
		t.Fatal("expected the sync waiter to be woken even on early return")
	}
}

// TestSyncWaiters_AllConcurrentWaitersWake is the H3.1 fix's core property:
// N concurrent waiters (e.g. overlapping /backlog calls) are ALL woken by
// one completed sync, where the old single buffered channel woke exactly
// one and left the rest to time out.
func TestSyncWaiters_AllConcurrentWaitersWake(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	const n = 3
	woken := make(chan int, n)
	for i := 0; i < n; i++ {
		waiter, _ := sm.addSyncWaiter()
		go func(i int) {
			select {
			case <-waiter:
				woken <- i
			case <-time.After(5 * time.Second):
			}
		}(i)
	}

	sm.notifySyncWaiters()

	for i := 0; i < n; i++ {
		select {
		case <-woken:
		case <-time.After(1 * time.Second):
			t.Fatalf("only %d of %d waiters woke after notify", i, n)
		}
	}
}

// TestSyncWaiters_RegistryClearsAndDoesNotDoubleClose verifies notify is
// idempotent across rounds: a second notify with no new waiters must not
// re-close already-closed channels (which would panic), and a waiter
// registered after a notify is not stale-woken by it.
func TestSyncWaiters_RegistryClearsAndDoesNotDoubleClose(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	_, _ = sm.addSyncWaiter()
	sm.notifySyncWaiters()
	// Registry is now empty; a second notify must be a no-op (a double
	// close of the first waiter's channel would panic here).
	sm.notifySyncWaiters()

	late, _ := sm.addSyncWaiter()
	select {
	case <-late:
		t.Fatal("a waiter registered after a notify must not be woken by that past notify")
	default:
		// still open, as expected
	}
}

// TestSyncWaiters_CancelDeregisters verifies the cancel func removes an
// abandoned waiter (master audit F6): after cancel, a later notify neither
// closes it (it is gone) nor panics, and the registry is empty.
func TestSyncWaiters_CancelDeregisters(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	waiter, cancel := sm.addSyncWaiter()
	cancel()

	sm.mu.RLock()
	n := len(sm.syncWaiters)
	sm.mu.RUnlock()
	if n != 0 {
		t.Fatalf("expected registry empty after cancel, got %d waiters", n)
	}

	// notify must not close the cancelled waiter (it was deregistered).
	sm.notifySyncWaiters()
	select {
	case <-waiter:
		t.Fatal("cancelled waiter should not be woken by a later notify")
	default:
	}

	// cancel is idempotent — calling it again (or after a notify) is a no-op.
	cancel()
}

// TestSyncWaiters_CancelAfterNotifyIsNoOp verifies cancel is safe once notify
// has already closed and cleared the waiter (the common cmdBacklog path where
// the sync completes before the deferred cancel runs).
func TestSyncWaiters_CancelAfterNotifyIsNoOp(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	waiter, cancel := sm.addSyncWaiter()
	sm.notifySyncWaiters() // closes and clears
	select {
	case <-waiter:
		// closed, as expected
	default:
		t.Fatal("waiter should have been closed by notify")
	}
	cancel() // must not panic or double-anything
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
