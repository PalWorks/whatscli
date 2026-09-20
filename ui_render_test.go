package main

import (
	"strings"
	"testing"
	"time"

	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
)

func TestIsWithin(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		path string
		want bool
	}{
		{"exact match", "/home/x/Downloads", "/home/x/Downloads", true},
		{"direct child", "/home/x/Downloads", "/home/x/Downloads/photo.jpg", true},
		{"nested child", "/home/x/Downloads", "/home/x/Downloads/sub/photo.jpg", true},
		{"sibling with dir as string prefix", "/home/x/Downloads", "/home/x/DownloadsEvil/photo.jpg", false},
		{"sibling with dir as string prefix, no subpath", "/home/x/Downloads", "/home/x/DownloadsEvil", false},
		{"parent escape via dotdot", "/home/x/Downloads", "/home/x", false},
		{"unrelated path", "/home/x/Downloads", "/etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWithin(tt.dir, tt.path)
			if got != tt.want {
				t.Errorf("isWithin(%q, %q) = %v, want %v", tt.dir, tt.path, got, tt.want)
			}
		})
	}
}

func TestFormatRelativeTime_Zero(t *testing.T) {
	got := formatRelativeTime(0)
	if got != "" {
		t.Errorf("expected empty string for timestamp 0, got %q", got)
	}
}

func TestFormatRelativeTime_Negative(t *testing.T) {
	got := formatRelativeTime(-100)
	if got != "" {
		t.Errorf("expected empty string for negative timestamp, got %q", got)
	}
}

func TestFormatRelativeTime_Future(t *testing.T) {
	future := time.Now().Add(10 * time.Minute).Unix()
	got := formatRelativeTime(future)
	if got != "just now" {
		t.Errorf("expected 'just now' for future timestamp, got %q", got)
	}
}

func TestFormatRelativeTime_JustNow(t *testing.T) {
	ts := time.Now().Add(-30 * time.Second).Unix()
	got := formatRelativeTime(ts)
	if got != "just now" {
		t.Errorf("expected 'just now', got %q", got)
	}
}

func TestFormatRelativeTime_Minutes(t *testing.T) {
	ts := time.Now().Add(-15 * time.Minute).Unix()
	got := formatRelativeTime(ts)
	if got != "15m" {
		t.Errorf("expected '15m', got %q", got)
	}
}

func TestFormatRelativeTime_Hours(t *testing.T) {
	// Use a timestamp a few hours ago that's still on the same calendar day.
	// To ensure same-day, we pick 2 hours ago only if it won't cross midnight.
	now := time.Now()
	if now.Hour() < 3 {
		t.Skip("skipping hours test near midnight to avoid calendar-day boundary")
	}
	ts := now.Add(-2 * time.Hour).Unix()
	got := formatRelativeTime(ts)
	if got != "2h" {
		t.Errorf("expected '2h', got %q", got)
	}
}

func TestFormatRelativeTime_Yesterday(t *testing.T) {
	ts := time.Now().Add(-36 * time.Hour).Unix()
	got := formatRelativeTime(ts)
	if got != "Yesterday" {
		t.Errorf("expected 'Yesterday', got %q", got)
	}
}

func TestFormatRelativeTime_Weekday(t *testing.T) {
	ts := time.Now().Add(-4 * 24 * time.Hour).Unix()
	got := formatRelativeTime(ts)
	expected := time.Unix(ts, 0).Weekday().String()[:3]
	if got != expected {
		t.Errorf("expected weekday %q, got %q", expected, got)
	}
}

func TestFormatRelativeTime_OlderSameYear(t *testing.T) {
	now := time.Now()
	// Pick a date at least 10 days ago in the same year.
	// Skip if we're in early January and 10 days ago crosses into last year.
	target := now.AddDate(0, 0, -10)
	if target.Year() != now.Year() {
		t.Skip("skipping same-year test near year boundary")
	}
	ts := target.Unix()
	got := formatRelativeTime(ts)
	expected := target.Format("Jan 02")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestFormatRelativeTime_DifferentYear(t *testing.T) {
	target := time.Now().AddDate(-1, 0, 0)
	ts := target.Unix()
	got := formatRelativeTime(ts)
	expected := target.Format("Jan 02, 06")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// ----- Phase 5.1: reply threading render -----

func TestGetTextMessageString_NoQuotePreviewLine(t *testing.T) {
	msg := &messages.Message{Id: "m1", Text: "hello", ContactShort: "Bob"}
	got := getTextMessageString(msg)
	if strings.Contains(got, "> ") {
		t.Errorf("expected no quote line for a non-reply message, got %q", got)
	}
}

func TestGetTextMessageString_IncludesQuotePreviewLine(t *testing.T) {
	msg := &messages.Message{Id: "m2", Text: "sure!", ContactShort: "Bob", QuotedPreview: "lunch at noon?"}
	got := getTextMessageString(msg)
	if !strings.Contains(got, "> lunch at noon?") {
		t.Errorf("expected quote preview line in output, got %q", got)
	}
	// The quote line must appear before the message's own text line.
	quoteIdx := strings.Index(got, "> lunch at noon?")
	textIdx := strings.Index(got, "sure!")
	if quoteIdx < 0 || textIdx < 0 || quoteIdx > textIdx {
		t.Errorf("expected quote line before message text, got %q", got)
	}
}

func TestGetTextMessageString_EscapesQuotePreview(t *testing.T) {
	msg := &messages.Message{Id: "m3", Text: "ok", ContactShort: "Bob", QuotedPreview: "[danger]"}
	got := getTextMessageString(msg)
	if strings.Contains(got, "[danger]") {
		t.Errorf("expected quote preview to be tview-escaped, got %q", got)
	}
}

// ----- Phase 5.2: reaction render suffix -----

func TestGetTextMessageString_NoReactionSuffixWhenNone(t *testing.T) {
	msg := &messages.Message{Id: "m4", Text: "hello", ContactShort: "Bob"}
	got := getTextMessageString(msg)
	if strings.Contains(got, "x1") || strings.Contains(got, "x2") {
		t.Errorf("expected no reaction suffix, got %q", got)
	}
}

func TestGetTextMessageString_IncludesReactionSuffix(t *testing.T) {
	msg := &messages.Message{
		Id: "m5", Text: "funny!", ContactShort: "Bob",
		Reactions: []messages.ReactionSummary{{Emoji: "😂", Count: 3}, {Emoji: "👍", Count: 1}},
	}
	got := getTextMessageString(msg)
	if !strings.Contains(got, "😂 x3") || !strings.Contains(got, "👍 x1") {
		t.Errorf("expected reaction suffix with both emoji counts, got %q", got)
	}
}

func TestReactionSuffix_PreservesGivenOrder(t *testing.T) {
	got := reactionSuffix([]messages.ReactionSummary{{Emoji: "❤️", Count: 2}, {Emoji: "👍", Count: 1}})
	want := "❤️ x2 👍 x1"
	if !strings.Contains(got, want) {
		t.Errorf("reactionSuffix() = %q, want it to contain %q", got, want)
	}
}

func TestReceiptSuffix(t *testing.T) {
	cases := []struct {
		name    string
		fromMe  bool
		status  int
		want    string
		wantAny bool // true: just assert non-empty
	}{
		{name: "incoming message never gets ticks", fromMe: false, status: messages.ReceiptRead, want: ""},
		{name: "own message with no receipt", fromMe: true, status: messages.ReceiptNone, want: ""},
		{name: "own message delivered", fromMe: true, status: messages.ReceiptDelivered, wantAny: true},
		{name: "own message read", fromMe: true, status: messages.ReceiptRead, wantAny: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := receiptSuffix(c.fromMe, c.status)
			if c.wantAny {
				if got == "" {
					t.Errorf("receiptSuffix(%v, %d) = empty, want tick markup", c.fromMe, c.status)
				}
				if !strings.Contains(got, "✓✓") {
					t.Errorf("receiptSuffix(%v, %d) = %q, want it to contain double check marks", c.fromMe, c.status, got)
				}
				return
			}
			if got != c.want {
				t.Errorf("receiptSuffix(%v, %d) = %q, want %q", c.fromMe, c.status, got, c.want)
			}
		})
	}
}

func TestReceiptSuffix_ReadUsesPositiveColorDeliveredIsDim(t *testing.T) {
	delivered := receiptSuffix(true, messages.ReceiptDelivered)
	read := receiptSuffix(true, messages.ReceiptRead)
	if !strings.Contains(delivered, "[::d]") {
		t.Errorf("delivered suffix %q should use dim styling", delivered)
	}
	if !strings.Contains(read, config.Config.Colors.Positive) {
		t.Errorf("read suffix %q should use the Positive theme color", read)
	}
}

func TestGetTextMessageString_AppendsReceiptTicksForOwnMessage(t *testing.T) {
	msg := messages.Message{
		Id: "tick1", ChatId: "peer@s.whatsapp.net", FromMe: true,
		Timestamp: 1000, Text: "hello", ReceiptStatus: messages.ReceiptRead,
	}
	out := getTextMessageString(&msg)
	if !strings.Contains(out, "✓✓") {
		t.Errorf("rendered own read message should contain tick marks, got %q", out)
	}
}
