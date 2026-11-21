// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_quote.go: how a reply shows the message it quotes. In a bubble: a
// coloured bar with the quoted sender's name and up to two dim lines of the
// quoted text; in the classic layout one dim line above the reply.

import (
	"strings"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/rivo/tview"
)

// quoteBubbleLines is how many lines of quoted text a bubble shows.
const quoteBubbleLines = 2

// quoteLabel is who a reply quotes and the colour of the quote: "You" for
// our own messages. When the name was not resolved, the open chat's
// messages are asked.
func quoteLabel(msg *messages.Message) (name, color string) {
	name = msg.QuoteName
	if name == "" && ctx != nil {
		for i := range ctx.CurRegions {
			if q := &ctx.CurRegions[i]; q.Id == msg.QuoteId {
				name = messages.QuoteYou
				if !q.FromMe {
					name = displayName(q.ContactShort)
				}
				break
			}
		}
	}
	if name == "" {
		name = "Message"
	}
	if name == messages.QuoteYou {
		return name, config.Config.Colors.StatusRead
	}
	return displayName(name), config.Config.Colors.ChatContact
}

// quoteText is the quoted text on one line: escaped, line breaks as spaces,
// without skin tones and the (edited) mark.
func quoteText(msg *messages.Message) string {
	t := strings.TrimSuffix(msg.QuoteText, " "+messages.EditedMark)
	t = strings.Join(strings.Fields(dropSkinTones(t)), " ")
	if t == "" {
		t = "…"
	}
	return tview.Escape(t)
}

// clip cuts a tagged string to width cells, ending it with … when cut.
func clip(s string, width int) string {
	if tview.TaggedStringWidth(s) <= width {
		return s
	}
	if width < 2 {
		return "…"
	}
	lines := tview.WordWrap(s, width-1)
	if len(lines) == 0 {
		return "…"
	}
	return strings.TrimRight(lines[0], " ") + "…"
}

// quoteBubble returns the quote lines for the top of a bubble whose text is
// at most width cells wide.
func quoteBubble(msg *messages.Message, width int) []string {
	text := config.Config.Colors.Text
	name, color := quoteLabel(msg)
	bar := "[" + color + "::]┃[" + text + "::] "
	inner := width - 2
	lines := []string{bar + "[" + color + "::b]" + clip(tview.Escape(name), inner) + "[" + text + "::-]"}
	wrapped := tview.WordWrap(quoteText(msg), inner)
	for i, l := range wrapped {
		l = strings.TrimRight(l, " ")
		if i == quoteBubbleLines-1 && len(wrapped) > quoteBubbleLines {
			l = clip(l+" "+strings.Join(wrapped[i+1:], " "), inner)
		}
		lines = append(lines, bar+"["+config.Config.Colors.BubbleTime+"::]"+l+"["+text+"::]")
		if i == quoteBubbleLines-1 {
			break
		}
	}
	return lines
}

// quoteClassic is the one dim quote line above a reply in the classic
// layout, at most width cells (0: not cut).
func quoteClassic(msg *messages.Message, width int) string {
	name, _ := quoteLabel(msg)
	line := "┃ " + tview.Escape(name) + ": " + quoteText(msg)
	if width > 0 {
		line = clip(line, width)
	}
	return "[::d]" + line + "[::-]"
}
