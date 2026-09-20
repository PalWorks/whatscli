package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

var VERSION string = "v1.8.0"

const batchSize = 50

func main() {
	// T-2: Parse --debug flag before anything else.
	debugMode := flag.Bool("debug", false, "Enable verbose WhatsApp protocol logging")
	logFile := flag.String("logfile", "", "Path to write the debug log to (default: <config dir>/whatscli/debug.log; only used with --debug)")
	flag.Parse()

	err := config.InitConfig()
	if err != nil {
		fmt.Printf("Failed to initialize config: %v\n", err)
		return
	}

	// Initialize the application context. This is the single wiring point:
	// ctx is a local, not a package-level global, and is injected into
	// UiHandler so the whole UI layer is reachable without process globals.
	ctx := &AppContext{
		MouseState: true,
		ChatLimit:  50,
	}
	ctx.initUpdates()

	ctx.UiHandler = UiHandler{ctx: ctx}
	ctx.SessionManager = &messages.SessionManager{}
	ctx.SessionManager.SetDebug(*debugMode)
	if *logFile != "" {
		ctx.SessionManager.SetLogFile(*logFile)
	}
	if err := ctx.SessionManager.Init(ctx.UiHandler); err != nil {
		fmt.Printf("Failed to initialize session: %v\n", err)
		return
	}

	ctx.App = tview.NewApplication()

	// Start the single UI-update consumer once App exists. draw wraps
	// QueueUpdateDraw (which blocks until the main loop draws), so updates
	// are applied strictly in the order they were enqueued.
	ctx.draw = func(f func()) { ctx.App.QueueUpdateDraw(f) }
	go ctx.processUpdates()

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

	ctx.PrintHelp()

	ctx.TextInput = tview.NewInputField()
	ctx.TextInput.SetBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	ctx.TextInput.SetFieldBackgroundColor(tcell.ColorNames[config.Config.Colors.Background])
	ctx.TextInput.SetFieldTextColor(tcell.ColorNames[config.Config.Colors.InputText])
	ctx.TextInput.SetDoneFunc(ctx.EnterCommand)
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
	gridLayout.AddItem(ctx.SetupLeftPane(), 1, 0, 2, 1, 0, 0, false)

	gridLayout.AddItem(ctx.TextView, 1, 1, 1, 3, 0, 0, false)
	gridLayout.AddItem(ctx.TextInput, 2, 1, 1, 3, 0, 0, false)

	ctx.App.SetRoot(gridLayout, true)
	ctx.App.EnableMouse(true)
	ctx.App.SetFocus(ctx.TextInput)
	if err := ctx.SessionManager.StartManager(); err != nil {
		ctx.PrintError(err)
	}
	ctx.LoadShortcuts()

	// An external SIGTERM/SIGINT (e.g. `kill`) stops the TUI event loop so
	// the graceful Shutdown() below runs — cancelling an in-progress voice
	// recording and closing the databases — instead of the process dying
	// with the recorder subprocess and temp file orphaned (plan H2.2).
	// Ctrl+C inside the TUI arrives as a tcell key event, not a signal, so
	// this only fires for signals from outside the terminal. SIGKILL cannot
	// be trapped; that residual case is documented in README.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sigCh {
			// App.Stop() is a no-op until Run() has created the screen, so a
			// signal landing in the tiny window before Run() would otherwise
			// be lost (master audit F7). Retry briefly until it takes effect;
			// once the app has stopped, Run() returns and the process exits,
			// so the extra idempotent Stop() calls are harmless.
			for i := 0; i < 50; i++ {
				ctx.App.Stop()
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	ctx.App.Run()
	ctx.SessionManager.Shutdown()
}
