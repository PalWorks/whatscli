package main

import (
	"sync"

	"code.rocketnine.space/tslocum/cbind"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

// AppContext holds all application state that was previously scattered across
// package-level variables. This makes dependencies explicit and the codebase
// easier to test and maintain.
//
// All UI-mutating helpers are methods on *AppContext (see ui_helpers.go,
// ui_render.go, ui_layout.go, ui_keybindings.go). There is no package-level
// AppContext singleton anymore: main() constructs one and injects it into
// UiHandler, so the whole UI layer is reachable from a test without touching
// process-global state.
type AppContext struct {
	// UI Widgets
	App         *tview.Application
	TextView    *tview.TextView
	TextInput   *tview.InputField
	TopBar      *tview.TextView
	LeftPane    *tview.Flex
	ChatTable   *tview.Table
	GroupTable  *tview.Table
	StatusTable *tview.Table

	// UI State
	CurrentReceiver messages.Chat
	CurRegions      []messages.Message
	MouseState      bool
	AllChats        []*messages.Conversation
	ChatLimit       int
	RenderingList   bool // true while RenderChatTable is restoring selection

	// Backend
	SessionManager *messages.SessionManager
	KeyBindings    *cbind.Configuration
	UiHandler      messages.UiMessageHandler

	// The update queue serializes every asynchronous UI mutation. Backend
	// goroutines enqueue closures via UiHandler; a single consumer
	// (processUpdates) applies them in FIFO order through draw. One ordered
	// queue instead of a goroutine per update guarantees deterministic render
	// ordering.
	//
	// The queue is an unbounded, mutex-guarded slice rather than a bounded
	// channel on purpose. draw wraps App.QueueUpdateDraw, which blocks the
	// consumer until the tview main loop has drawn. A bounded *blocking*
	// channel would let a full queue block a producer — and the command loop
	// is a producer (it calls uiHandler.PrintError/PrintText directly). That
	// creates a deadlock cycle: command loop blocked enqueuing a UI update →
	// cannot drain CommandChannel → main loop blocked sending to CommandChannel
	// (buffered 10) → main loop cannot draw → consumer stays blocked → queue
	// never drains. Making enqueue non-blocking breaks the cycle: producers
	// never wait on the UI, and memory is bounded by the current burst size.
	updMu    sync.Mutex
	updQueue []func()
	updWake  chan struct{} // buffered 1; signals the consumer that work exists
	// draw applies a single queued closure. In production it wraps
	// App.QueueUpdateDraw (blocks until the main loop has drawn, keeping the
	// consumer strictly in order). Tests substitute a synchronous sink.
	draw func(func())
}

// initUpdates prepares the update queue. Called once by main() (and by tests)
// before any enqueue.
func (ctx *AppContext) initUpdates() {
	ctx.updWake = make(chan struct{}, 1)
}

// enqueue schedules f to run on the UI thread. It never blocks the caller:
// closures are appended to an in-memory queue and the consumer is woken via a
// non-blocking send. This is what keeps the command loop from ever stalling on
// UI rendering (see the deadlock note on AppContext).
func (ctx *AppContext) enqueue(f func()) {
	ctx.updMu.Lock()
	ctx.updQueue = append(ctx.updQueue, f)
	ctx.updMu.Unlock()
	select {
	case ctx.updWake <- struct{}{}:
	default:
	}
}

// processUpdates is the single consumer of the update queue. It runs for the
// life of the process. On each wake it drains the queue completely, applying
// closures one at a time through draw (which blocks until the tview main loop
// has rendered), so updates land in the exact order they were enqueued.
func (ctx *AppContext) processUpdates() {
	for range ctx.updWake {
		for {
			ctx.updMu.Lock()
			if len(ctx.updQueue) == 0 {
				ctx.updMu.Unlock()
				break
			}
			f := ctx.updQueue[0]
			ctx.updQueue = ctx.updQueue[1:]
			ctx.updMu.Unlock()
			ctx.draw(f)
		}
	}
}
