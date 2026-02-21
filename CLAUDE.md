# CLAUDE.md — Project Instructions for Claude Code

> This file is automatically read by Claude Code when working in this repository.
> It provides project context, conventions, and rules.

## Project

**WhatsCLI** — a terminal-based WhatsApp client written in Go.  
Uses [whatsmeow](https://github.com/tulir/whatsmeow) (multi-device protocol) and [tview](https://github.com/rivo/tview) (TUI framework).

## Build & Test Commands

```bash
make build       # go build -o whatscli
make test        # go test -race ./...
make vet         # go vet ./...
make run         # go run .
make install     # go install .
whatscli --debug # Run with verbose WhatsApp protocol logging
```

Requires **CGO** (SQLite via go-sqlite3). On Linux: `apt install gcc`.

## Architecture

Two packages, cleanly separated:

| Package | Responsibility | Key Files |
|---------|---------------|-----------|
| `main` (root) | TUI layer (tview) | `main.go`, `app_context.go`, `ui_layout.go`, `ui_handler.go`, `ui_keybindings.go`, `ui_render.go`, `ui_helpers.go` |
| `messages/` | Business logic, WhatsApp connection, storage | `session_manager.go`, `connection.go`, `event_handler.go`, `history_sync.go`, `chat_state.go`, `storage.go`, `cmd_*.go` |
| `config/` | INI config with XDG paths | `settings.go` |
| `qrcode/` | ANSI QR code rendering | `qrcode.go` |

**Data flow:** UI → `CommandChannel` → `SessionManager.runManager()` event loop.  
Backend → `UiMessageHandler` interface → TUI updates.

## Critical Concurrency Rules

1. **Never read `sm.client` without a lock** — always use `sm.getClient()` (acquires RLock).
2. **Never hold `sm.mu` while doing DB writes** — acquire lock, mutate PQ/map, release lock, then write to SQLite.
3. **Use `snapshotPQ()`** to create a deep copy of the priority queue before UI updates.
4. **All channels are buffered (size 10)** — `BatteryChannel`, `StatusChannel`, `CommandChannel`, `ChatChannel`, `TextChannel`.
5. **Auto-reconnect** uses exponential backoff (2s → 30s, max 5 attempts). Respects `sm.loggedOut` and `sm.started` flags.

## Command System

Commands are dispatched via `commandRegistry` in `cmd_registry.go`.

**Handler signature:**
```go
func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```

**To add a new command:**
1. Create handler function in appropriate `cmd_*.go` file
2. Register in `commandRegistry` map in `cmd_registry.go`'s `init()`
3. Update help text in `ui_helpers.go` → `PrintHelp()`

## Storage

- **SQLite only** — all persistence through `MessageDatabase` in `storage.go`.
- DB path: `<XDG_CONFIG>/whatscli/session_meta.db` (WAL mode, 5s busy timeout).
- Tables: `conversations` (JID PK) and `messages` (ID PK, indexed on `chat_id`, `chat_id+timestamp`).
- No in-memory caches — every read queries SQLite directly.
- WhatsApp session stored separately in `session.db` (whatsmeow's own sqlstore).

## Testing Conventions

- Use **in-memory SQLite** (`:memory:`) for storage tests via `InitWithDB()`.
- Use `MockUiHandler` from `mock_ui_handler.go` for backend tests.
- All tests run with `-race` flag.
- `commandRegistry` is auto-populated via `init()` — available in all `messages` package tests.
- Table-driven tests preferred; see `storage_test.go` for examples.

## Code Style

- Return errors instead of swallowing — propagate to `sm.uiHandler.PrintError()`.
- Wrap errors with `fmt.Errorf("context: %w", err)`.
- Keep command handlers focused — one function per command.
- Use `checkParam()` for parameter validation.
- Never use `panic()` for normal error handling.
- Never hardcode paths — use `config.GetSessionFilePath()` and XDG.

## Configuration

INI format at `~/.config/whatscli/whatscli.config` with 4 sections:
- `[general]` — download paths, cmd prefix, notifications, show command
- `[keymap]` — all keyboard shortcuts
- `[ui]` — sidebar width
- `[colors]` — full theme customization (14 color keys)

Access via `config.Config.Section.Field` (e.g., `config.Config.General.CmdPrefix`).

## Common Pitfalls

1. `UiMessageHandler` interface has **17 methods** — any mock must implement all of them.
2. `storage.Init()` uses `config.GetSessionFilePath()` — tests must use `:memory:` via `InitWithDB()`.
3. The `qrcode` package renders ANSI art — terminal width matters for QR visibility.
4. `release.sh` updates a separate Homebrew tap repo — don't modify without understanding the full flow.
