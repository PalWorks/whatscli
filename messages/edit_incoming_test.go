// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// sendEdit delivers an edit of target, wrapped the way WhatsApp sends it, and
// unwrapped as whatsmeow does before the event reaches the handler.
func sendEdit(eh *eventHandler, src types.MessageSource, target, text string) {
	evt := &events.Message{
		Info: types.MessageInfo{ID: "EDIT-" + target, MessageSource: src, Timestamp: time.Unix(200, 0)},
		RawMessage: &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
				Key:           &waCommon.MessageKey{ID: proto.String(target)},
				EditedMessage: &waE2E.Message{Conversation: proto.String(text)},
			},
		}}},
	}
	evt.UnwrapRaw()
	eh.handleMessage(evt)
}

func TestIncomingEdit(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}
	bob := types.MessageSource{Chat: pnJID("bob"), Sender: pnJID("bob")}
	me := types.MessageSource{Chat: pnJID("bob"), Sender: pnJID("me"), IsFromMe: true}
	storeText(eh, "B1", bob, "first")
	storeText(eh, "M1", me, "mine")

	sendEdit(eh, bob, "B1", "second")
	if got := textOf(t, sm, "B1"); got != "second "+EditedMark {
		t.Errorf("sender's edit: %q", got)
	}
	sendEdit(eh, me, "M1", "mine, fixed")
	if got := textOf(t, sm, "M1"); got != "mine, fixed "+EditedMark {
		t.Errorf("own edit from another device: %q", got)
	}
}

func TestIncomingEdit_Refused(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	eh := &eventHandler{sm: sm}
	bob := types.MessageSource{Chat: pnJID("bob"), Sender: pnJID("bob")}
	storeText(eh, "B1", bob, "first")
	storeText(eh, "M1", types.MessageSource{Chat: pnJID("bob"), Sender: pnJID("me"), IsFromMe: true}, "mine")

	sendEdit(eh, types.MessageSource{Chat: pnJID("bob"), Sender: pnJID("eve")}, "B1", "spoof")
	sendEdit(eh, types.MessageSource{Chat: pnJID("eve"), Sender: pnJID("bob")}, "B1", "wrong chat")
	sendEdit(eh, bob, "M1", "not yours")
	sendEdit(eh, bob, "NOPE", "unknown")
	if got := textOf(t, sm, "B1"); got != "first" {
		t.Errorf("B1 changed to %q", got)
	}
	if got := textOf(t, sm, "M1"); got != "mine" {
		t.Errorf("M1 changed to %q", got)
	}

	if _, err := sm.db.MarkMessageRevoked("bob@s.whatsapp.net", "B1"); err != nil {
		t.Fatal(err)
	}
	sendEdit(eh, bob, "B1", "back from the dead")
	if got := textOf(t, sm, "B1"); got != RevokedText {
		t.Errorf("revoked message edited: %q", got)
	}
}

func TestIncomingEdit_LIDSender(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.lidMapperOverride = fakeLIDs{"111": "alice"}
	eh := &eventHandler{sm: sm}
	storeText(eh, "A1", types.MessageSource{Chat: lidJID("111"), Sender: lidJID("111")}, "hi")
	sendEdit(eh, types.MessageSource{Chat: pnJID("alice"), Sender: pnJID("alice")}, "A1", "hello")
	if got := textOf(t, sm, "A1"); got != "hello "+EditedMark {
		t.Errorf("edit across LID and phone-number forms: %q", got)
	}
}
