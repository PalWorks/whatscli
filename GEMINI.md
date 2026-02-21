# GEMINI.md — Project Instructions for Gemini

> This file provides project context for Google Gemini AI assistants (Gemini Code Assist, Jules, Antigravity, IDX).

## Project Overview

**WhatsCLI** is a terminal-based WhatsApp client written in Go 1.24.  
It uses [whatsmeow](https://github.com/tulir/whatsmeow) for the WhatsApp multi-device protocol and [tview](https://github.com/rivo/tview) for the terminal UI.

**Version:** v1.3.0  
**License:** MIT  
**Module:** `github.com/normen/whatscli`

## Quick Reference

```bash
# Build & Run
make build          # go build -o whatscli
make run            # go run .
whatscli --debug    # verbose protocol logging

# Test & Lint
make test           # go test -race ./...
make vet            # go vet ./...

# Release (maintainers)
make release        # cross-compile + GitHub Release + Homebrew update
```

**CGO required** — SQLite via `go-sqlite3`. Install gcc: `apt install gcc` (Linux).

## Architecture

```
main (TUI)                    messages/ (Business Logic)
┌──────────────────┐          ┌──────────────────────────┐
│ AppContext       │ ◄──────► │ SessionManager           │
│ ui_layout.go     │ Channel  │   connection.go          │
│ ui_handler.go    │ ───────► │   event_handler.go       │
│ ui_keybindings.go│          │   history_sync.go        │
│ ui_render.go     │          │   chat_state.go          │
│ ui_helpers.go    │          │   storage.go (SQLite)    │
└──────────────────┘          │   cmd_registry.go        │
                              │   cmd_basic/chat/media/  │
config/ (INI+XDG)             │     group/search/conn.go │
qrcode/ (ANSI QR)             └──────────────────────────┘
```

**Separation:** The `main` package handles only TUI concerns. `messages/` handles all WhatsApp communication, storage, and command logic. They communicate via Go channels and the `UiMessageHandler` interface.

## Concurrency Rules

These rules are non-negotiable. Violating them causes data races and deadlocks.

| Rule | Details |
|------|---------|
| **Client access** | Always use `sm.getClient()` — never read `sm.client` directly |
| **Lock discipline** | Hold `sm.mu` only for PQ/map mutations. Release before SQLite writes |
| **PQ snapshots** | Use `snapshotPQ()` for safe priority queue reads from the UI goroutine |
| **Channel sizes** | All channels are buffered (10). Don't change without understanding backpressure |
| **Reconnect guard** | `sm.reconnecting` flag prevents duplicate reconnect goroutines |

## Command Registry Pattern

All slash commands are registered in `cmd_registry.go` and dispatched by the `SessionManager` event loop.

**Handler signature:**
```go
func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```

**Adding a new command:**
1. Write handler in `messages/cmd_*.go` (group by concern)
2. Register in `commandRegistry` map in `cmd_registry.go` `init()`
3. Add help entry in `ui_helpers.go` → `PrintHelp()`

**Existing command files:**

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
- **Tables:** `conversations` (JID PK), `messages` (ID PK, indexes on `chat_id` and `chat_id+timestamp`)
- **No caching** — all reads query SQLite directly

## Testing

- Use `InitWithDB(db)` with in-memory SQLite for storage tests
- Use `MockUiHandler` from `mock_ui_handler.go` to stub the 17-method `UiMessageHandler` interface
- Tests always run with `-race` flag
- `commandRegistry` is auto-populated via `init()` — always available in package tests
- CI runs on Go 1.22 and 1.24 (GitHub Actions matrix)

## Configuration

INI file at `~/.config/whatscli/whatscli.config` (XDG standard).  
Access via `config.Config.Section.Field`:

| Section | Keys | Purpose |
|---------|------|---------|
| `[general]` | `download_path`, `cmd_prefix`, `show_command`, `enable_notifications`, etc. | Core settings |
| `[keymap]` | `switch_panels`, `focus_messages`, `command_backlog`, etc. | All key bindings |
| `[ui]` | `chat_sidebar_width` | Layout |
| `[colors]` | `background`, `text`, `list_contact`, `chat_me`, etc. (14 keys) | Full theming |

## Code Conventions

- **Error handling:** Return errors, wrap with `fmt.Errorf("context: %w", err)`, propagate to `sm.uiHandler.PrintError()`
- **No panics:** Never use `panic()` for normal error handling
- **Paths:** Always use `config.GetSessionFilePath()` and XDG — never hardcode
- **Commands:** One function per command, use `checkParam()` for validation
- **Exports:** Document all exported functions and types
