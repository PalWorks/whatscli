package messages

import (
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestMediaTypeMapping(t *testing.T) {
	// Verifies the real mediaTypesByCommand map used by uploadAndSendMedia.
	tests := []struct {
		cmd      string
		expected whatsmeow.MediaType
	}{
		{"sendimage", whatsmeow.MediaImage},
		{"sendvideo", whatsmeow.MediaVideo},
		{"sendaudio", whatsmeow.MediaAudio},
		{"upload", whatsmeow.MediaDocument},
		{"sendsticker", whatsmeow.MediaImage},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got, ok := mediaTypesByCommand[tt.cmd]
			if !ok {
				t.Fatalf("command %q not found in mediaTypesByCommand map", tt.cmd)
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
	sm, mock := newTestSessionManager(t)

	// Set a receiver so the JID parsing doesn't fail before hitting the client check
	sm.mu.Lock()
	sm.currentReceiver = "123@s.whatsapp.net"
	sm.mu.Unlock()

	// Should not panic with nil client, and must report "not connected"
	// like every other command handler (audit finding: cmd_media previously
	// silently did nothing on a nil client).
	cmdMedia(sm, nil, "sendimage", []string{"/tmp/nonexistent.jpg"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 PrintError call, got %d", len(mock.Errors))
	}
	if mock.Errors[0].Error() != "not connected" {
		t.Errorf("expected 'not connected' error, got %q", mock.Errors[0].Error())
	}
}

// ----- Phase 5.5: sticker sending -----

func TestIsWebpFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"sticker.webp", true},
		{"sticker.WEBP", true},
		{"/a/b/c/sticker.WebP", true},
		{"sticker.png", false},
		{"sticker", false},
		{"webp", false},
		{"sticker.webp.png", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isWebpFile(tt.path); got != tt.want {
				t.Errorf("isWebpFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCmdMedia_Sticker_RejectsNonWebpExtension(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "123@s.whatsapp.net"
	sm.mu.Unlock()

	// The webp guard runs before the connectivity check (like cmdDownload's
	// local checks), so this is reported even with a nil client.
	cmdMedia(sm, nil, "sendsticker", []string{"/tmp/notasticker.png"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "stickers must be a .webp file" {
		t.Fatalf("expected 'stickers must be a .webp file', got %v", mock.Errors)
	}
}

func TestCmdMedia_RefusesStatusBroadcast(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = STATUSSUFFIX
	sm.mu.Unlock()

	// The status guard fires before the connectivity check, so this is
	// reported even with a nil client.
	cmdMedia(sm, nil, "sendimage", []string{"/tmp/photo.jpg"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "cannot send media to Status (view only)" {
		t.Fatalf("expected the status-is-read-only error, got %v", mock.Errors)
	}
}

func TestCmdMedia_Sticker_WebpPassesGuard_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "123@s.whatsapp.net"
	sm.mu.Unlock()

	// A correctly-named .webp file must pass the extension guard and reach
	// the normal nil-client "not connected" error, not the webp-specific one.
	cmdMedia(sm, nil, "sendsticker", []string{"/tmp/sticker.webp"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}
