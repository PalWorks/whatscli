package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"code.rocketnine.space/tslocum/cbind"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
	"github.com/skratchdot/open-golang/open"
)

var VERSION string = "v1.0.42"

var sndTxt string = ""
var currentReceiver messages.Chat = messages.Chat{}
var curRegions []messages.Message

var textView *tview.TextView
var leftPane *tview.Flex
var chatTable *tview.Table
var groupTable *tview.Table
var statusTable *tview.Table
var chatHeader *tview.TextView
var groupHeader *tview.TextView
var textInput *tview.InputField
var topBar *tview.TextView

// var topBar *tview.TextView // Removed duplicate

var app *tview.Application
var mouseState bool = true // Track mouse state

var sessionManager *messages.SessionManager

var keyBindings *cbind.Configuration

var uiHandler messages.UiMessageHandler

// Chat list state for lazy loading
var allChats []*messages.Conversation
var chatLimit int = 50

const batchSize = 50

func main() {
	err := config.InitConfig()
	if err != nil {
		fmt.Printf("Failed to initialize config: %v\n", err)
		return
	}
	uiHandler = UiHandler{}
	sessionManager = &messages.SessionManager{}
	sessionManager.Init(uiHandler)

	app = tview.NewApplication()

	sideBarWidth := config.Config.Ui.ChatSidebarWidth
	gridLayout := tview.NewGrid()
	gridLayout.SetRows(1, 0, 1)
	gridLayout.SetColumns(sideBarWidth, 0, sideBarWidth)
	gridLayout.SetBorders(true)
	gridLayout.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	gridLayout.SetBordersColor(tcell.ColorNames[config.Config.Colors.Borders])

	cmdPrefix := config.Config.General.CmdPrefix
	topBar = tview.NewTextView()
	topBar.SetDynamicColors(true)
	topBar.SetScrollable(false)
	topBar.SetText("[::b] WhatsCLI " + VERSION + "  [-::d]Type " + cmdPrefix + "help or press " + config.Config.Keymap.CommandHelp + " for help")
	topBar.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])

	// Status Bar removed as per new layout

	textView = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	textView.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	textView.SetTextColor(tcell.ColorNames[config.Config.Colors.Text])

	PrintHelp()

	textInput = tview.NewInputField()
	textInput.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	textInput.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	textInput.SetFieldBackgroundColor(tcell.ColorNames[config.Config.Colors.Background]) // Matches background, removing blue fill
	textInput.SetFieldTextColor(tcell.ColorNames[config.Config.Colors.InputText])
	textInput.SetChangedFunc(func(change string) {
		sndTxt = change
	})
	textInput.SetDoneFunc(EnterCommand)
	textInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			// Jump back to top (Status)
			if statusTable != nil && statusTable.GetRowCount() > 0 {
				app.SetFocus(statusTable)
			} else {
				app.SetFocus(chatTable)
			}
			return nil
		}
		if event.Key() == tcell.KeyDown {
			offset, _ := textView.GetScrollOffset()
			offset += 1
			textView.ScrollTo(offset, 0)
			return nil
		}
		if event.Key() == tcell.KeyUp {
			offset, _ := textView.GetScrollOffset()
			offset -= 1
			textView.ScrollTo(offset, 0)
			return nil
		}
		if event.Key() == tcell.KeyPgDn {
			offset, _ := textView.GetScrollOffset()
			offset += 10
			textView.ScrollTo(offset, 0)
			return nil
		}
		if event.Key() == tcell.KeyPgUp {
			offset, _ := textView.GetScrollOffset()
			offset -= 10
			textView.ScrollTo(offset, 0)
			return nil
		}
		return event
	})

	gridLayout.AddItem(topBar, 0, 0, 1, 4, 0, 0, false)
	// Replace old layout items with new leftPane
	// Row 1, Col 0, Span 2 Rows (1 and 2), Width 1
	gridLayout.AddItem(SetupLeftPane(), 1, 0, 2, 1, 0, 0, false)

	gridLayout.AddItem(textView, 1, 1, 1, 3, 0, 0, false)
	gridLayout.AddItem(textInput, 2, 1, 1, 3, 0, 0, false)

	app.SetRoot(gridLayout, true)
	app.EnableMouse(true)
	app.SetFocus(textInput)
	if err := sessionManager.StartManager(); err != nil {
		PrintError(err)
	}
	LoadShortcuts()
	app.Run()
	sessionManager.Shutdown()
}

// SetupLeftPane initializes the left panel with Status, Chats, and Groups
// SetupLeftPane initializes the left panel with Status, Chats, and Groups
// Left Pane Global
// SetupLeftPane creates the left side panel with Status, Chats, and Groups
func SetupLeftPane() *tview.Flex {
	// 1. Status Section
	statusTable = tview.NewTable()
	statusTable.SetSelectable(true, false)
	statusTable.SetBorder(true)
	statusTable.SetTitle(" Status ")
	statusTable.SetTitleAlign(tview.AlignCenter)
	statusTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// 2. Chats Section
	chatTable = tview.NewTable()
	chatTable.SetSelectable(true, false)
	chatTable.SetBorder(true)
	chatTable.SetTitle(" Chats ")
	chatTable.SetTitleAlign(tview.AlignCenter)
	chatTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// 3. Groups Section
	groupTable = tview.NewTable()
	groupTable.SetSelectable(true, false)
	groupTable.SetBorder(true)
	groupTable.SetTitle(" Groups ")
	groupTable.SetTitleAlign(tview.AlignCenter)
	groupTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// Helper to update focus appearance (mutually exclusive)
	setActiveTable := func(active *tview.Table) {
		tables := []*tview.Table{statusTable, chatTable, groupTable}
		for _, t := range tables {
			if t == active {
				t.SetSelectable(true, false)
				t.SetSelectedStyle(tcell.StyleDefault.Reverse(true))
			} else {
				t.SetSelectable(false, false)
				t.SetSelectedStyle(tcell.StyleDefault)
			}
		}
	}

	// Define selection handlers
	statusSelectFunc := func(row, column int) {
		if statusTable.HasFocus() && row >= 0 && row < statusTable.GetRowCount() {
			cell := statusTable.GetCell(row, column)
			if cell != nil {
				ref := cell.GetReference()
				if ref != nil {
					conv := ref.(*messages.Conversation)
					chat := messages.Chat{
						Id:          conv.JID,
						Name:        conv.Name,
						Unread:      int(conv.Unread),
						LastMessage: conv.LastMsgTime,
						IsGroup:     strings.HasSuffix(conv.JID, messages.GROUPSUFFIX),
					}
					SetDisplayedChat(chat)
				}
			}
		}
	}

	chatSelectFunc := func(row, column int) {
		if chatTable.HasFocus() && row >= 0 && row < chatTable.GetRowCount() {
			cell := chatTable.GetCell(row, column)
			if cell != nil {
				ref := cell.GetReference()
				if ref != nil {
					conv := ref.(*messages.Conversation)
					chat := messages.Chat{
						Id:          conv.JID,
						Name:        conv.Name,
						Unread:      int(conv.Unread),
						LastMessage: conv.LastMsgTime,
						IsGroup:     strings.HasSuffix(conv.JID, messages.GROUPSUFFIX),
					}
					SetDisplayedChat(chat)
				}
			}
		}
		// Infinite scroll trigger for contacts
		if row >= chatTable.GetRowCount()-5 && chatLimit < len(allChats) {
			chatLimit += batchSize
			RenderChatTable()
		}
	}

	groupSelectFunc := func(row, column int) {
		if groupTable.HasFocus() && row >= 0 && row < groupTable.GetRowCount() {
			cell := groupTable.GetCell(row, column)
			if cell != nil {
				ref := cell.GetReference()
				if ref != nil {
					conv := ref.(*messages.Conversation)
					chat := messages.Chat{
						Id:          conv.JID,
						Name:        conv.Name,
						Unread:      int(conv.Unread),
						LastMessage: conv.LastMsgTime,
						IsGroup:     strings.HasSuffix(conv.JID, messages.GROUPSUFFIX),
					}
					SetDisplayedChat(chat)
				}
			}
		}
	}

	// Selection Changed Funcs
	statusTable.SetSelectionChangedFunc(statusSelectFunc)
	statusTable.SetFocusFunc(func() {
		setActiveTable(statusTable)
		row, col := statusTable.GetSelection()
		statusSelectFunc(row, col)
	})

	chatTable.SetSelectionChangedFunc(chatSelectFunc)
	chatTable.SetFocusFunc(func() {
		setActiveTable(chatTable)
		row, col := chatTable.GetSelection()
		chatSelectFunc(row, col)
	})

	groupTable.SetSelectionChangedFunc(groupSelectFunc)
	groupTable.SetFocusFunc(func() {
		setActiveTable(groupTable)
		row, col := groupTable.GetSelection()
		groupSelectFunc(row, col)
	})

	// Initialize styles (start with all inactive)
	setActiveTable(nil)

	leftPane = tview.NewFlex().SetDirection(tview.FlexRow)
	// Status (Fixed height: Title/Border + 1 row + Border = 4?)
	// Let's try height 5 to be safe (Title, TopBorder, Row, BottomBorder... wait)
	// tview table with border:
	// Border top, content, border bottom.
	// If 1 row of content: Top + 1 + Bottom = 3 lines.
	// Let's give it 3 lines.
	leftPane.AddItem(statusTable, 3, 1, false)

	leftPane.AddItem(chatTable, 0, 1, true)
	leftPane.AddItem(groupTable, 0, 1, false)

	return leftPane
}

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

// prints help to chat view
// prints help to chat view
func PrintHelp() {
	cmdPrefix := config.Config.General.CmdPrefix
	fmt.Fprintln(textView, "")
	fmt.Fprintln(textView, "[::b]Application & Connection[::-]")
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"connect / "+config.Config.Keymap.CommandConnect, "(Re)Connect"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"disconnect", "Disconnect"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"logout", "Logout & Delete Data"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"quit / "+config.Config.Keymap.CommandQuit, "Exit"))
	fmt.Fprintln(textView, "")

	fmt.Fprintln(textView, "[::b]Navigation & View[::-]")
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", "Up/Down", "Scroll Chats/History"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.SwitchPanels, "Switch Panel"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.FocusMessages, "Focus Messages"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", "Ctrl+p", "Toggle Mouse"))
	fmt.Fprintln(textView, "")

	fmt.Fprintln(textView, "[::b]Chat & Messages[::-]")
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"backlog / "+config.Config.Keymap.CommandBacklog, "Load Backlog"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"read / "+config.Config.Keymap.CommandRead, "Mark Read"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", "Up/Down", "Select Message"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageDownload, "Download"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageOpen, "Download & Open"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageShow, "Show Image ("+config.Config.General.ShowCommand+")"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageUrl, "Open URL"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageRevoke, "Revoke"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageInfo, "Info"))
	fmt.Fprintln(textView, "")

	fmt.Fprintln(textView, "[::b]Media[::-]")
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"upload <path>", "Send File"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"sendimage <path>", "Send Image"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"sendvideo <path>", "Send Video"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"sendaudio <path>", "Send Audio"))
	fmt.Fprintln(textView, "")

	fmt.Fprintln(textView, "[::b]Groups[::-]")
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"create <nums> <subj>", "Create"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"subject <subj>", "Subject"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"leave", "Leave"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"add <userid>", "Add"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"remove <userid>", "Kick"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"admin <userid>", "Promote"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"removeadmin <userid>", "Demote"))
	fmt.Fprintln(textView, "")

	fmt.Fprintln(textView, "[::b]Clipboard[::-]")
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.Copyuser, "Copy UserID"))
	fmt.Fprintln(textView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.Pasteuser, "Paste UserID"))
	fmt.Fprintln(textView, "")

	fmt.Fprintln(textView, "Config: ", config.GetConfigFilePath())
	fmt.Fprintln(textView, "Verify: ", VERSION)
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

// prints text to the TextView
func PrintText(txt string) {
	fmt.Fprintln(textView, txt)
}

// prints an error to the TextView
func PrintError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(textView, "["+config.Config.Colors.Negative+"]", err.Error(), "[-]")
}

// prints an error to the TextView
func PrintErrorMsg(text string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(textView, "["+config.Config.Colors.Negative+"]", text, err.Error(), "[-]")
}

// prints an image attachment to the TextView (by message id)
func PrintImage(path string) {
	// Sanitize path to prevent command injection / path traversal
	cleanPath := filepath.Clean(path)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		PrintError(fmt.Errorf("invalid file path: %v", err))
		return
	}

	// Verify the file exists
	if _, err := os.Stat(absPath); err != nil {
		PrintError(fmt.Errorf("file not found: %v", err))
		return
	}

	// Verify the path is under the configured download or preview directory
	downloadDir := filepath.Clean(config.Config.General.DownloadPath)
	previewDir := filepath.Clean(config.Config.General.PreviewPath)
	if !strings.HasPrefix(absPath, downloadDir) && !strings.HasPrefix(absPath, previewDir) {
		PrintError(fmt.Errorf("file path outside allowed directories"))
		return
	}

	cmdParts := strings.Split(config.Config.General.ShowCommand, " ")
	cmdParts = append(cmdParts, absPath)
	var cmd *exec.Cmd
	size := len(cmdParts)
	if size > 1 {
		cmd = exec.Command(cmdParts[0], cmdParts[1:]...)
	} else if size > 0 {
		cmd = exec.Command(cmdParts[0])
	}
	var stdout io.ReadCloser
	if stdout, err = cmd.StdoutPipe(); err == nil {
		if err = cmd.Start(); err == nil {
			reader := bufio.NewReader(stdout)
			io.Copy(tview.ANSIWriter(textView), reader)
			return
		}
	}
	PrintError(err)
}

// updates the status bar
func UpdateStatusBar(statusInfo messages.SessionStatus) {
	out := " "
	if statusInfo.Connected {
		out += "[" + config.Config.Colors.Positive + "]online[-]"
	} else {
		out += "[" + config.Config.Colors.Negative + "]offline[-]"
	}
	out += " "
	out += "[::d] ("
	out += fmt.Sprint(statusInfo.BatteryCharge)
	out += "%"
	if statusInfo.BatteryLoading {
		out += " [" + config.Config.Colors.Positive + "]L[-]"
	} else {
		out += " [" + config.Config.Colors.Negative + "]l[-]"
	}
	if statusInfo.BatteryPowersave {
		out += " [" + config.Config.Colors.Negative + "]S[-]"
	} else {
		out += " [" + config.Config.Colors.Positive + "]s[-]"
	}
	out += ")[::-] "
	out += statusInfo.LastSeen
	// InfoBar removed
	// infoBar.SetText(out)
}

// sets the current chat, loads text from storage to TextView
func SetDisplayedChat(chat messages.Chat) {
	currentReceiver = chat
	textView.Clear()
	textView.SetTitle(chat.Name)
	sessionManager.CommandChannel <- messages.Command{Name: "select", Params: []string{currentReceiver.Id}}
}

// get a string representation of all messages for chat
func getMessagesString(msgs []messages.Message) string {
	out := ""
	for _, msg := range msgs {
		out += getTextMessageString(&msg)
		out += "\n"
	}
	return out
}

// create a formatted string with regions based on message ID from a text message
// TODO: optimize, use Sprintf etc
func getTextMessageString(msg *messages.Message) string {
	colorMe := config.Config.Colors.ChatMe
	colorContact := config.Config.Colors.ChatContact
	out := ""
	text := tview.Escape(msg.Text)
	if msg.Forwarded {
		text = "[" + config.Config.Colors.ForwardedText + "]" + text + "[-]"
	}
	tim := time.Unix(int64(msg.Timestamp), 0)
	time := tim.Format("02-01-06 15:04:05")
	out += "[\""
	out += msg.Id
	out += "\"]"
	if msg.FromMe { //msg from me
		out += "[-::d](" + time + ") [" + colorMe + "::b]Me: [-::-]" + text
	} else { // message from others
		out += "[-::d](" + time + ") [" + colorContact + "::b]" + msg.ContactShort + ": [-::-]" + text
	}
	out += "[\"\"]"
	return out
}

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

// RenderChatTable renders the table based on allChats and chatLimit
func RenderChatTable() {
	// Snapshot current selection by JID
	var selectedStatusJID string
	var selectedChatJID string
	var selectedGroupJID string

	if rowS, _ := statusTable.GetSelection(); rowS >= 0 && rowS < statusTable.GetRowCount() {
		if cell := statusTable.GetCell(rowS, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedStatusJID = ref.(*messages.Conversation).JID
			}
		}
	}
	if rowC, _ := chatTable.GetSelection(); rowC >= 0 && rowC < chatTable.GetRowCount() {
		if cell := chatTable.GetCell(rowC, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedChatJID = ref.(*messages.Conversation).JID
			}
		}
	}
	if rowG, _ := groupTable.GetSelection(); rowG >= 0 && rowG < groupTable.GetRowCount() {
		if cell := groupTable.GetCell(rowG, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedGroupJID = ref.(*messages.Conversation).JID
			}
		}
	}

	displayList := allChats
	if len(displayList) > chatLimit {
		displayList = displayList[:chatLimit]
	}

	statusTable.Clear()
	chatTable.Clear()
	groupTable.Clear()

	sIdx := 0
	cIdx := 0
	gIdx := 0

	// Indexes to restore
	newRowS := 0
	newRowC := 0
	newRowG := 0

	for _, conv := range displayList {
		// Name cell
		name := conv.Name
		if name == "" {
			name = conv.JID // Fallback
		}

		// Unread status
		if conv.Unread > 0 {
			name += " ([" + config.Config.Colors.UnreadCount + "]" + fmt.Sprint(conv.Unread) + "[-])"
		}

		// Pinned indicator
		if conv.IsPinned {
			name = "📌 " + name
		}

		cell := tview.NewTableCell(name).
			SetReference(conv).
			SetExpansion(1).
			SetSelectable(true)

		// Color & Split
		if conv.JID == messages.STATUSSUFFIX {
			// Status
			cell.SetTextColor(tcell.ColorYellow) // Distinct color
			statusTable.SetCell(sIdx, 0, cell)
			if conv.JID == selectedStatusJID {
				newRowS = sIdx
			}
			sIdx++
		} else if strings.HasSuffix(conv.JID, messages.GROUPSUFFIX) {
			// Group
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListGroup])
			groupTable.SetCell(gIdx, 0, cell)
			if conv.JID == selectedGroupJID {
				newRowG = gIdx
			}
			gIdx++
		} else {
			// Contact
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListContact])
			chatTable.SetCell(cIdx, 0, cell)
			if conv.JID == selectedChatJID {
				newRowC = cIdx
			}
			cIdx++
		}
	}

	// Restore selection
	if statusTable.GetRowCount() > 0 {
		statusTable.Select(newRowS, 0)
	}

	if chatTable.GetRowCount() > 0 {
		chatTable.Select(newRowC, 0)
	}

	if groupTable.GetRowCount() > 0 {
		groupTable.Select(newRowG, 0)
	}
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
