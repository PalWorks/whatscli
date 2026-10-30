package messages

// revoke_incoming.go: authorisation of incoming REVOKE protocol messages
// (REVIEW-3 finding 1). WhatsApp does not validate revoke contents on the
// server, so every client must check them. A revoke is applied only when the
// stored target is in the same chat and the revoker is entitled to delete it:
//   - one of our own devices (IsFromMe): the account owner, trusted;
//   - the original sender of the target message (PN or LID form);
//   - in a group, a current group admin (checked asynchronously against the
//     group metadata, so the whatsmeow event handler is never blocked).
// Anything else is ignored and the stored row is left untouched.

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// groupInfoFetcher is client.GetGroupInfo; a seam for tests.
type groupInfoFetcher func(ctx context.Context, group types.JID) (*types.GroupInfo, error)

// groupAdminCheckTimeout bounds the group metadata request of an admin revoke.
const groupAdminCheckTimeout = 15 * time.Second

// revokeDecision is the outcome of checking an incoming revoke.
type revokeDecision int

const (
	revokeIgnore     revokeDecision = iota // not authorised, unknown target or wrong chat
	revokeApply                            // apply now
	revokeCheckAdmin                       // group revoke by someone else: apply only if the revoker is an admin
)

// handleIncomingRevoke checks and applies a REVOKE received from a contact or
// from one of our other devices.
func (sm *SessionManager) handleIncomingRevoke(src types.MessageSource, targetID string) {
	target, found, err := sm.db.GetMessage(targetID)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !found {
		return
	}
	switch sm.decideRevoke(src, target) {
	case revokeApply:
		sm.applyRevoke(target.ChatId, target.Id)
	case revokeCheckAdmin:
		go func() {
			if sm.isGroupAdmin(src.Chat, src.Sender, src.SenderAlt) {
				sm.applyRevoke(target.ChatId, target.Id)
			}
		}()
	}
}

// decideRevoke applies the chat and sender rules to a stored target message.
// It never holds sm.mu (sameUser and canonicalChatForSource may call getClient).
func (sm *SessionManager) decideRevoke(src types.MessageSource, target Message) revokeDecision {
	if !sm.revokeChatMatches(src, target.ChatId) {
		return revokeIgnore
	}
	if src.IsFromMe {
		// Our own account on another device; WhatsApp only lets it revoke
		// what we are allowed to revoke.
		return revokeApply
	}
	if !target.FromMe {
		storedSender, err := types.ParseJID(target.ContactId)
		if err == nil && (sm.sameUser(src.Sender, storedSender) || sm.sameUser(src.SenderAlt, storedSender)) {
			return revokeApply
		}
	}
	if src.Chat.Server == types.GroupServer {
		return revokeCheckAdmin
	}
	return revokeIgnore
}

// revokeChatMatches reports whether a stored chat ID is the chat the revoke
// arrived in, in either JID form (the stored row may predate LID mapping).
func (sm *SessionManager) revokeChatMatches(src types.MessageSource, storedChat string) bool {
	if storedChat == "" {
		return false
	}
	if storedChat == src.Chat.String() || storedChat == sm.canonicalChatForSource(src).String() {
		return true
	}
	stored, err := types.ParseJID(storedChat)
	if err != nil {
		return false
	}
	if stored.Server == types.GroupServer || src.Chat.Server == types.GroupServer {
		return false // groups have a single JID form
	}
	return sm.sameUser(stored, src.Chat)
}

// isGroupAdmin fetches the group's metadata and reports whether any of the
// given JIDs belongs to an admin. Errors count as "not an admin".
func (sm *SessionManager) isGroupAdmin(group types.JID, revoker ...types.JID) bool {
	fetch := sm.groupInfoOverride
	if fetch == nil {
		client := sm.getClient()
		if client == nil {
			return false
		}
		fetch = client.GetGroupInfo
	}
	ctx, cancel := context.WithTimeout(context.Background(), groupAdminCheckTimeout)
	defer cancel()
	info, err := fetch(ctx, group)
	if err != nil || info == nil {
		return false
	}
	return sm.groupAdminIn(info, revoker...)
}

// groupAdminIn reports whether any of ids is an admin or super admin of info.
func (sm *SessionManager) groupAdminIn(info *types.GroupInfo, ids ...types.JID) bool {
	for _, p := range info.Participants {
		if !p.IsAdmin && !p.IsSuperAdmin {
			continue
		}
		for _, id := range ids {
			if id.IsEmpty() {
				continue
			}
			for _, pj := range []types.JID{p.JID, p.PhoneNumber, p.LID} {
				if !pj.IsEmpty() && sm.sameUser(id, pj) {
					return true
				}
			}
		}
	}
	return false
}
