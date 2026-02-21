package main

import (
	"code.rocketnine.space/tslocum/cbind"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

// AppContext holds all application state that was previously scattered across
// package-level variables. This makes dependencies explicit and the codebase
// easier to test and maintain.
type AppContext struct {
	// UI Widgets
	App         *tview.Application
	TextView    *tview.TextView
	TextInput   *tview.InputField
	TopBar      *tview.TextView
	LeftPane    *tview.Flex
	ChatTable   *tview.Table
	GroupTable  *tview.Table
	StatusTable *tview.Table

	// UI State
	CurrentReceiver messages.Chat
	CurRegions      []messages.Message
	MouseState      bool
	AllChats        []*messages.Conversation
	ChatLimit       int
	RenderingList   bool // true while RenderChatTable is restoring selection

	// Backend
	SessionManager *messages.SessionManager
	KeyBindings    *cbind.Configuration
	UiHandler      messages.UiMessageHandler
}

// Singleton application context, initialized in main().
var ctx *AppContext
