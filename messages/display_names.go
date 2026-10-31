// SPDX-License-Identifier: AGPL-3.0-only

package messages

// display_names.go: who a message is shown as. Stored sender names are
// whatever was known when the message arrived, which is often a bare LID
// number. At display time the sender is resolved again: the name saved in
// the phone's contacts, else the phone number (+<digits>), else, for a LID
// whose phone number WhatsApp has not shared, the sender's own WhatsApp name
// (~Name), else the LID number. ContactId itself is never changed (row 21).

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// contactNames returns the saved (full) name and the WhatsApp push name of a
// contact, empty when unknown.
type contactNames func(jid types.JID) (saved, push string)

// contactLookupTimeout bounds one contact store lookup (a local SQLite read).
const contactLookupTimeout = 2 * time.Second

// contactNameSource returns the test override or the connected client's
// contact store, or nil when neither is available.
func (sm *SessionManager) contactNameSource() contactNames {
	if sm.contactNamesOverride != nil {
		return sm.contactNamesOverride
	}
	client := sm.getClient()
	if client == nil || client.Store == nil || client.Store.Contacts == nil {
		return nil
	}
	return func(jid types.JID) (string, string) {
		ctx, cancel := context.WithTimeout(context.Background(), contactLookupTimeout)
		defer cancel()
		c, err := client.Store.Contacts.GetContact(ctx, jid)
		if err != nil || !c.Found {
			return "", ""
		}
		return c.FullName, c.PushName
	}
}

// senderLabel is the name shown for a message sender given as a JID string.
// stored is the name kept with the message, used when nothing better is
// known. Never call it under sm.mu (it reads the LID and contact stores).
func (sm *SessionManager) senderLabel(contactID, stored string) string {
	jid, err := types.ParseJID(contactID)
	if err != nil || jid.User == "" {
		return stored
	}
	user := sm.canonicalUserJID(jid)
	names := sm.contactNameSource()
	var saved, push string
	if names != nil {
		saved, push = names(user)
		if saved == "" && user.Server != jid.ToNonAD().Server {
			// The contact may be stored under the LID instead.
			s, p := names(jid.ToNonAD())
			saved = s
			if push == "" {
				push = p
			}
		}
	}
	switch {
	case saved != "":
		return saved
	case user.Server == types.DefaultUserServer:
		return "+" + user.User
	case push != "":
		return "~" + push
	case stored != "" && !isBareNumberName(stored):
		return "~" + stored
	default:
		return user.User
	}
}

// labelSenders replaces each received message's ContactShort with its
// display name, resolving each sender once.
func (sm *SessionManager) labelSenders(msgs []Message) {
	cache := make(map[string]string)
	for i := range msgs {
		m := &msgs[i]
		if m.FromMe || m.ContactId == "" {
			continue
		}
		label, ok := cache[m.ContactId]
		if !ok {
			label = sm.senderLabel(m.ContactId, m.ContactShort)
			cache[m.ContactId] = label
		}
		m.ContactShort = label
	}
}
