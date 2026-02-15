package messages

import "go.mau.fi/whatsmeow"

// commandHandler is the function signature for all command handlers.
// sm provides access to SessionManager fields and helpers.
// client is the whatsmeow client captured once before dispatch (may be nil if disconnected).
// cmdName is the command name as typed by the user (useful for aliases and usage messages).
// params are the command parameters from the Command struct.
type commandHandler func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)

// commandRegistry maps command names (including aliases) to their handlers.
var commandRegistry map[string]commandHandler

func init() {
	commandRegistry = map[string]commandHandler{
		// Basic commands (cmd_basic.go)
		"help":      cmdHelp,
		"quit":      cmdQuit,
		"colorlist": cmdColorList,
		"more":      cmdMore,
		"info":      cmdInfo,
		"read":      cmdRead,

		// Connection commands (cmd_connection.go)
		"login":      cmdLogin,
		"connect":    cmdLogin, // alias
		"disconnect": cmdDisconnect,
		"logout":     cmdLogout,
		"reset":      cmdReset,

		// Chat commands (cmd_chat.go)
		"send":        cmdSend,
		"select":      cmdSelect,
		"backlog":     cmdBacklog,
		"sync-groups": cmdSyncGroups,

		// Group commands (cmd_group.go)
		"create":      cmdCreate,
		"subject":     cmdSubject,
		"leave":       cmdLeave,
		"add":         cmdAdd,
		"remove":      cmdRemove,
		"admin":       cmdAdmin,
		"promote":     cmdAdmin,       // alias
		"removeadmin": cmdRemoveAdmin,
		"demote":      cmdRemoveAdmin, // alias

		// Media commands (cmd_media.go)
		"upload":    cmdMedia,
		"sendimage": cmdMedia,
		"sendvideo": cmdMedia,
		"sendaudio": cmdMedia,

		// Search commands (cmd_search.go)
		"search":         cmdSearch,
		"search-contact": cmdSearchContact,
	}
}
