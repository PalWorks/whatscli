// SPDX-License-Identifier: AGPL-3.0-only

package main

// Helpers for tests that type into the input line of a real tview app
// running on a simulated screen.

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// runInputApp runs a real tview app on a simulated screen with the
// completion drop-down attached to its input field, so tests can type keys.
func runInputApp(t *testing.T) (*tview.Application, *tview.InputField, *tview.TextView, tcell.SimulationScreen) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	app := tview.NewApplication().SetScreen(screen)
	input := tview.NewInputField()
	view := tview.NewTextView()
	input.SetDoneFunc(EnterCommand)
	setupCommandCompletion(input)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(view, 0, 1, false).AddItem(input, 1, 0, true)
	saved := ctx
	ctx = &AppContext{App: app, TextInput: input, TextView: view}
	done := make(chan struct{})
	go func() { _ = app.SetRoot(layout, true).Run(); close(done) }()
	t.Cleanup(func() { app.Stop(); <-done; ctx = saved })
	return app, input, view, screen
}

// settle waits until the app has handled everything queued so far.
func settle(app *tview.Application) {
	for i := 0; i < 3; i++ {
		ch := make(chan struct{})
		app.QueueUpdate(func() { close(ch) })
		<-ch
	}
}

// waitFor polls cond on the app's event loop until it holds or 2s pass.
func waitFor(app *tview.Application, cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ok := make(chan bool, 1) // QueueUpdateDraw waits for the update to finish
		app.QueueUpdateDraw(func() { ok <- cond() })
		if <-ok {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// typeKeys feeds keys to the input field on the app's event loop, one at a
// time, then lets queued updates (drop-down refresh, picker) run.
func typeKeys(app *tview.Application, input *tview.InputField, keys ...any) {
	press := func(ev *tcell.EventKey) {
		done := make(chan struct{})
		app.QueueUpdate(func() {
			input.InputHandler()(ev, func(tview.Primitive) {})
			close(done)
		})
		<-done
		settle(app)
	}
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			for _, r := range k {
				press(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
			}
		case tcell.Key:
			press(tcell.NewEventKey(k, 0, tcell.ModNone))
		}
	}
}

func screenText(screen tcell.SimulationScreen) string {
	cells, w, h := screen.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if r := cells[y*w+x].Runes; len(r) > 0 {
				b.WriteRune(r[0])
			} else {
				b.WriteRune(' ')
			}
		}
		b.WriteRune('\n')
	}
	return b.String()
}

func screenContains(screen tcell.SimulationScreen, s string) bool {
	return strings.Contains(screenText(screen), s)
}
