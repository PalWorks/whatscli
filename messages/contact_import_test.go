// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestImportContacts_LID(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	for _, c := range []Conversation{
		{JID: "alice@s.whatsapp.net", Name: "old", LastMsgTime: 10, Preview: "hi"},
		{JID: "222@lid", Name: "222", LastMsgTime: 20, Preview: "yo"},
	} {
		conv := c
		sm.convByJID[c.JID] = &conv
	}

	sm.importContacts(map[types.JID]types.ContactInfo{
		// Mapped LID: renames the phone-number chat.
		lidJID("111"): {Found: true, FullName: "Alice A"},
		// Unmapped LID with an existing @lid chat: renamed in place.
		lidJID("222"): {Found: true, PushName: "Bea"},
		// Unmapped LID, no chat: no empty row.
		lidJID("333"): {Found: true, FullName: "Cy"},
		// Phone contact without messages: no empty row.
		pnJID("eve"): {Found: true, FullName: "Eve"},
		// Group: new chat.
		grpJID("g"): {Found: true},
		// Not found and foreign servers are skipped.
		pnJID("gone"):                     {Found: false, FullName: "Gone"},
		{User: "x", Server: "newsletter"}: {Found: true, FullName: "News"},
	}, map[string]string{"g@g.us": "Group G"})

	if got := sm.convByJID["alice@s.whatsapp.net"].Name; got != "Alice A" {
		t.Errorf("mapped LID contact name = %q, want Alice A", got)
	}
	if sm.convByJID["111@lid"] != nil {
		t.Error("mapped LID created a separate @lid chat")
	}
	if got := sm.convByJID["222@lid"].Name; got != "Bea" {
		t.Errorf("unmapped LID chat name = %q, want Bea", got)
	}
	for _, jid := range []string{"333@lid", "eve@s.whatsapp.net", "gone@s.whatsapp.net", "x@newsletter"} {
		if sm.convByJID[jid] != nil {
			t.Errorf("empty chat %s was created", jid)
		}
	}
	if g := sm.convByJID["g@g.us"]; g == nil || g.Name != "Group G" {
		t.Errorf("group chat = %+v, want named Group G", g)
	}
	convs, err := sm.db.GetConversations()
	if err != nil || len(convs) != 3 {
		t.Errorf("stored %d conversations, want 3 (err %v)", len(convs), err)
	}
}

func TestImportContacts_MappedLIDWithoutChatIsNotCreated(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"444": "di"}
	sm.importContacts(map[types.JID]types.ContactInfo{lidJID("444"): {Found: true, FullName: "Di"}}, nil)
	if len(sm.convByJID) != 0 {
		t.Errorf("created %d chats, want 0", len(sm.convByJID))
	}
}

func TestImportContacts_BareLIDDoesNotRename(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	sm.convByJID["alice@s.whatsapp.net"] = &Conversation{JID: "alice@s.whatsapp.net", Name: "Alice", LastMsgTime: 10}
	sm.importContacts(map[types.JID]types.ContactInfo{lidJID("111"): {Found: true}}, nil)
	if got := sm.convByJID["alice@s.whatsapp.net"].Name; got != "Alice" {
		t.Errorf("name = %q, want Alice kept", got)
	}
}
