// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// fakeLIDs is an in-memory lidMapper: lid user -> pn user.
type fakeLIDs map[string]string

func (f fakeLIDs) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	if pn, ok := f[lid.User]; ok {
		return types.JID{User: pn, Device: lid.Device, Server: types.DefaultUserServer}, nil
	}
	return types.JID{}, nil
}

func (f fakeLIDs) GetLIDForPN(_ context.Context, pn types.JID) (types.JID, error) {
	for l, p := range f {
		if p == pn.User {
			return types.JID{User: l, Device: pn.Device, Server: types.HiddenUserServer}, nil
		}
	}
	return types.JID{}, nil
}

func strPtr(s string) *string { return &s }

func pnJID(u string) types.JID  { return types.NewJID(u, types.DefaultUserServer) }
func lidJID(u string) types.JID { return types.NewJID(u, types.HiddenUserServer) }
func grpJID(u string) types.JID { return types.NewJID(u, types.GroupServer) }

// storeText stores a text message through the live message path.
func storeText(eh *eventHandler, id string, src types.MessageSource, text string) {
	eh.handleMessage(&events.Message{
		Info:    types.MessageInfo{ID: id, MessageSource: src, Timestamp: time.Unix(100, 0)},
		Message: &waE2E.Message{Conversation: strPtr(text)},
	})
}

// sendRevoke delivers a REVOKE for target through the live message path.
func sendRevoke(eh *eventHandler, src types.MessageSource, target string) {
	eh.handleMessage(&events.Message{
		Info: types.MessageInfo{ID: "REV-" + target, MessageSource: src, Timestamp: time.Unix(200, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  waCommonKey(target),
		}},
	})
}

func textOf(t *testing.T, sm *SessionManager, id string) string {
	t.Helper()
	msg, found, err := sm.db.GetMessage(id)
	if err != nil || !found {
		t.Fatalf("message %s: found=%v err=%v", id, found, err)
	}
	return msg.Text
}

// Regression for the REVIEW-3 proof (scratchpad revproof/zz_spoof_test.go):
// a revoke from an unrelated chat and sender must leave the row untouched.
func TestIncomingRevoke_SpoofFromOtherChatIgnored(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}
	alice := pnJID("alice")
	storeText(eh, "ALICEMSG", types.MessageSource{Chat: alice, Sender: alice}, "hello from alice")

	mallory := pnJID("mallory")
	sendRevoke(eh, types.MessageSource{Chat: mallory, Sender: mallory}, "ALICEMSG")

	if got := textOf(t, sm, "ALICEMSG"); got != "hello from alice" {
		t.Fatalf("spoofed revoke changed the row: %q", got)
	}
}

func TestIncomingRevoke_Rules(t *testing.T) {
	alice, bob, mallory := pnJID("alice"), pnJID("bob"), pnJID("mallory")
	group := grpJID("120363")
	tests := []struct {
		name    string
		stored  types.MessageSource // source of the target message
		revoker types.MessageSource
		mapping fakeLIDs
		want    bool // true: revoked
	}{
		{"DM: sender revokes own message", types.MessageSource{Chat: alice, Sender: alice},
			types.MessageSource{Chat: alice, Sender: alice}, nil, true},
		{"DM: sender device JID revokes", types.MessageSource{Chat: alice, Sender: alice},
			types.MessageSource{Chat: alice, Sender: types.JID{User: "alice", Device: 3, Server: types.DefaultUserServer}}, nil, true},
		{"DM: our other device revokes our message", types.MessageSource{Chat: alice, Sender: bob, IsFromMe: true},
			types.MessageSource{Chat: alice, Sender: bob, IsFromMe: true}, nil, true},
		{"DM: contact revokes our message", types.MessageSource{Chat: alice, Sender: bob, IsFromMe: true},
			types.MessageSource{Chat: alice, Sender: alice}, nil, false},
		{"cross chat: same ID revoked from another chat by the original sender", types.MessageSource{Chat: alice, Sender: alice},
			types.MessageSource{Chat: bob, Sender: alice}, nil, false},
		{"cross chat: our device revokes ID stored in another chat", types.MessageSource{Chat: alice, Sender: alice},
			types.MessageSource{Chat: bob, Sender: bob, IsFromMe: true}, nil, false},
		{"group: sender revokes own message", types.MessageSource{Chat: group, Sender: alice, IsGroup: true},
			types.MessageSource{Chat: group, Sender: alice, IsGroup: true}, nil, true},
		{"group: stranger (non-admin) revokes another member's message", types.MessageSource{Chat: group, Sender: alice, IsGroup: true},
			types.MessageSource{Chat: group, Sender: mallory, IsGroup: true}, nil, false},
		{"group: sender revokes in LID form, mapping known", types.MessageSource{Chat: group, Sender: alice, IsGroup: true},
			types.MessageSource{Chat: group, Sender: lidJID("111"), IsGroup: true}, fakeLIDs{"111": "alice"}, true},
		{"group: LID revoker with SenderAlt PN, no mapping", types.MessageSource{Chat: group, Sender: alice, IsGroup: true},
			types.MessageSource{Chat: group, Sender: lidJID("111"), SenderAlt: alice, IsGroup: true}, nil, true},
		{"group: unmapped LID revoker", types.MessageSource{Chat: group, Sender: alice, IsGroup: true},
			types.MessageSource{Chat: group, Sender: lidJID("999"), IsGroup: true}, fakeLIDs{"111": "alice"}, false},
		{"DM: stored under LID chat, revoke arrives with PN chat", types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")},
			types.MessageSource{Chat: alice, Sender: alice}, nil, false}, // no mapping: different chats
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, _ := newTestSessionManager(t)
			if tt.mapping != nil {
				sm.lidMapperOverride = tt.mapping
			}
			sm.groupInfoOverride = func(context.Context, types.JID) (*types.GroupInfo, error) {
				return nil, errors.New("not an admin check test")
			}
			eh := &eventHandler{sm: sm}
			storeText(eh, "T1", tt.stored, "original")
			sendRevoke(eh, tt.revoker, "T1")
			// Admin checks run in a goroutine; give a rejected one time to (not) apply.
			time.Sleep(20 * time.Millisecond)
			got := textOf(t, sm, "T1") == RevokedText
			if got != tt.want {
				t.Errorf("revoked=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncomingRevoke_LIDChatMappedToPN(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	eh := &eventHandler{sm: sm}
	// Message arrives in the @lid chat and is stored under the PN chat.
	storeText(eh, "L1", types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")}, "hi")
	if msg, _, _ := sm.db.GetMessage("L1"); msg.ChatId != "alice@s.whatsapp.net" {
		t.Fatalf("chat not normalised: %q", msg.ChatId)
	}
	// Revoke arrives in the @lid form of the same chat.
	sendRevoke(eh, types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")}, "L1")
	if got := textOf(t, sm, "L1"); got != RevokedText {
		t.Errorf("revoke in LID form of the same chat not applied: %q", got)
	}
}

func TestIncomingRevoke_GroupAdmin(t *testing.T) {
	alice, admin, mallory := pnJID("alice"), pnJID("admin"), pnJID("mallory")
	group := grpJID("120363")
	for _, tt := range []struct {
		name    string
		revoker types.JID
		want    bool
	}{
		{"admin revokes member message", admin, true},
		{"admin revokes in LID form", lidJID("222"), true},
		{"non-admin member revokes", mallory, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sm, _ := newTestSessionManager(t)
			sm.lidMapperOverride = fakeLIDs{"222": "admin"}
			var calls atomic.Int32
			sm.groupInfoOverride = func(_ context.Context, g types.JID) (*types.GroupInfo, error) {
				calls.Add(1)
				if g != group {
					return nil, errors.New("unexpected group")
				}
				return &types.GroupInfo{JID: group, Participants: []types.GroupParticipant{
					{JID: alice},
					{JID: mallory},
					{JID: admin, IsAdmin: true},
				}}, nil
			}
			eh := &eventHandler{sm: sm}
			storeText(eh, "G1", types.MessageSource{Chat: group, Sender: alice, IsGroup: true}, "member text")
			sendRevoke(eh, types.MessageSource{Chat: group, Sender: tt.revoker, IsGroup: true}, "G1")

			deadline := time.Now().Add(2 * time.Second)
			for calls.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			time.Sleep(20 * time.Millisecond) // let applyRevoke finish
			if got := textOf(t, sm, "G1") == RevokedText; got != tt.want {
				t.Errorf("revoked=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarkMessageRevoked_ScopedToChat(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "X", ChatId: "a@s.whatsapp.net", Text: "keep"}); err != nil {
		t.Fatal(err)
	}
	updated, err := sm.db.MarkMessageRevoked("b@s.whatsapp.net", "X")
	if err != nil || updated {
		t.Fatalf("wrong chat: updated=%v err=%v", updated, err)
	}
	if got := textOf(t, sm, "X"); got != "keep" {
		t.Fatalf("row changed: %q", got)
	}
	updated, err = sm.db.MarkMessageRevoked("a@s.whatsapp.net", "X")
	if err != nil || !updated {
		t.Fatalf("right chat: updated=%v err=%v", updated, err)
	}
}
