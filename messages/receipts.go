// SPDX-License-Identifier: AGPL-3.0-only

package messages

// receipts.go: the status of our own messages (sent, delivered, read). A
// message we send is "sent" once the server accepted it; delivery and read
// receipts from the recipient raise it, never lower it. History sync carries
// the status WhatsApp already knew. In a group, a receipt from any member
// counts, so "read" there means read by at least one member.
//
// A receipt can arrive before the sent message is stored (SendMessage
// returns after the server ack, and the recipient's phone may answer first).
// Such receipts are kept briefly in earlyReceipts and applied when the
// message is stored.

import (
	"sync"

	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Message status values, in order; a status only ever goes up.
const (
	StatusUnknown   = 0 // received messages, and own messages from before status was kept
	StatusSent      = 1
	StatusDelivered = 2
	StatusRead      = 3 // read, or a voice message played
)

// earlyReceiptLimit bounds the receipts kept for messages not stored yet.
const earlyReceiptLimit = 256

// earlyReceipts holds the best status seen for message IDs that were not in
// the database when their receipt arrived.
type earlyReceipts struct {
	mu     sync.Mutex
	status map[string]int
	order  []string
}

func (e *earlyReceipts) put(id string, status int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status == nil {
		e.status = make(map[string]int)
	}
	if old, ok := e.status[id]; ok {
		if status > old {
			e.status[id] = status
		}
		return
	}
	if len(e.order) >= earlyReceiptLimit {
		delete(e.status, e.order[0])
		e.order = e.order[1:]
	}
	e.status[id] = status
	e.order = append(e.order, id)
}

// take returns and forgets the status kept for id (StatusUnknown if none).
func (e *earlyReceipts) take(id string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.status[id]
	if !ok {
		return StatusUnknown
	}
	delete(e.status, id)
	for i, v := range e.order {
		if v == id {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
	return s
}

// receiptStatus maps a receipt from another user to a status; ok is false
// for receipt types that say nothing about delivery or reading.
func receiptStatus(t types.ReceiptType) (int, bool) {
	switch t {
	case types.ReceiptTypeDelivered:
		return StatusDelivered, true
	case types.ReceiptTypeRead, types.ReceiptTypePlayed:
		return StatusRead, true
	}
	return StatusUnknown, false
}

// historyStatus maps the status in a history sync message to ours.
func historyStatus(s waWeb.WebMessageInfo_Status) int {
	switch s {
	case waWeb.WebMessageInfo_SERVER_ACK:
		return StatusSent
	case waWeb.WebMessageInfo_DELIVERY_ACK:
		return StatusDelivered
	case waWeb.WebMessageInfo_READ, waWeb.WebMessageInfo_PLAYED:
		return StatusRead
	}
	return StatusUnknown
}

// withEarlyStatus raises msg's status by any receipt that arrived before it
// was stored. Only our own messages carry a status.
func (sm *SessionManager) withEarlyStatus(msg *Message) {
	if !msg.FromMe {
		return
	}
	if s := sm.earlyReceipts.take(msg.Id); s > msg.Status {
		msg.Status = s
	}
}

// handleReceipt applies a delivery or read receipt to our own messages and
// redraws the open chat if it changed. Receipts from our own other devices
// (read-self, sender) are ignored: they are about messages we received.
func (sm *SessionManager) handleReceipt(evt *events.Receipt) {
	if evt.IsFromMe || len(evt.MessageIDs) == 0 {
		return
	}
	status, ok := receiptStatus(evt.Type)
	if !ok {
		return
	}
	ids := make([]string, len(evt.MessageIDs))
	for i, id := range evt.MessageIDs {
		ids[i] = string(id)
	}
	chats, missing, err := sm.db.RaiseMessageStatus(ids, status)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	for _, id := range missing {
		sm.earlyReceipts.put(id, status)
	}
	for _, chat := range chats {
		if sm.refreshTarget(chat) != "" {
			sm.uiHandler.NewScreen(sm.getMessages(chat))
		}
	}
}
