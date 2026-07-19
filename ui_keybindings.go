package main

import (
	"strings"

	"code.rocketnine.space/tslocum/cbind"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
)

func (ctx *AppContext) handleFocusMessage(ev *tcell.EventKey) *tcell.EventKey {
	if !ctx.TextView.HasFocus() {
		ctx.App.SetFocus(ctx.TextView)
		if ctx.CurRegions != nil && len(ctx.CurRegions) > 0 {
			ctx.TextView.Highlight(ctx.CurRegions[len(ctx.CurRegions)-1].Id)
		}
	}
	return nil
}

func (ctx *AppContext) handleFocusInput(ev *tcell.EventKey) *tcell.EventKey {
	ctx.ResetMsgSelection()
	if !ctx.TextInput.HasFocus() {
		ctx.App.SetFocus(ctx.TextInput)
	}
	return nil
}

func (ctx *AppContext) handleFocusContacts(ev *tcell.EventKey) *tcell.EventKey {
	ctx.ResetMsgSelection()
	if !ctx.ChatTable.HasFocus() {
		ctx.App.SetFocus(ctx.ChatTable)
	}
	return nil
}

func (ctx *AppContext) handleSwitchPanels(ev *tcell.EventKey) *tcell.EventKey {
	ctx.ResetMsgSelection()
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

func (ctx *AppContext) handleCommand(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		ctx.SessionManager.CommandChannel <- messages.Command{Name: command, Params: nil}
		return nil
	}
}

func (ctx *AppContext) handleCopyUser(ev *tcell.EventKey) *tcell.EventKey {
	if hls := ctx.TextView.GetHighlights(); len(hls) > 0 {
		for _, val := range ctx.CurRegions {
			if val.Id == hls[0] {
				err := clipboard.WriteAll(val.ContactId)
				if err != nil {
					ctx.PrintText("failed to copy: " + err.Error())
				} else {
					ctx.PrintText("copied id of " + val.ContactName + " to clipboard")
				}
			}
		}
		ctx.ResetMsgSelection()
	} else if ctx.CurrentReceiver.Id != "" {
		err := clipboard.WriteAll(ctx.CurrentReceiver.Id)
		if err != nil {
			ctx.PrintText("failed to copy: " + err.Error())
		} else {
			ctx.PrintText("copied id of " + ctx.CurrentReceiver.Name + " to clipboard")
		}
	}
	return nil
}

func (ctx *AppContext) handlePasteUser(ev *tcell.EventKey) *tcell.EventKey {
	text, err := clipboard.ReadAll()
	if err != nil {
		ctx.PrintText("failed to paste: " + err.Error())
		return nil
	}
	ctx.TextInput.SetText(ctx.TextInput.GetText() + " " + text)
	return nil
}

func (ctx *AppContext) handleQuit(ev *tcell.EventKey) *tcell.EventKey {
	ctx.SessionManager.CommandChannel <- messages.Command{Name: "disconnect", Params: nil}
	ctx.App.Stop()
	return nil
}

func (ctx *AppContext) handleHelp(ev *tcell.EventKey) *tcell.EventKey {
	ctx.PrintHelp()
	return nil
}

func (ctx *AppContext) handleToggleMouse(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.MouseState {
		ctx.App.EnableMouse(false)
		ctx.MouseState = false
		ctx.PrintText("[::b]Mouse interaction DISABLED (Native selection enabled)[::-]")
	} else {
		ctx.App.EnableMouse(true)
		ctx.MouseState = true
		ctx.PrintText("[::b]Mouse interaction ENABLED (App selection enabled)[::-]")
	}
	return nil
}

func (ctx *AppContext) handleMessageCommand(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		hls := ctx.TextView.GetHighlights()
		if len(hls) > 0 {
			ctx.SessionManager.CommandChannel <- messages.Command{Name: command, Params: []string{hls[0]}}
			ctx.ResetMsgSelection()
			ctx.App.SetFocus(ctx.TextInput)
		}
		return nil
	}
}

// handleMessageCommandPrefill builds a keybinding handler for message-scoped
// commands that need free-text input in addition to the selected message ID
// (reply, edit): rather than dispatching immediately like handleMessageCommand,
// it fills the input field with "<prefix><command> <id> " and focuses it, so
// the user types their text and sends it through the normal EnterCommand path.
func (ctx *AppContext) handleMessageCommandPrefill(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		hls := ctx.TextView.GetHighlights()
		if len(hls) > 0 {
			ctx.TextInput.SetText(config.Config.General.CmdPrefix + command + " " + hls[0] + " ")
			ctx.ResetMsgSelection()
			ctx.App.SetFocus(ctx.TextInput)
		}
		return nil
	}
}

func (ctx *AppContext) handleMessagesMove(amount int) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
			return nil
		}
		hls := ctx.TextView.GetHighlights()
		if len(hls) > 0 {
			newId := ctx.GetOffsetMsgId(hls[0], amount)
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

func (ctx *AppContext) handleChatPanelUp(ev *tcell.EventKey) *tcell.EventKey {
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

func (ctx *AppContext) handleChatPanelDown(ev *tcell.EventKey) *tcell.EventKey {
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

func (ctx *AppContext) handleGroupPanelUp(ev *tcell.EventKey) *tcell.EventKey {
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

func (ctx *AppContext) handleGroupPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := ctx.GroupTable.GetSelection()
	if row < ctx.GroupTable.GetRowCount()-1 {
		ctx.GroupTable.Select(row+1, 0)
	}
	return nil
}

func (ctx *AppContext) handleStatusPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	// Jump to Chats
	if ctx.ChatTable.GetRowCount() > 0 {
		ctx.App.SetFocus(ctx.ChatTable)
		ctx.ChatTable.Select(0, 0)
	}
	return nil
}

func (ctx *AppContext) handleStatusPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.ChatTable)
	return nil
}

func (ctx *AppContext) handleChatPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.GroupTable)
	return nil
}

func (ctx *AppContext) handleGroupPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.TextView)
	return nil
}

func (ctx *AppContext) handleMessagePanelTab(ev *tcell.EventKey) *tcell.EventKey {
	ctx.App.SetFocus(ctx.TextInput)
	return nil
}

func (ctx *AppContext) handleMessagesLast(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return nil
	}
	ctx.TextView.Highlight(ctx.CurRegions[len(ctx.CurRegions)-1].Id)
	ctx.TextView.ScrollToHighlight()
	return nil
}

func (ctx *AppContext) handleMessagesFirst(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return nil
	}
	ctx.TextView.Highlight(ctx.CurRegions[0].Id)
	ctx.TextView.ScrollToHighlight()
	return nil
}

func (ctx *AppContext) handleExitMessages(ev *tcell.EventKey) *tcell.EventKey {
	if ctx.CurRegions == nil || len(ctx.CurRegions) == 0 {
		return nil
	}
	ctx.ResetMsgSelection()
	ctx.App.SetFocus(ctx.TextInput)
	return nil
}

// load the key map
func (ctx *AppContext) LoadShortcuts() {
	// global bindings for app
	ctx.KeyBindings = cbind.NewConfiguration()
	if err := ctx.KeyBindings.Set(config.Config.Keymap.FocusMessages, ctx.handleFocusMessage); err != nil {
		ctx.PrintErrorMsg("focus_messages:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.FocusInput, ctx.handleFocusInput); err != nil {
		ctx.PrintErrorMsg("focus_input:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.FocusChats, ctx.handleFocusContacts); err != nil {
		ctx.PrintErrorMsg("focus_contacts:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.SwitchPanels, ctx.handleSwitchPanels); err != nil {
		ctx.PrintErrorMsg("switch_panels:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandRead, ctx.handleCommand("read")); err != nil {
		ctx.PrintErrorMsg("command_read:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.Copyuser, ctx.handleCopyUser); err != nil {
		ctx.PrintErrorMsg("copyuser:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.Pasteuser, ctx.handlePasteUser); err != nil {
		ctx.PrintErrorMsg("pasteuser:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandBacklog, ctx.handleCommand("backlog")); err != nil {
		ctx.PrintErrorMsg("command_backlog:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandConnect, ctx.handleCommand("login")); err != nil {
		ctx.PrintErrorMsg("command_connect:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandQuit, ctx.handleQuit); err != nil {
		ctx.PrintErrorMsg("command_quit:", err)
	}
	if err := ctx.KeyBindings.Set(config.Config.Keymap.CommandHelp, ctx.handleHelp); err != nil {
		ctx.PrintErrorMsg("command_help:", err)
	}
	// Toggle mouse binding (Hardcoded for now as it's a new feature)
	ctx.KeyBindings.SetRune(tcell.ModCtrl, 'p', ctx.handleToggleMouse)

	ctx.App.SetInputCapture(ctx.KeyBindings.Capture)
	// bindings for chat message text view
	keysMessages := cbind.NewConfiguration()
	if err := keysMessages.Set(config.Config.Keymap.MessageDownload, ctx.handleMessageCommand("download")); err != nil {
		ctx.PrintErrorMsg("message_download:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageOpen, ctx.handleMessageCommand("open")); err != nil {
		ctx.PrintErrorMsg("message_open:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.Copyuser, ctx.handleCopyUser); err != nil {
		ctx.PrintErrorMsg("copyuser:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.Pasteuser, ctx.handlePasteUser); err != nil {
		ctx.PrintErrorMsg("pasteuser:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageShow, ctx.handleMessageCommand("show")); err != nil {
		ctx.PrintErrorMsg("message_show:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageUrl, ctx.handleMessageCommand("url")); err != nil {
		ctx.PrintErrorMsg("message_url:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageInfo, ctx.handleMessageCommand("info")); err != nil {
		ctx.PrintErrorMsg("message_info:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageRevoke, ctx.handleMessageCommand("revoke")); err != nil {
		ctx.PrintErrorMsg("message_revoke:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageReply, ctx.handleMessageCommandPrefill("reply")); err != nil {
		ctx.PrintErrorMsg("message_reply:", err)
	}
	if err := keysMessages.Set(config.Config.Keymap.MessageEdit, ctx.handleMessageCommandPrefill("edit")); err != nil {
		ctx.PrintErrorMsg("message_edit:", err)
	}
	keysMessages.SetKey(tcell.ModNone, tcell.KeyEscape, ctx.handleExitMessages)
	keysMessages.SetKey(tcell.ModNone, tcell.KeyUp, ctx.handleMessagesMove(-1))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyDown, ctx.handleMessagesMove(1))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyPgUp, ctx.handleMessagesMove(-10))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyPgDn, ctx.handleMessagesMove(10))
	keysMessages.SetRune(tcell.ModNone, 'k', ctx.handleMessagesMove(-1))
	keysMessages.SetRune(tcell.ModNone, 'j', ctx.handleMessagesMove(1))
	keysMessages.SetRune(tcell.ModNone, 'g', ctx.handleMessagesFirst)
	keysMessages.SetRune(tcell.ModNone, 'G', ctx.handleMessagesLast)
	keysMessages.SetRune(tcell.ModCtrl, 'u', ctx.handleMessagesMove(-10))
	keysMessages.SetRune(tcell.ModCtrl, 'd', ctx.handleMessagesMove(10))
	keysMessages.SetKey(tcell.ModNone, tcell.KeyTab, ctx.handleMessagePanelTab)
	ctx.TextView.SetInputCapture(keysMessages.Capture)
	keysChatPanel := cbind.NewConfiguration()
	keysChatPanel.SetRune(tcell.ModCtrl, 'u', ctx.handleChatPanelUp)
	keysChatPanel.SetRune(tcell.ModCtrl, 'd', ctx.handleChatPanelDown)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyUp, ctx.handleChatPanelUp)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyDown, ctx.handleChatPanelDown)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyTab, ctx.handleChatPanelTab)
	ctx.ChatTable.SetInputCapture(keysChatPanel.Capture)

	keysGroupPanel := cbind.NewConfiguration()
	keysGroupPanel.SetRune(tcell.ModCtrl, 'u', ctx.handleGroupPanelUp)
	keysGroupPanel.SetRune(tcell.ModCtrl, 'd', ctx.handleGroupPanelDown)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyUp, ctx.handleGroupPanelUp)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyDown, ctx.handleGroupPanelDown)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyTab, ctx.handleGroupPanelTab)
	ctx.GroupTable.SetInputCapture(keysGroupPanel.Capture)

	keysStatusPanel := cbind.NewConfiguration()
	keysStatusPanel.SetRune(tcell.ModCtrl, 'd', ctx.handleStatusPanelDown)
	keysStatusPanel.SetKey(tcell.ModNone, tcell.KeyDown, ctx.handleStatusPanelDown)
	keysStatusPanel.SetKey(tcell.ModNone, tcell.KeyTab, ctx.handleStatusPanelTab)
	ctx.StatusTable.SetInputCapture(keysStatusPanel.Capture)
}

// EnterCommand is the DoneFunc for the input field
func (ctx *AppContext) EnterCommand(key tcell.Key) {
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
				ctx.PrintText("no receiver")
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
func (ctx *AppContext) GetOffsetMsgId(curId string, offset int) string {
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
func (ctx *AppContext) ResetMsgSelection() {
	if len(ctx.TextView.GetHighlights()) > 0 {
		ctx.TextView.Highlight("")
	}
	ctx.TextView.ScrollToEnd()
}
