# WhatsCLI Project Status Report and Improvement Roadmap

**Generated:** 2026-07-18 07:15 IST
**Analyzer:** Claude Code (Fable 5)
**Scope:** Full repository read (all 31 Go source files, prior audit documents, CI config, dependency graph)
**Baseline commit:** 1f0c6f1 (master, clean working tree, last commit 2026-02-21)

---

## 1. Executive Summary

WhatsCLI is in good structural health. The seven phase refactor completed in February 2026 (SQLite persistence, command registry, AppContext, auto reconnect, CI) landed cleanly, and every Must Fix and Should Fix finding from the 2026-02-15 audit is verifiably implemented in the current code. The concurrency rules documented in CLAUDE.md are followed consistently across the backend.

The most significant issues found in this review are functional, not structural:

1. Five advertised features (message download, open, show, URL open, revoke) are bound to keys and listed in the help text but have no command handlers. They fail with "Unknown command".
2. Media metadata (media key, direct path, mimetype) is never persisted, so attachment download cannot be implemented from stored history without schema changes.
3. Read receipts from other devices are not handled, so unread counts only clear when chats are read inside WhatsCLI.
4. whatsmeow is 5 months stale (Feb 11 pin vs Jul 16 upstream). For a reverse engineered protocol library this is the single largest operational risk.

**Health assessment:** Green on architecture, concurrency, and storage. Amber on feature completeness and dependency freshness.

---

## 2. Verified Current State

### 2.1 Architecture (confirmed accurate against code)

Data flows exactly as documented in CLAUDE.md:

UI (`main` package) sends `Command` structs through `CommandChannel` to `SessionManager.runManager()` (messages/session_manager.go:86), which dispatches through `commandRegistry` (messages/cmd_registry.go). Backend pushes updates to the UI through the 17 method `UiMessageHandler` interface (messages/messages.go:4). Every UI mutation is wrapped in `go ctx.App.QueueUpdateDraw(...)` (ui_handler.go). Persistence is SQLite only, through `MessageDatabase` (messages/storage.go), WAL mode, two tables (`conversations`, `messages`) with a composite `(chat_id, timestamp)` index.

Concurrency discipline is consistently applied: `sm.getClient()` for client reads, copy under lock then DB write outside lock (event_handler.go:211-244, chat_state.go:79-91, history_sync.go, cmd_chat.go:153-190), `snapshotPQ()` before every UI list update, reconnect guarded by `loggedOut`/`started`/`reconnecting` flags with a post backoff recheck.

### 2.2 Prior audit closure

All 19 findings from audit-report-2026-02-15.md are implemented in the current tree. Spot verified examples:

Table A: Audit Closure Verification

| Finding | Fix location | Status |
|---------|--------------|--------|
| P-1 backlog blocked command loop | goroutine in cmd_chat.go:111 | Closed |
| S-1 LIKE metacharacter escape | `escapeLike()` storage.go:74 | Closed |
| P-4 composite index | storage.go:63 | Closed |
| R-4 reset left `_meta.db` | cmd_connection.go:60-76 | Closed |
| R-5 reconnect/shutdown race | recheck in connection.go:253-262 | Closed |
| CQ-1 silent JID parse errors | error returns throughout cmd_group.go | Closed |
| CQ-4 hardcoded MIME | `detectMIME()` cmd_media.go:136 | Closed |
| A2/A3 God file split | event_handler.go, connection.go, chat_state.go, history_sync.go | Closed |
| T-2 debug logging | `--debug` flag, main.go:19, connection.go:29-32 | Closed |
| S-2 symlink escape in PrintImage | `EvalSymlinks` ui_render.go:246 | Closed |

The one item from ClaudeCodeProjectReview_20260221.md still open by design is the singleton `ctx *AppContext` global (app_context.go:38), which keeps the UI layer untestable beyond pure functions.

### 2.3 Build and test status

`go vet ./...` and `go test -race -count=1 ./...` both pass (verified 2026-07-18 07:19 IST, see Appendix). 66 test functions exist across 7 test files. CI (.github/workflows/ci.yml) runs vet, build, and race tests on Go 1.22 and 1.24 for every push and PR to master.

**[Unverified]** CI badge state on GitHub was not checked from this environment.

### 2.4 Dependency freshness (verified via `go list -m -u`, 2026-07-18)

Table B: Dependency Staleness

| Module | Pinned | Latest available | Risk |
|--------|--------|------------------|------|
| go.mau.fi/whatsmeow | v0.0.0-20260211... | v0.0.0-20260716... | High: protocol drift, WhatsApp server changes |
| github.com/mattn/go-sqlite3 | v1.14.33 | v1.14.48 | Low |
| github.com/adrg/xdg | v0.3.3 | v0.5.3 | Low |
| gopkg.in/ini.v1 | v1.62.0 | v1.67.3 | Low |
| github.com/gen2brain/beeep | 2021 snapshot | v0.11.2 | Medium: notification breakage on modern desktops |
| github.com/gdamore/tcell/v2 | v2.13.8 | v2.13.10 | Low |
| github.com/rivo/tview | v0.42.0 | current | None |

---

## 3. New Findings (this review, 2026-07-18)

All findings below were verified by reading the current source unless labeled otherwise.

### 3.1 Broken advertised features: missing command handlers (High)

ui_keybindings.go:319-341 binds keys d/o/s/u/r to `handleMessageCommand("download")`, `"open"`, `"show"`, `"url"`, `"revoke"`, and ui_helpers.go:53-58 advertises them in the help screen. None of these names exist in `commandRegistry` (cmd_registry.go:16-58). Pressing any of them on a selected message prints "Unknown command". The README feature table ("Download, open, and preview attachments") is therefore inaccurate for the current build.

### 3.2 Media metadata not persisted (High, blocks 3.1)

`Message` (messages.go:52) and the `messages` table store only text. `extractMessageContent` (event_handler.go:59) reduces media messages to placeholder strings like `[IMAGE] caption`. MediaKey, DirectPath, FileSHA256, and mimetype are discarded, so `client.Download` can never be called for a stored message. Implementing download requires a schema migration plus retention of the raw proto or its media fields.

### 3.3 Receipt events not handled (Medium)

`eventHandler.Handle` (event_handler.go:22-54) processes Message, Connected, Disconnected, LoggedOut, and HistorySync only. `events.Receipt` is ignored, so reading a chat on the phone never clears the unread badge in WhatsCLI. Similarly, revoke and edit events are not processed, so deleted messages remain in local history without any marker.

### 3.4 Dead or vestigial code (Low)

- `BatteryChannel` is created but nothing ever sends to it; the multi device protocol does not deliver battery events. The status bar (ui_helpers.go:100-114) permanently renders "0% l s".
- `ChatChannel` and `TextChannel` (session_manager.go:29-30, 69-70) are created and never read or written.
- `SetChats` is a documented no-op (ui_handler.go:68) yet remains in the 17 method interface.
- `loadRecentMessages` (history_sync.go:321) is never called; it also reads `sm.currentReceiver` without the mutex at line 331, which would be a race if it were ever used.
- `Message.Forwarded` and `Conversation` color `forwarded_text` exist, but `Forwarded` is never set anywhere, so forwarded highlighting can never trigger.
- `Chat.LastMessage` and the `Contact` struct are unused by the backend.

### 3.5 Minor correctness and polish (Low)

- session_manager.go:123 has an empty `if statusMsg.err != nil {}` branch; the error in `StatusMsg` is silently discarded.
- ui_render.go:253 uses `strings.HasPrefix(resolvedPath, downloadDir)` without a trailing path separator, so `/home/x/DownloadsEvil` passes the `/home/x/Downloads` check. Low risk for a local TUI, but a one line fix (`filepath.Rel` or appending `string(os.PathSeparator)`).
- Every `UiMessageHandler` method spawns its own goroutine before `QueueUpdateDraw`, so consecutive updates can render out of order (two rapid `PrintText` calls may swap). Cosmetic, but explains occasional interleaved output.
- `ShowColorList` (ui_handler.go:104) iterates a map, so color order changes every invocation; it also bypasses `QueueUpdateDraw`.
- `cmdMedia` silently does nothing when `client` is nil (cmd_media.go:31); every other handler prints "not connected".
- `notify()` ignores `NotificationTimeout` config; the 30 second freshness window in event_handler.go:255 is hardcoded.
- `cmdBacklog`'s fixed 5 second wait then recheck (cmd_chat.go:111-133) is a heuristic; a HistorySync driven completion signal would be deterministic.
- `config.Config.General.BacklogMsgQuantity` exists but both `/more` and `/backlog` hardcode 50.

### 3.6 Test coverage gaps (unchanged since audit T-1, Medium)

Handlers in cmd_basic.go, cmd_chat.go, cmd_group.go, cmd_search.go, and cmd_connection.go have zero direct tests. Reconnect logic and `processHistorySync` are untested. The UI layer is untestable due to the `ctx` global (only the pure functions in ui_render.go are covered).

---

## 4. Improvement Roadmap

Ordered by recommended execution. Effort estimates are [Inference] based on code size and patterns already present in the repo.

### Phase 1: Truth and safety (small, do first)

Table C: Phase 1 Items

| # | Item | Effort | Notes |
|---|------|--------|-------|
| 1.1 | Upgrade whatsmeow to current and rebuild; smoke test login, send, receive, history sync | 0.5 to 2 days | API churn likely after 5 months; do this before anything else |
| 1.2 | Remove or hide the 5 dead keybindings and help entries (download/open/show/url/revoke), or print "not yet implemented" | 30 min | Stops advertising broken features |
| 1.3 | Fix README feature table to match reality | 15 min | |
| 1.4 | Delete dead code: ChatChannel, TextChannel, loadRecentMessages, Contact, Chat.LastMessage; drop battery segment from status bar or the whole BatteryChannel path | 1 hr | Shrinks UiMessageHandler surface too (SetChats) |
| 1.5 | Fix HasPrefix path check with separator; fix empty statusMsg.err branch | 30 min | |
| 1.6 | Update remaining outdated deps (beeep, xdg, ini, go-sqlite3, tcell) | 1 hr | beeep API changed since 2021 snapshot |

### Phase 2: Restore media pipeline (the headline feature gap)

Table D: Phase 2 Items

| # | Item | Effort | Notes |
|---|------|--------|-------|
| 2.1 | Schema migration: add media columns (media_type, media_key, direct_path, file_sha256, file_enc_sha256, file_length, mimetype) to `messages` | 2 hrs | Follow the existing `ALTER TABLE` pattern used for is_archived |
| 2.2 | Persist media fields in event_handler and history_sync | 2 hrs | |
| 2.3 | Implement `/download` and `/open` via `client.Download` into DownloadPath | 3 hrs | Registry pattern makes this mechanical |
| 2.4 | Implement `/show` (feed downloaded image to ShowCommand) and `/url` (extract first URL from message text, open via open-golang) | 2 hrs | PrintImage and OpenFile plumbing already exists |
| 2.5 | Implement `/revoke` via `client.RevokeMessage` | 1 hr | |

### Phase 3: Live correctness

Table E: Phase 3 Items

| # | Item | Effort | Notes |
|---|------|--------|-------|
| 3.1 | Handle `events.Receipt`: clear unread counts when chats are read elsewhere | 2 hrs | |
| 3.2 | Handle revoke/protocol messages: mark stored message as deleted | 2 hrs | |
| 3.3 | Set `Forwarded` from `ContextInfo.IsForwarded` so the existing color path works | 1 hr | |
| 3.4 | Honor BacklogMsgQuantity and NotificationTimeout config keys | 1 hr | |
| 3.5 | Replace cmdBacklog's 5s sleep with completion signaling from processHistorySync | 2 hrs | |

### Phase 4: Testability and observability

Table F: Phase 4 Items

| # | Item | Effort | Notes |
|---|------|--------|-------|
| 4.1 | Table driven tests for cmd_basic, cmd_search, cmd_chat, cmd_group (mock + in-memory DB, per audit T-1) | 3 to 4 hrs | Infrastructure (newTestSessionManager, MockUiHandler) already exists |
| 4.2 | File based debug log (in addition to `--debug` stdout) for post-mortem diagnosis | 2 hrs | |
| 4.3 | Refactor UiHandler to hold explicit references instead of the `ctx` global; serialize UI updates through one channel to fix ordering | 1 to 2 days | Largest structural item; unlocks UI testing |

### Phase 5: Feature growth (from README roadmap, prioritized)

[Inference] Priority order based on user value versus effort:

1. Reply threading display (quoted context is already in ExtendedTextMessage proto)
2. Reaction display attached to target messages instead of standalone `[REACTION]` rows
3. Message editing support (send and render edits)
4. Sticker sending
5. Status/story viewing
6. Voice note recording
7. Encrypted chat export

---

## 5. Mental Model Reference (for future sessions)

- One writer event loop (`runManager`) owns command dispatch; everything long running is spawned as a goroutine by the handler itself.
- Two SQLite files: `session.db` is whatsmeow's own store (never touch directly), `session_meta.db` is the app's (via MessageDatabase only).
- Chat ordering lives in three places that must stay in sync: `priorityQueue` + `convByJID` (in memory, under `sm.mu`), `conversations` table (persistence), `ctx.AllChats` (UI copy, sorted independently in UpdateChatList).
- The heap ordering is actually cosmetic for display: UpdateChatList re-sorts the snapshot with the same rules, so the PQ mainly provides O(log n) maintenance and the JID map O(1) lookup.
- UI selection restoration during list re-render is guarded by `ctx.RenderingList`; removing that guard reintroduces ghost chat switching.

---

*Report produced by repository analysis on 2026-07-18.*

## Appendix: Verification Run (completed 2026-07-18 07:19 IST)

```
$ go vet ./...
VET EXIT: 0

$ go test -race -count=1 ./...
ok    github.com/normen/whatscli           2.465s
ok    github.com/normen/whatscli/config    1.554s
ok    github.com/normen/whatscli/messages  1.095s
?     github.com/normen/whatscli/qrcode    [no test files]
TEST EXIT: 0
```

Both clean. The suite ran with the race detector against the dependency versions pinned in go.mod.
