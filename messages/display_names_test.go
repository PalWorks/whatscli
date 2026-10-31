// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// fakeContacts maps a JID string (no device) to saved and push names.
type fakeContacts map[string][2]string

func (f fakeContacts) names(jid types.JID) (string, string) {
	n := f[jid.ToNonAD().String()]
	return n[0], n[1]
}

func TestSenderLabel(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "919000000001", "222": "919000000002"}
	sm.contactNamesOverride = fakeContacts{
		"919000000001@s.whatsapp.net": {"Alice Saved", "Ali"},
		"919000000002@s.whatsapp.net": {"", "Bobby"},
		"333@lid":                     {"", "Carol"},
		"919000000005@s.whatsapp.net": {"", ""},
		"444@lid":                     {"Dee Saved", ""},
	}.names
	tests := []struct {
		id, stored, want string
	}{
		{"111:4@lid", "111", "Alice Saved"},                   // LID mapped, saved name
		{"919000000001@s.whatsapp.net", "Ali", "Alice Saved"}, // phone number, saved name
		{"222@lid", "Bobby", "+919000000002"},                 // mapped, no saved name: phone number
		{"919000000005@s.whatsapp.net", "", "+919000000005"},
		{"333@lid", "333", "~Carol"},    // unmapped LID: WhatsApp name
		{"555@lid", "Eve", "~Eve"},      // unmapped, only the stored name
		{"555@lid", "555", "555"},       // nothing known
		{"444@lid", "444", "Dee Saved"}, // saved under the LID
		{"", "Stored", "Stored"},
	}
	for _, tt := range tests {
		if got := sm.senderLabel(tt.id, tt.stored); got != tt.want {
			t.Errorf("senderLabel(%q, %q) = %q, want %q", tt.id, tt.stored, got, tt.want)
		}
	}
}

func TestSenderLabel_NoStores(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	if got := sm.senderLabel("919000000005@s.whatsapp.net", "Push"); got != "+919000000005" {
		t.Errorf("phone number without contact store: %q", got)
	}
	if got := sm.senderLabel("555@lid", "555"); got != "555" {
		t.Errorf("LID without stores: %q", got)
	}
}

func TestGetMessages_LabelsSendersNotMe(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "919000000001"}
	sm.contactNamesOverride = fakeContacts{"919000000001@s.whatsapp.net": {"Alice Saved", ""}}.names
	chat := "919000000001@s.whatsapp.net"
	for _, m := range []Message{
		{Id: "A", ChatId: chat, Timestamp: 1, ContactId: "111@lid", ContactShort: "111"},
		{Id: "B", ChatId: chat, Timestamp: 2, FromMe: true, ContactId: "me@s.whatsapp.net", ContactShort: "Me"},
	} {
		if _, err := sm.db.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	msgs := sm.getMessages(chat)
	if len(msgs) != 2 || msgs[0].ContactShort != "Alice Saved" || msgs[1].ContactShort != "Me" {
		t.Errorf("labels = %+v", msgs)
	}
	if m, _, _ := sm.db.GetMessage("A"); m.ContactShort != "111" || m.ContactId != "111@lid" {
		t.Errorf("stored row changed: %+v", m)
	}
}
