// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// bubbleSamples covers what can break a bubble's edge: wrapping, line
// breaks, emoji (also joined ones and ❤️ with its style marker), Hindi,
// double-width Chinese, a link longer than a bubble, forwarded and media
// messages.
func bubbleSamples(day time.Time) []messages.Message {
	at := func(h, m int) uint64 {
		return uint64(time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.Local).Unix())
	}
	them := func(id string, ts uint64, text string) messages.Message {
		return messages.Message{Id: id, Timestamp: ts, ContactId: "111@s.whatsapp.net", ContactShort: "Asha", Text: text}
	}
	me := func(id string, ts uint64, text string) messages.Message {
		return messages.Message{Id: id, Timestamp: ts, FromMe: true, ContactShort: "Me", Text: text}
	}
	prevDay := uint64(day.AddDate(0, 0, -1).Unix())
	return []messages.Message{
		them("a", prevDay, "yesterday's message"),
		them("b", at(20, 10), "mera dimag khrb ho rha h"),
		me("c", at(20, 10), "Q beta"),
		me("d", at(20, 26), "Yaar\nIdhr aryan apne birthday ki party de rha h, aur main soch rha hu ki tumko lata acche acche cafe me"),
		them("e", at(20, 30), "🔥🔥 ok 😀 👩‍👩‍👧 ❤️ done"),
		them("f", at(20, 31), "नमस्ते दोस्त, कैसे हो? सब ठीक है ना"),
		me("g", at(20, 32), "你好世界你好世界你好世界你好世界你好世界你好世界你好世界你好世界"),
		me("h", at(20, 33), "https://www.instagram.com/reel/Df9f9shxCGG/?bqrl=YzAyMDM1MGJkZA==&x="+strings.Repeat("a", 90)),
		{Id: "i", Timestamp: at(20, 34), ContactId: "111@s.whatsapp.net", ContactShort: "Asha", Forwarded: true, Text: "forwarded text"},
		me("j", at(20, 47), "[IMAGE]"),
		me("k", at(20, 48), "x"),
	}
}

// runBubbleView draws msgs in a text view filling a w x h simulated screen.
func runBubbleView(t *testing.T, w, h int, msgs []messages.Message, chatID string) (*tview.Application, tcell.SimulationScreen) {
	t.Helper()
	savedStyle := config.Config.Ui.MessageStyle
	config.Config.Ui.MessageStyle = "bubbles"
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	app := tview.NewApplication().SetScreen(screen)
	view := tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWordWrap(true)
	saved := ctx
	ctx = &AppContext{App: app, TextView: view}
	ctx.CurrentReceiver.Id = chatID
	app.SetAfterDrawFunc(func(tcell.Screen) { rewrapOnResize() })
	done := make(chan struct{})
	go func() { _ = app.SetRoot(view, true).Run(); close(done) }()
	t.Cleanup(func() {
		app.Stop()
		<-done
		ctx = saved
		messageWrapWidth = 0
		config.Config.Ui.MessageStyle = savedStyle
	})
	app.QueueUpdateDraw(func() { screen.SetSize(w, h) })
	app.QueueUpdateDraw(func() { renderScreen(msgs) })
	return app, screen
}

type bubbleBox struct {
	bg                       tcell.Color
	top, bottom, left, right int
	text                     string
}

// snapshot copies the screen's cells on the app's event loop.
func snapshot(app *tview.Application, screen tcell.SimulationScreen) ([]tcell.SimCell, int, int) {
	type snap struct {
		cells []tcell.SimCell
		w, h  int
	}
	ch := make(chan snap, 1)
	app.QueueUpdateDraw(func() {
		cells, w, h := screen.GetContents()
		ch <- snap{append([]tcell.SimCell(nil), cells...), w, h}
	})
	s := <-ch
	return s.cells, s.w, s.h
}

func cellsText(cells []tcell.SimCell, w, h int) string {
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

// bubbleBoxes finds the bubbles in a screen snapshot (runs of rows with a
// bubble background) and fails the test unless each is a solid rectangle.
func bubbleBoxes(t *testing.T, cells []tcell.SimCell, w, h int) []bubbleBox {
	t.Helper()
	me := tcell.GetColor(config.Config.Colors.BubbleMe)
	them := tcell.GetColor(config.Config.Colors.BubbleContact)
	var boxes []bubbleBox
	var cur *bubbleBox
	for y := 0; y < h; y++ {
		left, right := -1, -1
		var bg tcell.Color
		var row strings.Builder
		wide := false
		for x := 0; x < w; x++ {
			_, b, _ := cells[y*w+x].Style.Decompose()
			if wide {
				// The second cell of a double-width character: the
				// simulated screen leaves it unstyled, a terminal paints
				// the character's background over both cells.
				wide = false
				if left >= 0 && right == x-1 {
					right = x
				}
				continue
			}
			wide = tview.TaggedStringWidth(string(cells[y*w+x].Runes)) == 2
			if b != me && b != them {
				continue
			}
			if left >= 0 && (b != bg || x != right+1) {
				t.Fatalf("row %d: bubble background has a gap or two colours at column %d\n%s", y, x, cellsText(cells, w, h))
			}
			if left < 0 {
				left, bg = x, b
			}
			right = x
			if r := cells[y*w+x].Runes; len(r) > 0 {
				row.WriteString(string(r))
			}
		}
		if left < 0 {
			cur = nil
			continue
		}
		if cur != nil && (cur.bg != bg || cur.left != left || cur.right != right) {
			t.Fatalf("rows %d-%d: bubble edge is not straight (cols %d-%d, then %d-%d)\n%s",
				cur.top, y, cur.left, cur.right, left, right, cellsText(cells, w, h))
		}
		if cur == nil {
			boxes = append(boxes, bubbleBox{bg: bg, top: y, left: left, right: right})
			cur = &boxes[len(boxes)-1]
		}
		cur.bottom = y
		cur.text += row.String() + "\n"
	}
	return boxes
}

func TestBubblesAreRectanglesOnTheRightSide(t *testing.T) {
	for _, w := range []int{100, 60, 34} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			msgs := bubbleSamples(time.Now())
			app, screen := runBubbleView(t, w, 120, msgs, "111@s.whatsapp.net")
			if !waitFor(app, func() bool { return messageWrapWidth == w && screenContains(screen, "20:48") }) {
				t.Fatalf("not rendered at width %d:\n%s", w, screenText(screen))
			}
			cells, cw, ch := snapshot(app, screen)
			boxes := bubbleBoxes(t, cells, cw, ch)
			if len(boxes) != len(msgs) {
				t.Fatalf("found %d bubbles, want %d\n%s", len(boxes), len(msgs), screenText(screen))
			}
			meBg := tcell.GetColor(config.Config.Colors.BubbleMe)
			for i, b := range boxes {
				if msgs[i].FromMe != (b.bg == meBg) {
					t.Errorf("bubble %d has the wrong colour", i)
				}
				if msgs[i].FromMe && b.right != w-2 {
					t.Errorf("own bubble %d ends at column %d, want %d", i, b.right, w-2)
				}
				if !msgs[i].FromMe && b.left != 1 {
					t.Errorf("received bubble %d starts at column %d, want 1", i, b.left)
				}
				if b.right-b.left+1 > w-2 {
					t.Errorf("bubble %d is wider than the pane", i)
				}
				// Half-row edges above and below, exactly as wide.
				for _, e := range []struct {
					y     int
					block string
				}{{b.top - 1, "▄"}, {b.bottom + 1, "▀"}} {
					row := []rune(strings.Split(cellsText(cells, cw, ch), "\n")[e.y])
					got := string(row[b.left : b.right+1])
					if got != strings.Repeat(e.block, b.right-b.left+1) ||
						(b.left > 0 && row[b.left-1] != ' ') || (b.right+1 < len(row) && row[b.right+1] != ' ') {
						t.Errorf("bubble %d: edge row %d is %q, want %d x %s", i, e.y, string(row), b.right-b.left+1, e.block)
					}
				}
				// Two cells of space between the edge and the text.
				lines := strings.Split(strings.TrimRight(b.text, "\n"), "\n")
				for _, line := range lines {
					if !strings.HasPrefix(line, "  ") || !strings.HasSuffix(line, "  ") {
						t.Errorf("bubble %d: line %q lacks padding", i, line)
					}
				}
			}
			for _, want := range []string{"Today", "Yesterday", "Forwarded", "Q beta"} {
				if !screenContains(screen, want) {
					t.Errorf("%q not shown", want)
				}
			}
			if screenContains(screen, "Asha") {
				t.Error("names are for group chats only")
			}
		})
	}
}

func TestBubbleNamesInGroupsOncePerRun(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local)
	ts := func(m int) uint64 { return uint64(day.Add(20*time.Hour + time.Duration(m)*time.Minute).Unix()) }
	msgs := []messages.Message{
		{Id: "1", Timestamp: ts(0), ContactId: "1@s.whatsapp.net", ContactShort: "Asha", Text: "one"},
		{Id: "2", Timestamp: ts(1), ContactId: "1@s.whatsapp.net", ContactShort: "Asha", Text: "two"},
		{Id: "3", Timestamp: ts(2), ContactId: "2@s.whatsapp.net", ContactShort: "Bina", Text: "three"},
		{Id: "4", Timestamp: ts(30), ContactId: "2@s.whatsapp.net", ContactShort: "Bina", Text: "four"},
	}
	saved := config.Config.Ui.MessageStyle
	config.Config.Ui.MessageStyle = "bubbles"
	t.Cleanup(func() { config.Config.Ui.MessageStyle = saved })
	var out []string
	var prev *messages.Message
	for i := range msgs {
		out = append(out, messageBlock(prev, &msgs[i], 80, true))
		prev = &msgs[i]
	}
	names := []bool{true, false, true, true} // a new run after a long pause
	for i, block := range out {
		name := msgs[i].ContactShort
		if strings.Contains(block, name) != names[i] {
			t.Errorf("message %d: name shown = %v, want %v\n%s", i, !names[i], names[i], block)
		}
	}
}

func TestBubbleSelectionHighlightsBubbleOnly(t *testing.T) {
	msg := messages.Message{Id: "m1", FromMe: true, Text: "a long message that wraps onto a second line for sure, yes"}
	saved := config.Config.Ui.MessageStyle
	config.Config.Ui.MessageStyle = "bubbles"
	t.Cleanup(func() { config.Config.Ui.MessageStyle = saved })
	lines := strings.Split(bubble(&msg, 50, false), "\n")
	for _, line := range lines[1 : len(lines)-1] { // the half-row edges are not selectable
		if !strings.HasPrefix(strings.TrimLeft(line, " "), `["m1"]`) {
			t.Errorf("region must start after the padding: %q", line)
		}
	}
}

func TestClassicStyleSetting(t *testing.T) {
	saved := config.Config.Ui.MessageStyle
	t.Cleanup(func() { config.Config.Ui.MessageStyle = saved })
	msg := messages.Message{Id: "m1", FromMe: true, Text: "hi"}
	config.Config.Ui.MessageStyle = "Classic"
	if got := messageBlock(nil, &msg, 80, false); got != getTextMessageString(&msg, 80) {
		t.Errorf("classic style should give the one-line layout, got %q", got)
	}
}

func TestDayLabel(t *testing.T) {
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, time.Local)
	for _, tt := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-time.Hour), "Today"},
		{now.AddDate(0, 0, -1), "Yesterday"},
		{time.Date(2026, 11, 13, 9, 0, 0, 0, time.Local), "Friday, 13 Nov"},
		{time.Date(2025, 1, 2, 9, 0, 0, 0, time.Local), "2 Jan 2025"},
	} {
		if got := dayLabel(tt.t, now); got != tt.want {
			t.Errorf("dayLabel(%v) = %q, want %q", tt.t, got, tt.want)
		}
	}
}

// TestBubbleDemo shows the sample chat in your own terminal, to check how
// it really draws there. Run it from a terminal (not from CI):
//
//	WHATSCLI_BUBBLE_DEMO=1 go test -run TestBubbleDemo .
//
// q or Esc quits; resizing the window re-wraps the chat.
func TestBubbleDemo(t *testing.T) {
	if os.Getenv("WHATSCLI_BUBBLE_DEMO") == "" {
		t.Skip("set WHATSCLI_BUBBLE_DEMO=1 to show the demo in this terminal")
	}
	saved := config.Config.Ui.MessageStyle
	config.Config.Ui.MessageStyle = "bubbles"
	defer func() { config.Config.Ui.MessageStyle = saved }()
	app := tview.NewApplication()
	view := tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWordWrap(true)
	view.SetBorder(true).SetTitle(" bubble demo: q quits ")
	savedCtx := ctx
	defer func() { ctx = savedCtx; messageWrapWidth = 0 }()
	ctx = &AppContext{App: app, TextView: view}
	app.SetAfterDrawFunc(func(tcell.Screen) { rewrapOnResize() })
	view.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape || ev.Rune() == 'q' {
			app.Stop()
		}
		return ev
	})
	msgs := bubbleSamples(time.Now())
	ctx.CurRegions = msgs
	view.SetText(getMessagesString(msgs))
	if err := app.SetRoot(view, true).Run(); err != nil {
		t.Fatal(err)
	}
}
