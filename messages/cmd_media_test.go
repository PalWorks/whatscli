// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestMediaTypeMapping(t *testing.T) {
	// This mirrors the mapping in cmdMedia — verifies the correct MediaType per command
	mediaTypes := map[string]whatsmeow.MediaType{
		"sendimage": whatsmeow.MediaImage,
		"sendvideo": whatsmeow.MediaVideo,
		"sendaudio": whatsmeow.MediaAudio,
		"upload":    whatsmeow.MediaDocument,
	}

	tests := []struct {
		cmd      string
		expected whatsmeow.MediaType
	}{
		{"sendimage", whatsmeow.MediaImage},
		{"sendvideo", whatsmeow.MediaVideo},
		{"sendaudio", whatsmeow.MediaAudio},
		{"upload", whatsmeow.MediaDocument},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got, ok := mediaTypes[tt.cmd]
			if !ok {
				t.Fatalf("command %q not found in mediaTypes map", tt.cmd)
			}
			if got != tt.expected {
				t.Errorf("command %q: expected MediaType %v, got %v", tt.cmd, tt.expected, got)
			}
		})
	}
}

func TestCmdMedia_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// Call with empty params — should print usage
	cmdMedia(sm, nil, "sendimage", []string{})

	// checkParam returns false for empty params, so printCommandUsage is called
	found := false
	for _, text := range mock.Texts {
		if len(text) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected usage text for empty params, got nothing")
	}
}

func TestCmdMedia_NilClient(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	// Set a receiver so the JID parsing doesn't fail before hitting the client check
	sm.mu.Lock()
	sm.currentReceiver = "123@s.whatsapp.net"
	sm.mu.Unlock()

	// Should not panic with nil client — it silently returns because client == nil
	cmdMedia(sm, nil, "sendimage", []string{"/tmp/nonexistent.jpg"})
}

func TestParseSendFile(t *testing.T) {
	tests := []struct {
		name   string
		params []string
		chat   string
		cmd    string
		path   string
		ok     bool
	}{
		{"image", []string{"123@s.whatsapp.net", "sendimage", "/a/x.png"}, "123@s.whatsapp.net", "sendimage", "/a/x.png", true},
		{"document with spaces", []string{"g@g.us", "upload", "/a/my", "report.pdf"}, "g@g.us", "upload", "/a/my report.pdf", true},
		{"missing path", []string{"123@s.whatsapp.net", "sendimage"}, "", "", "", false},
		{"not a media command", []string{"123@s.whatsapp.net", "send", "/a/x"}, "", "", "", false},
		{"empty chat", []string{"", "upload", "/a/x"}, "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat, cmd, path, ok := parseSendFile(tt.params)
			if chat != tt.chat || cmd != tt.cmd || path != tt.path || ok != tt.ok {
				t.Fatalf("parseSendFile(%q) = %q, %q, %q, %v; want %q, %q, %q, %v",
					tt.params, chat, cmd, path, ok, tt.chat, tt.cmd, tt.path, tt.ok)
			}
		})
	}
}

func TestCmdSendFileRegisteredAndValidates(t *testing.T) {
	if _, ok := commandRegistry["sendfile"]; !ok {
		t.Fatal("sendfile is not registered")
	}
	sm, ui := newTestSessionManager(t)
	cmdSendFile(sm, nil, "sendfile", []string{"123@s.whatsapp.net", "send", "/a/x"})
	if len(ui.Texts) == 0 || !strings.Contains(ui.Texts[len(ui.Texts)-1], "Usage:") {
		t.Fatalf("expected a usage message, got %q", ui.Texts)
	}
}

// The file must go to the chat named in the command, not the open chat:
// with no client nothing is sent, but an invalid target JID is reported.
func TestCmdSendFileUsesGivenChat(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	sm.setCurrentReceiver("111@s.whatsapp.net")
	cmdSendFile(sm, nil, "sendfile", []string{"a.b.c@s.whatsapp.net", "upload", "/a/x"})
	if len(ui.Errors) == 0 || !strings.Contains(ui.Errors[0].Error(), "invalid JID") {
		t.Fatalf("expected an invalid JID error for the given chat, got %v", ui.Errors)
	}
}
