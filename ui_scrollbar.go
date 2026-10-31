// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_scrollbar.go: scroll bars for the message pane and the chat lists. A
// thumb is drawn on each panel's right border when its content is longer
// than the panel. With the mouse enabled, clicking the bar jumps there and
// dragging the thumb scrolls.

import (
	"github.com/PalWorks/whatscli/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// scrollPanel is one scrollable area and the border column its bar uses.
type scrollPanel struct {
	barX, top, height int       // bar column and the rows it spans
	total, offset     int       // content lines or rows, and the first shown
	scrollTo          func(int) // sets the first shown line or row
}

// thumb returns the thumb's first row (relative to top) and its length, or
// ok=false when everything fits and no bar is needed.
func (p scrollPanel) thumb() (start, length int, ok bool) {
	if p.height <= 0 || p.total <= p.height {
		return 0, 0, false
	}
	length = p.height * p.height / p.total
	if length < 1 {
		length = 1
	}
	maxOffset := p.total - p.height
	off := p.offset
	if off > maxOffset {
		off = maxOffset
	}
	if off < 0 {
		off = 0
	}
	start = off * (p.height - length) / maxOffset
	return start, length, true
}

// offsetAt is the content offset that puts the thumb's middle at row y.
func (p scrollPanel) offsetAt(y int) int {
	_, length, ok := p.thumb()
	if !ok {
		return 0
	}
	track := p.height - length
	pos := y - p.top - length/2
	if pos < 0 {
		pos = 0
	}
	if pos > track {
		pos = track
	}
	if track == 0 {
		return 0
	}
	return pos * (p.total - p.height) / track
}

// hit reports whether (x, y) is on this panel's bar.
func (p scrollPanel) hit(x, y int) bool {
	_, _, ok := p.thumb()
	return ok && x == p.barX && y >= p.top && y < p.top+p.height
}

// tablePanel describes a bordered table; its bar sits on the right border.
func tablePanel(t *tview.Table) scrollPanel {
	x, y, w, h := t.GetInnerRect()
	off, _ := t.GetOffset()
	return scrollPanel{barX: x + w, top: y, height: h, total: t.GetRowCount(), offset: off,
		scrollTo: func(row int) { t.SetOffset(row, 0) }}
}

// textPanel describes the message pane; its bar sits on the grid border to
// its right.
func textPanel(v *tview.TextView) scrollPanel {
	x, y, w, h := v.GetRect()
	off, _ := v.GetScrollOffset()
	return scrollPanel{barX: x + w, top: y, height: h, total: v.GetWrappedLineCount(), offset: off,
		scrollTo: func(line int) { v.ScrollTo(line, 0) }}
}

// scrollPanels lists the panels that get a bar. UI goroutine only.
func scrollPanels() []scrollPanel {
	return []scrollPanel{textPanel(ctx.TextView), tablePanel(ctx.StatusTable), tablePanel(ctx.ChatTable), tablePanel(ctx.GroupTable)}
}

// drawScrollBars draws every thumb after the rest of the screen.
func drawScrollBars(screen tcell.Screen) {
	if overlayOpen() {
		return // the panels are covered
	}
	style := tcell.StyleDefault.Foreground(tcell.ColorNames[config.Config.Colors.Borders]).Bold(true)
	for _, p := range scrollPanels() {
		start, length, ok := p.thumb()
		if !ok {
			continue
		}
		for i := 0; i < length; i++ {
			screen.SetContent(p.barX, p.top+start+i, '┃', nil, style)
		}
	}
}

// dragging is the panel whose thumb is being dragged, nil otherwise;
// swallowClick drops the click event tview sends after the drag's release.
var (
	dragging     *scrollPanel
	swallowClick bool
)

// scrollBarMouse handles clicks and drags on the bars; other events pass on.
func scrollBarMouse(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if overlayOpen() {
		return event, action
	}
	x, y := event.Position()
	switch action {
	case tview.MouseLeftDown:
		swallowClick = false // a click after a drag may never come
		for _, p := range scrollPanels() {
			if p.hit(x, y) {
				p := p
				dragging = &p
				p.scrollTo(p.offsetAt(y))
				return nil, action
			}
		}
	case tview.MouseMove:
		if dragging != nil && event.Buttons()&tcell.Button1 != 0 {
			dragging.scrollTo(dragging.offsetAt(y))
			return nil, action
		}
	case tview.MouseLeftUp:
		if dragging != nil {
			dragging = nil
			swallowClick = true
			return nil, action
		}
	case tview.MouseLeftClick:
		if swallowClick {
			swallowClick = false
			return nil, action
		}
	}
	return event, action
}
