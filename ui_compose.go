// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_compose.go: the input line around a chat.
//
//   - Drafts: text typed but not sent stays with its chat when another chat
//     is opened, and comes back when the chat is opened again.
//   - Edit: Up in an empty input loads the last own text message of the chat
//     (if it may still be edited) with the label "edit> "; Enter sends the
//     edit (/edit), Esc cancels.
//   - Typing: while text is typed the chat is told so; after a pause, on
//     send or on leaving the chat, it is told we stopped.
//   - Title: the chat title shows what the other side is doing ("typing…")
//     or their online state ("online", "last seen ...").
//   - Online while active: a key press marks us online, a few idle minutes
//     offline again (see messages/presence.go for why).
//
// Everything here runs on the UI goroutine; timers queue their work there.

import (
	"strings"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	// typingRefresh is how often "typing" is repeated while typing goes on.
	typingRefresh = 10 * time.Second
	// typingPause is how long without a key counts as having stopped.
	typingPause = 5 * time.Second
	// activityExpiry clears a typing notice that was never followed by
	// "stopped" (WhatsApp does the same).
	activityExpiry = 25 * time.Second
	// idleOffline is how long without a key before we are marked offline.
	idleOffline = 5 * time.Minute
	// editLabel is the input label while a message is being edited.
	editLabel = "edit> "
)

var (
	drafts       = map[string]string{} // chat ID -> text typed but not sent
	composeQuiet bool                  // set while the program changes the input text
	editing      *messages.Message     // the message being edited, nil if none
	replying     *messages.Message     // the message being replied to, nil if none

	typingChat  string // chat told we are typing, "" if none
	typingSent  time.Time
	typingTimer *time.Timer

	chatActivity   = map[string]string{} // chat ID -> "typing…" etc.
	chatPresence   = map[string]string{} // chat ID -> "online" etc.
	activityTimers = map[string]*time.Timer{}

	presenceActive bool
	idleTimer      *time.Timer
)

func isCommandText(text string) bool {
	return strings.HasPrefix(text, commandPrefix())
}

// setInputText changes the input text without counting it as typing or as
// a draft change.
func setInputText(text string) {
	composeQuiet = true
	ctx.TextInput.SetText(text)
	composeQuiet = false
}

// setupCompose attaches drafts, typing notices and Up-to-edit to the input
// field. Call it after the field's own key handler is set: Up in an empty
// input is handled first, every other key goes on to that handler.
func setupCompose() {
	ctx.TextInput.SetChangedFunc(onInputChanged)
	next := ctx.TextInput.GetInputCapture()
	ctx.TextInput.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyUp && ctx.TextInput.GetText() == "" && editing == nil && !commandListOpen() && startEdit() {
			return nil
		}
		if next != nil {
			return next(ev)
		}
		return ev
	})
}

func onInputChanged(text string) {
	chat := ctx.CurrentReceiver.Id
	if composeQuiet || chat == "" || isCommandText(text) {
		return
	}
	if editing == nil {
		if text == "" {
			delete(drafts, chat)
		} else {
			drafts[chat] = text
		}
	}
	noteTyping(chat, text != "")
}

// enterChat makes chat the open chat on the UI side: the title, and the
// input line (the chat's draft; editing and typing in the old chat end).
func enterChat(chat messages.Chat) {
	old := ctx.CurrentReceiver.Id
	if old != chat.Id {
		cancelEdit()
		cancelReply()
		stopTyping()
	}
	text := ctx.TextInput.GetText()
	ctx.CurrentReceiver = chat
	updateChatTitle()
	if old == chat.Id {
		return
	}
	if !isCommandText(text) {
		setInputText(drafts[chat.Id])
		return
	}
	// A command such as "/select asha" opened the chat; the draft comes back
	// once the command line has been cleared.
	id := chat.Id
	go ctx.App.QueueUpdateDraw(func() {
		if ctx.CurrentReceiver.Id == id && ctx.TextInput.GetText() == "" {
			setInputText(drafts[id])
		}
	})
}

// clearAfterSend empties the input after a message was sent and forgets the
// chat's draft.
func clearAfterSend(chat string) {
	delete(drafts, chat)
	stopTyping()
	setInputText("")
}

// lastOwn returns the chat's last own message, nil if there is none, and
// why it may not be edited (nil if it may).
func lastOwn(now time.Time) (*messages.Message, error) {
	for i := len(ctx.CurRegions) - 1; i >= 0; i-- {
		m := ctx.CurRegions[i]
		if !m.FromMe || strings.TrimSpace(m.Text) == "" {
			continue
		}
		return &m, messages.Editable(m, now)
	}
	return nil, nil
}

// startEdit loads the last own message for editing. If it may not be
// edited, a note in the message pane says why. False if nothing was loaded.
func startEdit() bool {
	m, err := lastOwn(time.Now())
	if m == nil {
		return false
	}
	if err != nil {
		PrintText("[::d]Cannot edit your last message: " + tview.Escape(err.Error()) + "[::-]")
		ctx.TextView.ScrollToEnd()
		return false
	}
	cancelReply()
	editing = m
	ctx.TextInput.SetLabel("[" + config.Config.Colors.ForwardedText + "]" + editLabel + "[-]")
	setInputText(strings.TrimSuffix(m.Text, " "+messages.EditedMark))
	return true
}

// cancelEdit leaves edit mode and brings the draft back.
func cancelEdit() {
	if editing == nil {
		return
	}
	editing = nil
	ctx.TextInput.SetLabel("")
	setInputText(drafts[ctx.CurrentReceiver.Id])
}

// finishEdit sends the edited text (unchanged or empty text just leaves
// edit mode) and returns to the draft.
func finishEdit(text string) {
	m := editing
	editing = nil
	ctx.TextInput.SetLabel("")
	stopTyping()
	old := strings.TrimSuffix(m.Text, " "+messages.EditedMark)
	if strings.TrimSpace(text) != "" && text != old {
		ctx.SessionManager.CommandChannel <- messages.Command{Name: "edit", Params: []string{m.ChatId, m.Id, text}}
	}
	setInputText(drafts[ctx.CurrentReceiver.Id])
}

// replyLabelWidth caps the "↳ Asha: text ›" label on the input line.
const replyLabelWidth = 40

// startReply starts a reply to m: the input line is labelled with who and
// what is quoted, and gets the focus. Enter sends the reply, Esc cancels.
func startReply(m messages.Message) {
	cancelEdit()
	name := messages.QuoteYou
	if !m.FromMe {
		name = displayName(m.ContactShort)
	}
	q := messages.Message{QuoteId: m.Id, QuoteName: name, QuoteText: m.Text}
	_, color := quoteLabel(&q)
	replying = &m
	ctx.TextInput.SetLabel("[" + color + "]↳ " + clip(tview.Escape(name)+": "+quoteText(&q), replyLabelWidth) + " › [-]")
	ctx.App.SetFocus(ctx.TextInput)
}

// cancelReply leaves reply mode; the typed text stays.
func cancelReply() {
	if replying == nil {
		return
	}
	replying = nil
	ctx.TextInput.SetLabel("")
}

// sendReply sends text as a reply to the message being replied to.
func sendReply(text string) {
	m := replying
	cancelReply()
	ctx.SessionManager.CommandChannel <- messages.Command{Name: "reply", Params: []string{m.ChatId, m.Id, text}}
	clearAfterSend(ctx.CurrentReceiver.Id)
}

// noteTyping tells chat that we type (repeated every typingRefresh) and
// that we stopped after typingPause without a change.
func noteTyping(chat string, typing bool) {
	if !config.Config.General.SharePresence || ctx.SessionManager == nil {
		return
	}
	if !typing {
		stopTyping()
		return
	}
	if typingChat != chat || time.Since(typingSent) > typingRefresh {
		if typingChat != "" && typingChat != chat {
			stopTyping()
		}
		ctx.SessionManager.SendTyping(chat, true)
		typingChat, typingSent = chat, time.Now()
	}
	if typingTimer != nil {
		typingTimer.Stop()
	}
	typingTimer = time.AfterFunc(typingPause, func() { ctx.App.QueueUpdate(stopTyping) })
}

// stopTyping tells the chat we typed in that we stopped.
func stopTyping() {
	if typingTimer != nil {
		typingTimer.Stop()
		typingTimer = nil
	}
	if typingChat == "" {
		return
	}
	if ctx.SessionManager != nil {
		ctx.SessionManager.SendTyping(typingChat, false)
	}
	typingChat = ""
}

// updateChatTitle shows the open chat's name, followed by what the other
// side is doing or their online state.
func updateChatTitle() {
	if ctx.TextView == nil {
		return
	}
	chat := ctx.CurrentReceiver
	title := displayName(chat.Name)
	if title == "" {
		title = chat.Id
	}
	if s := chatActivity[chat.Id]; s != "" {
		title += " · " + s
	} else if s := chatPresence[chat.Id]; s != "" {
		title += " · " + s
	}
	ctx.TextView.SetTitle(title)
}

// setChatActivity records what the other side of chat is doing; a notice
// that is not renewed is cleared after activityExpiry.
func setChatActivity(chat, text string) {
	if t := activityTimers[chat]; t != nil {
		t.Stop()
		delete(activityTimers, chat)
	}
	if text == "" {
		delete(chatActivity, chat)
	} else {
		chatActivity[chat] = text
		activityTimers[chat] = time.AfterFunc(activityExpiry, func() {
			ctx.App.QueueUpdateDraw(func() {
				if chatActivity[chat] == text {
					setChatActivity(chat, "")
				}
			})
		})
	}
	if chat == ctx.CurrentReceiver.Id {
		updateChatTitle()
	}
}

func setChatPresence(chat, text string) {
	if text == "" {
		delete(chatPresence, chat)
	} else {
		chatPresence[chat] = text
	}
	if chat == ctx.CurrentReceiver.Id {
		updateChatTitle()
	}
}

// noteUserActive marks us online on a key press and offline after
// idleOffline without one.
func noteUserActive() {
	if !config.Config.General.SharePresence || ctx.SessionManager == nil {
		return
	}
	if !presenceActive {
		presenceActive = true
		ctx.SessionManager.SetActive(true)
	}
	if idleTimer != nil {
		idleTimer.Stop()
	}
	idleTimer = time.AfterFunc(idleOffline, func() {
		ctx.App.QueueUpdate(func() {
			presenceActive = false
			ctx.SessionManager.SetActive(false)
		})
	})
}
