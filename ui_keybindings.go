package main

import (
	"strings"

	"code.rocketnine.space/tslocum/cbind"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
)

func handleFocusMessage(ev *tcell.EventKey) *tcell.EventKey {
	if !ctx.TextView.HasFocus() {
		ctx.App.SetFocus(ctx.TextView)
		if ctx.CurRegions != nil && len(ctx.CurRegions) > 0 {
			ctx.TextView.Highlight(ctx.CurRegions[len(ctx.CurRegions)-1].Id)
		}
	}
	return nil
}

func handleFocusInput(ev *tcell.EventKey) *tcell.EventKey {
	ResetMsgSelection()
	if !ctx.TextInput.HasFocus() {
		ctx.App.SetFocus(ctx.TextInput)
	}
	return nil
}

func handleFocusContacts(ev *tcell.EventKey) *tcell.EventKey {
	ResetMsgSelection()
	if !ctx.ChatTable.HasFocus() {
		ctx.App.SetFocus(ctx.ChatTable)
	}
	return nil
}

func handleSwitchPanels(ev *tcell.EventKey) *tcell.EventKey {
	ResetMsgSelection()
	focus := ctx.App.GetFocus()
	if focus == ctx.TextInput {
		if ctx.StatusTable != nil && ctx.StatusTable.GetRowCount() > 0 {
			ctx.App.SetFocus(ctx.StatusTable)
		} else {
			ctx.App.SetFocus(ctx.ChatTable)
		}
	} else if focus == ctx.StatusTable {
		ctx.App.SetFocus(ctx.ChatTable)
	} else if focus == ctx.ChatTable {
		ctx.App.SetFocus(ctx.GroupTable)
	} else {
		if ctx.StatusTable != nil && ctx.StatusTable.GetRowCount() > 0 {
			ctx.App.SetFocus(ctx.StatusTable)
		} else {
			ctx.App.SetFocus(ctx.ChatTable)
		}
	}
	return nil
}

func handleCommand(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		ctx.SessionManager.CommandChannel <- messages.Command{Name: command, Params: nil}
		return nil
	}
}

func handleCopyUser(ev *tcell.EventKey) *tcell.EventKey {
	if hls := ctx.TextView.GetHighlights(); len(hls) > 0 {
		for _, val := range ctx.CurRegions {
			if val.Id == hls[0] {
				err := clipboard.WriteAll(val.ContactId)
				if err != nil {
					PrintText("failed to copy: " + err.Error())
				} else {
					PrintText("copied id of " + val.ContactName + " to clipboard")
				}
			}
		}
		ResetMsgSelection()
	} else if ctx.CurrentReceiver.Id != "" {
		err := clipboard.WriteAll(ctx.CurrentReceiver.Id)
		if err != nil {
			PrintText("failed to copy: " + err.Error())
		} else {
			PrintText("copied id of " + ctx.CurrentReceiver.Name + " to clipboard")
		}
	}
	return nil
}

func handlePasteUser(ev *tcell.EventKey) *tcell.EventKey {
	text, err := clipboard.ReadAll()
	if err != nil {
		PrintText("failed to paste: " + err.Error())
		return nil
	}
	ctx.TextInput.SetText(ctx.TextInput.GetText() + " " + text)
	return nil
}

func handleQuit(ev *tcell.EventKey) *tcell.EventKey {
	ctx.SessionManager.CommandChannel <- messages.Command{Name: "disconnect", Params: nil}
	ctx.App.Stop()
	return nil
}

func handleHelp(ev *tcell.EventKey) *tcell.EventKey {
	PrintHelp()
	return nil
}

func handleToggleMouse(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.MouseState {
		ctx.App.EnableMouse(false)
		ctx.MouseState = false
		PrintText("[::b]Mouse interaction DISABLED (Native selection enabled)[::-]")
	} else {
		ctx.App.EnableMouse(true)
		ctx.MouseState = true
		PrintText("[::b]Mouse interaction ENABLED (App selection enabled)[::-]")
	}
	return nil
}

func handleMessageCommand(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		hls := ctx.TextView.GetHighlights()
		if len(hls) > 0 {
			ctx.SessionManager.CommandChannel <- messages.Command{Name: command, Params: []string{hls[0]}}
			ResetMsgSelection()
			ctx.App.SetFocus(ctx.TextInput)
		}
		return nil
	}
}

func handleMessagesMove(amount int) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
			return nil
		}
		hls := ctx.TextView.GetHighlights()
		if len(hls) > 0 {
			newId := GetOffsetMsgId(hls[0], amount)
			if newId != "" {
				ctx.TextView.Highlight(newId)
			}
		} else {
			if amount < 0 {
				ctx.TextView.Highlight(ctx.CurRegions[0].Id)
			} else {
				ctx.TextView.Highlight(ctx.CurRegions[len(ctx.CurRegions)-1].Id)
			}
		}
		ctx.TextView.ScrollToHighlight()
		return nil
	}
}

func handleChatPanelUp(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := ctx.ChatTable.GetSelection()
	if row > 0 {
		ctx.ChatTable.Select(row-1, 0)
	} else {
		// Jump to Status if at top
		if ctx.StatusTable.GetRowCount() > 0 {
			ctx.App.SetFocus(ctx.StatusTable)
			ctx.StatusTable.Select(0, 0)
		}
	}
	return nil
}

func handleChatPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := ctx.ChatTable.GetSelection()
	if row < ctx.ChatTable.GetRowCount()-1 {
		ctx.ChatTable.Select(row+1, 0)
	} else {
		// Jump to groups if at bottom
		if ctx.GroupTable.GetRowCount() > 0 {
			ctx.App.SetFocus(ctx.GroupTable)
			// Select first group
			ctx.GroupTable.Select(0, 0)
		}
	}
	return nil
}

func handleGroupPanelUp(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := ctx.GroupTable.GetSelection()
	if row > 0 {
		ctx.GroupTable.Select(row-1, 0)
	} else {
		// Jump back to chats if at top
		if ctx.ChatTable.GetRowCount() > 0 {
			ctx.App.SetFocus(ctx.ChatTable)
			// Select last chat
			ctx.ChatTable.Select(ctx.ChatTable.GetRowCount()-1, 0)
		}
	}
	return nil
}

func handleGroupPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := ctx.GroupTable.GetSelection()
	if row < ctx.GroupTable.GetRowCount()-1 {
		ctx.GroupTable.Select(row+1, 0)
	}
	return nil
}

func handleStatusPanelUp(ev *tcell.EventKey) *tcell.EventKey {
	// Status usually has only 1 item, so maybe no-op or cycle?
	// If at top, maybe nothing?
	return nil
}

func handleStatusPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	// Jump to Chats
	if ctx.ChatTable.GetRowCount() > 0 {
		ctx.App.SetFocus(ctx.ChatTable)
		ctx.ChatTable.Select(0, 0)
	}
	return nil
}

func handleStatusPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.ChatTable)
	return nil
}

func handleChatPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.GroupTable)
	return nil
}

func handleGroupPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.TextView)
	return nil
}

func handleMessagePanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.TextInput)
	return nil
}

func handleMessagesLast(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return nil
	}
	ctx.TextView.Highlight(ctx.CurRegions[len(ctx.CurRegions)-1].Id)
	ctx.TextView.ScrollToHighlight()
	return nil
}

func handleMessagesFirst(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return nil
	}
	ctx.TextView.Highlight(ctx.CurRegions[0].Id)
	ctx.TextView.ScrollToHighlight()
	return nil
}

func handleExitMessages(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return nil
	}
	ResetMsgSelection()
	ctx.App.SetFocus(ctx.TextInput)
	return nil
}

// load the key map
func LoadShortcuts() {
	// global bindings for app
	ctx.KeyBindings = cbind.NewConfiguration()
	if err := ctx.KeyBindings.Set(config.Config.Keymap.FocusMessages, handleFocusMessage); err != nil {
		PrintErrorMsg("focus_messages:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.FocusInput, handleFocusInput); err != nil {
		PrintErrorMsg("focus_input:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.FocusChats, handleFocusContacts); err != nil {
		PrintErrorMsg("focus_contacts:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.SwitchPanels, handleSwitchPanels); err != nil {
		PrintErrorMsg("switch_panels:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandRead, handleCommand("read")); err != nil {
		PrintErrorMsg("command_read:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.Copyuser, handleCopyUser); err != nil {
		PrintErrorMsg("copyuser:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.Pasteuser, handlePasteUser); err != nil {
		PrintErrorMsg("pasteuser:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandBacklog, handleCommand("backlog")); err != nil {
		PrintErrorMsg("command_backlog:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandConnect, handleCommand("login")); err != nil {
		PrintErrorMsg("command_connect:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandQuit, handleQuit); err != nil {
		PrintErrorMsg("command_quit:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandHelp, handleHelp); err != nil {
		PrintErrorMsg("command_help:", err)
	}
	// Toggle mouse binding (Hardcoded for now as it's a new feature)
	ctx.KeyBindings.SetRune(tcell.ModCtrl, 'p', handleToggleMouse)

	ctx.App.SetInputCapture(ctx.KeyBindings.Capture)
	// bindings for chat message text view
	keysMessages := cbind.NewConfiguration()
	if err := keysMessages.Set(config.Config.Keymap.MessageDownload, handleMessageCommand("download")); err != nil {
		PrintErrorMsg("message_download:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageOpen, handleMessageCommand("open")); err != nil {
		PrintErrorMsg("message_open:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.Copyuser, handleCopyUser); err != nil {
		PrintErrorMsg("copyuser:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.Pasteuser, handlePasteUser); err != nil {
		PrintErrorMsg("pasteuser:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageShow, handleMessageCommand("show")); err != nil {
		PrintErrorMsg("message_show:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageUrl, handleMessageCommand("url")); err != nil {
		PrintErrorMsg("message_url:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageInfo, handleMessageCommand("info")); err != nil {
		PrintErrorMsg("message_info:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageRevoke, handleMessageCommand("revoke")); err != nil {
		PrintErrorMsg("message_revoke:", err)
	}
	keysMessages.SetKey(tcell.ModNone, tcell.KeyEscape, handleExitMessages)
	keysMessages.SetKey(tcell.ModNone, tcell.KeyUp, handleMessagesMove(-1))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyDown, handleMessagesMove(1))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyPgUp, handleMessagesMove(-10))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyPgDn, handleMessagesMove(10))
	keysMessages.SetRune(tcell.ModNone, 'k', handleMessagesMove(-1))
	keysMessages.SetRune(tcell.ModNone, 'j', handleMessagesMove(1))
	keysMessages.SetRune(tcell.ModNone, 'g', handleMessagesFirst)
	keysMessages.SetRune(tcell.ModNone, 'G', handleMessagesLast)
	keysMessages.SetRune(tcell.ModCtrl, 'u', handleMessagesMove(-10))
	keysMessages.SetRune(tcell.ModCtrl, 'd', handleMessagesMove(10))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyTab, handleMessagePanelTab)
	ctx.TextView.SetInputCapture(keysMessages.Capture)
	keysChatPanel := cbind.NewConfiguration()
	keysChatPanel.SetRune(tcell.ModCtrl, 'u', handleChatPanelUp)
	keysChatPanel.SetRune(tcell.ModCtrl, 'd', handleChatPanelDown)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyUp, handleChatPanelUp)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyDown, handleChatPanelDown)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyTab, handleChatPanelTab)
	ctx.ChatTable.SetInputCapture(keysChatPanel.Capture)

	keysGroupPanel := cbind.NewConfiguration()
	keysGroupPanel.SetRune(tcell.ModCtrl, 'u', handleGroupPanelUp)
	keysGroupPanel.SetRune(tcell.ModCtrl, 'd', handleGroupPanelDown)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyUp, handleGroupPanelUp)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyDown, handleGroupPanelDown)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyTab, handleGroupPanelTab)
	ctx.GroupTable.SetInputCapture(keysGroupPanel.Capture)

	keysStatusPanel := cbind.NewConfiguration()
	keysStatusPanel.SetRune(tcell.ModCtrl, 'd', handleStatusPanelDown)
	keysStatusPanel.SetKey(tcell.ModNone, tcell.KeyDown, handleStatusPanelDown)
	keysStatusPanel.SetKey(tcell.ModNone, tcell.KeyTab, handleStatusPanelTab)
	ctx.StatusTable.SetInputCapture(keysStatusPanel.Capture)
}

// EnterCommand is the DoneFunc for the input field
func EnterCommand(key tcell.Key) {
	if key == tcell.KeyEnter {
		cmd := ctx.TextInput.GetText()
		if len(cmd) == 0 {
			return
		}
		if strings.HasPrefix(cmd, config.Config.General.CmdPrefix) {
			input := strings.Fields(cmd)
			// Input[0] is the command (e.g., "/help")
			// We remove the prefix to get "help"
			cm := strings.TrimPrefix(input[0], config.Config.General.CmdPrefix)
			var params []string
			if len(input) > 1 {
				params = input[1:]
			}
			ctx.SessionManager.CommandChannel <- messages.Command{Name: cm, Params: params}
		} else {
			if ctx.CurrentReceiver.Id == "" {
				PrintText("no receiver")
				ctx.TextInput.SetText("")
				return
			}
			ctx.SessionManager.CommandChannel <- messages.Command{Name: "send", Params: []string{ctx.CurrentReceiver.Id, cmd}}
		}
		ctx.TextInput.SetText("")
	} else if key == tcell.KeyEsc {
		ctx.TextInput.SetText("")
	}
}

// get the next message id to select (highlighted + offset)
func GetOffsetMsgId(curId string, offset int) string {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return ""
	}
	for idx, val := range ctx.CurRegions {
		if val.Id == curId {
			arrPos := idx + offset
			if len(ctx.CurRegions) > arrPos && arrPos >= 0 {
				return ctx.CurRegions[arrPos].Id
			}
		}
	}
	if offset > 0 {
		return ctx.CurRegions[0].Id
	} else {
		return ctx.CurRegions[len(ctx.CurRegions)-1].Id
	}
}

// resets the selection in the textView and scrolls it down
func ResetMsgSelection() {
	if len(ctx.TextView.GetHighlights()) > 0 {
		ctx.TextView.Highlight("")
	}
	ctx.TextView.ScrollToEnd()
}
