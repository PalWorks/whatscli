// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"

	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestDisplayNameOneLine(t *testing.T) {
	for in, want := range map[string]string{
		"Asha":              "Asha",
		"🤏🏻\n 🧚🏻‍♀️\n":      "🤏🏻 🧚🏻‍♀️",
		"a\tb\r\nc":         "a b c",
		"  spaced   name  ": "spaced name",
	} {
		if got := displayName(in); got != want {
			t.Errorf("displayName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHangingIndent(t *testing.T) {
	got := hangingIndent("one two three four five", 6, 16)
	want := "one two \n      three \n      four five"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := hangingIndent("a\nb", 4, 40); got != "a\n    b" {
		t.Fatalf("explicit line break: got %q", got)
	}
	if got := hangingIndent("a b", 4, 0); got != "a b" {
		t.Fatalf("unknown width should leave text alone, got %q", got)
	}
}

// Every line of a long message, and of one with line breaks, starts under
// the first character of its text, and a name with line breaks stays on the
// first line.
func TestMessagesAlignInPane(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	app := tview.NewApplication().SetScreen(screen)
	view := tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWordWrap(true)
	saved := ctx
	ctx = &AppContext{App: app, TextView: view}
	done := make(chan struct{})
	app.SetAfterDrawFunc(func(tcell.Screen) { rewrapOnResize() })
	go func() { _ = app.SetRoot(view, true).Run(); close(done) }()
	t.Cleanup(func() { app.Stop(); <-done; ctx = saved; messageWrapWidth = 0 })

	msgs := []messages.Message{
		{Id: "1", Timestamp: 0, ContactShort: "Bo\nb", Text: strings.Repeat("word ", 40)},
		{Id: "2", Timestamp: 0, FromMe: true, Text: "Yaar\nsecond line"},
	}
	app.QueueUpdateDraw(func() { renderScreen(msgs) })
	if !waitFor(app, func() bool { return messageWrapWidth > 0 && screenContains(screen, "second line") }) {
		t.Fatalf("not rendered; screen:\n%s", screenText(screen))
	}
	lines := strings.Split(screenText(screen), "\n")
	first := strings.Index(lines[0], "word")
	if !strings.Contains(lines[0], ") Bo b: word") || first < 0 {
		t.Fatalf("first line %q", lines[0])
	}
	for i := 1; i < 3; i++ {
		if strings.Index(lines[i], "word") != first || strings.TrimSpace(lines[i][:first]) != "" {
			t.Errorf("line %d not aligned under the text:\n%s", i, strings.Join(lines[:6], "\n"))
		}
	}
	me := -1
	for i, l := range lines {
		if strings.Contains(l, "Me: Yaar") {
			me = i
		}
	}
	if me < 0 || strings.Index(lines[me+1], "second line") != strings.Index(lines[me], "Yaar") {
		t.Errorf("line break not aligned:\n%s", screenText(screen))
	}
}
