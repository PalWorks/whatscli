# Architecture

System design of WhatsCLI as it is in the code at `v1.6.2`. Decisions and their reasons are in the maintainers' private decision log; agent rules in [AGENTS.md](AGENTS.md).

_Last updated: 2026-10-31T16:55:57+00:00 (describes v1.6.2)_

## 1. Components

```
 +-----------------------------+            +---------------------------------------+
 | main package (TUI, tview)   |  Command   | messages package                      |
 |                             |  Channel   |                                       |
 |  AppContext (shared state)  | ---------> |  SessionManager.runManager()          |
 |  ui_layout / ui_keybindings |            |    execCommand -> commandRegistry     |
 |  ui_render / ui_helpers     | <--------- |    eventHandler (whatsmeow events)    |
 |  UiHandler                  | UiMessage  |    history_sync, chat_state           |
 |                             | Handler    |    MessageDatabase (SQLite)           |
 +-----------------------------+            +-------------------+-------------------+
                                                                |
   config/ (INI, XDG)      qrcode/ (ANSI QR)                    v
                                                    whatsmeow client -> WhatsApp
                                                    session.db (whatsmeow store)
```

**Table AR1: Components**

| Component | Files | Responsibility |
|---|---|---|
| TUI | `main.go`, `app_context.go`, `ui_layout.go`, `ui_handler.go`, `ui_keybindings.go`, `ui_render.go`, `ui_helpers.go`, `ui_opener.go` | tview widgets, key bindings (cbind), rendering, help text, image viewer (`renderImage`), system opener (`openerCommand`, `startOpener`). `UiHandler` implements `messages.UiMessageHandler` and queues every update with `go ctx.App.QueueUpdateDraw` |
| Session | `messages/session_manager.go`, `connection.go`, `partfiles.go` | `SessionManager`: event loop `runManager`, login and QR flow, reconnect with backoff, `getClient()`, root context cancelled first in `Shutdown`, startup sweep of leftover download temp files |
| Events | `messages/event_handler.go`, `history_sync.go`, `chat_state.go`, `chat_view.go`, `delete_for_me.go` | Handle whatsmeow events (message, connected, disconnected, logged out, history sync, delete for me), store messages, keep the chat priority queue; the open chat shows messages from a view anchor (newest 200, `/more` adds 200) |
| Commands | `messages/cmd_*.go`, `cmd_registry.go` | One handler per slash command; registry filled in `init()` |
| Media and safety | `messages/media.go`, `pathcheck.go`, `revoke_incoming.go`, `jid_norm.go`, `lid_merge.go` | Media metadata, file name sanitisation, open allowlist, containment, revoke authorisation, canonical JIDs, merge of old `@lid` chats |
| Storage | `messages/storage.go` | `MessageDatabase`: `conversations` and `messages` tables |
| Config | `config/settings.go` | INI at `<XDG_CONFIG>/whatscli/whatscli.config`; four sections |
| QR | `qrcode/qrcode.go` | Renders a QR code as ANSI with the full 4-module quiet zone |

## 2. Data flow

**UI to backend.** A key or typed command becomes `messages.Command{Name, Params}` on `CommandChannel` (buffered, 10). `runManager` receives it and `execCommand` looks it up in `commandRegistry` and calls the handler with the client fetched once by `getClient()` (may be nil). The unknown-command case prints a red message.

**Backend to UI.** The backend calls the 18 methods of `UiMessageHandler`. The backend never imports tview.

**Whatsmeow to backend.** `eventHandler.Handle` switches on the event type:

- `*events.Message`: `handleMessage` unwraps, `processIncomingMessage` builds a `Message` (with media metadata via `attachMediaInfo`), stores it under the canonical chat JID, updates the conversation, and notifies. REVOKE protocol messages go to `handleIncomingRevoke` instead.
- `*events.Connected`, `Disconnected`, `LoggedOut`: update status; `Disconnected` starts `scheduleReconnect` unless logged out, already reconnecting or not started.
- `*events.DeleteForMe`: `handleDeleteForMe` (`delete_for_me.go`) deletes the stored row (chat matched in either JID form), moves the chat preview to the newest remaining message and redraws the open chat.
- `*events.HistorySync`: `processHistorySync` in a goroutine; `storeHistorySync` stores conversations and messages and redraws the open chat only when a row changed (`AddMessage` returns a `StoreResult`), then merges known `@lid` chats (`lid_merge.go`).

**Chat list.** `loadRecentChats` imports the contact list (`importContacts`): a mapped `@lid` contact names its phone-number chat, an unmapped one stays `@lid`, only groups create new chats, then known `@lid` chats are merged.

**Chat view.** The open chat shows every stored message at or after `viewAnchor` (`chat_view.go`). Opening a chat sets the anchor to the oldest of its newest 200 messages; `/more` moves it back by 200 and says when all stored messages are shown; older messages stored by backlog or history sync lower the anchor so they appear.

**Message commands.** The highlighted message ID is the only parameter. `runDownload` validates synchronously (message exists, has media metadata, connected, image for `show`), then downloads in a goroutine through `downloadMedia` under the root context; a file recorded in `local_path` is reused when it is still a regular file directly in the target folder, otherwise the new path is recorded. `deliverDownload` then either prints the path (`download`), renders (`show`, via `UiMessageHandler.PrintFile`: `renderImage` checks the path with `ResolveFileWithin` and runs `show_command` off the UI goroutine with a 15 s timeout) or passes an allowlisted file to the system opener (`open`, `ui_opener.go`, started without a shell and reaped in the background).

**Info.** `/info` (key `i`) prints contact info for a one-to-one chat; for a group it calls `GetGroupInfo` in a goroutine with a 15 s timeout from the root context.

**Backlog.** `cmdBacklog` anchors on the oldest stored message of the chat, builds `BuildHistorySyncRequest` and sends it with `SendPeerMessage` in a goroutine, polls the stored count for up to 5 s, and refreshes the chat. The phone's answer arrives later as a `HistorySync` event.

## 3. Concurrency model

- Goroutines: the TUI (tview), `runManager`, whatsmeow's event goroutines, short-lived goroutines for network work (downloads, revoke send, backlog, group admin lookup, `/info` group lookup, reconnect, history sync), the image viewer run by `show`, and the background `Wait` of each system opener.
- `sm.mu` (RWMutex) guards the priority queue, `convByJID`, `currentReceiver`, `viewAnchor`, `client`, and the flags `started`, `reconnecting`, `loggedOut`. It is never held across SQLite writes or network calls.
- Root context: `sm.rootContext()` (created once, cancelled first in `Shutdown`) parents downloads, backlog and its poll, own revokes, the incoming-revoke admin check and the `/info` group lookup, so quitting stops them.
- `snapshotPQ()` gives the UI a deep copy of the queue.
- Rules and rationale: AGENTS.md section 6. UI updates are queued from separate goroutines, so their order is not guaranteed; status that must follow a redraw goes through `NewScreenWithStatus` (one queued update).
- Test seams, nil in production: `lidMapperOverride`, `groupInfoOverride` (SessionManager), `mediaDownloader` interface, `webMessageParser`.

## 4. Storage

SQLite via go-sqlite3 (CGO), WAL mode, 5 s busy timeout. No in-memory caches; every read queries SQLite.

**Table AR2: Files**

| File | Content |
|---|---|
| `<XDG_CONFIG>/whatscli/session_meta.db` | App data: `conversations`, `messages` |
| `<XDG_CONFIG>/whatscli/session.db` | WhatsApp device session (whatsmeow `sqlstore`) |
| `<XDG_CONFIG>/whatscli/whatscli.config` | User configuration |

**Table AR3: conversations**

| Column | Type |
|---|---|
| `jid` | TEXT, primary key |
| `name`, `preview` | TEXT |
| `last_msg_time`, `unread` | INTEGER |
| `is_pinned` | BOOLEAN |
| `is_archived` | BOOLEAN DEFAULT 0 (added by migration) |

**Table AR4: messages**

| Column | Type | Note |
|---|---|---|
| `id` | TEXT, primary key | `INSERT OR IGNORE`: first write wins; later copies only fill empty media columns (not on revoked rows) |
| `chat_id`, `contact_id`, `contact_name`, `contact_short` | TEXT | `contact_id` is stored as received (phone number or LID) |
| `timestamp` | INTEGER | |
| `from_me`, `forwarded` | BOOLEAN | |
| `text` | TEXT | `[message revoked]` after a revoke |
| `media_kind` | TEXT NOT NULL DEFAULT '' | image, video, audio, document, sticker, or empty |
| `mime_type`, `file_name` | TEXT NOT NULL DEFAULT '' | sender-declared, raw and unsanitised |
| `media_proto` | BLOB, nullable | serialised `waE2E.Message` holding only the media sub-message, thumbnails stripped |
| `local_path` | TEXT, nullable | where the media was last saved; reused only if still a regular file directly in the target folder; cleared on revoke |

Indexes: `idx_messages_chat_id (chat_id)`, `idx_messages_chat_ts (chat_id, timestamp)`.

**Migration strategy.** `InitWithDB` creates tables with `IF NOT EXISTS`, then adds columns with `addColumnIfMissing` (`ALTER TABLE ADD COLUMN`, tolerating only "duplicate column name"). Old rows get the defaults. Older binaries still work on a migrated database because they name their columns explicitly. Never drop or retype columns. Messages stored before the media columns existed report "this is not a downloadable message" (accepted limitation).

## 5. Chat identity: phone number JIDs and LIDs

WhatsApp can address one person as `<number>@s.whatsapp.net` (PN) or `<id>@lid`. Groups are `<id>@g.us`.

- Chats are stored under one canonical JID: an `@lid` chat becomes the PN JID when the mapping is known, else it stays `@lid` (`canonicalChatJID`, `canonicalChatForSource`). Existing rows are not rewritten.
- People are compared with `sameUser` (device parts ignored; LID and PN equal when mapped), never by string equality.
- The mapping comes from the message source (`SenderAlt`, `RecipientAlt`), the history conversation's `PnJID`, and whatsmeow's `Store.LIDs`.
- `contact_id` is not normalised (ROADMAP row 21). An old `@lid` chat stays separate until merged (row 13).

## 6. Security-relevant paths

Incoming revoke: `handleIncomingRevoke` loads the target, `decideRevoke` checks chat and sender, group admins are confirmed with `GetGroupInfo` in a goroutine, `applyRevoke` writes with `WHERE id = ? AND chat_id = ?`. Download: `sanitizeFileName`, `safeJoin`, temp file, rename; `local_path` reuse only through `ResolveFileWithin`. Startup sweep: only regular `.whatscli-<digits>.part` files directly in the download and preview folders. Open: `safeToOpen`, then `ui_opener.go` without a shell. Show: `ResolveFileWithin`. Details and threat notes: [SECURITY.md](SECURITY.md).

## 7. External dependencies

**Table AR5: Main dependencies (from go.mod)**

| Module | Version | Use |
|---|---|---|
| `go.mau.fi/whatsmeow` | v0.0.0-20261030132714-6f46ecaf9e95 | WhatsApp multi-device protocol |
| `github.com/rivo/tview` | v0.42.0 | TUI widgets |
| `github.com/gdamore/tcell/v2` | v2.13.10 | Terminal cells |
| `github.com/mattn/go-sqlite3` | v1.14.52 | SQLite (CGO) |
| `codeberg.org/tslocum/cbind` | v0.1.6 | Key bindings |
| `github.com/adrg/xdg` | v0.5.3 | XDG paths |
| `gopkg.in/ini.v1` | v1.67.3 | Config |
| `github.com/gen2brain/beeep` | v0.11.3 | Desktop notifications |
| `github.com/atotto/clipboard` | v0.1.4 | Clipboard |
| `github.com/skip2/go-qrcode` | v0.0.0-20200617195104-da1b6568686e | QR generation |

Toolchain: Go directive 1.26.0, `toolchain go1.27.3`, CGO required.

## 8. Scaling and limits

Single user, single account, local data. The chat lists show every stored chat and keep their scroll position across updates; an open chat shows its newest 200 stored messages and `/more` adds 200 older ones at a time (`chat_view.go`, `NthTimestampBefore`, `GetMessagesSince`). There is no server component. Reconnect is capped at 5 attempts.

## 9. Glossary

**Table AR6: Terms**

| Term | Meaning |
|---|---|
| JID | WhatsApp address: `user@server` |
| PN | Phone-number JID, server `s.whatsapp.net` |
| LID | Privacy address, server `lid` |
| History sync | Bulk or ON_DEMAND delivery of old messages from the phone |
| Backlog | `/backlog`: request older messages for the open chat |
| Revoke | Delete for everyone |
| Backlog anchor | Oldest stored message, the reference point for a backlog request |
| View anchor | `viewAnchor`: oldest timestamp shown in the open chat; `/more` lowers it |
| Delete for me | Removal on your own devices only, synced as app state (`events.DeleteForMe`), not a revoke |
