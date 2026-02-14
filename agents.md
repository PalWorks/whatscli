# WhatsCLI — Agent Guide

> This file provides essential context for AI agents working on this project.

## Project Overview

**WhatsCLI** is a terminal-based WhatsApp client written in Go. It uses the [whatsmeow](https://github.com/tulir/whatsmeow) library for WhatsApp Multi-Device protocol and [tview](https://github.com/rivo/tview) for the TUI.

## Architecture

```
whatscli/
├── main.go              # Entry point, grid layout, main()
├── ui_handler.go        # UiHandler struct (implements UiMessageHandler interface)
├── ui_helpers.go        # PrintText, PrintError, PrintHelp, UpdateStatusBar
├── ui_keybindings.go    # LoadShortcuts, keyboard handler functions
├── ui_layout.go         # SetupLeftPane, table setup, selection logic
├── ui_render.go         # RenderChatTable, SetDisplayedChat, message formatting
├── config/
│   └── settings.go      # INI config via XDG paths
├── messages/
│   ├── messages.go      # Data types (Message, Chat, Conversation, Command) + UiMessageHandler interface
│   ├── session_manager.go  # Core backend: auth, connection, event handling, command dispatch
│   ├── storage.go       # SQLite-only persistence (conversations + messages)
│   ├── priority_queue.go   # Heap-based chat ordering (pinned first, then by time)
│   ├── cmd_registry.go  # Central command name → handler map
│   ├── cmd_basic.go     # help, quit, colorlist, more, info, read
│   ├── cmd_connection.go   # login/connect, disconnect, logout, reset
│   ├── cmd_chat.go      # send, select, backlog, sync-groups
│   ├── cmd_group.go     # create, subject, leave, add, remove, admin, removeadmin
│   └── cmd_media.go     # upload, sendimage, sendvideo, sendaudio
└── qrcode/
    └── qrcode.go        # ASCII QR code rendering for login
```

## Key Design Patterns

### Command Registry
Commands are dispatched via `commandRegistry` in `cmd_registry.go`. Each handler has the signature:
```go
func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```
To add a new command: create the handler function, add it to the `commandRegistry` map in `init()`.

### Concurrency Model
- `SessionManager.mu` (sync.RWMutex) protects `client`, `currentReceiver`, `priorityQueue`, and `convByJID`.
- Always use `sm.getClient()` (which uses RLock) to read `sm.client` — never read it bare.
- DB writes should happen **outside** the lock when possible.
- `snapshotPQ()` creates a deep copy of the priority queue for safe UI updates.

### Storage
- **SQLite only** — all persistence goes through `MessageDatabase` in `storage.go`.
- DB path: `<XDG_CONFIG>/whatscli/session_meta.db` (WAL mode, 5s busy timeout).
- Tables: `conversations` (JID primary key) and `messages` (ID primary key, indexed on `chat_id`).
- No in-memory caches — every read queries SQLite directly.

### UI ↔ Backend Communication
- Backend calls `UiMessageHandler` interface methods to update the UI.
- UI sends commands via `SessionManager.CommandChannel` (channel of `Command` structs).
- The `runManager()` goroutine processes commands from the channel.

## Build & Test

```bash
make build       # go build -o whatscli
make test        # go test -race -v ./...
make vet         # go vet ./...
```

Requires **CGO** (for `go-sqlite3`). On Linux: `apt install gcc`.

## Known Stubs / Incomplete Features

| Command | Status |
|---------|--------|
| `read`  | Stub — prints "not implemented yet" |
| `info`  | Stub — prints "not yet implemented" |
| `more`  | Stub — not implemented |
| `backlog` | Partial — uses 3 fallback methods for history sync |

## Recent Refactoring History

The codebase underwent a major refactor (Feb 2026) addressing findings from a code audit:
- Eliminated deadlocks and data races in session manager
- Fixed command injection vulnerability in `PrintImage`
- Replaced `panic()` with error returns in storage init
- Split `main.go` (1159 lines) into 5 focused UI modules
- Replaced 550-line `execCommand` switch with command registry pattern
- Added `convByJID` map for O(1) conversation lookups
- Completely removed legacy Gob persistence
- Enabled SQLite WAL mode

See `ClaudeCodeProjectReview 20260214.md` for the original audit and `WhatsCLI-Remaining-Work-NextSteps.md` for the remaining work plan.

## Common Pitfalls

1. **Never read `sm.client` without a lock** — use `sm.getClient()` instead.
2. **Never do DB writes inside `sm.mu.Lock()`** — hold lock only for PQ/map mutations, then release before writing to SQLite.
3. **`storage.Init()` uses `config.GetSessionFilePath()`** — in tests, you must either mock this or use `:memory:` SQLite.
4. **The `UiMessageHandler` interface has 16 methods** — any mock must implement all of them.
5. **`commandRegistry` is populated in `init()`** — tests in the `messages` package will automatically have it available.
