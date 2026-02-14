package main

import (
	"fmt"

	"code.rocketnine.space/tslocum/cbind"
	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

var VERSION string = "v1.0.42"

var currentReceiver messages.Chat = messages.Chat{}
var curRegions []messages.Message

var textView *tview.TextView
var leftPane *tview.Flex
var chatTable *tview.Table
var groupTable *tview.Table
var statusTable *tview.Table
var textInput *tview.InputField
var topBar *tview.TextView

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
	if err := sessionManager.Init(uiHandler); err != nil {
		fmt.Printf("Failed to initialize session: %v\n", err)
		return
	}

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
	textInput.SetFieldBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	textInput.SetFieldTextColor(tcell.ColorNames[config.Config.Colors.InputText])
	textInput.SetDoneFunc(EnterCommand)
	textInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
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
