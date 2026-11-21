// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
)

func quoteSamples() []messages.Message {
	now := uint64(time.Now().Unix())
	long := strings.Repeat("bahut lamba message hai yeh ", 12)
	return []messages.Message{
		{Id: "orig", ChatId: chatA.Id, ContactId: chatA.Id, ContactShort: "Asha", Text: "mera dimag khrb ho rha h", Timestamp: now - 300},
		{Id: "r1", ChatId: chatA.Id, FromMe: true, Text: "Q beta, kya hua?", Timestamp: now - 200,
			QuoteId: "orig", QuoteName: "Asha", QuoteText: "mera dimag khrb ho rha h"},
		{Id: "r2", ChatId: chatA.Id, ContactId: chatA.Id, ContactShort: "Asha", Text: "kuch nahi yaar", Timestamp: now - 100,
			QuoteId: "r1", QuoteName: messages.QuoteYou, QuoteText: "Q beta, kya hua?"},
		{Id: "r3", ChatId: chatA.Id, FromMe: true, Text: "ok", Timestamp: now - 50,
			QuoteId: "gone", QuoteName: "Asha", QuoteText: long},
	}
}

// Replies show the quote at the top of the bubble, and every bubble stays a
// rectangle; a long quote is cut to two lines ending in …
func TestReplyBubblesShowQuote(t *testing.T) {
	msgs := quoteSamples()
	for _, w := range []int{100, 50} {
		t.Run(fmt.Sprint(w), func(t *testing.T) { checkReplyBubbles(t, w, msgs) })
	}
}

func checkReplyBubbles(t *testing.T, w int, msgs []messages.Message) {
	{
		app, screen := runBubbleView(t, w, 60, msgs, chatA.Id)
		if !waitFor(app, func() bool {
			return messageWrapWidth == w && screenContains(screen, "ok") && screenContains(screen, "┃ You")
		}) {
			t.Fatalf("width %d:\n%s", w, screenText(screen))
		}
		cells, cw, ch := snapshot(app, screen)
		boxes := bubbleBoxes(t, cells, cw, ch)
		if len(boxes) != len(msgs) {
			t.Fatalf("width %d: %d bubbles\n%s", w, len(boxes), cellsText(cells, cw, ch))
		}
		r1, r3 := boxes[1].text, boxes[3].text
		if !strings.Contains(r1, "┃ Asha") || !strings.Contains(r1, "┃ mera dimag") || !strings.Contains(r1, "Q beta, kya hua?") {
			t.Errorf("width %d: reply bubble:\n%s", w, r1)
		}
		if strings.Index(r1, "┃ Asha") > strings.Index(r1, "Q beta") {
			t.Errorf("quote must come before the reply text:\n%s", r1)
		}
		if n := strings.Count(r3, "┃"); n != 3 || !strings.Contains(r3, "…") {
			t.Errorf("width %d: long quote should be a name and two lines ending in …:\n%s", w, r3)
		}
	}
}

func TestReplyClassicShowsQuoteLine(t *testing.T) {
	saved, savedCtx := config.Config.Ui.MessageStyle, ctx
	ctx = &AppContext{}
	config.Config.Ui.MessageStyle = "classic"
	t.Cleanup(func() { config.Config.Ui.MessageStyle = saved; ctx = savedCtx })
	out := getMessagesString(quoteSamples()[:2])
	lines := strings.Split(out, "\n")
	var q, r int = -1, -1
	for i, l := range lines {
		if strings.Contains(l, "┃ Asha: mera dimag") {
			q = i
		}
		if strings.Contains(l, "Q beta, kya hua?") {
			r = i
		}
	}
	if q < 0 || r != q+1 {
		t.Fatalf("quote line must be right above the reply:\n%s", out)
	}
}

func TestReplyFlow(t *testing.T) {
	app, input, cmds := composeApp(t)
	msgs := quoteSamples()
	run(app, func() {
		enterChat(chatA)
		ctx.CurRegions = msgs
		ctx.TextView.SetText(getMessagesString(msgs))
		ctx.TextView.Highlight("orig")
	})
	run(app, func() { handleReplyToSelected(nil) })
	var label string
	run(app, func() { label = input.GetLabel() })
	if !strings.Contains(label, "↳ Asha: mera dimag") {
		t.Fatalf("label = %q", label)
	}
	typeKeys(app, input, "kya hua", tcell.KeyEnter)
	cmd := <-cmds
	if cmd.Name != "reply" || strings.Join(cmd.Params, "|") != chatA.Id+"|orig|kya hua" {
		t.Fatalf("sent %+v", cmd)
	}
	run(app, func() { label = input.GetLabel() })
	if label != "" || replying != nil || inputText(app, input) != "" {
		t.Fatalf("after sending: label %q, input %q", label, inputText(app, input))
	}

	// Esc cancels the reply but keeps the text.
	run(app, func() { ctx.TextView.Highlight("orig"); handleReplyToSelected(nil) })
	typeKeys(app, input, "half typed", tcell.KeyEscape)
	run(app, func() { label = input.GetLabel() })
	if label != "" || replying != nil || inputText(app, input) != "half typed" {
		t.Fatalf("after Esc: label %q, input %q", label, inputText(app, input))
	}
	select {
	case c := <-cmds:
		t.Fatalf("Esc sent %+v", c)
	default:
	}
}

func TestJumpToQuote(t *testing.T) {
	app, _, _ := composeApp(t)
	msgs := quoteSamples()
	run(app, func() {
		enterChat(chatA)
		ctx.CurRegions = msgs
		ctx.TextView.SetText(getMessagesString(msgs))
		ctx.TextView.Highlight("r2")
		handleJumpToQuote(nil)
	})
	var hl []string
	run(app, func() { hl = ctx.TextView.GetHighlights() })
	if len(hl) != 1 || hl[0] != "r1" {
		t.Fatalf("highlight = %v, want r1", hl)
	}
	run(app, func() { ctx.TextView.Highlight("r3"); handleJumpToQuote(nil) })
	var pane string
	run(app, func() { pane = ctx.TextView.GetText(true); hl = ctx.TextView.GetHighlights() })
	if !strings.Contains(pane, "quoted message is not loaded") || hl[0] != "r3" {
		t.Fatalf("missing quote: highlight %v, pane end %q", hl, pane[max(0, len(pane)-120):])
	}
}
