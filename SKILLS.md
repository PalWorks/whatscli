# SKILLS.md: Development Patterns & Technical Reference

> This document describes the key development patterns, conventions, and technical skills required to effectively contribute to WhatsCLI. It serves as a deep-dive companion to `CONTRIBUTING.md`. Rules live in [AGENTS.md](AGENTS.md); design in [ARCHITECTURE.md](ARCHITECTURE.md).
>
> _Last updated: 2026-10-31T15:51:30+00:00 (v1.6.0 audit: interface listing, schema, storage methods, event list, mock fields, root context)_

## Required Skills

**Table SK1: Required skills**

| Skill | Relevance |
|-------|-----------|
| **Go 1.26+** (toolchain 1.27.3, CGO) | Primary language; generics, error wrapping, context propagation |
| **Concurrent Go** | Goroutines, channels, `sync.RWMutex`, select statements |
| **SQLite** | Persistence layer (WAL mode, indexes, UPSERT, migrations) |
| **Terminal UI (tview/tcell)** | TUI framework for layout, input handling, rendering |
| **WhatsApp Protocol** | whatsmeow library for multi-device API integration |
| **INI Configuration** | go-ini library with XDG path conventions |

---

## Pattern 1: Command Registry

WhatsCLI uses a centralized **command registry** pattern rather than a monolithic switch statement.

### How It Works

```
User types "/sendimage photo.jpg"
      ↓
TextInput.EnterCommand() parses → Command{Name: "sendimage", Params: ["photo.jpg"]}
      ↓
Sent to SessionManager.CommandChannel (buffered chan, size 10)
      ↓
runManager() event loop calls execCommand()
      ↓
commandRegistry["sendimage"] → cmdMedia() handler
```

### Registry Implementation

```go
// cmd_registry.go
var commandRegistry map[string]commandHandler

func init() {
    commandRegistry = map[string]commandHandler{
        "sendimage": cmdMedia,
        "upload":    cmdMedia,  // alias
        // ...
    }
}
```

### Handler Contract

```go
type commandHandler func(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string)
```

**Rules:**
- `client` may be `nil`: always check before using
- Use `checkParam(params, n)` to validate parameter count
- Report errors via `sm.uiHandler.PrintError(err)`, never `panic()`
- Use `cmdName` in error messages (supports aliases)

---

## Pattern 2: Channel-Based Architecture

The TUI and backend communicate exclusively through Go channels and an interface.

### UI → Backend (Commands)

```go
// UI goroutine sends:
ctx.SessionManager.CommandChannel <- messages.Command{Name: "select", Params: []string{chatID}}

// Backend goroutine receives in runManager():
case command := <-sm.CommandChannel:
    sm.execCommand(command)
```

### Backend → UI (Updates)

The `UiMessageHandler` interface (18 methods) allows the backend to update the UI without importing tview:

```go
type UiMessageHandler interface {
    NewMessage(Message)
    NewScreen([]Message)
    NewScreenWithStatus(msgs []Message, status string, errs ...error)
    SetChats([]Chat)
    UpdateChatList([]*Conversation)
    PrintError(error)
    PrintText(string)
    PrintFile(string)
    PrintQR(string)
    SetStatus(SessionStatus)
    OpenFile(string) error
    ShowColorList()
    Clear()
    ReplaceText(string)
    UpdateQR(qr string, attempt int, timeout int)
    PrintCommands()
    PrintHelp()
    Quit()
}
```

**Why this matters:** It enables testing the entire backend with `MockUiHandler`: no TUI required.

---

## Pattern 3: Thread-Safe Client Access

The `whatsmeow.Client` is a shared resource accessed by multiple goroutines.

### Rules

```go
// CORRECT: use getClient() (acquires RLock)
func (sm *SessionManager) getClient() *whatsmeow.Client {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    return sm.client
}

// WRONG: direct field access without lock
client := sm.client  // DATA RACE
```

### Lock Discipline

```go
// CORRECT pattern: lock for PQ, unlock before DB write
sm.mu.Lock()
heap.Push(&sm.priorityQueue, conv)
sm.convByJID[conv.JID] = conv
sm.mu.Unlock()
// NOW safe to write to SQLite
sm.db.UpsertConversation(*conv)

// WRONG: holding lock during DB write causes potential deadlock
sm.mu.Lock()
sm.db.UpsertConversation(*conv)  // DEADLOCK RISK
sm.mu.Unlock()
```

---

## Pattern 4: Priority Queue for Chat Ordering

Conversations are ordered using a heap-based priority queue that sorts by pinned status and last message time.

```go
// priority_queue.go implements container/heap.Interface
type PriorityQueue []*Conversation

// Ordering: pinned chats first, then by LastMsgTime (newest first)
func (pq PriorityQueue) Less(i, j int) bool {
    if pq[i].IsPinned != pq[j].IsPinned {
        return pq[i].IsPinned
    }
    return pq[i].LastMsgTime > pq[j].LastMsgTime
}
```

**Lookup map:** `sm.convByJID` provides O(1) access by JID, while `sm.priorityQueue` maintains the display order.

**Thread safety:** Use `snapshotPQ()` to create a deep copy before passing to the UI goroutine.

---

## Pattern 5: SQLite Persistence

### Database Initialization

```go
// Production: file-based
func (md *MessageDatabase) Init() error {
    dbPath := config.GetSessionFilePath() + "_meta.db"
    db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
    return md.InitWithDB(db)
}

// Testing: in-memory
func (md *MessageDatabase) InitWithDB(db *sql.DB) error {
    md.db = db
    // CREATE TABLE IF NOT EXISTS ...
}
```

### Schema

```sql
-- Conversations
CREATE TABLE conversations (
    jid TEXT PRIMARY KEY,
    name TEXT, last_msg_time INTEGER, preview TEXT,
    unread INTEGER, is_pinned BOOLEAN, is_archived BOOLEAN
);

-- Messages (indexed for chat queries and pagination)
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    chat_id TEXT, contact_id TEXT, contact_name TEXT, contact_short TEXT,
    timestamp INTEGER, from_me BOOLEAN, forwarded BOOLEAN, text TEXT,
    -- added by addColumnIfMissing (idempotent ALTER TABLE):
    media_kind TEXT NOT NULL DEFAULT '', mime_type TEXT NOT NULL DEFAULT '',
    file_name TEXT NOT NULL DEFAULT '', media_proto BLOB,
    local_path TEXT  -- where the media was last saved; NULL until the first download
);
CREATE INDEX idx_messages_chat_id ON messages(chat_id);
CREATE INDEX idx_messages_chat_ts ON messages(chat_id, timestamp);
```

### Key Operations

**Table SK2: MessageDatabase operations**

| Method | Purpose |
|--------|---------|
| `AddMessage(msg)` | INSERT OR IGNORE, then fills empty media columns of a known row in the same chat (never a revoked row); returns a `StoreResult` so callers redraw only on change |
| `UpsertConversation(c)` | INSERT ON CONFLICT UPDATE: create or update chat metadata |
| `GetMessage(id)` / `GetOldestMessage(chat)` / `GetLatestMessage(chat)` / `CountMessages(chat)` | Single message, backlog anchor, chat preview, count for the backlog wait and `/more` |
| `NthTimestampBefore(chat, before, n)` / `GetMessagesSince(chat, since)` | Chat view paging: anchor of the next page, messages at or after the anchor |
| `MarkMessageRevoked(chatID, id)` | Sets `[message revoked]`, clears media and `local_path`; scoped by chat |
| `DeleteMessage(chatID, id)` | Removes a row ("Delete for me"); scoped by chat |
| `SetLocalPath(chatID, id, path)` | Records where media was saved; scoped by chat |
| `DeleteEmptyConversations()` | Removes one-to-one chat rows with no messages, time, unread count or pin; groups kept (startup) |
| `GetMessages(chatId)` | All messages for a chat, ordered by timestamp ASC |
| `GetMessagesPaginated(chatId, before, limit)` | Older messages before timestamp, DESC (caller reverses); not used by the chat view |
| `SearchMessages(chatId, keyword, limit)` | LIKE search with proper escaping, newest first |

---

## Pattern 6: Auto-Reconnect with Exponential Backoff

```go
func (sm *SessionManager) scheduleReconnect() {
    backoff := 2 * time.Second
    const maxBackoff = 30 * time.Second
    const maxAttempts = 5

    for attempt := 1; attempt <= maxAttempts; attempt++ {
        // Check abort conditions (logout, shutdown)
        // Wait with backoff, respecting stop channel
        // Attempt login, double backoff on failure
    }
}
```

**Guard:** `sm.reconnecting` flag prevents duplicate reconnect goroutines. The flag is checked with the lock before spawning.

---

## Pattern 7: Event-Driven Message Handling

WhatsApp events flow through `event_handler.go`:

```
whatsmeow Client
    ↓ AddEventHandler()
eventHandler.Handle(evt interface{})
    ↓ type switch
    ├── *events.Message → process (or authorise a revoke), store in SQLite, notify UI
    ├── *events.Connected → clear reconnect and logout flags, update status
    ├── *events.Disconnected → update status, schedule reconnect
    ├── *events.LoggedOut → mark logged out, print the reason
    ├── *events.DeleteForMe → remove the stored row, move the preview
    └── *events.HistorySync → bulk message import (goroutine)
```

Other event types are ignored.

---

## Testing Patterns

### In-Memory Database Setup

```go
func TestSomething(t *testing.T) {
    db, _ := sql.Open("sqlite3", ":memory:")
    md := &MessageDatabase{}
    md.InitWithDB(db)
    defer md.Close()
    // ... test with md
}
```

### Mock UI Handler

```go
type MockUiHandler struct {
    Texts       []string
    Errors      []error
    Screens     [][]Message
    ScreenTexts []string // status printed with each NewScreenWithStatus
    OpenErr     error    // returned by OpenFile
    // ... captures all UI calls for assertions
}

// ... implements all 18 UiMessageHandler methods
```

### Race Detection

All tests run with `-race`. The CI enforces this:
```bash
go test -v -race -count=1 ./...
```

---

## Pattern 8: Untrusted input from senders

Anything a sender controls (file name, mime type, text, protocol messages) is hostile until checked. Full threat table in [SECURITY.md](SECURITY.md).

```go
// File name: reduce to one inert path element, then confine to the directory.
name, ok := sanitizeFileName(msg.FileName)   // messages/media.go
full, err := safeJoin(dir, name)

// Opening: only an allowlist reaches the system opener.
if !safeToOpen(path) { /* save, print path, do not open */ }

// Incoming revoke: same chat and authorised revoker, else ignore.
switch sm.decideRevoke(src, target) { /* revokeApply, revokeCheckAdmin, revokeIgnore */ }
```

## Pattern 9: One chat per person (phone number versus LID)

```go
chat := sm.canonicalChatJID(jid)            // @lid -> phone-number JID when mapped
same := sm.sameUser(a, b)                    // device-insensitive, LID aware
```

Never compare JIDs as strings to decide whether two addresses are the same person. Test with the `lidMapperOverride` seam (`messages/jid_norm_test.go`).

## Pattern 10: Network work off the event loop

```go
// Validate on the event loop, then:
go func() {
    client := sm.getClient()          // re-fetch, the captured client may be stale
    if client == nil { return }
    ctx := sm.rootContext()           // cancelled by Shutdown, so quitting stops the work
    // download, send, poll ... with ctx
}()
```

Seams for tests: `mediaDownloader`, `lidMapperOverride`, `groupInfoOverride`, `webMessageParser`.
