package main

import (
	"fmt"
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
	"github.com/skratchdot/open-golang/open"
)

// UiHandler adapts the backend's messages.UiMessageHandler interface to the
// tview UI. Every method that mutates the UI enqueues a closure onto the
// AppContext update queue (ctx.enqueue) instead of spawning its own goroutine.
// A single consumer applies them in FIFO order, so backend calls render in the
// exact order they were made (fixes the previous out-of-order rendering).
type UiHandler struct {
	ctx *AppContext
}

func (u UiHandler) NewMessage(msg messages.Message) {
	u.ctx.enqueue(func() {
		u.ctx.CurRegions = append(u.ctx.CurRegions, msg)
		u.ctx.PrintText(getTextMessageString(&msg))
	})
}

func (u UiHandler) NewScreen(msgs []messages.Message) {
	u.ctx.enqueue(func() {
		u.ctx.TextView.Clear()
		screen := getMessagesString(msgs)
		u.ctx.TextView.SetText(screen)
		u.ctx.CurRegions = msgs
		if screen == "" {
			if u.ctx.CurrentReceiver.Id == "" {
				u.ctx.PrintHelp()
			} else {
				u.ctx.PrintText("[::d] ~~~ no messages, press " + config.Config.Keymap.CommandBacklog + " to load backlog if available ~~~[::-]")
			}
		}
	})
}

// UpdateChatList updates the table with the PriorityQueue content
func (u UiHandler) UpdateChatList(pq []*messages.Conversation) {
	u.ctx.enqueue(func() {
		// Update the store
		u.ctx.AllChats = make([]*messages.Conversation, len(pq))
		copy(u.ctx.AllChats, pq)

		// Sort the full list once
		sort.Slice(u.ctx.AllChats, func(i, j int) bool {
			a, b := u.ctx.AllChats[i], u.ctx.AllChats[j]

			// Pinned check
			if a.IsPinned && !b.IsPinned {
				return true
			}
			if !a.IsPinned && b.IsPinned {
				return false
			}

			// Time check (descending)
			return a.LastMsgTime > b.LastMsgTime
		})

		u.ctx.RenderChatTable()
	})
}

func (u UiHandler) PrintError(err error) {
	u.ctx.enqueue(func() {
		u.ctx.PrintError(err)
	})
}

func (u UiHandler) PrintText(msg string) {
	u.ctx.enqueue(func() {
		u.ctx.PrintText(msg)
	})
}

func (u UiHandler) PrintQR(qr string) {
	u.ctx.enqueue(func() {
		fmt.Fprint(tview.ANSIWriter(u.ctx.TextView), qr+"\n")
	})
}

func (u UiHandler) PrintFile(path string) {
	u.ctx.enqueue(func() {
		u.ctx.PrintImage(path)
	})
}

func (u UiHandler) OpenFile(path string) {
	open.Run(path)
}

func (u UiHandler) SetStatus(status messages.SessionStatus) {
	u.ctx.enqueue(func() {
		u.ctx.UpdateStatusBar(status)
	})
}

func (u UiHandler) ShowColorList() {
	names := make([]string, 0, len(tcell.ColorNames))
	for name := range tcell.ColorNames {
		names = append(names, name)
	}
	sort.Strings(names)

	out := ""
	for _, name := range names {
		out = out + "[" + name + "]" + name + "[-]\n"
	}
	u.ctx.enqueue(func() {
		u.ctx.PrintText(out)
	})
}

func (u UiHandler) Clear() {
	u.ctx.enqueue(func() {
		u.ctx.TextView.Clear()
		u.ctx.PrintHelp()
	})
}

func (u UiHandler) PrintHelp() {
	u.ctx.enqueue(func() {
		u.ctx.PrintHelp()
	})
}

func (u UiHandler) PrintCommands() {
	u.ctx.enqueue(func() {
		u.ctx.PrintHelp()
	})
}

func (u UiHandler) Quit() {
	u.ctx.enqueue(func() {
		u.ctx.App.Stop()
	})
}

func (u UiHandler) UpdateQR(qr string, attempt int, timeout int) {
	// Pre-calculate the content to ensure atomic rendering
	// Translate ANSI codes in the QR string to tview tags to avoid using ANSIWriter during draw
	qtTrans := tview.TranslateANSI(qr)

	u.ctx.enqueue(func() {
		// 1. Clear Screen
		u.ctx.TextView.Clear()

		// 2. Print Help
		u.ctx.PrintHelp()

		// 3. Construct Full Output (Header + QR)
		var output string
		if timeout > 0 {
			output = fmt.Sprintf("\n\n=== QR Code Generated (Attempt %d) - Auto-refreshing in %ds ===\n\nScan this with WhatsApp:\n\n", attempt, timeout)
		} else {
			output = fmt.Sprintf("\n\n=== QR Code Generated (Attempt %d) - Awaiting new QR code from WhatsApp Server ===\n\nScan this with WhatsApp:\n\n", attempt)
		}

		output += qtTrans + "\n"

		// 4. Atomic Write (Standard Writer)
		fmt.Fprint(u.ctx.TextView, output)

		// 5. Auto-scroll to QR code so it's visible without manual scrolling
		u.ctx.TextView.ScrollToEnd()

		// 6. Ensure input field is focused and cleared so commands can be entered
		u.ctx.App.SetFocus(u.ctx.TextInput)
	})
}
