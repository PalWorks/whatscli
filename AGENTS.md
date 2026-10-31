# AGENTS.md: WhatsCLI agent contract

> Canonical instructions for every AI agent working in this repository (Claude Code, Gemini, Cursor, Copilot, Codex). `CLAUDE.md`, `GEMINI.md` and `.cursorrules` point here. Change rules here only.
>
> _Last updated: 2026-10-31T16:18:40+00:00 (ui_complete.go, version.go, command hint rule)_

## 1. Project

**WhatsCLI** is a terminal WhatsApp client in Go, built on [whatsmeow](https://github.com/tulir/whatsmeow) (multi-device protocol) and [tview](https://github.com/rivo/tview) (TUI). This repository (`github.com/PalWorks/whatscli`) is an independent fork of `normen/whatscli`. Never merge upstream wholesale; port fixes selectively (see DECISIONS.md D1).

- Module path: `github.com/PalWorks/whatscli`
- Go directive `1.26.0`, `toolchain go1.27.3` (whatsmeow requires 1.26.0). An older system Go auto-downloads the toolchain.
- CGO is required (SQLite via `go-sqlite3`). Linux: `apt install gcc`.
- Version string: `make build`, `make install` and the release workflow set `VERSION` in `main.go` with `-X main.VERSION=...`. When it is empty, `appVersion()` (`version.go`) uses the module version recorded by `go install`, else `dev`.
- Licence: dual, AGPL-3.0-only (`LICENSE`) plus paid commercial (`LICENSE-COMMERCIAL.md`). Every Go file starts with an SPDX line (`qrcode/qrcode.go` stays BSD-3-Clause); `.github/scripts/check-spdx.sh` enforces it in CI.

## 2. Where to read what

**Table A1: Document map**

| Need | Read |
|---|---|
| What the product is, install, usage | README.md |
| Components, data flow, storage schema, JID handling | ARCHITECTURE.md |
| Why something was chosen | DECISIONS.md (maintainers only, gitignored; absent in public clones) |
| What is open, what is next | ROADMAP.md |
| What shipped | CHANGELOG.md |
| How to test, live-account checklist | TESTING.md |
| Threats, secrets, how to report | SECURITY.md |
| How to contribute, add a command | CONTRIBUTING.md |
| Development patterns with code | SKILLS.md |
| Old audits and reviews (history only) | docs/archive/ |

## 3. Commands

```bash
make build       # go build with version injected, output ./whatscli
make test        # go test -race ./...
make vet         # go vet ./...
make lint        # alias of vet
make run         # go run .
make install     # go install with version injected
make release     # prints how to tag; releases are built by CI
whatscli --debug # verbose WhatsApp protocol logging
```

`go build ./...`, `go vet ./...` and `go test -race -count=1 ./...` must pass before any change is called done.

## 4. Roles, allowed and restricted actions

**Table A2: Agent permissions**

| Area | Allowed | Restricted |
|---|---|---|
| `messages/`, root `*.go`, `config/`, `qrcode/` | Edit, add tests, refactor within the rules below | Breaking the concurrency rules (section 6) |
| `.github/workflows/`, `.github/scripts/`, `Makefile`, `go.mod` | Change only when the task is about release, CI or dependencies | Adding secrets, publish steps or tokens without owner approval |
| Release | Prepare notes and checks | Never tag, push or publish without being asked |
| `~/.config/whatscli/` (real session and databases) | Nothing | Never run the app against a real account or touch these files in automated work; live checks are done by the owner (TESTING.md) |
| Docs | Keep current in the same change | Inventing features or facts; unlabelled guesses |

Do not commit or push unless asked. Do not add secrets, keys or tokens anywhere (SECURITY.md).

## 5. Architecture in brief

Two main packages plus two helpers. Full detail in ARCHITECTURE.md.

```
main (root)   TUI: main.go, app_context.go, ui_layout.go, ui_handler.go,
              ui_keybindings.go, ui_render.go, ui_helpers.go, ui_opener.go,
              ui_complete.go (command drop-down and echo), version.go
messages/     business logic, WhatsApp connection, SQLite, commands
config/       INI config with XDG paths (settings.go)
qrcode/       ANSI QR rendering (qrcode.go)
```

**Data flow:** UI sends `messages.Command` on `CommandChannel`; `SessionManager.runManager()` dispatches through `commandRegistry`. The backend calls back through the `UiMessageHandler` interface (18 methods, implemented by `UiHandler` in `ui_handler.go` and by `MockUiHandler` in `messages/mock_ui_handler.go`).

**Files in `messages/`:**

**Table A3: messages package files**

| File | Role |
|---|---|
| `messages.go` | Data types and the `UiMessageHandler` interface |
| `session_manager.go` | `SessionManager`, event loop, lifecycle, root context (`rootContext()`, cancelled first in `Shutdown`), part-file sweep at `Init`, test seams |
| `connection.go` | Login, QR flow, auto-reconnect, `getClient()` |
| `event_handler.go` | whatsmeow events: messages, connect, disconnect, logout, delete for me, history sync, incoming revokes |
| `history_sync.go` | History sync storage, ON_DEMAND refresh, `@lid` handling, contact list import (`importContacts`) |
| `chat_state.go` | Current chat, unread, `snapshotPQ()`, `sendText` |
| `storage.go` | `MessageDatabase` (SQLite), migrations, `StoreResult`, `RevokedText` |
| `priority_queue.go` | Heap ordering of conversations |
| `media.go` | Media metadata extraction and storage, file name sanitisation, open allowlist, `firstURL` |
| `jid_norm.go` | One canonical chat JID per person (`@lid` to phone number) |
| `lid_merge.go` | Merges chats stored under `@lid` into the phone-number chat once the mapping is known |
| `chat_view.go` | Paging of the open chat: `viewAnchor` (under `sm.mu`), page sizes, anchor lowered when older messages arrive |
| `delete_for_me.go` | "Delete for me" from your other devices (`events.DeleteForMe`): removes the stored row and moves the chat preview |
| `partfiles.go` | Startup sweep of leftover `.whatscli-<digits>.part` download temp files |
| `revoke_incoming.go` | Authorisation of incoming revokes |
| `pathcheck.go` | `ResolveFileWithin`: symlink-aware directory containment |
| `cmd_registry.go` | Command name to handler map |
| `cmd_basic.go` | `/help /quit /colorlist /more /info /read` |
| `cmd_connection.go` | `/login /connect /disconnect /logout /reset` |
| `cmd_chat.go` | `/send /select /backlog /sync-groups` |
| `cmd_group.go` | `/create /subject /leave /add /remove /admin /promote /removeadmin /demote` |
| `cmd_media.go` | `/upload /sendimage /sendvideo /sendaudio` |
| `cmd_message.go` | `/download /open /show /url /revoke` (take a message ID) |
| `cmd_search.go` | `/search /search-contact` |
| `mock_ui_handler.go` | Mock UI for tests |

## 6. Concurrency rules (non-negotiable)

1. Never read `sm.client` directly. Use `sm.getClient()` (takes the RLock). Goroutines started by a command must call `getClient()` again, because the client passed to the handler can be stale.
2. Never hold `sm.mu` during a SQLite write or a network call. Lock, mutate the queue or map, unlock, then write.
3. Use `snapshotPQ()` for a deep copy of the priority queue before UI updates.
4. All channels are buffered (size 10): `BatteryChannel`, `StatusChannel`, `CommandChannel`, `ChatChannel`, `TextChannel`. Do not change sizes without understanding back-pressure.
5. Never block `runManager`. Validation runs on the event loop; network and waits (downloads, revoke send, backlog 5 s poll, group admin lookup, `/info` group lookup) run in goroutines.
6. Auto-reconnect uses exponential backoff (2 s to 30 s, 5 attempts) and respects `sm.loggedOut`, `sm.started` and `sm.reconnecting`.
7. `UiHandler` methods queue work with `go ctx.App.QueueUpdateDraw`, so ordering between two calls is not guaranteed. Print status that follows a redraw with `NewScreenWithStatus` (one queued update).
8. Work that outlives a command (downloads, backlog and its poll, own revokes, the incoming-revoke admin check, `/info` group lookup) derives its context from `sm.rootContext()`, which `Shutdown` cancels first.
9. `sm.viewAnchor` (oldest timestamp shown in the open chat, `chat_view.go`) is guarded by `sm.mu` like `currentReceiver`.

## 7. Commands

Dispatch is through `commandRegistry` in `messages/cmd_registry.go`, filled in `init()`.

```go
func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```

To add a command:

1. Write the handler in the right `messages/cmd_*.go` file; one function per command; use `checkParam()` and `sm.printCommandUsage()`.
2. Register it in the map in `cmd_registry.go` `init()`.
3. Add it to `PrintHelp()` in `ui_helpers.go` and to `commandHints` in `ui_complete.go` (`TestCommandHintsCoverRegistry` fails otherwise).
4. If it needs a key, add a `[keymap]` field in `config/settings.go` and bind it in `ui_keybindings.go`.
5. Add tests, update README command table, CHANGELOG and this file.

Message commands (`download`, `open`, `show`, `url`, `revoke`) take a message ID as the only parameter; the UI sends the highlighted message ID. The `info` key sends `info` without an ID and prints chat or group info.

## 8. Storage

- SQLite only, all through `MessageDatabase` in `storage.go`. WAL mode, 5 s busy timeout. No in-memory caches; reads query SQLite.
- App DB: `<XDG_CONFIG>/whatscli/session_meta.db` (tables `conversations`, `messages`). WhatsApp session: `session.db` (whatsmeow's own store). Never hardcode paths; use `config.GetSessionFilePath()`.
- Schema changes use `addColumnIfMissing` (idempotent `ALTER TABLE ADD COLUMN`). Never drop or rewrite columns that older binaries read.
- `AddMessage` inserts with `INSERT OR IGNORE` (first write wins for text, sender and time), then fills empty media columns of a known row in the same chat unless it is revoked, and returns a `StoreResult` so callers redraw only on change. `messages.local_path` records where media was saved.
- Tests use `:memory:` through `InitWithDB()`; `Init()` touches the real config path.

## 9. Security-sensitive code (read SECURITY.md before changing)

- Incoming revokes: `revoke_incoming.go`. Never apply a revoke without the chat and sender checks. `MarkMessageRevoked` is scoped by `chat_id` and must stay so.
- Opening files: `open` passes only allowlisted extensions to the system opener (`safeToOpen` in `media.go`). `/url` passes only http and https URLs with a host. The opener (`ui_opener.go`) runs without a shell and is never waited on by the caller.
- File names from senders go through `sanitizeFileName` and `safeJoin`; writes go to a temp file then rename. `show` uses `ResolveFileWithin`; a repeat download reuses `messages.local_path` only if `ResolveFileWithin` places it directly in the target folder as a regular, non-symlink file.
- The startup sweep (`partfiles.go`) removes only regular files named exactly `.whatscli-<digits>.part`, directly in the download and preview folders.
- No secrets, tokens or keys in code, docs, tests, logs or screenshots.

## 10. Code conventions

- Return errors; wrap with `fmt.Errorf("context: %w", err)`; report to the user with `sm.uiHandler.PrintError()`.
- No `panic()` for normal errors. Recover only around third-party calls that panic (clipboard).
- Document exported functions and types. Run `gofmt` on files you touch.
- Table-driven tests preferred. Use `MockUiHandler`, in-memory SQLite, and seams (`mediaDownloader`, `lidMapperOverride`, `groupInfoOverride`, `webMessageParser`) instead of a live client.
- Config access: `config.Config.Section.Field`.
- Conventional commit prefixes (`feat:`, `fix:`, `chore:`, `docs:`, `ci:`, `style:`).

## 11. Common pitfalls

1. Reading `sm.client` without the lock.
2. DB writes while holding `sm.mu`.
3. Using `storage.Init()` in tests (it opens the real config path).
4. `UiMessageHandler` has 18 methods; any new mock must implement all of them.
5. A chat can arrive as `@lid` or `@s.whatsapp.net`. Store under the canonical JID (`canonicalChatJID`), compare people with `sameUser`, never with string equality.
6. whatsmeow `BuildHistorySyncRequest` dereferences its anchor; never pass nil.
7. `go test` without `-race` hides data races; CI uses `-race`.
8. Pre-upgrade media rows have empty media columns and stay not downloadable (decision D13).

## 12. Documentation rules (owner standard)

- Every doc update carries a timestamp to the second in ISO 8601 in UTC, from `date -u --iso-8601=seconds` (public repository).
- Every table has a visible numbered label line above it.
- No emojis, no em-dashes or double dashes as punctuation.
- Every problem raised carries options, trade-offs and a recommendation. End reports with the decision needed or the next action.
- Keep README, CHANGELOG, ROADMAP, DECISIONS and this file current in the same change as the code.
- Label unverified claims `[Unverified]` or `[Inference]`.
