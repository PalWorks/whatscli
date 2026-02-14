# WhatsCLI - Remaining Work & Next Steps

## Completed So Far (12 commits on master)

| # | Commit | Description |
|---|--------|-------------|
| 1 | `94071c8` | Fix deadlock and data race in login/connection flow |
| 2 | `0533ff3` | Fix command injection vulnerability in PrintImage |
| 3 | `227ac8f` | Eliminate data races on sm.client reads, correct media upload types |
| 4 | `7e8b8e6` | Replace panics with error returns, add SQLite WAL mode, fix QR race |
| 5 | `24fe406` | Replace 550-line execCommand switch with command registry pattern |
| 6 | `c7d2ddc` | Remove unused globals (chatHeader, groupHeader, sndTxt) |
| 7 | `1ccd880` | Split main.go God file (1191 lines) into 5 focused UI modules |
| 8 | `b94b230` | Add convByJID map for O(1) lookups + snapshotPQ helper |
| 9 | `4699f9c` | Remove legacy Gob persistence, SQLite-only storage |
| 10 | `3467422` | Batch lock operations, move DB writes outside locks |
| 11 | `67156a9` | Add test and vet Makefile targets |

**Net impact:** ~2585 lines added, ~2272 lines removed across the refactoring sessions.

---

## Remaining Work

### 1. Test Coverage (Priority: HIGH)

Currently only `messages/priority_queue_test.go` exists (71 lines). The codebase has zero test coverage for core functionality.

#### 1a. Storage Tests (`messages/storage_test.go`)
- Test `Init()` creates tables correctly (use in-memory SQLite: `":memory:"`)
- Test `AddMessage()` / `AddMessageToDB()` — insert and verify
- Test `GetMessages()` — insert multiple messages, verify ordering by timestamp
- Test `UpsertConversation()` — insert new, then update existing, verify both
- Test `GetConversations()` — insert several, verify all returned
- Test edge cases: empty DB, duplicate message IDs (INSERT OR IGNORE), special characters in text

#### 1b. Command Handler Tests (`messages/cmd_*_test.go`)
- Create a mock `UiMessageHandler` that captures calls
- Test `cmdSend` — verify it sends to CommandChannel with correct params
- Test `cmdSelect` — verify it calls `setCurrentReceiver`
- Test `cmdSyncGroups` — mock `client.GetJoinedGroups()`, verify PQ and DB updates
- Test `cmdMedia` — verify correct MediaType selection per command name
- Test `cmdCreate`, `cmdAdd`, `cmdRemove` — verify group operations

#### 1c. Session Manager Tests (`messages/session_manager_test.go`)
- Test `Init()` — verify PQ and convByJID are populated from DB
- Test `snapshotPQ()` — verify deep copy (modify original, verify copy unchanged)
- Test `execCommand()` — verify registry dispatch
- Test `setCurrentReceiver()` — verify state update
- Test command routing — unknown command prints error

#### 1d. Config Tests (`config/settings_test.go`)
- Test `InitConfig()` with temporary config file
- Test default values when config file doesn't exist
- Test parsing of custom values

**Estimated effort:** 2-3 sessions, ~500-800 lines of test code

---

### 2. CI/CD Pipeline (Priority: HIGH)

No `.github/workflows/` exists. Need automated testing on PRs.

#### Deliverables:
- `.github/workflows/ci.yml` — Run on push/PR to master:
  - `go build ./...`
  - `go vet ./...`
  - `go test -race ./...`
  - Matrix: Go 1.24.x on ubuntu-latest
- Optional: Add `golangci-lint` for static analysis
- Optional: Code coverage reporting with `go test -coverprofile`

**Estimated effort:** 1 session, ~50 lines

---

### 3. Encapsulate Global Variables into AppContext (Priority: MEDIUM)

`main.go` still has 12+ package-level globals that prevent testability:

```
currentReceiver, curRegions, textView, leftPane, chatTable,
groupTable, statusTable, textInput, topBar, app, mouseState,
sessionManager, keyBindings, uiHandler, allChats, chatLimit
```

#### Approach:
- Create an `AppContext` struct holding all UI state
- Pass `*AppContext` to `SetupLeftPane()`, `LoadShortcuts()`, `EnterCommand()`, all `handle*` functions
- Update `UiHandler` to hold a reference to `AppContext`
- This enables unit testing of UI logic without a running TUI

#### Files affected:
- `main.go` — define struct, pass to functions
- `ui_layout.go` — accept `*AppContext` parameter
- `ui_keybindings.go` — accept `*AppContext` parameter (25+ handler functions)
- `ui_handler.go` — `UiHandler` holds `*AppContext`
- `ui_helpers.go` — accept `*AppContext` or access via receiver
- `ui_render.go` — accept `*AppContext` parameter

**Estimated effort:** 1-2 sessions, significant but mechanical refactor

---

### 4. Improve Message Handling (Priority: MEDIUM)

The `handleMessage()` function in `session_manager.go` only handles text and image messages. Other message types are silently dropped.

#### Missing message types:
- Video messages (`GetVideoMessage()`)
- Audio messages (`GetAudioMessage()`)
- Document messages (`GetDocumentMessage()`)
- Sticker messages (`GetStickerMessage()`)
- Contact messages
- Location messages
- Reaction messages
- Reply/quoted messages (show "Re: ..." context)

#### Approach:
- Add cases to `handleMessage()` for each type
- Display type-specific previews: `[VIDEO]`, `[AUDIO]`, `[DOCUMENT: filename]`, etc.
- Each case follows the same pattern: create `Message`, call `AddMessage`, call `updatePQ`, update UI

**Estimated effort:** 1 session, ~150-200 lines

---

### 5. Implement Missing Commands (Priority: MEDIUM)

Several commands are stubs or not yet implemented:

| Command | Current State | What's Needed |
|---------|--------------|---------------|
| `read` | Stub: "not implemented yet" | Call `client.MarkRead()` with message IDs |
| `info` | Stub: "not yet implemented" | Query message receipt status from whatsmeow |
| `more` | Stub: "not implemented yet" | Load older messages (pagination) |
| `backlog` | Partially works | Uses 3 different history sync approaches, needs cleanup |

**Estimated effort:** 1-2 sessions depending on whatsmeow API complexity

---

### 6. Error Handling Improvements (Priority: LOW)

#### 6a. Graceful SQLite error handling
- `storage.go` methods like `AddMessage()` currently print to stdout on error
- Should propagate errors to UI via `PrintError()` instead of `fmt.Printf`
- Add `Close()` method to `MessageDatabase` for clean shutdown

#### 6b. Connection retry logic
- Currently no automatic reconnection on disconnect
- Add exponential backoff retry in `runManager()` when connection drops

**Estimated effort:** 1 session

---

### 7. Code Quality & Documentation (Priority: LOW)

#### 7a. Remove dead/commented code
- `messages/messages.go` has an unused import or field (1-line diff in git status)
- Scattered TODO comments that are outdated after refactoring
- Phase references (Phase 4, 5, 6) in comments are now obsolete

#### 7b. README update
- `README.md` has pending changes (286 lines modified in git status)
- Should reflect new architecture: command registry, SQLite-only storage, UI module split

#### 7c. Architecture documentation
- Document the new file layout for contributors
- Document the command handler pattern for adding new commands

**Estimated effort:** 1 session

---

### 8. Pre-existing Uncommitted Changes (Priority: CLARIFY)

The following files have uncommitted changes that predate the refactoring sessions:

| File | Status | Notes |
|------|--------|-------|
| `README.md` | Modified (164+/123-) | Major rewrite, needs review before committing |
| `messages/messages.go` | Modified (1 line removed) | Likely a cleanup, needs review |
| `ClaudeCodeProjectReview 20260214.md` | Untracked | Code review document |
| `agents.md` | Untracked | Agent documentation |
| `public/` | Untracked | Unknown purpose |

**Action needed:** Review and decide whether to commit, discard, or .gitignore these.

---

## Recommended Priority Order

1. **Test coverage** (storage + commands) — safety net for future changes
2. **CI/CD pipeline** — automate quality gates
3. **Handle more message types** — user-visible improvement
4. **Implement missing commands** — user-visible improvement
5. **AppContext refactor** — enables UI testing
6. **Error handling** — robustness
7. **Documentation cleanup** — maintainability
