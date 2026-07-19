package messages

import "testing"

// commandTestCoverage tracks, for every name in commandRegistry, whether it
// has direct test coverage elsewhere in this package. A command is only
// allowed a `false` entry with an explicit reason in the comment next to
// it — bare untracked commands are what TestAllCommandsHaveTestCoverage
// below exists to catch.
//
// When adding a new command: register it in cmd_registry.go, write its
// test, then add an entry here with `true`. If it genuinely cannot be
// tested hermetically (e.g. it always makes a real network call with no
// guardable early-exit, or it touches paths that can't be sandboxed — see
// cmd_connection_test.go's notes on cmdLogin and cmdReset), add `false` and
// explain why in a comment, matching the existing exemptions.
var commandTestCoverage = map[string]bool{
	"help":      true, // TestExecCommand_DispatchesToRegistry (session_manager_test.go)
	"quit":      true, // TestCmdQuit
	"colorlist": true, // TestCmdColorList
	"more":      true, // TestCmdMore_* (cmd_basic_test.go)
	"info":      true, // TestCmdInfo_*
	"read":      true, // TestCmdRead_*

	"login":   false, // no guardable early exit; sm.login() always attempts a real network connection/QR pairing — see cmd_connection_test.go
	"connect": false, // alias of "login", same exemption
	"reset":   false, // deletes real files via config.GetSessionFilePath(); the adrg/xdg package resolves XDG_CONFIG_HOME in its own init(), so t.Setenv cannot sandbox it — see cmd_connection_test.go

	"disconnect": true, // TestCmdDisconnect_NoClient
	"logout":     true, // TestCmdLogout_AlreadyLoggedOut

	"send":        true, // TestCmdSend_*
	"select":      true, // TestCmdSelect_*
	"backlog":     true, // TestCmdBacklog_*
	"sync-groups": true, // TestCmdSyncGroups_NilClient

	"create":      true, // TestCmdCreate_*
	"subject":     true, // TestCmdSubject_*
	"leave":       true, // TestCmdLeave_NilClient
	"add":         true, // TestCmdAdd_*
	"remove":      true, // TestCmdRemove_*
	"admin":       true, // TestCmdAdmin_*
	"promote":     true, // alias of "admin"; TestGroupCommandAliases_Registered
	"removeadmin": true, // TestCmdRemoveAdmin_*
	"demote":      true, // alias of "removeadmin"; TestGroupCommandAliases_Registered

	"upload":      true, // TestCmdMedia_* (cmd_media_test.go) — cmdMedia dispatches on cmdName
	"sendimage":   true, // TestCmdMedia_* / TestMediaTypeMapping
	"sendvideo":   true, // TestMediaTypeMapping
	"sendaudio":   true, // TestMediaTypeMapping
	"sendsticker": true, // TestCmdMedia_Sticker_*/TestIsWebpFile/TestMediaTypeMapping

	"search":         true, // TestCmdSearch_*
	"search-contact": true, // TestCmdSearchContact_*

	"download": true, // TestCmdDownload_*
	"open":     true, // TestCmdOpen_*
	"show":     true, // TestCmdShow_*
	"url":      true, // TestCmdURL_*
	"revoke":   true, // TestCmdRevoke_* (event_handler_test.go covers the storage-level handleRevoke; cmd_message_test.go covers the /revoke command's guards)
	"reply":    true, // TestCmdReply_* (cmd_message_test.go); TestHandleReaction_*/TestResolveQuote_* (event_handler_test.go) cover the underlying quote-resolution machinery
	"edit":     true, // TestCmdEdit_* (cmd_message_test.go)

	"record":        true, // TestCmdRecord_*/TestRunRecorder_* (cmd_record_test.go)
	"stoprecording": true, // TestCmdStopRecording_* (cmd_record_test.go)

	"export": true, // TestCmdExport_*/TestExportMessagesEncrypted_* (cmd_export_test.go)
}

// TestAllCommandsHaveTestCoverage keeps commandTestCoverage in sync with
// commandRegistry in both directions: a command registered but missing from
// the manifest fails immediately (catches "landed untested"); an entry in
// the manifest for a command no longer registered also fails (catches
// stale bookkeeping). Commands with a `false` entry are reported as
// intentionally exempt, not silently skipped.
func TestAllCommandsHaveTestCoverage(t *testing.T) {
	var untested []string

	for name := range commandRegistry {
		covered, known := commandTestCoverage[name]
		if !known {
			t.Errorf("command %q is registered in commandRegistry but has no entry in commandTestCoverage — add one (true + a test, or false + a reason)", name)
			continue
		}
		if !covered {
			untested = append(untested, name)
		}
	}

	for name := range commandTestCoverage {
		if _, ok := commandRegistry[name]; !ok {
			t.Errorf("commandTestCoverage has a stale entry for %q, which is no longer in commandRegistry", name)
		}
	}

	if len(untested) > 0 {
		t.Logf("commands with an explicit test exemption (see comments in commandTestCoverage): %v", untested)
	}
}
