// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/PalWorks/whatscli/messages"
)

func TestScrollPanelThumb(t *testing.T) {
	tests := []struct {
		name          string
		p             scrollPanel
		start, length int
		ok            bool
	}{
		{"fits, no bar", scrollPanel{height: 10, total: 10}, 0, 0, false},
		{"top", scrollPanel{height: 10, total: 100, offset: 0}, 0, 1, true},
		{"bottom", scrollPanel{height: 10, total: 100, offset: 90}, 9, 1, true},
		{"past the end clamps", scrollPanel{height: 10, total: 100, offset: 500}, 9, 1, true},
		{"half", scrollPanel{height: 10, total: 20, offset: 5}, 2, 5, true},
		{"zero height", scrollPanel{height: 0, total: 20}, 0, 0, false},
	}
	for _, tt := range tests {
		start, length, ok := tt.p.thumb()
		if start != tt.start || length != tt.length || ok != tt.ok {
			t.Errorf("%s: thumb() = %d,%d,%v; want %d,%d,%v", tt.name, start, length, ok, tt.start, tt.length, tt.ok)
		}
	}
}

func TestScrollPanelOffsetAt(t *testing.T) {
	p := scrollPanel{top: 5, height: 10, total: 100}
	if got := p.offsetAt(5); got != 0 {
		t.Errorf("top row: %d", got)
	}
	if got := p.offsetAt(14); got != 90 {
		t.Errorf("bottom row: %d, want 90", got)
	}
	if got := p.offsetAt(100); got != 90 {
		t.Errorf("below the bar clamps: %d", got)
	}
	if got := p.offsetAt(0); got != 0 {
		t.Errorf("above the bar clamps: %d", got)
	}
	mid := p.offsetAt(9)
	if mid < 30 || mid > 60 {
		t.Errorf("middle row: %d", mid)
	}
}

func TestScrollPanelHit(t *testing.T) {
	p := scrollPanel{barX: 29, top: 5, height: 10, total: 100}
	if !p.hit(29, 5) || !p.hit(29, 14) {
		t.Error("bar rows not hit")
	}
	if p.hit(28, 6) || p.hit(29, 15) || p.hit(29, 4) {
		t.Error("hit outside the bar")
	}
	if (scrollPanel{barX: 29, top: 5, height: 10, total: 5}).hit(29, 6) {
		t.Error("no bar when content fits")
	}
}

func TestChatTimeText(t *testing.T) {
	if got := chatTimeText(&messages.Conversation{}); got != "" {
		t.Errorf("no time, no unread: %q", got)
	}
	got := chatTimeText(&messages.Conversation{Unread: 3, LastMsgTime: time.Now().Add(-5 * time.Minute).Unix()})
	if !strings.HasPrefix(got, "([") || !strings.Contains(got, "]3[-])") || !strings.HasSuffix(got, " 5m") {
		t.Errorf("unread and time: %q", got)
	}
	if got := chatTimeText(&messages.Conversation{Unread: 2}); !strings.Contains(got, "]2[-])") || strings.HasSuffix(got, " ") {
		t.Errorf("unread only: %q", got)
	}
}
