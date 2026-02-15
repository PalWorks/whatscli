# Contributing to WhatsCLI

Thanks for your interest in contributing! This guide covers the architecture and how to add new features.

## Adding a New Slash Command

WhatsCLI uses a **command registry pattern**. Each command is a function registered in an `init()` block.

### Step 1: Create or Choose a File

Commands are grouped by concern in `messages/cmd_*.go` files:

| File | Commands |
|------|----------|
| `cmd_basic.go` | `/select`, `/read`, `/info`, `/more` |
| `cmd_chat.go` | `/backlog` |
| `cmd_search.go` | `/search` |
| `cmd_media.go` | `/sendimage`, `/sendvideo`, `/sendaudio`, `/upload` |
| `cmd_group.go` | `/create`, `/add`, `/remove`, `/sync-groups` |

For a new command that doesn't fit these, create a new `cmd_yourfeature.go` file.

### Step 2: Write the Handler Function

Every command handler has the same signature:

```go
func cmdExample(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
    // client may be nil if disconnected — check before using
    if client == nil {
        sm.uiHandler.PrintError(fmt.Errorf("not connected"))
        return
    }

    // Validate parameters
    if !checkParam(sm, cmdName, params, 1) {
        return // checkParam prints usage for you
    }

    // Access the database
    msgs, err := sm.db.GetMessages(sm.currentChat, 50)
    if err != nil {
        sm.uiHandler.PrintError(err)
        return
    }

    // Send output to the UI
    sm.uiHandler.PrintText("Done!")
}
```

**Key resources available via `sm`:**

| Field | Type | Purpose |
|-------|------|---------|
| `sm.db` | `*MessageDatabase` | SQLite read/write |
| `sm.uiHandler` | `UiMessageHandler` | Print to UI |
| `sm.currentChat` | `string` | Currently selected chat JID |
| `sm.CommandChannel` | `chan Command` | Send commands to event loop |

### Step 3: Register the Command

Add an `init()` block in your file:

```go
func init() {
    registerCommand("example", cmdExample)

    // For aliases, register the same handler under multiple names:
    registerCommand("ex", cmdExample)
}
```

That's it — the command is now callable as `/example` from the UI.

### Step 4: Add to Help Text

Update the `PrintCommands()` or `PrintHelp()` functions in `ui_helpers.go` to document your new command.

## Testing

```bash
make test    # go test -v ./...
make vet     # go vet ./...
go build ./...
```

Tests use in-memory SQLite (`:memory:`) and `MockUiHandler` from `mock_ui_handler.go`. See `storage_test.go` and `cmd_media_test.go` for examples.

## Code Style

- Return errors instead of swallowing them — propagate to `sm.uiHandler.PrintError()`
- Access `sm.client` only via `sm.getClient()` (thread-safe)
- Keep command handlers focused — one function per command
- Use `checkParam()` for parameter validation with automatic usage printing
