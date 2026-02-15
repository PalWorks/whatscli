package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

// SetupLeftPane initializes the left panel with Status, Chats, and Groups
func SetupLeftPane() *tview.Flex {
	// 1. Status Section
	ctx.StatusTable = tview.NewTable()
	ctx.StatusTable.SetSelectable(true, false)
	ctx.StatusTable.SetBorder(true)
	ctx.StatusTable.SetTitle(" Status ")
	ctx.StatusTable.SetTitleAlign(tview.AlignCenter)
	ctx.StatusTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// 2. Chats Section
	ctx.ChatTable = tview.NewTable()
	ctx.ChatTable.SetSelectable(true, false)
	ctx.ChatTable.SetBorder(true)
	ctx.ChatTable.SetTitle(" Chats ")
	ctx.ChatTable.SetTitleAlign(tview.AlignCenter)
	ctx.ChatTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// 3. Groups Section
	ctx.GroupTable = tview.NewTable()
	ctx.GroupTable.SetSelectable(true, false)
	ctx.GroupTable.SetBorder(true)
	ctx.GroupTable.SetTitle(" Groups ")
	ctx.GroupTable.SetTitleAlign(tview.AlignCenter)
	ctx.GroupTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// Helper to update focus appearance (mutually exclusive)
	setActiveTable := func(active *tview.Table) {
		tables := []*tview.Table{ctx.StatusTable, ctx.ChatTable, ctx.GroupTable}
		for _, t := range tables {
			if t == active {
				t.SetSelectable(true, false)
				t.SetSelectedStyle(tcell.StyleDefault.Reverse(true))
			} else {
				t.SetSelectable(false, false)
				t.SetSelectedStyle(tcell.StyleDefault)
			}
		}
	}

	// Define selection handlers
	statusSelectFunc := func(row, column int) {
		if ctx.StatusTable.HasFocus() && row >= 0 && row < ctx.StatusTable.GetRowCount() {
			cell := ctx.StatusTable.GetCell(row, column)
			if cell != nil {
				ref := cell.GetReference()
				if ref != nil {
					conv := ref.(*messages.Conversation)
					chat := messages.Chat{
						Id:          conv.JID,
						Name:        conv.Name,
						Unread:      int(conv.Unread),
						LastMessage: conv.LastMsgTime,
						IsGroup:     strings.HasSuffix(conv.JID, messages.GROUPSUFFIX),
					}
					SetDisplayedChat(chat)
				}
			}
		}
	}

	chatSelectFunc := func(row, column int) {
		if ctx.ChatTable.HasFocus() && row >= 0 && row < ctx.ChatTable.GetRowCount() {
			cell := ctx.ChatTable.GetCell(row, column)
			if cell != nil {
				ref := cell.GetReference()
				if ref != nil {
					conv := ref.(*messages.Conversation)
					chat := messages.Chat{
						Id:          conv.JID,
						Name:        conv.Name,
						Unread:      int(conv.Unread),
						LastMessage: conv.LastMsgTime,
						IsGroup:     strings.HasSuffix(conv.JID, messages.GROUPSUFFIX),
					}
					SetDisplayedChat(chat)
				}
			}
		}
		// Infinite scroll trigger for contacts
		if row >= ctx.ChatTable.GetRowCount()-5 && ctx.ChatLimit < len(ctx.AllChats) {
			ctx.ChatLimit += batchSize
			RenderChatTable()
		}
	}

	groupSelectFunc := func(row, column int) {
		if ctx.GroupTable.HasFocus() && row >= 0 && row < ctx.GroupTable.GetRowCount() {
			cell := ctx.GroupTable.GetCell(row, column)
			if cell != nil {
				ref := cell.GetReference()
				if ref != nil {
					conv := ref.(*messages.Conversation)
					chat := messages.Chat{
						Id:          conv.JID,
						Name:        conv.Name,
						Unread:      int(conv.Unread),
						LastMessage: conv.LastMsgTime,
						IsGroup:     strings.HasSuffix(conv.JID, messages.GROUPSUFFIX),
					}
					SetDisplayedChat(chat)
				}
			}
		}
	}

	// Selection Changed Funcs
	ctx.StatusTable.SetSelectionChangedFunc(statusSelectFunc)
	ctx.StatusTable.SetFocusFunc(func() {
		setActiveTable(ctx.StatusTable)
		row, col := ctx.StatusTable.GetSelection()
		statusSelectFunc(row, col)
	})

	ctx.ChatTable.SetSelectionChangedFunc(chatSelectFunc)
	ctx.ChatTable.SetFocusFunc(func() {
		setActiveTable(ctx.ChatTable)
		row, col := ctx.ChatTable.GetSelection()
		chatSelectFunc(row, col)
	})

	ctx.GroupTable.SetSelectionChangedFunc(groupSelectFunc)
	ctx.GroupTable.SetFocusFunc(func() {
		setActiveTable(ctx.GroupTable)
		row, col := ctx.GroupTable.GetSelection()
		groupSelectFunc(row, col)
	})

	// Initialize styles (start with all inactive)
	setActiveTable(nil)

	ctx.LeftPane = tview.NewFlex().SetDirection(tview.FlexRow)
	// Status: Border top + 1 row + Border bottom = 3 lines
	ctx.LeftPane.AddItem(ctx.StatusTable, 3, 1, false)

	ctx.LeftPane.AddItem(ctx.ChatTable, 0, 1, true)
	ctx.LeftPane.AddItem(ctx.GroupTable, 0, 1, false)

	return ctx.LeftPane
}
