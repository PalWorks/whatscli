package main

import (
	"testing"

	"github.com/PalWorks/whatscli/messages"
)

func TestChatForID(t *testing.T) {
	saved := ctx
	t.Cleanup(func() { ctx = saved })
	ctx = &AppContext{AllChats: []*messages.Conversation{
		{JID: "123@s.whatsapp.net", Name: "Alice"},
		{JID: "456@g.us", Name: "Team"},
	}}
	tests := []struct {
		id, name string
		group    bool
	}{
		{"123@s.whatsapp.net", "Alice", false},
		{"456@g.us", "Team", true},
		{"789@s.whatsapp.net", "789@s.whatsapp.net", false},
	}
	for _, tt := range tests {
		got := chatForID(tt.id)
		if got.Id != tt.id || got.Name != tt.name || got.IsGroup != tt.group {
			t.Errorf("chatForID(%q) = %+v", tt.id, got)
		}
	}
}
