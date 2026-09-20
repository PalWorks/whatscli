package messages

import (
	"testing"
	"time"

	"github.com/normen/whatscli/config"
)

// setNetworkTimeouts temporarily overrides the two timeout config values,
// restoring the originals when the test ends.
func setNetworkTimeouts(t *testing.T, request, transfer int) {
	t.Helper()
	origReq := config.Config.General.NetworkTimeout
	origTr := config.Config.General.NetworkTransferTimeout
	config.Config.General.NetworkTimeout = request
	config.Config.General.NetworkTransferTimeout = transfer
	t.Cleanup(func() {
		config.Config.General.NetworkTimeout = origReq
		config.Config.General.NetworkTransferTimeout = origTr
	})
}

// deadlineWithin asserts ctx has a deadline roughly `want` from now
// (tolerance one second, generous enough for any test-runner scheduling).
func deadlineWithin(t *testing.T, deadline time.Time, want time.Duration) {
	t.Helper()
	got := time.Until(deadline)
	if got > want || got < want-time.Second {
		t.Errorf("deadline is %v from now, want ~%v", got, want)
	}
}

func TestBoundedCtx_UsesConfiguredSeconds(t *testing.T) {
	ctx, cancel := boundedCtx(7, defaultNetworkTimeout)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected a deadline, got none")
	}
	deadlineWithin(t, deadline, 7*time.Second)
}

func TestBoundedCtx_FallsBackOnZeroAndNegative(t *testing.T) {
	for _, seconds := range []int{0, -5} {
		ctx, cancel := boundedCtx(seconds, 42*time.Second)
		deadline, ok := ctx.Deadline()
		if !ok {
			cancel()
			t.Fatalf("seconds=%d: expected a deadline, got none", seconds)
		}
		deadlineWithin(t, deadline, 42*time.Second)
		cancel()
	}
}

func TestNetCtx_UsesConfiguredRequestTimeout(t *testing.T) {
	setNetworkTimeouts(t, 3, 9)

	ctx, cancel := netCtx()
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("netCtx: expected a deadline, got none")
	}
	deadlineWithin(t, deadline, 3*time.Second)
}

func TestNetTransferCtx_UsesConfiguredTransferTimeout(t *testing.T) {
	setNetworkTimeouts(t, 3, 9)

	ctx, cancel := netTransferCtx()
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("netTransferCtx: expected a deadline, got none")
	}
	deadlineWithin(t, deadline, 9*time.Second)
}
