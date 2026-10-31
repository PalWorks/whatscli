// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"

	"github.com/PalWorks/whatscli/config"
)

func TestHelpTextListsEveryCommandAndKey(t *testing.T) {
	km := config.Config.Keymap
	got := helpText(km, "/")
	for _, h := range commandHints {
		if !strings.Contains(got, "/"+h.name) {
			t.Errorf("help lacks /%s", h.name)
		}
	}
	for _, key := range []string{km.FindChat, km.FocusMessages, km.CommandQuit, km.MessageDownload, "F1"} {
		if !strings.Contains(got, key) {
			t.Errorf("help lacks key %q", key)
		}
	}
}

func TestWelcomeTextIsShort(t *testing.T) {
	got := welcomeText(config.Config.Keymap, "/")
	if n := strings.Count(got, "\n"); n > 11 {
		t.Errorf("welcome card has %d lines; keep it to the essentials", n)
	}
	for _, want := range []string{"Welcome to WhatsCLI", config.Config.Keymap.FindChat, "F1", "/help"} {
		if !strings.Contains(got, want) {
			t.Errorf("welcome lacks %q", want)
		}
	}
}

func TestHintAndStatusText(t *testing.T) {
	h := hintText(config.Config.Keymap, "/")
	for _, want := range []string{"F1", "Help", config.Config.Keymap.FindChat, "Find chat", config.Config.Keymap.CommandQuit} {
		if !strings.Contains(h, want) {
			t.Errorf("hint bar lacks %q: %q", want, h)
		}
	}
	if !strings.Contains(statusText(true), "online") || !strings.Contains(statusText(false), "offline") {
		t.Error("status text lacks the connection state")
	}
	if strings.Contains(statusText(true), "%") {
		t.Error("status text still shows the battery")
	}
}
