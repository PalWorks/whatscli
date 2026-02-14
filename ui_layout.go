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
	statusTable = tview.NewTable()
	statusTable.SetSelectable(true, false)
	statusTable.SetBorder(true)
	statusTable.SetTitle(" Status ")
	statusTable.SetTitleAlign(tview.AlignCenter)
	statusTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// 2. Chats Section
	chatTable = tview.NewTable()
	chatTable.SetSelectable(true, false)
	chatTable.SetBorder(true)
	chatTable.SetTitle(" Chats ")
	chatTable.SetTitleAlign(tview.AlignCenter)
	chatTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// 3. Groups Section
	groupTable = tview.NewTable()
	groupTable.SetSelectable(true, false)
	groupTable.SetBorder(true)
	groupTable.SetTitle(" Groups ")
	groupTable.SetTitleAlign(tview.AlignCenter)
	groupTable.SetBorderColor(tcell.ColorNames[config.Config.Colors.Borders])

	// Helper to update focus appearance (mutually exclusive)
	setActiveTable := func(active *tview.Table) {
		tables := []*tview.Table{statusTable, chatTable, groupTable}
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
		if statusTable.HasFocus() && row >= 0 && row < statusTable.GetRowCount() {
			cell := statusTable.GetCell(row, column)
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
		if chatTable.HasFocus() && row >= 0 && row < chatTable.GetRowCount() {
			cell := chatTable.GetCell(row, column)
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
		if row >= chatTable.GetRowCount()-5 && chatLimit < len(allChats) {
			chatLimit += batchSize
			RenderChatTable()
		}
	}

	groupSelectFunc := func(row, column int) {
		if groupTable.HasFocus() && row >= 0 && row < groupTable.GetRowCount() {
			cell := groupTable.GetCell(row, column)
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
	statusTable.SetSelectionChangedFunc(statusSelectFunc)
	statusTable.SetFocusFunc(func() {
		setActiveTable(statusTable)
		row, col := statusTable.GetSelection()
		statusSelectFunc(row, col)
	})

	chatTable.SetSelectionChangedFunc(chatSelectFunc)
	chatTable.SetFocusFunc(func() {
		setActiveTable(chatTable)
		row, col := chatTable.GetSelection()
		chatSelectFunc(row, col)
	})

	groupTable.SetSelectionChangedFunc(groupSelectFunc)
	groupTable.SetFocusFunc(func() {
		setActiveTable(groupTable)
		row, col := groupTable.GetSelection()
		groupSelectFunc(row, col)
	})

	// Initialize styles (start with all inactive)
	setActiveTable(nil)

	leftPane = tview.NewFlex().SetDirection(tview.FlexRow)
	// Status: Border top + 1 row + Border bottom = 3 lines
	leftPane.AddItem(statusTable, 3, 1, false)

	leftPane.AddItem(chatTable, 0, 1, true)
	leftPane.AddItem(groupTable, 0, 1, false)

	return leftPane
}
