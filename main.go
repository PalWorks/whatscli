package main

import (
	"flag"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

var VERSION string = "v1.3.0"

const batchSize = 50

func main() {
	// T-2: Parse --debug flag before anything else.
	debugMode := flag.Bool("debug", false, "Enable verbose WhatsApp protocol logging")
	flag.Parse()

	err := config.InitConfig()
	if err != nil {
		fmt.Printf("Failed to initialize config: %v\n", err)
		return
	}

	// Initialize the application context (replaces package-level globals)
	ctx = &AppContext{
		MouseState: true,
		ChatLimit:  50,
	}

	ctx.UiHandler = UiHandler{}
	ctx.SessionManager = &messages.SessionManager{}
	ctx.SessionManager.SetDebug(*debugMode)
	if err := ctx.SessionManager.Init(ctx.UiHandler); err != nil {
		fmt.Printf("Failed to initialize session: %v\n", err)
		return
	}

	ctx.App = tview.NewApplication()

	sideBarWidth := config.Config.Ui.ChatSidebarWidth
	gridLayout := tview.NewGrid()
	gridLayout.SetRows(1, 0, 1)
	gridLayout.SetColumns(sideBarWidth, 0, sideBarWidth)
	gridLayout.SetBorders(true)
	gridLayout.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	gridLayout.SetBordersColor(tcell.ColorNames[config.Config.Colors.Borders])

	cmdPrefix := config.Config.General.CmdPrefix
	ctx.TopBar = tview.NewTextView()
	ctx.TopBar.SetDynamicColors(true)
	ctx.TopBar.SetScrollable(false)
	ctx.TopBar.SetText("[::b] WhatsCLI " + VERSION + "  [-::d]Type " + cmdPrefix + "help or press " + config.Config.Keymap.CommandHelp + " for help")
	ctx.TopBar.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])

	ctx.TextView = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetChangedFunc(func() {
			ctx.App.Draw()
		})
	ctx.TextView.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	ctx.TextView.SetTextColor(tcell.ColorNames[config.Config.Colors.Text])

	PrintHelp()

	ctx.TextInput = tview.NewInputField()
	ctx.TextInput.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	ctx.TextInput.SetFieldBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	ctx.TextInput.SetFieldTextColor(tcell.ColorNames[config.Config.Colors.InputText])
	ctx.TextInput.SetDoneFunc(EnterCommand)
	ctx.TextInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			if ctx.StatusTable != nil && ctx.StatusTable.GetRowCount() > 0 {
				ctx.App.SetFocus(ctx.StatusTable)
			} else {
				ctx.App.SetFocus(ctx.ChatTable)
			}
			return nil
		}
		if event.Key() == tcell.KeyDown {
			offset, _ := ctx.TextView.GetScrollOffset()
			offset += 1
			ctx.TextView.ScrollTo(offset, 0)
			return nil
		}
		if event.Key() == tcell.KeyUp {
			offset, _ := ctx.TextView.GetScrollOffset()
			offset -= 1
			ctx.TextView.ScrollTo(offset, 0)
			return nil
		}
		if event.Key() == tcell.KeyPgDn {
			offset, _ := ctx.TextView.GetScrollOffset()
			offset += 10
			ctx.TextView.ScrollTo(offset, 0)
			return nil
		}
		if event.Key() == tcell.KeyPgUp {
			offset, _ := ctx.TextView.GetScrollOffset()
			offset -= 10
			ctx.TextView.ScrollTo(offset, 0)
			return nil
		}
		return event
	})

	gridLayout.AddItem(ctx.TopBar, 0, 0, 1, 4, 0, 0, false)
	gridLayout.AddItem(SetupLeftPane(), 1, 0, 2, 1, 0, 0, false)

	gridLayout.AddItem(ctx.TextView, 1, 1, 1, 3, 0, 0, false)
	gridLayout.AddItem(ctx.TextInput, 2, 1, 1, 3, 0, 0, false)

	ctx.App.SetRoot(gridLayout, true)
	ctx.App.EnableMouse(true)
	ctx.App.SetFocus(ctx.TextInput)
	if err := ctx.SessionManager.StartManager(); err != nil {
		PrintError(err)
	}
	LoadShortcuts()
	ctx.App.Run()
	ctx.SessionManager.Shutdown()
}
