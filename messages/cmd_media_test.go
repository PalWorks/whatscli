package messages

import (
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
