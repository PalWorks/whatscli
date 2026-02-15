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

type UiHandler struct{}

func (u UiHandler) NewMessage(msg messages.Message) {
	// QueueUpdateDraw must be called from a goroutine to avoid blocking the caller.
	go ctx.App.QueueUpdateDraw(func() {
		ctx.CurRegions = append(ctx.CurRegions, msg)
		PrintText(getTextMessageString(&msg))
	})
}

func (u UiHandler) NewScreen(msgs []messages.Message) {
	go ctx.App.QueueUpdateDraw(func() {
		ctx.TextView.Clear()
		screen := getMessagesString(msgs)
		ctx.TextView.SetText(screen)
		ctx.CurRegions = msgs
		if screen == "" {
			if ctx.CurrentReceiver.Id == "" {
				PrintHelp()
			} else {
				PrintText("[::d] ~~~ no messages, press " + config.Config.Keymap.CommandBacklog + " to load backlog if available ~~~[::-]")
			}
		}
	})
}

// UpdateChatList updates the table with the PriorityQueue content
func (u UiHandler) UpdateChatList(pq []*messages.Conversation) {
	go ctx.App.QueueUpdateDraw(func() {
		// Update the store
		ctx.AllChats = make([]*messages.Conversation, len(pq))
		copy(ctx.AllChats, pq)

		// Sort the full list once
		sort.Slice(ctx.AllChats, func(i, j int) bool {
			a, b := ctx.AllChats[i], ctx.AllChats[j]

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

		RenderChatTable()
	})
}

// SetChats is a legacy no-op kept for UiMessageHandler interface compatibility.
func (u UiHandler) SetChats(ids []messages.Chat) {}

func (u UiHandler) PrintError(err error) {
	go ctx.App.QueueUpdateDraw(func() {
		PrintError(err)
	})
}

func (u UiHandler) PrintText(msg string) {
	go ctx.App.QueueUpdateDraw(func() {
		PrintText(msg)
	})
}

func (u UiHandler) PrintQR(qr string) {
	go ctx.App.QueueUpdateDraw(func() {
		fmt.Fprint(tview.ANSIWriter(ctx.TextView), qr+"\n")
	})
}

func (u UiHandler) PrintFile(path string) {
	go ctx.App.QueueUpdateDraw(func() {
		PrintImage(path)
	})
}

func (u UiHandler) OpenFile(path string) {
	open.Run(path)
}

func (u UiHandler) SetStatus(status messages.SessionStatus) {
	go ctx.App.QueueUpdateDraw(func() {
		UpdateStatusBar(status)
	})
}

func (u UiHandler) ShowColorList() {
	out := ""
	for idx, _ := range tcell.ColorNames {
		out = out + "[" + idx + "]" + idx + "[-]\n"
	}
	PrintText(out)
}

func (u UiHandler) Clear() {
	go ctx.App.QueueUpdateDraw(func() {
		ctx.TextView.Clear()
		PrintHelp()
	})
}

func (u UiHandler) PrintHelp() {
	go ctx.App.QueueUpdateDraw(func() {
		PrintHelp()
	})
}

func (u UiHandler) PrintCommands() {
	go ctx.App.QueueUpdateDraw(func() {
		PrintHelp()
	})
}

func (u UiHandler) Quit() {
	go ctx.App.QueueUpdateDraw(func() {
		ctx.App.Stop()
	})
}

func (u UiHandler) UpdateQR(qr string, attempt int, timeout int) {
	// Pre-calculate the content to ensure atomic rendering
	// Translate ANSI codes in the QR string to tview tags to avoid using ANSIWriter during draw
	qtTrans := tview.TranslateANSI(qr)

	go ctx.App.QueueUpdateDraw(func() {
		// 1. Clear Screen
		ctx.TextView.Clear()

		// 2. Print Help
		PrintHelp()

		// 3. Construct Full Output (Header + QR)
		var output string
		if timeout > 0 {
			output = fmt.Sprintf("\n\n=== QR Code Generated (Attempt %d) - Auto-refreshing in %ds ===\n\nScan this with WhatsApp:\n\n", attempt, timeout)
		} else {
			output = fmt.Sprintf("\n\n=== QR Code Generated (Attempt %d) - Awaiting new QR code from WhatsApp Server ===\n\nScan this with WhatsApp:\n\n", attempt)
		}

		output += qtTrans + "\n"

		// 4. Atomic Write (Standard Writer)
		fmt.Fprint(ctx.TextView, output)

		// 5. Ensure input field is focused and cleared so commands can be entered
		ctx.App.SetFocus(ctx.TextInput)
	})
}
