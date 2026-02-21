## Project

WhatsCLI is a terminal-based WhatsApp client in Go 1.24 using whatsmeow (multi-device protocol) and tview (TUI).

## Build & Test

- `make build` — compile binary
- `make test` — run tests with race detection (`go test -race ./...`)
- `make vet` — static analysis
- Requires CGO for SQLite (`apt install gcc` on Linux)

## Architecture

Two packages separated by concern:

- `main` (root) — TUI layer: `main.go`, `app_context.go`, `ui_layout.go`, `ui_handler.go`, `ui_keybindings.go`, `ui_render.go`, `ui_helpers.go`
- `messages/` — Business logic: `session_manager.go`, `connection.go`, `event_handler.go`, `history_sync.go`, `chat_state.go`, `storage.go`, `cmd_registry.go`, `cmd_*.go`
- `config/` — INI configuration with XDG paths
- `qrcode/` — ANSI QR code rendering

Communication: UI sends commands via `SessionManager.CommandChannel`; backend calls back through `UiMessageHandler` interface (17 methods).

## Command Pattern

Commands are registered in `cmd_registry.go` using handler signature:
```go
func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```

To add a command: create handler in `cmd_*.go`, register in `init()`, update `PrintHelp()` in `ui_helpers.go`.

## Concurrency Rules

- Always use `sm.getClient()` — never read `sm.client` directly
- Hold `sm.mu` only for PQ/map mutations, release before SQLite writes
- Use `snapshotPQ()` for safe priority queue reads from the UI goroutine
- All channels buffered with size 10

## Storage

SQLite with WAL mode. Tables: `conversations` (JID PK), `messages` (ID PK, indexed on `chat_id` and `chat_id+timestamp`). No in-memory caches.

## Testing

Use `InitWithDB(db)` with in-memory SQLite. Use `MockUiHandler` for backend tests. Always run with `-race`. CI runs Go 1.22 and 1.24 matrix.

## Code Style

- Return and wrap errors: `fmt.Errorf("context: %w", err)`
- Never `panic()` for error handling
- One function per command, use `checkParam()` for validation
- Use XDG paths via `config.GetSessionFilePath()`
- Document all exported functions and types
