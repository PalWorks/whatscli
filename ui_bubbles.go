// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_bubbles.go: the "bubbles" message style. Received messages are drawn on
// the left and your own on the right, each in a block with a tinted
// background and its time (HH:MM) at the bottom right. A centred line marks
// each new day; in group chats the sender's name heads a run of messages.
//
// Every line of a bubble is padded with spaces to the same width, so the
// background is always a rectangle. Widths come from tview, the same
// measure the text view draws with. The region tag that makes a message
// selectable is opened after the left padding of each line, so a selected
// message highlights the bubble only.

import (
	"strings"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/rivo/tview"
)

const (
	// runGap is the longest pause between two messages of one sender that
	// still counts as one run (one name shown in group chats).
	runGap = 10 * time.Minute
	// bubbleShare is the widest a bubble may be, as a share of the pane.
	bubbleShare = 0.75
	// fallbackWidth is used before the message pane has been drawn once.
	fallbackWidth = 80
	// bubblePadX is the space between a bubble's edge and its text.
	bubblePadX = 2
)

// classicStyle reports whether the one-line-per-message layout is set
// (ui.message_style = classic).
func classicStyle() bool {
	return strings.EqualFold(strings.TrimSpace(config.Config.Ui.MessageStyle), "classic")
}

// isGroupChat reports whether the open chat is a group.
func isGroupChat() bool {
	return strings.HasSuffix(ctx.CurrentReceiver.Id, messages.GROUPSUFFIX)
}

// statusMark is the tick text shown after the time of our own message:
// ✓ sent, ✓✓ delivered, blue ✓✓ read; "" when the status is not known.
func statusMark(status int) string {
	switch status {
	case messages.StatusSent:
		return "[" + config.Config.Colors.BubbleTime + "::]✓"
	case messages.StatusDelivered:
		return "[" + config.Config.Colors.BubbleTime + "::]✓✓"
	case messages.StatusRead:
		return "[" + config.Config.Colors.StatusRead + "::]✓✓"
	}
	return ""
}

// messageBlock returns the text for msg, which follows prev (nil for the
// first message shown), in the configured style, without a final newline.
// last is set for the last message of the chat: only that one shows its
// status, and only if it is ours.
func messageBlock(prev, msg *messages.Message, width int, group, last bool) string {
	mark := ""
	if last && msg.FromMe {
		mark = statusMark(msg.Status)
	}
	if classicStyle() {
		return getTextMessageString(msg, width, mark)
	}
	if width <= 0 {
		width = fallbackWidth
	}
	at := time.Unix(int64(msg.Timestamp), 0)
	newDay := prev == nil || !sameDay(time.Unix(int64(prev.Timestamp), 0), at)
	var b strings.Builder
	switch {
	case newDay:
		if prev != nil {
			b.WriteString("\n")
		}
		b.WriteString(daySeparator(at, time.Now(), width))
		b.WriteString("\n\n")
	}
	showName := group && !msg.FromMe && (newDay || !sameRun(prev, msg))
	b.WriteString(bubble(msg, width, showName, mark))
	return b.String()
}

func sameDay(a, b time.Time) bool {
	ya, ma, da := a.Date()
	yb, mb, db := b.Date()
	return ya == yb && ma == mb && da == db
}

// sameRun reports whether msg continues prev's run: same sender, at most
// runGap later.
func sameRun(prev, msg *messages.Message) bool {
	if prev == nil || prev.FromMe != msg.FromMe {
		return false
	}
	if !msg.FromMe && prev.ContactId != msg.ContactId {
		return false
	}
	return time.Duration(msg.Timestamp-prev.Timestamp)*time.Second <= runGap
}

// dayLabel names the day of t as seen at now.
func dayLabel(t, now time.Time) string {
	switch {
	case sameDay(t, now):
		return "Today"
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case t.Year() == now.Year():
		return t.Format("Monday, 2 Jan")
	default:
		return t.Format("2 Jan 2006")
	}
}

// daySeparator is a dim line with the day's name centred in width.
func daySeparator(t, now time.Time, width int) string {
	label := " " + dayLabel(t, now) + " "
	side := (width - tview.TaggedStringWidth(label)) / 2
	if side > 12 {
		side = 12
	}
	if side < 2 {
		side = 2
	}
	line := strings.Repeat("─", side) + label + strings.Repeat("─", side)
	pad := (width - tview.TaggedStringWidth(line)) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + "[::d]" + line + "[::-]"
}

// bubbleLayout is a bubble before it is turned into text: its lines
// (tagged, unpadded) with their widths, and the width of its content.
type bubbleLayout struct {
	lines   []string
	widths  []int
	content int
}

func (l *bubbleLayout) add(line string) {
	w := tview.TaggedStringWidth(line)
	l.lines = append(l.lines, line)
	l.widths = append(l.widths, w)
	if w > l.content {
		l.content = w
	}
}

// layoutBubble wraps msg into lines of at most maxContent cells: the name
// (if shown), "Forwarded" (if so), the text, and the time (followed by mark,
// the status ticks, if any) at the bottom right, on the last text line when
// it fits.
func layoutBubble(msg *messages.Message, maxContent int, showName bool, mark string) bubbleLayout {
	text := config.Config.Colors.Text
	var l bubbleLayout
	if showName {
		name := tview.Escape(displayName(msg.ContactShort))
		if lines := tview.WordWrap(name, maxContent); len(lines) > 0 {
			name = strings.TrimRight(lines[0], " ")
		}
		l.add("[" + config.Config.Colors.ChatContact + "::b]" + name + "[" + text + "::-]")
	}
	if msg.Forwarded {
		l.add("[" + config.Config.Colors.ForwardedText + "::i]Forwarded[" + text + "::-]")
	}
	body := tview.Escape(dropSkinTones(msg.Text))
	for _, para := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		wrapped := tview.WordWrap(para, maxContent)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for _, line := range wrapped {
			l.add(strings.TrimRight(line, " "))
		}
	}
	stamp := time.Unix(int64(msg.Timestamp), 0).Format("15:04")
	stampTag := "[" + config.Config.Colors.BubbleTime + "::]" + stamp
	if mark != "" {
		stampTag += " " + mark
	}
	stampTag += "[" + text + "::]"
	sw := tview.TaggedStringWidth(stampTag)
	last := len(l.lines) - 1
	if l.widths[last]+2+sw <= maxContent {
		// Time on the last line, at the right edge of the content.
		width := l.content
		if need := l.widths[last] + 2 + sw; need > width {
			width = need
		}
		l.lines[last] += strings.Repeat(" ", width-l.widths[last]-sw) + stampTag
		l.widths[last] = width
		l.content = width
	} else {
		width := l.content
		if sw > width {
			width = sw
		}
		l.add(strings.Repeat(" ", width-sw) + stampTag)
		l.widths[len(l.widths)-1] = width
		l.content = width
	}
	return l
}

// bubble returns the lines of one message bubble placed in a pane width
// cells wide: received on the left after a one-cell margin, your own ending
// one cell before the right edge.
func bubble(msg *messages.Message, width int, showName bool, mark string) string {
	maxBubble := int(float64(width) * bubbleShare)
	if maxBubble < 24 {
		maxBubble = 24
	}
	if maxBubble > width-2 {
		maxBubble = width - 2
	}
	maxContent := maxBubble - 2*bubblePadX
	if maxContent < 8 {
		return getTextMessageString(msg, width, mark) // too narrow for bubbles
	}
	l := layoutBubble(msg, maxContent, showName, mark)
	bg := config.Config.Colors.BubbleContact
	outer := l.content + 2*bubblePadX
	left := 1
	if msg.FromMe {
		bg = config.Config.Colors.BubbleMe
		left = width - 1 - outer
		if left < 0 {
			left = 0
		}
	}
	margin := strings.Repeat(" ", left)
	pad := strings.Repeat(" ", bubblePadX)
	// Half a row of bubble colour above and below the text: the lower half
	// block on the row above, the upper half block on the row below. They
	// also keep two bubbles in a row apart.
	edge := func(block string) string {
		return margin + "[" + bg + ":-:-]" + strings.Repeat(block, outer) + "[-:-:-]"
	}
	open := "[\"" + msg.Id + "\"][" + config.Config.Colors.Text + ":" + bg + ":-]" + pad
	closing := pad + "[-:-:-][\"\"]"
	out := []string{edge("▄")}
	for i, line := range l.lines {
		out = append(out, margin+open+line+strings.Repeat(" ", l.content-l.widths[i])+closing)
	}
	out = append(out, edge("▀"))
	return strings.Join(out, "\n")
}
