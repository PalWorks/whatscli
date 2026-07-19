# WhatsCLI Implementation Plan (Phases 1 to 5)

**Generated:** 2026-07-18 07:22 IST
**Companion document:** ProjectStatusReport_20260718.md (same date; finding numbers referenced below are from that report)
**Baseline:** commit 1f0c6f1, master, all tests passing with race detector

This plan turns the 5 roadmap phases into concrete, ordered work items with files, function level changes, test requirements, and acceptance criteria. Effort estimates are [Inference]. Any whatsmeow API name marked "verify" must be checked against the upgraded library during item 1.1, since the upgrade lands before everything else.

## Ground Rules (apply to every item)

- One branch per phase (`phase-1-truth-safety`, `phase-2-media`, ...), one commit per item, PR against master.
- Gate for every commit: `make vet && make test` clean, plus the manual smoke test listed in the item.
- Respect existing concurrency rules: client access via `sm.getClient()`, no DB writes under `sm.mu`, `snapshotPQ()` before UI list updates.
- New commands follow the registry pattern: handler in the matching `cmd_*.go`, registration in `cmd_registry.go` `init()`, help entry in `ui_helpers.go` `PrintHelp()`.
- Schema changes use the additive `ALTER TABLE` pattern already established for `is_archived` (storage.go:48), so old databases upgrade silently and downgrade safely.

---

## Phase 1: Truth and Safety

Goal: the app stops advertising features it does not have, dependencies are current, and known small defects are closed. No new features.

### 1.1 Upgrade whatsmeow and remaining dependencies

The riskiest item, done first and alone in its own commit.

Steps:
1. `go get go.mau.fi/whatsmeow@latest && go mod tidy`, then `go build ./...` and walk compile errors. Expected churn areas after 5 months: context parameters on store/contact calls, proto package moves (`waE2E` vs `binary/proto`), event struct fields. The codebase already imports both `go.mau.fi/whatsmeow/binary/proto` (session_manager.go, cmd_media.go, chat_state.go) and `go.mau.fi/whatsmeow/proto/waE2E` (event_handler.go); unify on `waE2E` if the upgrade deprecates the alias.
2. `go get github.com/gen2brain/beeep@latest`. The 2021 snapshot's `beeep.Notify(title, msg, icon)` signature changed in v0.11 (verify); adjust `notify()` in session_manager.go:196.
3. `go get github.com/adrg/xdg@latest gopkg.in/ini.v1@latest github.com/mattn/go-sqlite3@latest github.com/gdamore/tcell/v2@latest && go mod tidy`. xdg v0.5 may change `xdg.ConfigHome` semantics used in config/settings.go (verify paths resolve identically; a changed config path would strand user sessions).
4. Full suite: `make vet && make test`.
5. Manual smoke test (needs a real account): `make run`, verify in order: QR login, session restore on second launch, receive text, send text, `/backlog`, `/sync-groups`, `/search`, sendimage, auto reconnect (toggle network), `/logout`.

Acceptance: all tests pass, smoke checklist passes, `go list -m -u all` shows no stale direct dependencies.

Risk and rollback: if whatsmeow HEAD breaks against current WhatsApp servers, pin to the newest tag that passes the smoke test and record the blocking issue in CHANGELOG.md. Keep this commit isolated so `git revert` restores the working baseline.

### 1.2 Quarantine the five dead keybindings honestly

Until Phase 2 lands, `download`, `open`, `show`, `url`, `revoke` must not fail as "Unknown command".

Steps:
1. Add a stub handler in cmd_basic.go:
   `func cmdNotImplemented(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)` printing: `command '<name>' is not implemented in this build yet (planned: see ImplementationPlan Phase 2)`.
2. Register all five names to it in cmd_registry.go.
3. In `PrintHelp()` (ui_helpers.go:53-58), tag the five entries with `(soon)`.
4. Test: table driven test in a new `cmd_basic_test.go` asserting each of the five names resolves in `commandRegistry` and prints the stub message via `MockUiHandler`.

Acceptance: pressing d/o/s/u/r on a selected message shows the honest stub text, never "Unknown command".

### 1.3 Correct README

Update the Features table and Command Reference to current reality: media send yes, media download/open/show/url/revoke pending (link the plan), battery indicator removed (after 1.4). Remove or annotate the README Roadmap entries that Phase 2/3 will supersede.

Acceptance: every command named in README exists in `commandRegistry`.

### 1.4 Delete dead and vestigial code

Steps, all mechanical:
1. Remove `ChatChannel`, `TextChannel` fields and their `make()` calls (session_manager.go:29-30, 69-70).
2. Remove `loadRecentMessages` (history_sync.go:320-335).
3. Remove `Contact` struct and `Chat.LastMessage`... caution: `Chat.LastMessage` is set in ui_layout.go's three `openFrom*` closures; delete those assignments too. `Chat` itself stays (used by `SetDisplayedChat`).
4. Remove `SetChats` from the `UiMessageHandler` interface, `UiHandler`, and `MockUiHandler`; interface drops to 16 methods. Update the "17 methods" notes in CLAUDE.md and CONTRIBUTING.md.
5. Battery: remove `BatteryChannel`, `BatteryMsg`, the battery case in `runManager`, battery fields in `SessionStatus`, and the battery segment of `UpdateStatusBar` (ui_helpers.go:100-114). The status bar becomes `online/offline` plus `LastSeen`.
6. Remove the empty `if statusMsg.err != nil {}` branch (session_manager.go:123) by logging the error: `sm.uiHandler.PrintError(statusMsg.err)` guarded to avoid spam, or drop the `err` field from `StatusMsg` entirely if no sender ever sets it to non nil (current senders: all pass nil except connection.go:173). Simplest correct move: keep the field, print it when non nil.
7. `make vet && make test`; update any test referencing removed symbols (mock_ui_handler.go).

Acceptance: `grep -rn "BatteryChannel\|TextChannel\|ChatChannel\|loadRecentMessages\|SetChats"` returns nothing outside CHANGELOG/history docs.

### 1.5 Path prefix and small fixes

1. ui_render.go:253: replace the two `strings.HasPrefix` checks with a helper:
   ```go
   func isWithin(dir, path string) bool {
       rel, err := filepath.Rel(dir, path)
       return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
   }
   ```
   Unit test in ui_render_test.go with the `DownloadsEvil` case.
2. cmd_media.go:31: add an explicit `else { sm.uiHandler.PrintError(errors.New("not connected")) }` for the nil client path (also covered by existing cmd_media_test.go pattern; extend it).
3. `ShowColorList` (ui_handler.go:104): sort color names before printing and wrap in `QueueUpdateDraw` like every other method.

Acceptance: new unit tests pass; `/colorlist` output is stable across invocations.

### 1.6 Honor existing config keys

1. `/more` and the backlog request count use `config.Config.General.BacklogMsgQuantity` instead of hardcoded 50 (cmd_basic.go:148, cmd_chat.go:95).
2. `notify()` respects `NotificationTimeout`: keep a `lastNotification time.Time` per chat (map guarded by `sm.mu`, or a single global timestamp to match the config's original intent; check upstream normen/whatscli behavior for the key's meaning and document it in README).
3. Replace the hardcoded 30 second freshness window (event_handler.go:255) with the same config value.

Acceptance: setting `backlog_msg_quantity = 5` visibly loads 5 messages with `/more`.

Phase 1 exit criteria: all six items merged, CHANGELOG.md updated, version bumped to v1.3.1 (no new features, so patch bump).

---

## Phase 2: Restore the Media Pipeline

Goal: d/o/s/u/r work end to end. Design principle: persist exactly the fields `client.Download` needs, not the whole proto.

### 2.1 Schema and struct migration

1. Extend `Message` (messages/messages.go):
   ```go
   MediaType     string // "", "image", "video", "audio", "document", "sticker"
   Mimetype      string
   MediaKey      []byte
   DirectPath    string
   FileSHA256    []byte
   FileEncSHA256 []byte
   FileLength    uint64
   FileName      string // documents only
   ```
2. `InitWithDB` gains additive ALTERs following the is_archived pattern, one per column: `media_type TEXT DEFAULT ''`, `mimetype TEXT DEFAULT ''`, `media_key BLOB`, `direct_path TEXT DEFAULT ''`, `file_sha256 BLOB`, `file_enc_sha256 BLOB`, `file_length INTEGER DEFAULT 0`, `file_name TEXT DEFAULT ''`.
3. Update `AddMessage`, `collectMessages`, and the three query column lists in storage.go. Consider a `const messageColumns = "id, chat_id, ..."` string to stop repeating the list four times.
4. Tests (storage_test.go): round trip a media message including BLOB fields; verify a pre migration database (create the old schema in memory, then call `InitWithDB` again) upgrades and still reads legacy rows with empty media fields.

Acceptance: `make test` passes; opening an existing production `session_meta.db` upgrades in place without data loss (manual check on a copy).

### 2.2 Capture media fields on receive and history sync

1. New pure function in event_handler.go:
   `func extractMediaInfo(msg *waE2E.Message) (mediaInfo, bool)` where `mediaInfo` carries the eight fields above. Sources: `ImageMessage`, `VideoMessage`, `AudioMessage`, `DocumentMessage`, `StickerMessage`; each provides `GetMediaKey()`, `GetDirectPath()`, `GetFileSHA256()`, `GetFileEncSHA256()`, `GetFileLength()`, `GetMimetype()` (verify getters post upgrade).
2. Call it from `processIncomingMessage` and from `processHistorySync`'s message loop; fill the new `Message` fields.
3. Tests: extend msg_handler_test.go's proto builders with media key bytes and assert they land in the stored message.

Acceptance: receiving an image then inspecting the DB shows populated media_key/direct_path.

### 2.3 Implement `/download` and `/open`

New file `messages/cmd_message.go` (message scoped commands take a message ID as params[0], which is exactly what `handleMessageCommand` already sends):

1. `func getMessageByID(sm *SessionManager, id string) (*Message, error)`: new storage method `GetMessageByID(id string) (Message, error)` (indexed primary key lookup).
2. `cmdDownload`: guard client non nil and connected; fetch message; error if `MediaType == ""` ("selected message has no attachment"); call whatsmeow download. Preferred API so we avoid re parsing protos: `client.DownloadMediaWithPath(ctx, directPath, fileEncSHA256, fileSHA256, mediaKey, fileLength, mediaType, mmsType)` (verify exact signature and the `MediaType` to MMS type mapping post upgrade). Write to `config.Config.General.DownloadPath` with a name derived from FileName or `<chat>_<id><ext>` (ext from mimetype via `mime.ExtensionsByType`). Never overwrite: append a numeric suffix. Run in a goroutine (network I/O must not block the command loop); report via `PrintText` on success (`Saved: <path>`) and `PrintError` on failure.
3. `cmdOpen`: reuse cmdDownload's core (extract `downloadMessage(sm, msg) (path string, err error)`), then `sm.uiHandler.OpenFile(path)`.
4. Register `download` and `open`, replacing the 1.2 stubs. Update help text (drop `(soon)`).
5. Tests: nil client, no chat selected, message without media, unknown message ID (all against mock + in memory DB, no network). The actual download path is exercised only in manual smoke testing.

Acceptance: `d` on a received image saves the file; `o` opens it with the platform opener.

### 2.4 Implement `/show` and `/url`

1. `cmdShow`: `downloadMessage` (or reuse the already downloaded file if present: check expected path first) into `PreviewPath`, then `sm.uiHandler.PrintFile(path)` which already routes to `PrintImage` and the configured `show_command`. Restrict to `MediaType == "image"` (and sticker if desired).
2. `cmdURL`: no download needed. Fetch message text, extract the first URL with a conservative regexp (`https?://\S+`), strip trailing punctuation, and call `sm.uiHandler.OpenFile(url)` (open-golang handles URLs; verify on Linux it uses xdg-open). If no URL found, print "no URL in selected message".
3. Tests: URL extraction table test (plain URL, URL mid sentence, trailing paren, no URL); show command rejection for non image types.

Acceptance: `s` renders the image inline via jp2a/pixterm; `u` opens a link from a message in the browser.

### 2.5 Implement `/revoke`

1. `cmdRevoke` in cmd_message.go: fetch message; only allow `FromMe` messages (server rejects revoking others' messages outside group admin cases; keep v1 simple); build and send the revoke: `client.SendMessage(ctx, chatJID, client.BuildRevoke(chatJID, types.EmptyJID, msg.Id))` (verify `BuildRevoke` signature post upgrade).
2. On success, mark locally: new storage method `MarkMessageDeleted(id string)` setting `text = '[DELETED]'` plus a `deleted BOOLEAN DEFAULT 0` column (add in the 2.1 migration batch to avoid a second migration), then refresh the screen if the chat is current.
3. Tests: guard paths (not FromMe, no selection, nil client); MarkMessageDeleted round trip.

Acceptance: `r` on an own message revokes it on the phone and shows `[DELETED]` locally.

Phase 2 exit criteria: all five message keys functional, README updated, version bump to v1.4.0.

---

## Phase 3: Live Correctness

Goal: local state converges with what the phone shows.

### 3.1 Clear unread when chats are read elsewhere

Note the correct event: reading a chat on the phone arrives as an app state event `*events.MarkChatAsRead` (verify name post upgrade), not `events.Receipt` (receipts signal the counterpart read YOUR sent messages). Handle both:

1. `case *events.MarkChatAsRead:` in `eventHandler.Handle`: look up `v.JID.String()` in `convByJID` under lock, zero `Unread`, snapshot, unlock, `UpsertConversation` outside lock, `UpdateChatList`. This mirrors the tail of `markChatAsRead` (chat_state.go:79-93); extract that tail into `sm.clearUnread(jidStr string)` and call it from both places.
2. `case *events.Receipt:` with `v.Type == types.ReceiptTypeRead`: optional v1 scope, record nothing yet (message level read markers need a per message delivery state column; defer to Phase 5 or a dedicated item). Leave a typed TODO.
3. Test: unit test `clearUnread` against mock + in memory DB.

Acceptance: reading a chat on the phone clears its unread badge in a running WhatsCLI within seconds.

### 3.2 Handle incoming revokes and protocol messages

1. In `extractMessageContent` the revoke arrives as `msg.GetProtocolMessage()` with `GetType() == waE2E.ProtocolMessage_REVOKE` and `GetKey().GetID()` naming the target (verify). Currently it falls through and returns empty text (dropped).
2. Add a branch in `handleMessage` before `processIncomingMessage`: on revoke, call `MarkMessageDeleted(targetID)` (from 2.5), refresh screen if current chat, and return.
3. Optionally handle `ProtocolMessage_MESSAGE_EDIT` the same way: update the stored row's text with the edited content and an `(edited)` suffix.
4. Tests: proto builder for a revoke message; assert DB row flips to deleted.

Acceptance: deleting a message on the phone marks it `[DELETED]` in WhatsCLI history.

### 3.3 Wire the Forwarded flag

1. New pure helper `extractForwarded(msg *waE2E.Message) bool` reading `GetContextInfo().GetIsForwarded()` from whichever sub message is present (ExtendedText, Image, Video, Audio, Document); keep `extractMessageContent`'s signature untouched so its 18 tests stay valid.
2. Set `msg.Forwarded` in `processIncomingMessage` and the history sync loop. The render path (ui_render.go:38-40, `forwarded_text` color) already works.
3. Test: table test for the helper.

Acceptance: a forwarded message renders in the configured forwarded color.

### 3.4 Deterministic backlog completion (replaces the 5s sleep)

1. Add to SessionManager: `historySyncDone chan struct{}` (buffered 1, created in `Init`).
2. At the end of `processHistorySync`, non blocking send: `select { case sm.historySyncDone <- struct{}{}: default: }`.
3. In `cmdBacklog`'s goroutine, replace `time.Sleep(5s)` with `select { case <-sm.historySyncDone: case <-time.After(30 * time.Second): case <-sm.stop: return }`, then the existing recount and refresh logic. Drain the channel before sending the request to avoid consuming a stale signal.
4. Test: simulated `processHistorySync` completion unblocks a waiting goroutine (can be tested without network by calling `processHistorySync` with an empty proto... note it returns early on no conversations, so signal before that return too, or test the channel mechanics directly).

Acceptance: `/backlog` reports results as soon as the sync arrives, not on a fixed timer; still resolves within 30s when nothing arrives.

Phase 3 exit criteria: version bump to v1.5.0; CHANGELOG documents behavior changes.

---

## Phase 4: Testability and Observability

### 4.1 Command handler test suite

One test file per untested cmd file, all table driven against `newTestSessionManager` + `MockUiHandler` + in memory SQLite, no network:

- `cmd_basic_test.go`: cmdMore (empty chat, pagination happy path via seeded rows, ordering), cmdRead guards, cmdInfo guards (nil client, no receiver), cmdHelp/cmdQuit/cmdColorList dispatch.
- `cmd_search_test.go`: cmdSearch (hit, miss, LIKE metacharacters `%`/`_`, scoping to current chat), cmdSearchContact guards.
- `cmd_chat_test.go`: cmdSend param validation, cmdSelect, cmdBacklog guards (no receiver, nil client).
- `cmd_group_test.go`: every handler's invalid JID and missing param paths (the API calling paths need a client and are covered by guards only).
- `cmd_connection_test.go`: cmdReset clears PQ/convByJID and recreates the DB (use `InitWithDB`, assert conversations table empty afterwards)... note cmdReset calls `sm.db.Init()` which opens the file path; refactor cmdReset to call a `sm.reinitDB()` seam that tests can override, or extract the in memory state reset into its own testable method.

Target: every command name in `commandRegistry` has at least one direct test. Add a meta test asserting exactly that, so future commands cannot land untested:
```go
func TestAllCommandsHaveTests(...) // walks commandRegistry against a manifest map
```

### 4.2 File based debug log

1. New `--logfile <path>` flag (default `<XDG_STATE or config dir>/whatscli/debug.log` when `--debug` is set).
2. In `getConnection`, when debug is on, use `waLog.Stdout` replaced by a logger writing to the file (whatsmeow's `waLog` interface has a simple 4 method surface; implement `fileLogger` in a new `messages/logging.go`).
3. Route `PrintError` backend calls through the same logger (UI display unchanged), giving post mortem data for connection failures.
4. Rotate crudely: truncate at start of each run.

Acceptance: `whatscli --debug` produces a readable protocol log file; normal runs create no file.

### 4.3 Remove the `ctx` global from the UI layer (structural)

Two sub steps, kept separate:

1. Ordering fix (small, immediate value): give `UiHandler` a single `updates chan func()` consumed by one goroutine that calls `ctx.App.QueueUpdateDraw(f)` sequentially. All 16 interface methods enqueue instead of spawning goroutines. Fixes the out of order rendering noted in finding 3.5. Bounded channel (size 64) with drop oldest or block policy... block is safe because the consumer never takes locks.
2. Dependency injection (larger): convert `UiHandler` from empty struct to `type UiHandler struct { ctx *AppContext }`, constructed in main.go; move `PrintText`/`PrintError`/`PrintHelp`/`UpdateStatusBar`/`RenderChatTable` from free functions to methods on a `UI` struct owning the widgets. `ctx` shrinks to construction time wiring. Do it file by file (ui_helpers, ui_render, ui_keybindings, ui_layout); the compiler drives the refactor.
3. New tests: UiHandler ordering test (enqueue N updates, assert FIFO), and previously impossible unit tests for `RenderChatTable` selection restoration using tview's offscreen `SimulationScreen` (tcell provides one; verify import path `tcell.NewSimulationScreen`).

Acceptance: no package level `ctx` reads outside main.go wiring; race detector clean; UI output ordering deterministic.

Phase 4 exit criteria: version v1.6.0. Coverage report (`go test -cover`) recorded in CHANGELOG for the messages package (expect a jump from command handler coverage).

---

## Phase 5: Feature Growth

Less prescriptive; each feature gets its own mini design at pickup time. Ordering and v1 scope:

Table G: Phase 5 Feature Scopes

| # | Feature | v1 scope | Key touch points |
|---|---------|----------|------------------|
| 5.1 | Reply threading display | Render `> quoted snippet` line above messages whose ContextInfo has QuotedMessage; no reply sending yet | extractMessageContent sibling helper; Message gains QuotedID/QuotedPreview columns |
| 5.2 | Reactions attached to messages | Aggregate `[REACTION]` rows onto target message as `👍 x2` suffix instead of standalone rows | reactions table (message_id, sender, emoji) + render join; stop storing reactions as messages |
| 5.3 | Send replies | `/reply <text>` on selected message builds ContextInfo | cmd_message.go; needs 5.1's stored quoted metadata |
| 5.4 | Message edit send/receive | Receive side from 3.2's edit handling; send via `/edit <text>` on own message | whatsmeow BuildEdit (verify) |
| 5.5 | Sticker sending | `/sendsticker <path.webp>` reusing cmdMedia plumbing with StickerMessage | cmd_media.go |
| 5.6 | Status/story viewing | List status@broadcast conversations already skipped today; read only view | history_sync.go JID filter, ui_layout Status pane |
| 5.7 | Voice note recording | Shell out to configurable `record_command` (sox/arecord), send as PTT audio | config key + cmd_media.go |
| 5.8 | Encrypted chat export | `/export <chat> <path>` writing age or GPG encrypted JSON | new cmd_export.go; decide crypto dependency carefully |

Each feature: registry + help + tests per Ground Rules, one minor version bump per released batch.

---

## Sequencing Summary and Checkpoints

Table H: Milestone Checkpoints

| Milestone | Contents | Verify by |
|-----------|----------|-----------|
| v1.3.1 | Phase 1 complete | Smoke checklist (1.1) plus honest help screen |
| v1.4.0 | Phase 2 complete | d/o/s/u/r end to end on a real account |
| v1.5.0 | Phase 3 complete | Phone side read/delete reflected live |
| v1.6.0 | Phase 4 complete | Coverage jump, ordered UI, no ctx global |
| v1.7.x | Phase 5 batches | Per feature acceptance rows above |

Total estimated effort [Inference]: Phase 1 roughly 1 to 3 days (dominated by the whatsmeow upgrade), Phase 2 roughly 2 days, Phase 3 roughly 1 day, Phase 4 roughly 2 to 3 days, Phase 5 open ended.

Standing risks:
- whatsmeow API drift can invalidate any "verify" marker above; resolve them during 1.1 and annotate this document with actual signatures.
- Manual smoke tests require a live WhatsApp account; there is no protocol level integration test. Keep a dedicated test account, never the primary one, since third party clients carry a nonzero ban risk (README caveat already says so).
- Schema migrations are additive only; never rename or drop columns on databases users already have.
