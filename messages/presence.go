// SPDX-License-Identifier: AGPL-3.0-only

package messages

// presence.go: typing and online status. WhatsApp sends typing notifications
// and a contact's online / last seen state only to clients that marked
// themselves online, and being online is visible to others and can hold back
// notifications on the phone. So, like WhatsApp Web, WhatsCLI is online only
// while it is used: the UI calls SetActive(true) on a key press and
// SetActive(false) after a few idle minutes. With general.share_presence off
// nothing is sent: no online state, no typing, and none is received.
//
// All network calls run in goroutines; none is made under sm.mu.

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/PalWorks/whatscli/config"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// presenceTimeout bounds one presence request.
const presenceTimeout = 10 * time.Second

// presenceState is whether the user is active in WhatsCLI.
type presenceState struct {
	mu     sync.Mutex
	active bool
}

func sharePresence() bool { return config.Config.General.SharePresence }

// SetActive marks the user online (true) or offline (false) on WhatsApp,
// if presence sharing is on and the state changed. It does not block.
func (sm *SessionManager) SetActive(active bool) {
	if !sharePresence() {
		return
	}
	sm.presence.mu.Lock()
	changed := sm.presence.active != active
	sm.presence.active = active
	sm.presence.mu.Unlock()
	if changed {
		go sm.sendPresence()
	}
}

// sendPresence sends the current online state; when online it also asks for
// the open chat's presence. Called after SetActive and on connect.
func (sm *SessionManager) sendPresence() {
	if !sharePresence() {
		return
	}
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		return
	}
	sm.presence.mu.Lock()
	active := sm.presence.active
	sm.presence.mu.Unlock()
	state := types.PresenceUnavailable
	if active {
		state = types.PresenceAvailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), presenceTimeout)
	defer cancel()
	if err := client.SendPresence(ctx, state); err != nil {
		return // presence is best effort
	}
	if active {
		sm.subscribeChatPresence(sm.getCurrentReceiver())
	}
}

// subscribeChatPresence asks for the online state of the contact of a
// one-to-one chat. Groups have no presence. Best effort.
func (sm *SessionManager) subscribeChatPresence(chat string) {
	if !sharePresence() || chat == "" || strings.HasSuffix(chat, GROUPSUFFIX) {
		return
	}
	client := sm.getClient()
	if client == nil || !client.IsConnected() {
		return
	}
	jid, err := types.ParseJID(chat)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), presenceTimeout)
	defer cancel()
	_ = client.SubscribePresence(ctx, jid)
}

// SendTyping tells the chat that the user is typing (true) or stopped
// (false). It does not block.
func (sm *SessionManager) SendTyping(chat string, typing bool) {
	if !sharePresence() || chat == "" {
		return
	}
	go func() {
		client := sm.getClient()
		if client == nil || !client.IsConnected() {
			return
		}
		jid, err := types.ParseJID(chat)
		if err != nil {
			return
		}
		state := types.ChatPresencePaused
		if typing {
			state = types.ChatPresenceComposing
		}
		ctx, cancel := context.WithTimeout(context.Background(), presenceTimeout)
		defer cancel()
		_ = client.SendChatPresence(ctx, jid, state, types.ChatPresenceMediaText)
	}()
}

// chatActivityText describes a typing notification: "typing…", "recording
// audio…", in groups with the sender's name; "" when they stopped.
func chatActivityText(state types.ChatPresence, media types.ChatPresenceMedia, group bool, sender string) string {
	if state != types.ChatPresenceComposing {
		return ""
	}
	text := "typing…"
	if media == types.ChatPresenceMediaAudio {
		text = "recording audio…"
	}
	if group && sender != "" {
		return sender + " is " + text
	}
	return text
}

// presenceText describes a contact's presence: "online", "last seen ...",
// or "" when they hide it.
func presenceText(unavailable bool, lastSeen, now time.Time) string {
	if !unavailable {
		return "online"
	}
	if lastSeen.IsZero() {
		return ""
	}
	sameDate := func(a, b time.Time) bool {
		ya, ma, da := a.Date()
		yb, mb, db := b.Date()
		return ya == yb && ma == mb && da == db
	}
	switch {
	case sameDate(lastSeen, now):
		return "last seen today at " + lastSeen.Format("15:04")
	case sameDate(lastSeen, now.AddDate(0, 0, -1)):
		return "last seen yesterday at " + lastSeen.Format("15:04")
	}
	return "last seen " + lastSeen.Format("2 Jan 15:04")
}

func (sm *SessionManager) handleChatPresence(evt *events.ChatPresence) {
	if evt.IsFromMe {
		return
	}
	chat := sm.canonicalChatForSource(evt.MessageSource).String()
	sender := ""
	if evt.IsGroup {
		sender = sm.senderLabel(evt.Sender.String(), "")
	}
	sm.uiHandler.SetChatActivity(chat, chatActivityText(evt.State, evt.Media, evt.IsGroup, sender))
}

func (sm *SessionManager) handlePresence(evt *events.Presence) {
	chat := sm.canonicalChatJID(evt.From.ToNonAD()).String()
	sm.uiHandler.SetChatPresence(chat, presenceText(evt.Unavailable, evt.LastSeen, time.Now()))
}
