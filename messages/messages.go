// this package manages the messages
package messages

type UiMessageHandler interface {
	NewMessage(Message)
	NewScreen([]Message)
	UpdateChatList([]*Conversation)
	PrintError(error)
	PrintText(string)
	PrintFile(string)
	PrintQR(string)
	SetStatus(SessionStatus)
	OpenFile(string)
	ShowColorList()
	Clear()
	UpdateQR(qr string, attempt int, timeout int)
	PrintCommands()
	PrintHelp()
	Quit()
}

// data struct for current session status
type SessionStatus struct {
	Connected bool
	LastSeen  string
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

	// Media fields, populated for messages that carry an attachment.
	// MediaType is empty for plain text messages.
	MediaType     string // "", "image", "video", "audio", "document", "sticker"
	Mimetype      string
	MediaKey      []byte
	DirectPath    string
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
	FileName      string // documents only

	// Deleted marks a message that was revoked (locally or by the sender);
	// Text is set to a placeholder when this is true.
	Deleted bool

	// QuotedID is the message ID this message replies to, or "" if it isn't
	// a reply. QuotedPreview is a short display string for that quoted
	// message, resolved at receive/send time (from the local DB if the
	// quoted message is known, or from the embedded quote otherwise) so
	// rendering never needs a live lookup.
	QuotedID      string
	QuotedPreview string

	// Reactions is populated by the storage layer at read time (a join
	// against the reactions table, keyed by message ID) — it is not a column
	// on the messages table and is never set by AddMessage.
	Reactions []ReactionSummary
}

// ReactionSummary aggregates all reactions of one emoji on a message, e.g.
// "👍" with Count 2 for two people reacting with a thumbs-up.
type ReactionSummary struct {
	Emoji string
	Count int
}

// internal contact representation to abstract from message lib
type Chat struct {
	Id      string
	IsGroup bool
	Name    string
	Unread  int
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
