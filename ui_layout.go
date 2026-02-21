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

	// --- Selection handlers: open chat on click/Enter ---
	// These fire via SetSelectionChangedFunc (mouse click, arrow keys) and
	// SetSelectedFunc (Enter key).  The RenderingList guard prevents
	// ghost-scrolling when RenderChatTable restores selection.

	openFromStatus := func(row, column int) {
		if ctx.RenderingList {
			return
		}
		if row >= 0 && row < ctx.StatusTable.GetRowCount() {
			cell := ctx.StatusTable.GetCell(row, 0)
			if cell != nil {
				if ref := cell.GetReference(); ref != nil {
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

	openFromChat := func(row, column int) {
		if ctx.RenderingList {
			return
		}
		if row >= 0 && row < ctx.ChatTable.GetRowCount() {
			cell := ctx.ChatTable.GetCell(row, 0)
			if cell != nil {
				if ref := cell.GetReference(); ref != nil {
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
		// Infinite scroll trigger
		if row >= ctx.ChatTable.GetRowCount()-5 && ctx.ChatLimit < len(ctx.AllChats) {
			ctx.ChatLimit += batchSize
			RenderChatTable()
		}
	}

	openFromGroup := func(row, column int) {
		if ctx.RenderingList {
			return
		}
		if row >= 0 && row < ctx.GroupTable.GetRowCount() {
			cell := ctx.GroupTable.GetCell(row, 0)
			if cell != nil {
				if ref := cell.GetReference(); ref != nil {
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

	// Wire: SelectionChanged fires on click + arrow keys; Selected fires on Enter.
	ctx.StatusTable.SetSelectionChangedFunc(openFromStatus)
	ctx.StatusTable.SetSelectedFunc(openFromStatus)

	ctx.ChatTable.SetSelectionChangedFunc(openFromChat)
	ctx.ChatTable.SetSelectedFunc(openFromChat)

	ctx.GroupTable.SetSelectionChangedFunc(openFromGroup)
	ctx.GroupTable.SetSelectedFunc(openFromGroup)

	ctx.StatusTable.SetFocusFunc(func() {
		setActiveTable(ctx.StatusTable)
	})

	ctx.ChatTable.SetFocusFunc(func() {
		setActiveTable(ctx.ChatTable)
	})

	ctx.GroupTable.SetFocusFunc(func() {
		setActiveTable(ctx.GroupTable)
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
