// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"strings"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/rivo/tview"
)

// prints text to the TextView
func PrintText(txt string) {
	fmt.Fprintln(ctx.TextView, txt)
}

// prints an error to the TextView
func PrintError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(ctx.TextView, "["+config.Config.Colors.Negative+"]", err.Error(), "[-]")
}

// prints an error to the TextView
func PrintErrorMsg(text string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(ctx.TextView, "["+config.Config.Colors.Negative+"]", text, err.Error(), "[-]")
}

// PrintHelp prints the welcome screen in the message pane (shown while no
// chat is open); the full list of keys and commands is in the help window
// (ui_overlay.go).
func PrintHelp() {
	fmt.Fprint(ctx.TextView, welcomeText(config.Config.Keymap, config.Config.General.CmdPrefix))
	fmt.Fprintln(ctx.TextView, "")
	fmt.Fprintln(ctx.TextView, "[::d]Settings: "+tview.Escape(config.GetConfigFilePath())+"[::-]")
}

// UpdateStatusBar shows the connection state in the top bar. (The phone
// battery fields of SessionStatus are not sent by multi-device WhatsApp and
// are not shown.)
func UpdateStatusBar(statusInfo messages.SessionStatus) {
	out := statusText(statusInfo.Connected)
	if statusInfo.LastSeen != "" {
		out += "  [::d]" + tview.Escape(statusInfo.LastSeen) + "[::-]"
	}
	ctx.TopBar.SetText(out)
}

// chatForID returns the chat for id from the chat list, or a chat named by
// its id when the list does not have it (used by /select).
func chatForID(id string) messages.Chat {
	for _, c := range ctx.AllChats {
		if c != nil && c.JID == id {
			return messages.Chat{Id: id, Name: c.Name, IsGroup: strings.HasSuffix(id, "@g.us")}
		}
	}
	return messages.Chat{Id: id, Name: id, IsGroup: strings.HasSuffix(id, "@g.us")}
}

// sets the current chat, loads text from storage to TextView
func SetDisplayedChat(chat messages.Chat) {
	ctx.CurrentReceiver = chat
	ctx.TextView.Clear()
	ctx.TextView.SetTitle(chat.Name)
	ctx.SessionManager.CommandChannel <- messages.Command{Name: "select", Params: []string{ctx.CurrentReceiver.Id}}
}
