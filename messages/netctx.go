package messages

// netctx.go — bounded contexts for whatsmeow network calls (plan H1.1).
// Every network operation gets a deadline so a dead or stalled connection
// can never hang a goroutine indefinitely. Deliberately NOT used for: the
// QR pairing channel (user-paced, whatsmeow manages its lifecycle),
// client.Connect (whatsmeow handles its own dial/handshake and the
// auto-reconnect path recovers), and local sqlstore/contact-store calls
// (not network I/O).

import (
	"context"
	"time"

	"github.com/normen/whatscli/config"
)

// Fallbacks when the configured value is zero or negative.
const (
	defaultNetworkTimeout         = 60 * time.Second
	defaultNetworkTransferTimeout = 300 * time.Second
)

// netCtx returns a bounded context for request-response network operations
// (message sends, group operations, read receipts), using the configured
// [general] network_timeout.
func netCtx() (context.Context, context.CancelFunc) {
	return boundedCtx(config.Config.General.NetworkTimeout, defaultNetworkTimeout)
}

// netTransferCtx returns a bounded context for bulk media uploads and
// downloads, using the configured [general] network_transfer_timeout.
func netTransferCtx() (context.Context, context.CancelFunc) {
	return boundedCtx(config.Config.General.NetworkTransferTimeout, defaultNetworkTransferTimeout)
}

// boundedCtx converts a configured timeout in seconds into a context with a
// deadline, falling back to def when the configured value is zero or
// negative: a user setting 0 to "disable" the timeout must not produce an
// already-expired context that breaks every network call.
func boundedCtx(seconds int, def time.Duration) (context.Context, context.CancelFunc) {
	d := time.Duration(seconds) * time.Second
	if d <= 0 {
		d = def
	}
	return context.WithTimeout(context.Background(), d)
}
