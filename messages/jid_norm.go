// SPDX-License-Identifier: AGPL-3.0-only

package messages

// jid_norm.go: one canonical chat JID per person. WhatsApp addresses the same
// user either by phone number (@s.whatsapp.net) or by LID (@lid). Chats are
// stored under the phone-number form whenever the mapping is known, so live
// messages, history sync and /backlog agree and one person does not show up
// as two chats. Without a known mapping the @lid JID is kept as received.

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// lidMapper is the subset of whatsmeow's store.LIDStore used here, so tests
// can supply a fake mapping. Unknown mappings return an empty JID and nil error.
type lidMapper interface {
	GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error)
	GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error)
}

// lidLookupTimeout bounds a single mapping lookup (normally a cache hit or one
// local SQLite query).
const lidLookupTimeout = 2 * time.Second

// lidMap returns the LID mapping source: the test override if set, otherwise
// the connected client's store, or nil when neither is available.
func (sm *SessionManager) lidMap() lidMapper {
	if sm.lidMapperOverride != nil {
		return sm.lidMapperOverride
	}
	client := sm.getClient()
	if client == nil || client.Store == nil || client.Store.LIDs == nil {
		return nil
	}
	return client.Store.LIDs
}

// canonicalUserJID maps an @lid user JID to its phone-number JID when known.
// Device parts are dropped. Other JIDs are returned without their device part.
func (sm *SessionManager) canonicalUserJID(jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if jid.Server != types.HiddenUserServer {
		return jid
	}
	m := sm.lidMap()
	if m == nil {
		return jid
	}
	ctx, cancel := context.WithTimeout(context.Background(), lidLookupTimeout)
	defer cancel()
	pn, err := m.GetPNForLID(ctx, jid)
	if err != nil || pn.IsEmpty() || pn.Server != types.DefaultUserServer {
		return jid
	}
	return pn.ToNonAD()
}

// canonicalChatJID returns the JID a chat is stored under. Only @lid chats are
// rewritten (to the phone-number form when the mapping is known); groups,
// phone-number chats and everything else are returned unchanged.
func (sm *SessionManager) canonicalChatJID(chat types.JID) types.JID {
	if chat.Server != types.HiddenUserServer {
		return chat
	}
	return sm.canonicalUserJID(chat)
}

// canonicalChatForSource is canonicalChatJID for a live message. For a 1:1
// @lid chat the alternate address carried on the message (SenderAlt for
// incoming, RecipientAlt for our own) is used first, then the LID store.
func (sm *SessionManager) canonicalChatForSource(src types.MessageSource) types.JID {
	chat := src.Chat
	if chat.Server != types.HiddenUserServer {
		return chat
	}
	if !src.IsFromMe && src.Sender.User == chat.User && src.SenderAlt.Server == types.DefaultUserServer {
		return src.SenderAlt.ToNonAD()
	}
	if src.IsFromMe && src.RecipientAlt.Server == types.DefaultUserServer {
		return src.RecipientAlt.ToNonAD()
	}
	return sm.canonicalUserJID(chat)
}

// sameUser reports whether a and b address the same WhatsApp account,
// ignoring device parts and treating a LID and its mapped phone number as equal.
func (sm *SessionManager) sameUser(a, b types.JID) bool {
	a, b = a.ToNonAD(), b.ToNonAD()
	if a.IsEmpty() || b.IsEmpty() || a.User == "" || b.User == "" {
		return false
	}
	if a == b {
		return true
	}
	if a.Server == b.Server {
		return false // same namespace, different user
	}
	return sm.canonicalUserJID(a) == sm.canonicalUserJID(b)
}
