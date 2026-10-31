// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_overlay.go: help and onboarding. The message pane greets a new user
// with the essentials (welcomeText, printed by PrintHelp while no chat is
// open); a help window (F1, /help, Ctrl+?) lists every key and command over
// the main screen; the top bar shows the connection state and the main keys.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/PalWorks/whatscli/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// helpKey opens the help window; F1 is not configurable, Ctrl+? (keymap
// command_help) and /help also work.
const helpKey = "F1"

// overlay state; UI goroutine only.
var (
	pages        *tview.Pages
	overlayName  string // "" or "help"
	overlayView  *tview.TextView
	overlayFocus tview.Primitive // focus to restore on close
)

// keyLine formats one "key  description" row.
func keyLine(key, desc string) string {
	return fmt.Sprintf("  [::b]%-22s[::-] %s", tview.Escape(key), desc)
}

// helpText is the full help: keys first, then every command.
func helpText(km *config.Keymap, prefix string) string {
	var b strings.Builder
	section := func(title string) { fmt.Fprintf(&b, "\n[::b]%s[::-]\n", title) }
	b.WriteString("[::d]Esc, Enter or q closes this window; Up and Down scroll.[::-]\n")
	section("Getting around")
	for _, l := range [][2]string{
		{km.FindChat, "Find a chat by name"},
		{km.FocusInput, "Type a message or a command"},
		{km.FocusChats, "Go to the chat list (Up/Down, Enter opens)"},
		{km.SwitchPanels, "Next panel"},
		{km.FocusMessages, "Select a message (then the keys below)"},
		{"Esc", "Back to typing"},
		{km.CommandBacklog, "Ask the phone for older messages"},
		{km.CommandRead, "Mark the chat read"},
		{km.CommandConnect, "Reconnect"},
		{"Ctrl+p", "Mouse on or off (off lets the terminal select text)"},
		{helpKey + " / " + km.CommandHelp, "This help"},
		{km.CommandQuit, "Quit"},
	} {
		b.WriteString(keyLine(l[0], l[1]) + "\n")
	}
	section("With a message selected")
	for _, l := range [][2]string{
		{km.MessageDownload, "Download the attachment"},
		{km.MessageOpen, "Download and open it"},
		{km.MessageShow, "Show an image in the terminal"},
		{km.MessageUrl, "Open the first link"},
		{km.MessageRevoke, "Delete your own message for everyone"},
		{km.MessageInfo, "Chat info"},
		{km.Copyuser + " / " + km.Pasteuser, "Copy or paste the sender's ID"},
	} {
		b.WriteString(keyLine(l[0], l[1]) + "\n")
	}
	section("Commands (type " + prefix + " to pick from a list)")
	hints := append([]commandHint(nil), commandHints...)
	sort.Slice(hints, func(i, j int) bool { return hints[i].name < hints[j].name })
	for _, h := range hints {
		usage := prefix + h.name
		if h.args != "" {
			usage += " " + h.args
		}
		b.WriteString(fmt.Sprintf("  [::b]%-28s[::-] %s\n", tview.Escape(usage), h.desc))
	}
	return b.String()
}

// welcomeText is the start screen of the message pane: the essentials only.
func welcomeText(km *config.Keymap, prefix string) string {
	var b strings.Builder
	b.WriteString("\n[::b]Welcome to WhatsCLI " + appVersion() + "[::-]\n")
	b.WriteString("[::d]Pick a chat on the left, or:[::-]\n\n")
	for _, l := range [][2]string{
		{km.FindChat, "Find a chat by name"},
		{"Type, then Enter", "Send a message to the open chat"},
		{prefix, "Start a command; a list shows what you can do"},
		{km.FocusMessages, "Select a message to download, open or delete it"},
		{helpKey + " or " + prefix + "help", "Every key and command"},
	} {
		b.WriteString(keyLine(l[0], l[1]) + "\n")
	}
	return b.String()
}

// newOverlay builds a centred, bordered text window of the given size.
func newOverlay(title string, width, height int) (tview.Primitive, *tview.TextView) {
	v := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWordWrap(true)
	v.SetBorder(true).SetTitle(" " + title + " ")
	v.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	v.SetBorderColor(tcell.ColorNames[config.Config.Colors.Positive])
	box := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(v, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
	return box, v
}

// showOverlay opens the help or welcome window. UI goroutine only.
func showOverlay(name string) {
	if pages == nil {
		return
	}
	if overlayName != "" {
		pages.RemovePage(overlayName)
	} else {
		overlayFocus = ctx.App.GetFocus()
	}
	km := config.Config.Keymap
	prefix := config.Config.General.CmdPrefix
	box, view := newOverlay("Help", 84, 32)
	overlayView = view
	overlayView.SetText(helpText(km, prefix))
	overlayName = name
	pages.AddPage(name, box, true, true)
	ctx.App.SetFocus(overlayView)
}

// closeOverlay hides the open window and gives focus back.
func closeOverlay() {
	if overlayName == "" {
		return
	}
	pages.RemovePage(overlayName)
	overlayName, overlayView = "", nil
	if overlayFocus != nil {
		ctx.App.SetFocus(overlayFocus)
	} else {
		ctx.App.SetFocus(ctx.TextInput)
	}
}

// globalKeys handles F1 and the overlay's own keys before the key map.
func globalKeys(ev *tcell.EventKey) *tcell.EventKey {
	if overlayName != "" {
		switch {
		case ev.Key() == tcell.KeyEscape, ev.Key() == tcell.KeyEnter, ev.Key() == tcell.KeyF1,
			ev.Key() == tcell.KeyRune && ev.Rune() == 'q':
			closeOverlay()
		case ev.Key() == tcell.KeyUp, ev.Key() == tcell.KeyDown, ev.Key() == tcell.KeyPgUp,
			ev.Key() == tcell.KeyPgDn, ev.Key() == tcell.KeyHome, ev.Key() == tcell.KeyEnd:
			return ev // scrolls the window
		}
		return nil // nothing else reaches the screen behind
	}
	if ev.Key() == tcell.KeyF1 {
		showOverlay("help")
		return nil
	}
	return ctx.KeyBindings.Capture(ev)
}

// statusText is the left part of the top bar.
func statusText(connected bool) string {
	state := "[" + config.Config.Colors.Negative + "]offline[-]"
	if connected {
		state = "[" + config.Config.Colors.Positive + "]online[-]"
	}
	return "[::b] WhatsCLI " + appVersion() + "[::-]  " + state
}

// hintText is the right part of the top bar: the main keys.
func hintText(km *config.Keymap, prefix string) string {
	return fmt.Sprintf("[::d]%s[::-] Help   [::d]%s[::-] Find chat   [::d]%s[::-] Commands   [::d]%s[::-] Quit ",
		helpKey, km.FindChat, prefix, km.CommandQuit)
}

// overlayOpen reports whether a window covers the screen (scroll bars of the
// panels behind it are then not drawn).
func overlayOpen() bool { return overlayName != "" }
