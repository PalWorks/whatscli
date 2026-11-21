// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// composeApp is runInputApp with the compose features attached and a
// session manager whose commands the test can read.
func composeApp(t *testing.T) (*tview.Application, *tview.InputField, chan messages.Command) {
	t.Helper()
	app, input, _, _ := runInputApp(t)
	cmds := make(chan messages.Command, 8)
	savedShare := config.Config.General.SharePresence
	config.Config.General.SharePresence = false // no network in tests
	t.Cleanup(func() {
		config.Config.General.SharePresence = savedShare
		drafts = map[string]string{}
		editing = nil
		chatActivity = map[string]string{}
		chatPresence = map[string]string{}
	})
	run(app, func() {
		ctx.SessionManager = &messages.SessionManager{CommandChannel: cmds}
		ctx.TextView.SetDynamicColors(true).SetRegions(true) // as in main.go: selectable messages
		setupCompose()
	})
	return app, input, cmds
}

// run executes f on the app's event loop and waits for it.
func run(app *tview.Application, f func()) {
	done := make(chan struct{})
	app.QueueUpdateDraw(func() { f(); close(done) })
	<-done
}

func inputText(app *tview.Application, input *tview.InputField) string {
	var s string
	run(app, func() { s = input.GetText() })
	return s
}

var chatA = messages.Chat{Id: "111@s.whatsapp.net", Name: "Asha"}
var chatB = messages.Chat{Id: "222@s.whatsapp.net", Name: "Bina"}

func TestDraftsStayWithTheirChat(t *testing.T) {
	app, input, cmds := composeApp(t)
	run(app, func() { enterChat(chatA) })
	typeKeys(app, input, "hello asha")
	run(app, func() { enterChat(chatB) })
	if got := inputText(app, input); got != "" {
		t.Fatalf("chat B should start empty, got %q", got)
	}
	typeKeys(app, input, "hi bina")
	run(app, func() { enterChat(chatA) })
	if got := inputText(app, input); got != "hello asha" {
		t.Fatalf("draft of A = %q", got)
	}
	run(app, func() { enterChat(chatB) })
	if got := inputText(app, input); got != "hi bina" {
		t.Fatalf("draft of B = %q", got)
	}

	// Sending clears the draft for good.
	typeKeys(app, input, tcell.KeyEnter)
	if cmd := <-cmds; cmd.Name != "send" || cmd.Params[1] != "hi bina" {
		t.Fatalf("sent %+v", cmd)
	}
	run(app, func() { enterChat(chatA); enterChat(chatB) })
	if got := inputText(app, input); got != "" {
		t.Fatalf("sent text came back as a draft: %q", got)
	}
}

// Opening a chat with "/select <name>" brings its draft back after the
// command line is cleared.
func TestDraftBackAfterSelectCommand(t *testing.T) {
	app, input, _ := composeApp(t)
	run(app, func() { enterChat(chatA) })
	typeKeys(app, input, "draft for a")
	run(app, func() { enterChat(chatB) })
	run(app, func() {
		setInputText("/select asha") // as typed; EnterCommand then opens A and clears
		enterChat(chatA)
		setInputText("")
	})
	if !waitFor(app, func() bool { return input.GetText() == "draft for a" }) {
		t.Fatalf("draft not restored: %q", inputText(app, input))
	}
}

func TestUpEditsLastOwnMessage(t *testing.T) {
	app, input, cmds := composeApp(t)
	now := uint64(time.Now().Unix())
	run(app, func() {
		enterChat(chatA)
		ctx.CurRegions = []messages.Message{
			{Id: "m1", ChatId: chatA.Id, FromMe: true, Text: "first", Timestamp: now - 60},
			{Id: "m2", ChatId: chatA.Id, Text: "their reply", Timestamp: now - 50},
			{Id: "m3", ChatId: chatA.Id, FromMe: true, Text: "helo (edited)", Timestamp: now - 30},
		}
	})
	typeKeys(app, input, "my draft")
	typeKeys(app, input, tcell.KeyCtrlU) // clear the line (tview: delete to start)
	run(app, func() { setInputText("") })
	typeKeys(app, input, tcell.KeyUp)
	if got := inputText(app, input); got != "helo" {
		t.Fatalf("Up loaded %q, want the last own message without (edited)", got)
	}
	var label string
	run(app, func() { label = input.GetLabel() })
	if !strings.Contains(label, "edit>") {
		t.Fatalf("label = %q", label)
	}
	run(app, func() { setInputText("hello") })
	typeKeys(app, input, tcell.KeyEnter)
	cmd := <-cmds
	if cmd.Name != "edit" || strings.Join(cmd.Params, "|") != chatA.Id+"|m3|hello" {
		t.Fatalf("sent %+v", cmd)
	}
	run(app, func() { label = input.GetLabel() })
	if label != "" || editing != nil {
		t.Fatalf("still editing after Enter (label %q)", label)
	}

	// Esc cancels without sending.
	typeKeys(app, input, tcell.KeyUp)
	typeKeys(app, input, "x", tcell.KeyEscape)
	select {
	case c := <-cmds:
		t.Fatalf("Esc sent %+v", c)
	default:
	}
	if got := inputText(app, input); got != "" || editing != nil {
		t.Fatalf("after Esc: input %q, editing %v", got, editing != nil)
	}
}

// Up on a last message that may not be edited loads nothing and says why;
// with no own message it stays quiet.
func TestUpExplainsWhyNotEditable(t *testing.T) {
	app, input, _ := composeApp(t)
	old := uint64(time.Now().Add(-messages.EditWindow - time.Minute).Unix())
	now := uint64(time.Now().Unix())
	for _, tt := range []struct {
		regions []messages.Message
		note    string
	}{
		{[]messages.Message{{Id: "a", FromMe: true, Text: "old one", Timestamp: old}}, "too late to edit"},
		{[]messages.Message{{Id: "b", FromMe: true, Text: "[IMAGE]", MediaKind: "image", Timestamp: now}}, "only text messages"},
		{[]messages.Message{{Id: "c", FromMe: true, Text: "failed", Status: messages.StatusFailed, Timestamp: now}}, "was not sent"},
		{[]messages.Message{{Id: "d", Text: "only theirs", Timestamp: now}}, ""},
		{nil, ""},
	} {
		run(app, func() { enterChat(chatA); ctx.CurRegions = tt.regions; setInputText(""); ctx.TextView.Clear() })
		typeKeys(app, input, tcell.KeyUp)
		if got := inputText(app, input); got != "" || editing != nil {
			t.Errorf("%+v: Up loaded %q", tt.regions, got)
		}
		var pane string
		run(app, func() { pane = ctx.TextView.GetText(true) })
		switch {
		case tt.note == "" && strings.Contains(pane, "Cannot edit"):
			t.Errorf("%+v: unexpected note %q", tt.regions, pane)
		case tt.note != "" && (!strings.Contains(pane, "Cannot edit your last message") || !strings.Contains(pane, tt.note)):
			t.Errorf("%+v: pane = %q, want a note with %q", tt.regions, pane, tt.note)
		}
	}
}

func TestTitleShowsActivityThenPresence(t *testing.T) {
	app, _, _ := composeApp(t)
	title := func() string {
		var s string
		run(app, func() { s = ctx.TextView.GetTitle() })
		return s
	}
	run(app, func() { enterChat(chatA); setChatPresence(chatA.Id, "online") })
	if got := title(); got != "Asha · online" {
		t.Fatalf("title = %q", got)
	}
	run(app, func() { setChatActivity(chatA.Id, "typing…") })
	if got := title(); got != "Asha · typing…" {
		t.Fatalf("title = %q", got)
	}
	run(app, func() { setChatActivity(chatA.Id, "") })
	if got := title(); got != "Asha · online" {
		t.Fatalf("after typing stopped: %q", got)
	}
	run(app, func() { setChatActivity(chatB.Id, "typing…") }) // another chat
	if got := title(); got != "Asha · online" {
		t.Fatalf("other chat's typing shown: %q", got)
	}
	run(app, func() { enterChat(chatB) })
	if got := title(); got != "Bina · typing…" {
		t.Fatalf("title = %q", got)
	}
}

func TestTypingStateFollowsInput(t *testing.T) {
	app, input, _ := composeApp(t)
	run(app, func() {
		config.Config.General.SharePresence = true
		enterChat(chatA)
	})
	typeKeys(app, input, "hey")
	var chat string
	run(app, func() { chat = typingChat })
	if chat != chatA.Id {
		t.Fatalf("typing not noted: %q", chat)
	}
	typeKeys(app, input, tcell.KeyEnter)
	run(app, func() { chat = typingChat })
	if chat != "" {
		t.Fatalf("still typing after send: %q", chat)
	}
	typeKeys(app, input, "/he")
	run(app, func() { chat = typingChat })
	if chat != "" {
		t.Fatal("a command counted as typing")
	}
}

// A failed send is marked wherever it is in the chat, not only last.
func TestFailedSendMarkedAnywhere(t *testing.T) {
	saved, savedCtx := config.Config.Ui.MessageStyle, ctx
	ctx = &AppContext{}
	t.Cleanup(func() { config.Config.Ui.MessageStyle = saved; ctx = savedCtx })
	for _, style := range []string{"bubbles", "classic"} {
		config.Config.Ui.MessageStyle = style
		out := getMessagesString([]messages.Message{
			{Id: "1", FromMe: true, Text: "lost", Status: messages.StatusFailed, Timestamp: 1000},
			{Id: "2", Text: "later reply", Timestamp: 1001},
		})
		if !strings.Contains(out, "! not sent") {
			t.Errorf("%s: failed send not marked:\n%s", style, out)
		}
	}
}
