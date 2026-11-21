// SPDX-License-Identifier: AGPL-3.0-only

package messages

// cmd_edit.go: editing our own last messages (/edit, used by Up in an empty
// input) and sending a failed message again (/retry, the retry key on a
// message marked as not sent).

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// EditWindow is how long after sending a message can be edited (WhatsApp's
// limit is 15 minutes).
const EditWindow = 15 * time.Minute

// Editable reports why msg cannot be edited now, or nil: only our own sent
// text messages, not revoked, within EditWindow.
func Editable(msg Message, now time.Time) error {
	switch {
	case !msg.FromMe:
		return errors.New("only your own messages can be edited")
	case msg.MediaKind != "":
		return errors.New("only text messages can be edited")
	case msg.Text == RevokedText:
		return errors.New("a deleted message cannot be edited")
	case msg.Status == StatusFailed:
		return errors.New("this message was not sent; send it again instead")
	case now.Sub(time.Unix(int64(msg.Timestamp), 0)) > EditWindow:
		return errors.New("too late to edit: WhatsApp allows edits for 15 minutes")
	}
	return nil
}

// cmdEdit replaces the text of our message: /edit <chat-id> <msg-id> <text>.
func cmdEdit(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if len(params) < 3 || strings.TrimSpace(strings.Join(params[2:], " ")) == "" {
		sm.printCommandUsage(cmdName, "<chat-id> <msg-id> <new text>")
		return
	}
	chat, id, text := params[0], params[1], strings.Join(params[2:], " ")
	msg, found, err := sm.db.GetMessage(id)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !found || msg.ChatId != chat {
		sm.uiHandler.PrintError(errors.New("message not found in this chat"))
		return
	}
	if err := Editable(msg, time.Now()); err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	jid, err := types.ParseJID(chat)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
		return
	}
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp: message not edited"))
		return
	}
	go func() {
		edit := client.BuildEdit(jid, types.MessageID(id), &waProto.Message{Conversation: proto.String(text)})
		if _, err := client.SendMessage(sm.rootContext(), jid, edit); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("edit not sent: %v", err))
			return
		}
		updated, err := sm.db.UpdateMessageText(chat, id, text+" "+EditedMark)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		if updated && sm.refreshTarget(chat) != "" {
			sm.uiHandler.NewScreen(sm.getMessages(chat))
		}
	}()
}

// cmdRetry sends a message marked as not sent again, under its own ID:
// /retry <msg-id>.
func cmdRetry(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if len(params) != 1 {
		sm.printCommandUsage(cmdName, "<msg-id>")
		return
	}
	msg, found, err := sm.db.GetMessage(params[0])
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !found || !msg.FromMe || msg.Status != StatusFailed {
		sm.uiHandler.PrintError(errors.New("only a message marked as not sent can be sent again"))
		return
	}
	if msg.MediaKind != "" {
		sm.uiHandler.PrintError(errors.New("send the file again to retry a media message"))
		return
	}
	jid, err := types.ParseJID(msg.ChatId)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
		return
	}
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp: try again once connected"))
		return
	}
	go func() {
		out := &waProto.Message{Conversation: proto.String(msg.Text)}
		if msg.QuoteId != "" {
			out = textMessage(msg.Text, msg.QuoteId, msg.QuoteSender, sm.quoteForRetry(msg))
		}
		if _, err := client.SendMessage(sm.rootContext(), jid, out, whatsmeow.SendRequestExtra{ID: types.MessageID(msg.Id)}); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("message not sent: %v", err))
			return
		}
		status := StatusSent
		if early := sm.earlyReceipts.take(msg.Id); early > status {
			status = early
		}
		chats, _, err := sm.db.RaiseMessageStatus([]string{msg.Id}, status)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		for _, chat := range chats {
			if sm.refreshTarget(chat) != "" {
				sm.uiHandler.NewScreen(sm.getMessages(chat))
			}
		}
	}()
}
