# Changelog

All notable changes to the WhatsApp CLI application will be documented in this file.

## [v1.7.0] - 2026-07-18

Phase 5 ("Feature Growth") of the implementation plan, items 5.1 through 5.8,
delivered as one batch.

### Added
- **Reply threading** (5.1/5.3): incoming replies show a quoted preview line
  above the message, resolved from the local copy of the quoted message when
  available. `/reply` (keybinding `y`) sends a quoted reply to a selected
  message.
- **Reactions** (5.2): reactions are no longer stored as standalone
  `[REACTION]` rows — they're aggregated onto their target message and shown
  inline (e.g. `👍 x2 ❤️ x1`). Applies to both live messages and history sync.
- **Message editing** (5.4): `/edit` (keybinding `e`) edits one of your own
  messages within WhatsApp's 20-minute edit window.
- **Sticker sending** (5.5): `/sendsticker <path.webp>`.
- **Status viewing** (5.6): incoming status updates are no longer dropped
  during history sync and now populate their own read-only Status pane.
  Composing into Status (`/send`, media, `/reply`, `/record`) is refused with
  a clear error, since WhatsCLI has no status-posting feature.
- **Voice note recording** (5.7): `/record` records audio via a configurable
  external command (`record_command`, e.g. sox or arecord) and sends it as a
  voice note; `/stoprecording` ends it early. Bounded by `record_max_seconds`
  so a forgotten recording can't run forever.
- **Encrypted chat export** (5.8): `/export <chat-jid> <path>` writes a
  chat's full message history to an [age](https://age-encryption.org)
  -encrypted file, using a passphrase read from the
  `WHATSCLI_EXPORT_PASSPHRASE` environment variable (never a command
  parameter, so it isn't echoed to the screen). The output is standard age
  ciphertext — decryptable with any age-compatible tool, including the
  official CLI (`age -d -p export.age > messages.json`) — so WhatsCLI does
  not need to implement decryption itself.

### Fixed
- Corrected several long-standing documentation gaps found while updating
  these docs for Phase 5: the Phase 2/3 message-scoped commands
  (`/download`, `/open`, `/show`, `/url`, `/revoke`) were never listed in
  README's command reference; several docs still said the
  `UiMessageHandler` interface has 16 or 17 methods after it was reduced to
  15 in an earlier phase.

### Testing
- Added unit tests for every new command's guard chain, the reaction/quote
  storage layer, and the age encrypt/decrypt round trip (verified against
  the real `age` library, not just internal self-consistency).
- The voice-recording tests exercise a real subprocess with genuine SIGINT
  handling (a self-re-exec test helper, the same pattern used by the Go
  standard library's own `os/exec` tests) rather than a shell script — an
  earlier shell-script-based version of these tests was silently unreliable
  under this system's `/bin/sh` (dash), which does not run a trap handler
  while blocked in a non-interactive wait on a foreground child process.
- Known, accepted testing limits (consistent with earlier phases): actual
  microphone capture and a live WhatsApp send cannot be exercised without
  real hardware/a live account respectively.

## [v1.6.0] - 2026-07-18

Phase 4 ("Testability and Observability") of the implementation plan. Items
4.1 (command-handler test suite) and 4.2 (file-based debug log) landed in the
preceding work; this release completes the phase with item 4.3 and a full
cross-phase audit.

### Changed
- The UI layer no longer relies on a package-level `ctx` global. `AppContext`
  is constructed once in `main()` and injected into `UiHandler`; every UI
  helper and key handler is now a method on `*AppContext`. This makes the
  render and selection logic unit-testable in isolation (see the new
  `RenderChatTable` selection-restoration tests) with no process-global state.
- All asynchronous UI updates are now applied by a single ordered consumer
  instead of one goroutine per update. Backend calls therefore render in the
  exact order they were made, fixing out-of-order rendering.

### Fixed
- **Deadlock (introduced while building the ordered UI queue, fixed before
  release):** a bounded, blocking update queue could deadlock the app —
  command loop blocked enqueuing a UI update → could not drain
  `CommandChannel` → main loop blocked sending to `CommandChannel` → could not
  draw → queue never drained. The queue is now a non-blocking, order-preserving
  unbounded queue, so a UI-producing goroutine (including the command loop)
  can never stall on rendering. Regression-tested.
- **Security (media download, path traversal):** a malicious document's
  sender-controlled `FileName` (e.g. `../../../home/user/.bashrc`) was joined
  with the download directory without sanitization, allowing writes outside
  `download_path`. Document names are now reduced to their base component.
- **Correctness (media download):** two different documents that shared a
  sender-chosen filename collided, so the reuse-if-exists check could return
  the first document's bytes when the second was requested. Downloaded
  document names now include the message ID and are unique per message.
- A failed disk write during download no longer leaves a partial/zero-byte
  file behind that could masquerade as a complete attachment.

### Testing
- Test coverage recorded for the `messages` package: 41.4% of statements
  (command-handler suite from item 4.1). Root UI package: 14.2%.
- Added UI update-ordering, deadlock-safety, and `RenderChatTable`
  selection-restoration tests using tcell's offscreen `SimulationScreen`.

## [v1.5.0] - 2026-07-18

Phase 3 ("Live Correctness") of the implementation plan: local state now
converges with what actually happened on the WhatsApp server or another
linked device, instead of only reflecting what WhatsCLI itself did.

### Added
- Reading a chat on another linked device (phone, WhatsApp Web) now clears
  its unread badge in WhatsCLI within seconds, instead of only when read
  locally.
- Revoking a message from another device or participant now marks it
  deleted locally (text replaced with `[DELETED]`) instead of being
  silently dropped.
- Editing a message now updates the stored text locally, marked `(edited)`.
- The `Forwarded` flag is now actually populated on received messages
  (image, video, audio, document, sticker, and extended-text messages),
  so the existing forwarded-message color styling finally activates.

### Changed
- `/backlog` now waits for the actual history-sync response instead of a
  fixed 5-second sleep, with a 30-second fallback timeout if no response
  arrives — results (or the "no immediate history" message) appear as soon
  as the server actually replies.

## [v1.4.0] - 2026-07-18

Phase 2 ("Restore the Media Pipeline") of the implementation plan: attachment
download, in-terminal preview, URL opening, and message revocation — the five
keys (`d`/`o`/`s`/`u`/`r`) that Phase 1 turned into honest "not implemented"
stubs are now real.

### Added
- Media metadata (media key, direct path, file hashes, length, mimetype,
  document filename) is now captured on every received message — live and
  via `/backlog` history sync — and persisted to SQLite (additive schema
  migration; existing databases upgrade in place with no data loss).
- `d` / `/download` — downloads the attachment on a selected message to
  the configured download path.
- `o` / `/open` — downloads and opens the attachment with the OS default
  handler.
- `s` / `/show` — downloads an image or sticker attachment to the preview
  path and renders it in-terminal via the configured `show_command`.
- `u` / `/url` — extracts and opens the first URL found in a selected
  message's text.
- `r` / `/revoke` — revokes (deletes for everyone) a message the local user
  sent; the local copy is marked deleted and its text replaced with
  `[DELETED]` once the server confirms the revoke.
- Repeated `/download` or `/show` on the same message reuses the
  already-downloaded file instead of re-fetching it from the network or
  creating numbered duplicates.

## [v1.3.1] - 2026-07-18

Phase 1 ("Truth and Safety") of the implementation plan: dependency currency,
honest UX for unimplemented features, dead code removal, and small
correctness fixes. No new user-facing features.

### Changed
- Upgraded `whatsmeow` from a February 2026 build to the July 2026 build, and
  refreshed `beeep`, `xdg`, `ini`, `go-sqlite3`, and `tcell` to current
  releases. The `go` directive moved to 1.25 (required by the updated
  `whatsmeow`); CI now builds on Go 1.25 only.
- `/more` and `/backlog` now use the configured `[general] backlog_msg_quantity`
  instead of a hardcoded 50-message page size.
- Desktop/terminal-bell notifications are now throttled to at most one per
  `[general] notification_timeout` seconds, honoring a config key that was
  previously read but never enforced.
- The five message-scoped commands bound to keys but not yet implemented
  (`download`/`d`, `open`/`o`, `show`/`s`, `url`/`u`, `revoke`/`r`) now print
  an honest "not implemented" message instead of falling through to
  "Unknown command"; the help screen and README mark them `(soon)`.
- `PrintImage`'s download/preview-directory containment check now uses
  `filepath.Rel` instead of a bare string-prefix comparison, so a sibling
  directory that merely starts with the same name (e.g. `DownloadsEvil`) is
  no longer treated as inside the allowed directory.
- `cmdMedia` now reports "not connected" on a nil client, matching every
  other command handler, instead of silently doing nothing.
- `/colorlist` output is now sorted deterministically and routed through the
  same `QueueUpdateDraw` path as every other UI update.

### Removed
- Dead code with no remaining callers: `BatteryChannel`/`BatteryMsg` and the
  battery segment of the status bar (the multi-device protocol never
  delivers battery events), `ChatChannel`, `TextChannel`, the unreachable
  `loadRecentMessages`, the unused `Contact` struct, `Chat.LastMessage`, and
  the `SetChats` no-op (the `UiMessageHandler` interface is now 16 methods).

### Fixed
- An empty `if statusMsg.err != nil {}` branch in the session event loop
  silently discarded connection errors (e.g. "login timed out"); it now
  surfaces them via `PrintError`.

## [Unreleased] - 2023-03-17

### Changed
- Migrated from deprecated `go-whatsapp` library to the newer `whatsmeow` library
- Complete rewrite of the session management system for better reliability
- Enhanced message handling with improved error handling and feedback
- Updated UI interactions to work with the new backend

### Added
- Automatic loading of recent chats after successful login or session restoration
- Loading of the 20 most recent messages for each chat, including text and image messages with captions
- Loading of WhatsApp contacts from the address book to ensure proper name display
- Improved backlog command with multiple strategies for retrieving message history:
  - Read receipt trigger method
  - Chat presence (typing status) trigger method
  - Direct history sync notification method
- Better error handling with user-friendly messages
- More robust session restoration and QR code login process

### Fixed
- Nil pointer dereference errors in message history retrieval
- Type mismatches in the backlog command implementation
- Issues with contact name display in messages and chat list
- Improved session reconnection after disconnect

### Security
- Updated to the latest WhatsApp protocol implementation
- Better handling of encrypted sessions 
