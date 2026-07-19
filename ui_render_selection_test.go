package main

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

// newRenderTestContext builds a minimal AppContext with the three sidebar
// tables wired the way SetupLeftPane wires them, but without the selection
// callbacks (which would reach into a SessionManager). This is exactly what
// RenderChatTable needs, and is the kind of isolated construction that was
// impossible while the widgets lived in a package-level ctx global.
func newRenderTestContext() *AppContext {
	ctx := &AppContext{ChatLimit: 100}
	ctx.StatusTable = tview.NewTable()
	ctx.StatusTable.SetSelectable(true, false)
	ctx.ChatTable = tview.NewTable()
	ctx.ChatTable.SetSelectable(true, false)
	ctx.GroupTable = tview.NewTable()
	ctx.GroupTable.SetSelectable(true, false)
	return ctx
}

func selectedJID(tbl *tview.Table) string {
	row, _ := tbl.GetSelection()
	cell := tbl.GetCell(row, 0)
	if cell == nil {
		return ""
	}
	ref := cell.GetReference()
	if ref == nil {
		return ""
	}
	return ref.(*messages.Conversation).JID
}

// TestRenderChatTable_RestoresSelectionByJID verifies that when the chat list
// is re-rendered after a new conversation arrives (shifting every row index),
// the highlighted row still points at the same JID it did before — i.e. the
// selection follows the chat, not the row number.
func TestRenderChatTable_RestoresSelectionByJID(t *testing.T) {
	ctx := newRenderTestContext()

	// Offscreen screen so we exercise the real draw path, not just the
	// in-memory selection bookkeeping.
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	defer screen.Fini()
	draw := func() {
		for _, tbl := range []*tview.Table{ctx.StatusTable, ctx.ChatTable, ctx.GroupTable} {
			tbl.SetRect(0, 0, 40, 12)
			tbl.Draw(screen)
		}
	}

	mk := func(jid, name string, ts int64) *messages.Conversation {
		return &messages.Conversation{JID: jid, Name: name, LastMsgTime: ts}
	}
	alice := mk("111@s.whatsapp.net", "Alice", 300)
	bob := mk("222@s.whatsapp.net", "Bob", 200)
	carol := mk("333@s.whatsapp.net", "Carol", 100)
	ctx.AllChats = []*messages.Conversation{alice, bob, carol}

	ctx.RenderChatTable()
	draw()

	if got := ctx.ChatTable.GetRowCount(); got != 3 {
		t.Fatalf("expected 3 contact rows, got %d", got)
	}

	// Select Bob (row 1).
	ctx.ChatTable.Select(1, 0)
	if jid := selectedJID(ctx.ChatTable); jid != bob.JID {
		t.Fatalf("precondition failed: expected Bob selected, got %q", jid)
	}

	// A newer chat arrives at the top, pushing everyone down one row.
	zoe := mk("000@s.whatsapp.net", "Zoe", 400)
	ctx.AllChats = []*messages.Conversation{zoe, alice, bob, carol}
	ctx.RenderChatTable()
	draw()

	// Bob is now at row 2; the selection must have moved with him.
	row, _ := ctx.ChatTable.GetSelection()
	if row != 2 {
		t.Errorf("expected Bob to be restored at row 2, got row %d", row)
	}
	if jid := selectedJID(ctx.ChatTable); jid != bob.JID {
		t.Errorf("expected selection to stay on Bob (%s), got %q", bob.JID, jid)
	}
}

// TestRenderChatTable_SplitsByJIDKind verifies the routing of conversations
// into the Status, Group, and Contact tables.
func TestRenderChatTable_SplitsByJIDKind(t *testing.T) {
	ctx := newRenderTestContext()

	status := &messages.Conversation{JID: messages.STATUSSUFFIX, Name: "Status", LastMsgTime: 500}
	group := &messages.Conversation{JID: "123456" + messages.GROUPSUFFIX, Name: "Group", LastMsgTime: 400}
	contact := &messages.Conversation{JID: "999@s.whatsapp.net", Name: "Contact", LastMsgTime: 300}
	ctx.AllChats = []*messages.Conversation{status, group, contact}

	ctx.RenderChatTable()

	if got := ctx.StatusTable.GetRowCount(); got != 1 {
		t.Errorf("expected 1 status row, got %d", got)
	}
	if got := ctx.GroupTable.GetRowCount(); got != 1 {
		t.Errorf("expected 1 group row, got %d", got)
	}
	if got := ctx.ChatTable.GetRowCount(); got != 1 {
		t.Errorf("expected 1 contact row, got %d", got)
	}
	if jid := selectedJID(ctx.GroupTable); jid != group.JID {
		t.Errorf("expected group table to hold the group JID, got %q", jid)
	}
}

// TestRenderChatTable_SkipsArchived verifies archived chats are not rendered.
func TestRenderChatTable_SkipsArchived(t *testing.T) {
	ctx := newRenderTestContext()

	visible := &messages.Conversation{JID: "1@s.whatsapp.net", Name: "Visible", LastMsgTime: 200}
	archived := &messages.Conversation{JID: "2@s.whatsapp.net", Name: "Archived", LastMsgTime: 100, IsArchived: true}
	ctx.AllChats = []*messages.Conversation{visible, archived}

	ctx.RenderChatTable()

	if got := ctx.ChatTable.GetRowCount(); got != 1 {
		t.Errorf("expected only the non-archived contact to render, got %d rows", got)
	}
	if jid := selectedJID(ctx.ChatTable); jid != visible.JID {
		t.Errorf("expected visible contact, got %q", jid)
	}
}
