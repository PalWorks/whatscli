# Contributing to WhatsCLI

Thanks for helping. Read [AGENTS.md](AGENTS.md) (rules, concurrency, security-sensitive code) and [ARCHITECTURE.md](ARCHITECTURE.md) first; they apply to people and AI agents alike.

_Last updated: 2026-10-31T17:32:09+00:00 (new commands go in commandHints)_

## Licensing of contributions

New Go files start with `// SPDX-License-Identifier: AGPL-3.0-only` followed by a blank line; CI checks this.

WhatsCLI is dual licensed under AGPL-3.0-only and a paid commercial license ([LICENSE-COMMERCIAL.md](LICENSE-COMMERCIAL.md)). By opening a pull request you agree that PalWorks may distribute your contribution under both licenses. Questions: support@palworks.ai.

## Workflow

- **Branches:** `master` is the integration branch; CI runs on pushes and pull requests to it. Work on `feature/<name>`, `fix/<name>` or `chore/<name>` and open a pull request against `PalWorks/whatscli`.
- **Commits:** conventional prefixes (`feat:`, `fix:`, `chore:`, `docs:`, `ci:`, `style:`), one concern per commit.
- **Before opening a pull request:** `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` pass; `gofmt` run on files you touched; docs updated (below).
- **Upstream fixes:** this is an independent fork. Port fixes from `normen/whatscli` selectively and name the upstream commit in your message.
- **Releases** are maintainer-only: a `v*` tag runs `.github/workflows/release.yml` (README, "Deployment and Releases").

## Review checklist

- Concurrency rules respected (`sm.getClient()`, no lock across DB or network, nothing blocks `runManager`).
- Data from senders is sanitised or authorised, with a negative test (SECURITY.md section 6).
- Errors are wrapped and shown through `PrintError`; no `panic`.
- Schema changes use `addColumnIfMissing` and keep old databases working.
- Tests added; they fail without the change.
- No secrets, keys, session files or chat screenshots in the diff.
- README, CHANGELOG (`[Unreleased]`) and ROADMAP updated when the change is more than trivial. Each update carries a UTC timestamp from `date -u --iso-8601=seconds`.

## Adding a slash command

Commands live in `messages/cmd_*.go` and are dispatched through the `commandRegistry` map.

### Step 1: pick a file

**Table CO1: Command files**

| File | Commands |
|---|---|
| `cmd_basic.go` | `/help`, `/quit`, `/colorlist`, `/more`, `/info`, `/read` |
| `cmd_connection.go` | `/login` (`/connect`), `/disconnect`, `/logout`, `/reset` |
| `cmd_chat.go` | `/send`, `/select`, `/backlog`, `/sync-groups` |
| `cmd_group.go` | `/create`, `/subject`, `/leave`, `/add`, `/remove`, `/admin`, `/removeadmin` |
| `cmd_media.go` | `/upload`, `/sendimage`, `/sendvideo`, `/sendaudio` |
| `cmd_message.go` | `/download`, `/open`, `/show`, `/url`, `/revoke` |
| `cmd_search.go` | `/search`, `/search-contact` |

Create a new `cmd_<feature>.go` if none fits.

### Step 2: write the handler

```go
func cmdExample(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
    // client may be nil when disconnected
    if client == nil || !client.IsConnected() {
        sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
        return
    }
    if !checkParam(params, 1) {
        sm.printCommandUsage(cmdName, "[argument]")
        return
    }
    // Validate here (synchronously). Do network work in a goroutine and call
    // sm.getClient() again inside it; never block the event loop. Derive its
    // context from sm.rootContext() so quitting cancels it.
    sm.uiHandler.PrintText("Done")
}
```

Useful fields: `sm.db` (`*MessageDatabase`), `sm.uiHandler` (`UiMessageHandler`), `sm.currentReceiver` (current chat JID, guard with `sm.mu.RLock()`).

### Step 3: register it

Add the name to the map in `init()` in `messages/cmd_registry.go` (aliases are extra entries with the same handler):

```go
"example": cmdExample,
```

### Step 4: help, keys, docs

- Add it to `commandHints` in `ui_complete.go`; the command drop-down and the `F1` help window are built from it.
- For a key binding: add a field to `Keymap` and its default in `config/settings.go`, and bind it in `ui_keybindings.go`.
- Update the README command tables, CHANGELOG, and AGENTS.md if the file map changes.

## Testing

```bash
make test     # go test -race ./...
make vet
```

Tests use in-memory SQLite (`InitWithDB`, never `Init`), `MockUiHandler`, and the seams listed in [TESTING.md](TESTING.md). See `messages/storage_test.go`, `messages/media_safety_test.go` and `messages/cmd_message_test.go` for examples. Anything that talks to WhatsApp is verified with the live-account checklist in TESTING.md, not in unit tests.

## Code style

- Return errors; wrap with `fmt.Errorf("context: %w", err)`; report with `sm.uiHandler.PrintError()`.
- Access the client only through `sm.getClient()`.
- One function per command; `checkParam()` for parameter validation.
- Document exported functions and types; run `gofmt`.
- No emojis or em-dashes in documentation; every table has a numbered label line.
