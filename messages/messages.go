// SPDX-License-Identifier: AGPL-3.0-only

// this package manages the messages
package messages

type UiMessageHandler interface {
	NewMessage(Message)
	NewScreen([]Message)
	SetChats([]Chat)
	UpdateChatList([]*Conversation)
	PrintError(error)
	PrintText(string)
	PrintFile(string)
	PrintQR(string)
	SetStatus(SessionStatus)
	OpenFile(string)
	ShowColorList()
	Clear()
	// ReplaceText clears the message pane and prints text in a single draw.
	ReplaceText(string)
	UpdateQR(qr string, attempt int, timeout int)
	PrintCommands()
	PrintHelp()
	Quit()
}

// data struct for current session status
type SessionStatus struct {
	BatteryCharge    int
	BatteryLoading   bool
	BatteryPowersave bool
	Connected        bool
	LastSeen         string
}

// message struct for battery messages
type BatteryMsg struct {
	charge    int
	loading   bool
	powersave bool
}

// message struct for status messages
type StatusMsg struct {
	connected bool
	err       error
}

// message object for commands
type Command struct {
	Name   string
	Params []string
}

// internal message representation to abstract from message lib
type Message struct {
	Id           string
	ChatId       string // the source of the message (group id or contact id)
	ContactId    string
	ContactName  string
	ContactShort string
	Timestamp    uint64
	FromMe       bool
	Forwarded    bool
	Text         string
	// Media metadata (empty for text messages). MediaProto holds a serialized
	// waE2E.Message containing only the media sub-message, which carries the
	// DirectPath/MediaKey/hashes needed to download it later.
	MediaKind  string
	MimeType   string
	FileName   string
	MediaProto []byte
	// LocalPath is where the media was last saved by a download, or "".
	// It is reused only after the checks in reusableLocalPath.
	LocalPath string
}

// internal contact representation to abstract from message lib
type Chat struct {
	Id          string
	IsGroup     bool
	Name        string
	Unread      int
	LastMessage int64
}

type Contact struct {
	Id    string
	Name  string
	Short string
}

const GROUPSUFFIX = "@g.us"
const CONTACTSUFFIX = "@s.whatsapp.net"
const STATUSSUFFIX = "status@broadcast"

// Conversation represents a lightweight chat metadata for the list view
type Conversation struct {
	JID         string
	Name        string
	LastMsgTime int64
	Preview     string
	Unread      uint16
	IsPinned    bool
	IsArchived  bool
	Index       int // Heap index for internal use
}
