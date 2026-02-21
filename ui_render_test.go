package main

import (
	"testing"
	"time"
)

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
