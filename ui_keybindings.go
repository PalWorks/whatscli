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
	if !textView.HasFocus() {
		app.SetFocus(textView)
		if curRegions != nil && len(curRegions) > 0 {
			textView.Highlight(curRegions[len(curRegions)-1].Id)
		}
	}
	return nil
}

func handleFocusInput(ev *tcell.EventKey) *tcell.EventKey {
	ResetMsgSelection()
	if !textInput.HasFocus() {
		app.SetFocus(textInput)
	}
	return nil
}

func handleFocusContacts(ev *tcell.EventKey) *tcell.EventKey {
	ResetMsgSelection()
	if !chatTable.HasFocus() {
		app.SetFocus(chatTable)
	}
	return nil
}

func handleSwitchPanels(ev *tcell.EventKey) *tcell.EventKey {
	ResetMsgSelection()
	focus := app.GetFocus()
	if focus == textInput {
		if statusTable != nil && statusTable.GetRowCount() > 0 {
			app.SetFocus(statusTable)
		} else {
			app.SetFocus(chatTable)
		}
	} else if focus == statusTable {
		app.SetFocus(chatTable)
	} else if focus == chatTable {
		app.SetFocus(groupTable)
	} else {
		if statusTable != nil && statusTable.GetRowCount() > 0 {
			app.SetFocus(statusTable)
		} else {
			app.SetFocus(chatTable)
		}
	}
	return nil
}

func handleCommand(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		sessionManager.CommandChannel <- messages.Command{Name: command, Params: nil}
		return nil
	}
}

func handleCopyUser(ev *tcell.EventKey) *tcell.EventKey {
	if hls := textView.GetHighlights(); len(hls) > 0 {
		for _, val := range curRegions {
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
	} else if currentReceiver.Id != "" {
		err := clipboard.WriteAll(currentReceiver.Id)
		if err != nil {
			PrintText("failed to copy: " + err.Error())
		} else {
			PrintText("copied id of " + currentReceiver.Name + " to clipboard")
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
	textInput.SetText(textInput.GetText() + " " + text)
	return nil
}

func handleQuit(ev *tcell.EventKey) *tcell.EventKey {
	sessionManager.CommandChannel <- messages.Command{Name: "disconnect", Params: nil}
	app.Stop()
	return nil
}

func handleHelp(ev *tcell.EventKey) *tcell.EventKey {
	PrintHelp()
	return nil
}

func handleToggleMouse(ev *tcell.EventKey) *tcell.EventKey {
	if mouseState {
		app.EnableMouse(false)
		mouseState = false
		PrintText("[::b]Mouse interaction DISABLED (Native selection enabled)[::-]")
	} else {
		app.EnableMouse(true)
		mouseState = true
		PrintText("[::b]Mouse interaction ENABLED (App selection enabled)[::-]")
	}
	return nil
}

func handleMessageCommand(command string) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		hls := textView.GetHighlights()
		if len(hls) > 0 {
			sessionManager.CommandChannel <- messages.Command{Name: command, Params: []string{hls[0]}}
			ResetMsgSelection()
			app.SetFocus(textInput)
		}
		return nil
	}
}

func handleMessagesMove(amount int) func(ev *tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		if curRegions == nil || len(curRegions) == 0 {
			return nil
		}
		hls := textView.GetHighlights()
		if len(hls) > 0 {
			newId := GetOffsetMsgId(hls[0], amount)
			if newId != "" {
				textView.Highlight(newId)
			}
		} else {
			if amount < 0 {
				textView.Highlight(curRegions[0].Id)
			} else {
				textView.Highlight(curRegions[len(curRegions)-1].Id)
			}
		}
		textView.ScrollToHighlight()
		return nil
	}
}

func handleChatPanelUp(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := chatTable.GetSelection()
	if row > 0 {
		chatTable.Select(row-1, 0)
	} else {
		// Jump to Status if at top
		if statusTable.GetRowCount() > 0 {
			app.SetFocus(statusTable)
			statusTable.Select(0, 0)
		}
	}
	return nil
}

func handleChatPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := chatTable.GetSelection()
	if row < chatTable.GetRowCount()-1 {
		chatTable.Select(row+1, 0)
	} else {
		// Jump to groups if at bottom
		if groupTable.GetRowCount() > 0 {
			app.SetFocus(groupTable)
			// Select first group
			groupTable.Select(0, 0)
		}
	}
	return nil
}

func handleGroupPanelUp(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := groupTable.GetSelection()
	if row > 0 {
		groupTable.Select(row-1, 0)
	} else {
		// Jump back to chats if at top
		if chatTable.GetRowCount() > 0 {
			app.SetFocus(chatTable)
			// Select last chat
			chatTable.Select(chatTable.GetRowCount()-1, 0)
		}
	}
	return nil
}

func handleGroupPanelDown(ev *tcell.EventKey) *tcell.EventKey {
	row, _ := groupTable.GetSelection()
	if row < groupTable.GetRowCount()-1 {
		groupTable.Select(row+1, 0)
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
	if chatTable.GetRowCount() > 0 {
		app.SetFocus(chatTable)
		chatTable.Select(0, 0)
	}
	return nil
}

func handleStatusPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	app.SetFocus(chatTable)
	return nil
}

func handleChatPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	app.SetFocus(groupTable)
	return nil
}

func handleGroupPanelTab(ev *tcell.EventKey) *tcell.EventKey {
	app.SetFocus(textView)
	return nil
}

func handleMessagePanelTab(ev *tcell.EventKey) *tcell.EventKey {
	app.SetFocus(textInput)
	return nil
}

func handleMessagesLast(ev *tcell.EventKey) *tcell.EventKey {
	if curRegions == nil || len(curRegions) == 0 {
		return nil
	}
	textView.Highlight(curRegions[len(curRegions)-1].Id)
	textView.ScrollToHighlight()
	return nil
}

func handleMessagesFirst(ev *tcell.EventKey) *tcell.EventKey {
	if curRegions == nil || len(curRegions) == 0 {
		return nil
	}
	textView.Highlight(curRegions[0].Id)
	textView.ScrollToHighlight()
	return nil
}

func handleExitMessages(ev *tcell.EventKey) *tcell.EventKey {
	if curRegions == nil || len(curRegions) == 0 {
		return nil
	}
	ResetMsgSelection()
	app.SetFocus(textInput)
	return nil
}

// load the key map
func LoadShortcuts() {
	// global bindings for app
	keyBindings = cbind.NewConfiguration()
	if err := keyBindings.Set(config.Config.Keymap.FocusMessages, handleFocusMessage); err != nil {
		PrintErrorMsg("focus_messages:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.FocusInput, handleFocusInput); err != nil {
		PrintErrorMsg("focus_input:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.FocusChats, handleFocusContacts); err != nil {
		PrintErrorMsg("focus_contacts:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.SwitchPanels, handleSwitchPanels); err != nil {
		PrintErrorMsg("switch_panels:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.CommandRead, handleCommand("read")); err != nil {
		PrintErrorMsg("command_read:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.Copyuser, handleCopyUser); err != nil {
		PrintErrorMsg("copyuser:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.Pasteuser, handlePasteUser); err != nil {
		PrintErrorMsg("pasteuser:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.CommandBacklog, handleCommand("backlog")); err != nil {
		PrintErrorMsg("command_backlog:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.CommandConnect, handleCommand("login")); err != nil {
		PrintErrorMsg("command_connect:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.CommandQuit, handleQuit); err != nil {
		PrintErrorMsg("command_quit:", err)
	}
	if err := keyBindings.Set(config.Config.Keymap.CommandHelp, handleHelp); err != nil {
		PrintErrorMsg("command_help:", err)
	}
	// Toggle mouse binding (Hardcoded for now as it's a new feature)
	keyBindings.SetRune(tcell.ModCtrl, 'p', handleToggleMouse)

	app.SetInputCapture(keyBindings.Capture)
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
	textView.SetInputCapture(keysMessages.Capture)
	keysChatPanel := cbind.NewConfiguration()
	keysChatPanel.SetRune(tcell.ModCtrl, 'u', handleChatPanelUp)
	keysChatPanel.SetRune(tcell.ModCtrl, 'd', handleChatPanelDown)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyUp, handleChatPanelUp)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyDown, handleChatPanelDown)
	keysChatPanel.SetKey(tcell.ModNone, tcell.KeyTab, handleChatPanelTab)
	chatTable.SetInputCapture(keysChatPanel.Capture)

	keysGroupPanel := cbind.NewConfiguration()
	keysGroupPanel.SetRune(tcell.ModCtrl, 'u', handleGroupPanelUp)
	keysGroupPanel.SetRune(tcell.ModCtrl, 'd', handleGroupPanelDown)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyUp, handleGroupPanelUp)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyDown, handleGroupPanelDown)
	keysGroupPanel.SetKey(tcell.ModNone, tcell.KeyTab, handleGroupPanelTab)
	groupTable.SetInputCapture(keysGroupPanel.Capture)

	keysStatusPanel := cbind.NewConfiguration()
	keysStatusPanel.SetRune(tcell.ModCtrl, 'd', handleStatusPanelDown)
	keysStatusPanel.SetKey(tcell.ModNone, tcell.KeyDown, handleStatusPanelDown)
	keysStatusPanel.SetKey(tcell.ModNone, tcell.KeyTab, handleStatusPanelTab)
	statusTable.SetInputCapture(keysStatusPanel.Capture)
}

// EnterCommand is the DoneFunc for the input field
func EnterCommand(key tcell.Key) {
	if key == tcell.KeyEnter {
		cmd := textInput.GetText()
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
			sessionManager.CommandChannel <- messages.Command{Name: cm, Params: params}
		} else {
			if currentReceiver.Id == "" {
				PrintText("no receiver")
				textInput.SetText("")
				return
			}
			sessionManager.CommandChannel <- messages.Command{Name: "send", Params: []string{currentReceiver.Id, cmd}}
		}
		textInput.SetText("")
	} else if key == tcell.KeyEsc {
		textInput.SetText("")
	}
}

// get the next message id to select (highlighted + offset)
func GetOffsetMsgId(curId string, offset int) string {
	if curRegions == nil || len(curRegions) == 0 {
		return ""
	}
	for idx, val := range curRegions {
		if val.Id == curId {
			arrPos := idx + offset
			if len(curRegions) > arrPos && arrPos >= 0 {
				return curRegions[arrPos].Id
			}
		}
	}
	if offset > 0 {
		return curRegions[0].Id
	} else {
		return curRegions[len(curRegions)-1].Id
	}
}

// resets the selection in the textView and scrolls it down
func ResetMsgSelection() {
	if len(textView.GetHighlights()) > 0 {
		textView.Highlight("")
	}
	textView.ScrollToEnd()
}
