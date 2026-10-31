## Executive Summary

WhatsCLI is a Go TUI WhatsApp client with a two-layer architecture (UI in `main.go`, backend in `messages/`) communicating via channels — a sound foundation. However, the codebase suffers from several structural problems that would block production-grade reliability. The two largest files (`main.go` at 1171 lines, `session_manager.go` at 1504 lines) are God objects concentrating too many responsibilities. There is a command injection vulnerability in the media viewer path. The priority queue uses O(n) linear scans for every lookup despite having a heap, missing the obvious map-backed index. Test coverage is near-zero — only the priority queue has tests. Error handling is inconsistent, ranging from `panic()` on database failure to silently swallowed errors. Mutex discipline is inconsistent, with several unprotected reads of `sm.client`. The legacy Gob persistence layer adds unnecessary complexity and should be fully removed.

---

## 1. Repository Mapping

| Aspect | Details |
|:---|:---|
| **Language** | Go 1.24 |
| **UI Framework** | `tview` / `tcell` (TUI) |
| **Protocol** | `whatsmeow` (WhatsApp Multi-Device) |
| **Storage** | SQLite (`go-sqlite3` CGO) + legacy Gob (being phased out) |
| **Build** | `Makefile` (simple: `go build`) |
| **CI/CD** | None |
| **Testing** | 1 test file (`priority_queue_test.go`), no integration tests |
| **Release** | `release.sh` — cross-compiles, creates GitHub release via `gh` |

**Components:**
- `main.go` — UI layout, keybindings, event handlers, rendering
- `messages/session_manager.go` — Auth, connection, command dispatch, event handling, media
- `messages/storage.go` — SQLite + Gob persistence, migration
- `messages/messages.go` — Data types, interfaces, constants
- `messages/priority_queue.go` — Heap-based chat ordering
- `qrcode/qrcode.go` — ASCII QR code rendering
- `config/settings.go` — INI config loading via XDG

---

## 2. Architecture & Design Review

- **Severity: High** | **Location: `main.go`**
  - **Problem:** God file — 1171 lines handling layout construction, keybinding registration, all input capture handlers, message rendering, UI handler implementation, and help text.
  - **Why it matters:** Any change to the UI risks breaking unrelated functionality. Impossible to test rendering logic in isolation.
  - **Recommendation:** Extract into packages: `ui/layout.go`, `ui/keybindings.go`, `ui/handler.go`, `ui/render.go`. The `UiHandler` struct and its methods alone are ~120 lines that belong in their own file.

- **Severity: High** | **Location: `messages/session_manager.go`**
  - **Problem:** `execCommand()` is a 550+ line switch statement handling 17+ commands spanning auth, messaging, media upload, group management, and history sync.
  - **Why it matters:** Adding a new command requires modifying a function that's already unmanageable. Each command branch has different error handling patterns.
  - **Recommendation:** Use a command registry pattern:
    ```go
    type CommandHandler func(sm *SessionManager, params []string)
    var commands = map[string]CommandHandler{
        "send":        handleSend,
        "sync-groups": handleSyncGroups,
        // ...
    }
    ```
    Each handler in its own file under `messages/commands/`.

- **Severity: Medium** | **Location: `messages/storage.go`**
  - **Problem:** Dual persistence (SQLite + Gob) with legacy maps still maintained in parallel. `AddChat()` writes to in-memory map, `AddMessage()` writes to SQLite, `Save()` serializes maps to Gob. Three sources of truth.
  - **Why it matters:** Data inconsistency risk. Every write path must update two systems. The Gob file is loaded on startup just to check if migration is needed.
  - **Recommendation:** Complete the migration. Remove `messages` and `messagesById` maps, the `storageDump` struct, `Save()`/`Load()` Gob methods, and the `chats` map. SQLite is already the primary store — cut the cord.

- **Severity: Medium** | **Location: Global state in `main.go`**
  - **Problem:** 15+ package-level `var` declarations (`textView`, `chatTable`, `groupTable`, `app`, `sessionManager`, `allChats`, `chatLimit`, etc.) create implicit coupling between all functions.
  - **Why it matters:** Any function can mutate any global at any time. Makes reasoning about state changes impossible and prevents any future testing.
  - **Recommendation:** Encapsulate in an `App` struct that holds all UI components and state, passed explicitly to handlers.

---

## 3. Code Quality & Maintainability

- **Severity: Medium** | **Location: `messages/session_manager.go:441-450`, `:897-902`, `:1346-1351`**
  - **Problem:** Linear scan of `priorityQueue` slice to find a conversation by JID — repeated in at least 5 locations with identical `for _, item := range sm.priorityQueue` loops.
  - **Why it matters:** O(n) per lookup. With 10k+ chats (the stated design goal in `public/part2_memory_efficient_architecture.md`), this becomes a bottleneck on every incoming message.
  - **Recommendation:** Maintain a `map[string]*Conversation` alongside the heap. Lookup becomes O(1):
    ```go
    type SessionManager struct {
        priorityQueue PriorityQueue
        convByJID     map[string]*Conversation // index
    }
    ```

- **Severity: Medium** | **Location: `messages/session_manager.go:1128-1218`**
  - **Problem:** The `upload`/`sendimage`/`sendvideo`/`sendaudio` case always calls `sm.client.Upload(ctx, data, whatsmeow.MediaImage)` regardless of media type. Line 1149 hardcodes `MediaImage` even for video, audio, and documents.
  - **Why it matters:** Upload to WhatsApp servers with wrong media type may cause delivery failures or incorrect rendering on recipients' devices.
  - **Recommendation:** Map command to media type:
    ```go
    mediaTypes := map[string]whatsmeow.MediaType{
        "sendimage": whatsmeow.MediaImage,
        "sendvideo": whatsmeow.MediaVideo,
        "sendaudio": whatsmeow.MediaAudio,
        "upload":    whatsmeow.MediaDocument,
    }
    uploaded, err := sm.client.Upload(ctx, data, mediaTypes[cmd])
    ```

- **Severity: Low** | **Location: `main.go:161-163`**
  - **Problem:** Triple duplicate comment on `SetupLeftPane`:
    ```go
    // SetupLeftPane initializes the left panel with Status, Chats, and Groups
    // SetupLeftPane initializes the left panel with Status, Chats, and Groups
    // Left Pane Global
    // SetupLeftPane creates the left side panel with Status, Chats, and Groups
    ```
  - **Recommendation:** Keep one comment. Delete the rest.

- **Severity: Low** | **Location: `main.go:96-97`**
  - **Problem:** Duplicate `SetBackgroundColor` call on `textInput`.
  - **Recommendation:** Remove the duplicate line 97.

- **Severity: Low** | **Location: `main.go:676`**
  - **Problem:** Duplicate comment `// prints help to chat view`.
  - **Recommendation:** Delete the duplicate.

---

## 4. Performance & Scalability

- **Severity: High** | **Location: `messages/session_manager.go` — every `updatePQ` / `loadRecentChats` / `sync-groups`**
  - **Problem:** Every UI update deep-copies the entire priority queue into `safeList`:
    ```go
    safeList := make([]*Conversation, len(sm.priorityQueue))
    for i, item := range sm.priorityQueue {
        copiedItem := new(Conversation)
        *copiedItem = *item
        safeList[i] = copiedItem
    }
    ```
    This runs on every single incoming message.
  - **Why it matters:** With 10k chats, each message triggers 10k allocations + copies. Under high message volume, this creates significant GC pressure.
  - **Recommendation:** Instead of copying the full list, send only the delta (changed conversation) to the UI and let the UI update in-place. Or batch updates with a debounce timer (e.g., max 1 UI refresh per 100ms).

- **Severity: Medium** | **Location: `messages/session_manager.go:895-921`**
  - **Problem:** In `sync-groups`, the mutex is locked/unlocked per group inside the loop. For 200 groups, that's 200 Lock/Unlock cycles contending with the event handler goroutine.
  - **Why it matters:** Lock contention causes incoming message processing to stall during group sync.
  - **Recommendation:** Batch the entire loop under a single lock:
    ```go
    sm.mu.Lock()
    for _, group := range groups {
        // all PQ operations
    }
    sm.mu.Unlock()
    ```

- **Severity: Medium** | **Location: `messages/storage.go:47`**
  - **Problem:** SQLite opened without WAL mode or connection pool tuning.
  - **Why it matters:** Default journal mode causes write locks that block reads. Concurrent goroutines (auto-saver, event handler, command handler) will contend.
  - **Recommendation:** Add `?_journal_mode=WAL&_busy_timeout=5000` to the DSN.

---

## 5. Security & Reliability

- **Severity: Critical** | **Location: `main.go:814-833`**
  - **Problem:** `PrintImage` passes unsanitized paths to `exec.Command`:
    ```go
    cmdParts := strings.Split(config.Config.General.ShowCommand, " ")
    cmdParts = append(cmdParts, path)
    cmd = exec.Command(cmdParts[0], cmdParts[1:]...)
    ```
    The `path` comes from downloaded file paths which could contain shell metacharacters or be crafted by a malicious sender.
  - **Why it matters:** Command injection. A file named `; rm -rf ~/` or containing backticks could execute arbitrary commands.
  - **Recommendation:** Validate `path` is an actual file path (exists, no special characters) before passing to exec. Use `filepath.Clean()` and verify it's under the download directory:
    ```go
    cleanPath := filepath.Clean(path)
    if !strings.HasPrefix(cleanPath, config.Config.General.DownloadPath) {
        PrintError(fmt.Errorf("path outside download directory"))
        return
    }
    ```

- **Severity: High** | **Location: `messages/storage.go:49`**
  - **Problem:** `panic()` on database initialization failure:
    ```go
    panic(fmt.Sprintf("Failed to open metadata db: %v", err))
    ```
    Also at line 66 and 87.
  - **Why it matters:** Unrecoverable crash with no cleanup. The TUI is already running at this point — `panic` will corrupt the terminal state.
  - **Recommendation:** Return errors from `Init()` and handle gracefully in the caller. Show a user-friendly message and exit cleanly.

- **Severity: High** | **Location: `messages/session_manager.go` — multiple locations**
  - **Problem:** `sm.client` is read without holding the mutex in several places:
    - Line 374: `if sm.client == nil || !sm.client.IsConnected()`
    - Line 708: `if sm.client != nil && sm.client.IsConnected()`
    - Line 871 (inside goroutine): `if sm.client == nil`
    - Line 989: `if sm.client != nil`
  - **Why it matters:** Data race. `sm.client` is set to `nil` under `sm.mu.Lock()` in `login()` (line 227) and `logout()` (line 660), but these reads have no synchronization.
  - **Recommendation:** Always read `sm.client` under `sm.mu.RLock()`, or capture a local copy early in each function as done in `sendText()`.

- **Severity: Medium** | **Location: `messages/session_manager.go:276-279`**
  - **Problem:** Nested lock acquisition — `loginWithConnection` holds no lock but calls `sm.getConnection()` which acquires `sm.mu.Lock()`, and then the code at line 277 does `sm.mu.Lock()` again after calling `sm.getConnection()`. But line 276-278:
    ```go
    sm.mu.Lock()
    sm.client = nil
    client, err = sm.getConnection()  // this also calls sm.mu.Lock()
    sm.mu.Unlock()
    ```
  - **Why it matters:** `getConnection()` calls `sm.mu.Lock()` internally (line 193). Calling it while already holding the lock = **deadlock**.
  - **Recommendation:** Either make `getConnection` lock-free (require caller to hold lock), or restructure to avoid nested acquisition.

- **Severity: Medium** | **Location: `messages/session_manager.go:360-361`**
  - **Problem:** `sm.uiHandler.Clear()` called via `go` inside the QR code ticker, but also `sm.uiHandler.UpdateQR()` called on the next line. Race between clearing and updating.
  - **Recommendation:** Remove the `go sm.uiHandler.Clear()` call — `UpdateQR` already clears and rewrites the screen.

---

## 6. Testing & DevOps

- **Severity: High** | **Location: Project-wide**
  - **Problem:** Only 1 test file (`priority_queue_test.go`, 70 lines). Zero test coverage for:
    - `session_manager.go` (the core of the app)
    - `storage.go` (data persistence)
    - `config/settings.go` (config parsing)
    - `main.go` (UI logic)
    - `qrcode/qrcode.go` (rendering)
  - **Why it matters:** The switch-case bug we just fixed would have been caught by even a basic unit test for `execCommand`.
  - **Recommendation (prioritized):**
    1. **`storage_test.go`** — Test `AddMessage`, `GetMessages`, `UpsertConversation`, `GetConversations`, `MigrateToSQLite` with an in-memory SQLite (`":memory:"`).
    2. **`session_manager_test.go`** — Mock `UiMessageHandler` interface and `whatsmeow.Client` to test `execCommand` dispatch, `sendText`, and connection state transitions.
    3. **`config/settings_test.go`** — Test INI parsing with custom temp files.

- **Severity: Medium** | **Location: Project-wide**
  - **Problem:** No CI/CD pipeline. No `.github/workflows/` directory.
  - **Recommendation:** Add a basic GitHub Actions workflow:
    ```yaml
    on: [push, pull_request]
    jobs:
      test:
        runs-on: ubuntu-latest
        steps:
          - uses: actions/checkout@v4
          - uses: actions/setup-go@v5
            with: { go-version: '1.24' }
          - run: go vet ./...
          - run: go test -race ./...
    ```

- **Severity: Low** | **Location: `Makefile`**
  - **Problem:** No `test` target in the Makefile.
  - **Recommendation:** Add `test: go test -race ./...`

---

## Priority Summary

| # | Severity | Issue | Location |
|:--|:---------|:------|:---------|
| 1 | **Critical** | Command injection in `PrintImage` | `main.go:814-833` |
| 2 | **High** | `panic()` in storage init crashes app | `storage.go:49,66,87` |
| 3 | **High** | Deadlock: nested mutex in `loginWithConnection` | `session_manager.go:276-279` |
| 4 | **High** | Data races on `sm.client` reads without lock | `session_manager.go` (multiple) |
| 5 | **High** | Near-zero test coverage | Project-wide |
| 6 | **High** | O(n) PQ scans need map index | `session_manager.go` (5+ locations) |
| 7 | **High** | Full PQ deep-copy on every message | `session_manager.go` |
| 8 | **High** | `execCommand` is 550-line monolith | `session_manager.go:669-1219` |
| 9 | **Medium** | Wrong `MediaType` for video/audio/doc uploads | `session_manager.go:1149` |
| 10 | **Medium** | Legacy Gob/map dual persistence | `storage.go` |
| 11 | **Medium** | SQLite missing WAL mode | `storage.go:47` |
| 12 | **Medium** | `main.go` God file (1171 lines, 15 globals) | `main.go` |
| 13 | **Medium** | Per-group locking in sync-groups loop | `session_manager.go:895-921` |
| 14 | **Low** | Duplicate comments/lines | `main.go:96-97, 161-163, 676` |
| 15 | **Low** | No CI/CD, no Makefile test target | Project config |

Items 1-4 are bugs that will cause crashes or security issues in production. These should be addressed before any feature work.
