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
	//TODO: its stupid to "go" this as its supposed to run
	//on the ui thread anyway. But QueueUpdate blocks...?
	go app.QueueUpdateDraw(func() {
		curRegions = append(curRegions, msg)
		PrintText(getTextMessageString(&msg))
	})
}

func (u UiHandler) NewScreen(msgs []messages.Message) {
	go app.QueueUpdateDraw(func() {
		textView.Clear()
		screen := getMessagesString(msgs)
		textView.SetText(screen)
		curRegions = msgs
		if screen == "" {
			if currentReceiver.Id == "" {
				PrintHelp()
			} else {
				PrintText("[::d] ~~~ no messages, press " + config.Config.Keymap.CommandBacklog + " to load backlog if available ~~~[::-]")
			}
		}
	})
}

// UpdateChatList updates the table with the PriorityQueue content
func (u UiHandler) UpdateChatList(pq []*messages.Conversation) {
	go app.QueueUpdateDraw(func() {
		// Update the store
		allChats = make([]*messages.Conversation, len(pq))
		copy(allChats, pq)

		// Sort the full list once
		sort.Slice(allChats, func(i, j int) bool {
			a, b := allChats[i], allChats[j]

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

		// Reset limit if list seems to be refreshed significantly, or keep it?
		// REMOVED: chatLimit = batchSize to prevent jumping to top on new message events while scrolled down.
		// We trust the user's scroll expansion.

		RenderChatTable()
	})
}

// Deprecated: loads the chat data from storage to the TreeView - Kept for interface compat
func (u UiHandler) SetChats(ids []messages.Chat) {
	// No-op for now, as we moved to Table
}

func (u UiHandler) PrintError(err error) {
	go app.QueueUpdateDraw(func() {
		PrintError(err)
	})
}

func (u UiHandler) PrintText(msg string) {
	go app.QueueUpdateDraw(func() {
		PrintText(msg)
	})
}

func (u UiHandler) PrintQR(qr string) {
	go app.QueueUpdateDraw(func() {
		fmt.Fprint(tview.ANSIWriter(textView), qr+"\n")
	})
}

func (u UiHandler) PrintFile(path string) {
	go app.QueueUpdateDraw(func() {
		PrintImage(path)
	})
}

func (u UiHandler) OpenFile(path string) {
	open.Run(path)
}

func (u UiHandler) SetStatus(status messages.SessionStatus) {
	go app.QueueUpdateDraw(func() {
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
	go app.QueueUpdateDraw(func() {
		textView.Clear()
		PrintHelp()
	})
}

func (u UiHandler) PrintHelp() {
	go app.QueueUpdateDraw(func() {
		PrintHelp()
	})
}

func (u UiHandler) Quit() {
	go app.QueueUpdateDraw(func() {
		app.Stop()
	})
}

func (u UiHandler) UpdateQR(qr string, attempt int, timeout int) {
	// Pre-calculate the content to ensure atomic rendering
	// Translate ANSI codes in the QR string to tview tags to avoid using ANSIWriter during draw
	qtTrans := tview.TranslateANSI(qr)

	go app.QueueUpdateDraw(func() {
		// 1. Clear Screen
		textView.Clear()

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
		fmt.Fprint(textView, output)

		// 5. Ensure input field is focused and cleared so commands can be entered
		app.SetFocus(textInput)
	})
}
