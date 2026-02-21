# WhatsCLI — Agent Guide

> This file provides essential context for AI agents working on this project.  
> For tool-specific instructions, see also: [CLAUDE.md](CLAUDE.md), [GEMINI.md](GEMINI.md), [SKILLS.md](SKILLS.md).

## Project Overview

**WhatsCLI** is a terminal-based WhatsApp client written in Go 1.24.  
It uses [whatsmeow](https://github.com/tulir/whatsmeow) for the WhatsApp multi-device protocol and [tview](https://github.com/rivo/tview) for the TUI framework.

**Version:** v1.3.0 · **License:** MIT · **Module:** `github.com/normen/whatscli`

## Quick Reference

```bash
make build       # go build -o whatscli
make test        # go test -race ./...
make vet         # go vet ./...
make run         # go run .
whatscli --debug # Verbose WhatsApp protocol logging
```

**CGO required** — SQLite via `go-sqlite3`. Install gcc: `apt install gcc` (Linux).

## Architecture

```
whatscli/
├── main.go              # Entry point, grid layout, main()
├── app_context.go       # AppContext struct — centralized shared UI state
├── ui_layout.go         # SetupLeftPane, table setup, selection logic
├── ui_handler.go        # UiHandler struct (implements UiMessageHandler interface)
├── ui_keybindings.go    # LoadShortcuts, keyboard handler functions
├── ui_render.go         # RenderChatTable, message formatting
├── ui_helpers.go        # PrintText, PrintError, PrintHelp, UpdateStatusBar
├── config/
│   └── settings.go      # INI config via XDG paths (4 sections, 30+ keys)
├── messages/
│   ├── messages.go      # Data types + UiMessageHandler interface (17 methods)
│   ├── session_manager.go  # Event loop, command dispatch, lifecycle
│   ├── connection.go    # WhatsApp login, QR code flow, auto-reconnect
│   ├── event_handler.go # WhatsApp event processing
│   ├── history_sync.go  # History synchronization
│   ├── chat_state.go    # Chat state management
│   ├── storage.go       # MessageDatabase — SQLite persistence
│   ├── priority_queue.go # Heap-based chat ordering
│   ├── cmd_registry.go  # Command name → handler map
│   ├── cmd_basic.go     # /help, /quit, /colorlist, /more, /info, /read
│   ├── cmd_connection.go # /login, /connect, /disconnect, /logout, /reset
│   ├── cmd_chat.go      # /send, /select, /backlog, /sync-groups
│   ├── cmd_group.go     # /create, /subject, /leave, /add, /remove, /admin, /removeadmin
│   ├── cmd_media.go     # /upload, /sendimage, /sendvideo, /sendaudio
│   └── cmd_search.go    # /search, /search-contact
└── qrcode/
    └── qrcode.go        # ANSI QR code rendering for terminal display
```

**Data flow:** UI → `CommandChannel` → `SessionManager.runManager()` event loop. Backend → `UiMessageHandler` interface → TUI updates.

## Concurrency Rules (Non-Negotiable)

| Rule | Details |
|------|---------|
| **Client access** | Always use `sm.getClient()` — never read `sm.client` directly |
| **Lock discipline** | Hold `sm.mu` only for PQ/map mutations. Release before SQLite writes |
| **PQ snapshots** | Use `snapshotPQ()` for safe priority queue reads from UI goroutine |
| **Channel sizes** | All channels buffered (10). Don't change without understanding backpressure |
| **Reconnect guard** | `sm.reconnecting` flag prevents duplicate reconnect goroutines |

## Command Registry

All slash commands registered in `cmd_registry.go`, dispatched by `SessionManager` event loop.

**Handler signature:**
```go
func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```

**Adding a new command:**
1. Write handler in `messages/cmd_*.go` (group by concern)
2. Register in `commandRegistry` map in `cmd_registry.go` `init()`
3. Add help entry in `ui_helpers.go` → `PrintHelp()`

**Command files:**

| File | Commands |
|------|----------|
| `cmd_basic.go` | `/help`, `/quit`, `/colorlist`, `/more`, `/info`, `/read` |
| `cmd_connection.go` | `/connect`, `/disconnect`, `/logout`, `/reset` |
| `cmd_chat.go` | `/send`, `/select`, `/backlog`, `/sync-groups` |
| `cmd_group.go` | `/create`, `/subject`, `/leave`, `/add`, `/remove`, `/admin`, `/removeadmin` |
| `cmd_media.go` | `/upload`, `/sendimage`, `/sendvideo`, `/sendaudio` |
| `cmd_search.go` | `/search`, `/search-contact` |

## Storage Layer

- **Engine:** SQLite 3 with WAL mode and 5s busy timeout
- **App DB:** `<XDG_CONFIG>/whatscli/session_meta.db` — messages and conversations
- **Session DB:** `<XDG_CONFIG>/whatscli/session.db` — whatsmeow device store
- **Tables:** `conversations` (JID PK), `messages` (ID PK, indexed on `chat_id` and `chat_id+timestamp`)
- **No caching** — all reads query SQLite directly

## Testing

- Use `InitWithDB(db)` with in-memory SQLite for storage tests
- Use `MockUiHandler` from `mock_ui_handler.go` — 17 methods
- Tests always run with `-race` flag
- `commandRegistry` auto-populated via `init()` — available in package tests
- CI matrix: Go 1.22 + 1.24 on GitHub Actions

## Configuration

INI file at `~/.config/whatscli/whatscli.config` (XDG standard).

| Section | Purpose |
|---------|---------|
| `[general]` | Download paths, cmd prefix, notifications, show command |
| `[keymap]` | All keyboard shortcuts (16 bindings) |
| `[ui]` | Sidebar width |
| `[colors]` | Full theme (14 color keys) |

Access: `config.Config.Section.Field` (e.g., `config.Config.General.CmdPrefix`).

## Code Conventions

- **Error handling:** Return errors, wrap with `fmt.Errorf("context: %w", err)`, propagate to `sm.uiHandler.PrintError()`
- **No panics:** Never `panic()` for normal error handling
- **Paths:** Always use `config.GetSessionFilePath()` and XDG — never hardcode
- **Commands:** One function per command, use `checkParam()` for validation
- **Exports:** Document all exported functions and types
- **Testing:** Table-driven tests, in-memory SQLite, race detector, MockUiHandler

## Common Pitfalls

1. **Never read `sm.client` without a lock** — use `sm.getClient()`.
2. **Never do DB writes inside `sm.mu.Lock()`** — hold lock only for PQ/map mutations.
3. **`storage.Init()` uses `config.GetSessionFilePath()`** — tests must use `:memory:` via `InitWithDB()`.
4. **`UiMessageHandler` interface has 17 methods** — any mock must implement all of them.
5. **`commandRegistry` is populated in `init()`** — always available in `messages` package tests.

## Related Documentation

| File | Purpose |
|------|---------|
| [README.md](README.md) | User-facing project documentation |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contributor guide with command authoring walkthrough |
| [CHANGELOG.md](CHANGELOG.md) | Version history |
| [SKILLS.md](SKILLS.md) | Deep-dive into development patterns and conventions |
| [CLAUDE.md](CLAUDE.md) | Claude Code-specific project instructions |
| [GEMINI.md](GEMINI.md) | Gemini AI-specific project instructions |
| [.github/copilot-instructions.md](.github/copilot-instructions.md) | GitHub Copilot instructions |
| [.cursorrules](.cursorrules) | Cursor AI configuration |
