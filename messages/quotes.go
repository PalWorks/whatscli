// SPDX-License-Identifier: AGPL-3.0-only

package messages

// quotes.go: replies. A reply carries the ID, sender and a copy of the
// message it quotes (ContextInfo). The ID, sender JID and quoted text are
// stored with the reply (quote_id, quote_sender, quote_text), so the quote
// shows even when the original is not stored. Who is quoted is resolved at
// display time ("You" for our own messages), like sender names.

import (
	"errors"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// QuoteYou is the name shown for a quoted message of our own.
const QuoteYou = "You"

// contextInfoOf returns the ContextInfo of the message kinds that can be a
// reply, or nil.
func contextInfoOf(m *waE2E.Message) *waE2E.ContextInfo {
	if m == nil {
		return nil
	}
	for _, ci := range []*waE2E.ContextInfo{
		m.GetExtendedTextMessage().GetContextInfo(),
		m.GetImageMessage().GetContextInfo(),
		m.GetVideoMessage().GetContextInfo(),
		m.GetAudioMessage().GetContextInfo(),
		m.GetDocumentMessage().GetContextInfo(),
		m.GetStickerMessage().GetContextInfo(),
		m.GetLocationMessage().GetContextInfo(),
		m.GetContactMessage().GetContextInfo(),
	} {
		if ci != nil {
			return ci
		}
	}
	return nil
}

// attachQuote fills msg's quote fields when m is a reply.
func attachQuote(msg *Message, m *waE2E.Message) {
	ci := contextInfoOf(m)
	if ci.GetStanzaID() == "" {
		return
	}
	msg.QuoteId = ci.GetStanzaID()
	msg.QuoteSender = ci.GetParticipant()
	msg.QuoteText, _ = extractMessageContent(ci.GetQuotedMessage())
}

// ownJID is our own user JID, empty when not logged in.
func (sm *SessionManager) ownJID() types.JID {
	client := sm.getClient()
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return types.EmptyJID
	}
	return client.Store.ID.ToNonAD()
}

// labelQuote sets QuoteName (who is quoted). When the quoted message is
// stored, its name and text come from our copy, not from the copy the
// replying side sent (which it could have changed). Never call it under
// sm.mu.
func (sm *SessionManager) labelQuote(msg *Message) {
	if msg.QuoteId == "" {
		return
	}
	if q, found, err := sm.db.GetMessage(msg.QuoteId); err == nil && found {
		msg.QuoteText = q.Text
		if q.FromMe {
			msg.QuoteName = QuoteYou
		} else {
			msg.QuoteName = sm.senderLabel(q.ContactId, q.ContactShort)
		}
		return
	}
	sender, err := types.ParseJID(msg.QuoteSender)
	if err != nil || sender.User == "" {
		return
	}
	if own := sm.ownJID(); !own.IsEmpty() && sm.sameUser(sender, own) {
		msg.QuoteName = QuoteYou
		return
	}
	msg.QuoteName = sm.senderLabel(msg.QuoteSender, "")
}

// quotedCopy is the copy of q put into a reply: its media sub-message when
// stored (so the other side shows a thumbnail-less media quote), else its
// text.
func quotedCopy(q Message) *waE2E.Message {
	if len(q.MediaProto) > 0 {
		var m waE2E.Message
		if proto.Unmarshal(q.MediaProto, &m) == nil {
			return &m
		}
	}
	return &waE2E.Message{Conversation: proto.String(strings.TrimSuffix(q.Text, " "+EditedMark))}
}

// textMessage builds a text message, as a reply when quoteID is set.
func textMessage(text, quoteID, quoteSender string, quoted *waE2E.Message) *waProto.Message {
	if quoteID == "" {
		return &waProto.Message{Conversation: proto.String(text)}
	}
	ci := &waE2E.ContextInfo{StanzaID: proto.String(quoteID), QuotedMessage: quoted}
	if quoteSender != "" {
		ci.Participant = proto.String(quoteSender)
	}
	return &waProto.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text), ContextInfo: ci}}
}

// quoteOf returns the quote fields for a reply to the stored message id:
// its sender JID and the copy to send.
func (sm *SessionManager) quoteOf(q Message) (sender string, copy *waE2E.Message) {
	if q.FromMe {
		if own := sm.ownJID(); !own.IsEmpty() {
			sender = own.String()
		}
	} else if jid, err := types.ParseJID(q.ContactId); err == nil {
		sender = jid.ToNonAD().String()
	}
	return sender, quotedCopy(q)
}

// cmdReply sends a reply: /reply <chat-id> <msg-id> <text>.
func cmdReply(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if len(params) < 3 || strings.TrimSpace(strings.Join(params[2:], " ")) == "" {
		sm.printCommandUsage(cmdName, "<chat-id> <msg-id> <text>")
		return
	}
	chat, id, text := params[0], params[1], strings.Join(params[2:], " ")
	q, found, err := sm.db.GetMessage(id)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !found || q.ChatId != chat {
		sm.uiHandler.PrintError(errors.New("message to reply to not found in this chat"))
		return
	}
	if q.Text == RevokedText {
		sm.uiHandler.PrintError(errors.New("a deleted message cannot be replied to"))
		return
	}
	sm.sendTextQuoted(chat, text, &q)
}

// quoteForRetry rebuilds the quote of a stored reply for sending it again.
func (sm *SessionManager) quoteForRetry(msg Message) *waE2E.Message {
	if q, found, err := sm.db.GetMessage(msg.QuoteId); err == nil && found {
		return quotedCopy(q)
	}
	return &waE2E.Message{Conversation: proto.String(msg.QuoteText)}
}
