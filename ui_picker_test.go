// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PalWorks/whatscli/messages"
	"github.com/PalWorks/whatscli/picker"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func testExtensions() picker.Extensions {
	return picker.Extensions{
		Image: picker.ParseExtensions("jpg,jpeg,png,webp"),
		Video: picker.ParseExtensions("mp4,3gp,mov,mkv,webm"),
		Audio: picker.ParseExtensions("mp3,ogg,opus,m4a,aac,amr,wav"),
	}
}

func TestOpensPicker(t *testing.T) {
	tests := []struct {
		cmd    string
		params []string
		want   bool
	}{
		{"upload", nil, true},
		{"sendimage", nil, true},
		{"sendvideo", []string{}, true},
		{"sendaudio", nil, true},
		{"upload", []string{"/a/x.pdf"}, false},
		{"send", nil, false},
		{"sendfile", nil, false},
	}
	for _, tt := range tests {
		if got := opensPicker(tt.cmd, tt.params); got != tt.want {
			t.Errorf("opensPicker(%q, %q) = %v, want %v", tt.cmd, tt.params, got, tt.want)
		}
	}
}

func TestPickedCommandsFixedCommand(t *testing.T) {
	got := pickedCommands([]string{"/a/x.png", "/a/y.txt"}, "chatA", "sendimage", false, testExtensions())
	want := []messages.Command{
		{Name: "sendfile", Params: []string{"chatA", "sendimage", "/a/x.png"}},
		{Name: "sendfile", Params: []string{"chatA", "sendimage", "/a/y.txt"}},
	}
	if !slices.EqualFunc(got, want, commandEqual) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPickedCommandsAuto(t *testing.T) {
	got := pickedCommands([]string{"/a/x.JPG", "/a/v.mp4", "/a/s.mp3", "/a/d.pdf"}, "chatA", "upload", true, testExtensions())
	cmds := make([]string, len(got))
	for i, c := range got {
		cmds[i] = c.Params[1]
	}
	if want := []string{"sendimage", "sendvideo", "sendaudio", "upload"}; !slices.Equal(cmds, want) {
		t.Fatalf("got %v, want %v", cmds, want)
	}
}

func TestPickedCommandsKeepsSpaces(t *testing.T) {
	got := pickedCommands([]string{"/a/my report.pdf"}, "chatA", "upload", false, testExtensions())
	if len(got) != 1 || !slices.Equal(got[0].Params, []string{"chatA", "upload", "/a/my report.pdf"}) {
		t.Fatalf("got %v", got)
	}
}

func commandEqual(a, b messages.Command) bool {
	return a.Name == b.Name && slices.Equal(a.Params, b.Params)
}

func TestPickerHint(t *testing.T) {
	tests := []struct {
		text string
		cmd  string
		ok   bool
	}{
		{"/upload ", "upload", true},
		{"/sendimage   ", "sendimage", true},
		{"/SendVideo ", "sendvideo", true},
		{"/sendaudio ", "sendaudio", true},
		{"/upload", "", false},             // still typing the command: command list
		{"/upload /home/x.pdf", "", false}, // a path is being typed
		{"/send ", "", false},
		{"upload ", "", false}, // no prefix: a chat message
	}
	for _, tt := range tests {
		cmd, ok := pickerHint(tt.text, "/")
		if cmd != tt.cmd || ok != tt.ok {
			t.Errorf("pickerHint(%q) = %q, %v; want %q, %v", tt.text, cmd, ok, tt.cmd, tt.ok)
		}
	}
}

func TestPickerHintEntrySaysPressEnter(t *testing.T) {
	for cmd, noun := range map[string]string{"upload": "files", "sendimage": "images", "sendvideo": "videos", "sendaudio": "audio files"} {
		e := pickerHintEntry(cmd)
		if !strings.Contains(e, "Enter") || !strings.Contains(e, "search "+noun+" with fzf") || !strings.Contains(e, "type a path") {
			t.Errorf("pickerHintEntry(%q) = %q", cmd, e)
		}
	}
}

func TestInputEntriesShowPickerHint(t *testing.T) {
	saved := ctx
	t.Cleanup(func() { ctx = saved })
	ctx = &AppContext{}
	got := inputEntries("/sendimage ")
	if len(got) != 1 || got[0] != pickerHintEntry("sendimage") {
		t.Fatalf("inputEntries(/sendimage ) = %q", got)
	}
	if got := inputEntries("/sendimage /a/b.png"); len(got) != 0 {
		t.Fatalf("hint should hide once a path is typed, got %q", got)
	}
}

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

// Picking /upload from the command list leaves "/upload " with the
// "Enter: search files" line showing; Enter on it opens the picker (here it
// reports that a chat must be open, since none is).
func TestUploadFromCommandListShowsHintAndEnterOpensPicker(t *testing.T) {
	app, input, view, screen := runInputApp(t)
	typeKeys(app, input, "/upl", tcell.KeyEnter)
	if !waitFor(app, func() bool { return input.GetText() == "/upload " }) {
		t.Fatalf("after picking the command, input = %q, want %q", input.GetText(), "/upload ")
	}
	if !waitFor(app, func() bool { return screenContains(screen, "search files with fzf") }) {
		t.Fatalf("hint not shown; screen:\n%s", screenText(screen))
	}
	typeKeys(app, input, tcell.KeyEnter)
	if !waitFor(app, func() bool {
		return input.GetText() == "" && strings.Contains(view.GetText(true), "needs an open chat")
	}) {
		t.Fatal("Enter on the hint did not clear the input and reach the picker")
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
