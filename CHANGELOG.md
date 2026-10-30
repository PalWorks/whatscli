# Changelog

All notable changes to this project are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions are git tags (`vX.Y.Z`).

_Last updated: 2026-10-31T01:11:55+05:30 (added [Unreleased] for the chore/catch-up branch, fixed the mislabelled 2025 entry, added v1.3.0 to v1.1.0 summaries from git history)_

## [Unreleased]

Branch `chore/catch-up` (`eee54c7` onward, 2026-10-31). Not yet merged or tagged. Release gate: the live-account checklist in [TESTING.md](TESTING.md) (ROADMAP row 15).

### Added
- Dual licence: AGPL-3.0-only (`LICENSE`) plus paid commercial licence (`LICENSE-COMMERCIAL.md`, support@palworks.ai). GitHub private vulnerability reporting enabled.
- Message commands and keys, previously bound to commands that did not exist: `/download` (`d`), `/open` (`o`), `/show` (`s`, images only), `/url` (`u`), `/revoke` (`r`, own messages only). All take a message ID.
- Media metadata is stored with every message (kind, mime type, file name, minimal serialised media proto) for live messages, history sync and sent media. Sent media is now stored and shown like sent text.
- Idempotent schema migration for the four new `messages` columns (`media_kind`, `mime_type`, `file_name`, `media_proto`).
- Incoming revokes (delete for everyone from a contact, your phone or a group admin) update the stored message to `[message revoked]`.
- `/backlog` rewritten on whatsmeow's ON_DEMAND history request (`BuildHistorySyncRequest` + `SendPeerMessage`), non-blocking, honours `backlog_msg_quantity`; open chat refreshes when a history sync stores new rows.
- `@lid` chats: history sync and live messages accept them and store one chat per person under the phone-number JID when the mapping is known (`messages/jid_norm.go`).
- Tag-driven release workflow `.github/workflows/release.yml`: native CGO builds for linux/amd64, linux/arm64, windows/amd64, SHA256 checksums, GitHub release notes from git log. AUR `PKGBUILD` and `.SRCINFO` are generated as a workflow artifact only.
- `make build` and `make install` inject the version from `git describe`; new `make lint` (alias of `vet`).
- Tests: migration, media round trip, sanitisation, traversal and collision, command validation, backlog, revoke authorisation, `@lid` normalisation, path containment, QR quiet zone.
- Documentation set: AGENTS.md, ARCHITECTURE.md, DECISIONS.md, ROADMAP.md, TESTING.md, SECURITY.md; historic reports moved to `docs/archive/`.

### Changed
- Module path `github.com/normen/whatscli` is now `github.com/PalWorks/whatscli`. Install with `go install github.com/PalWorks/whatscli@latest`.
- Go directive 1.26.0 with `toolchain go1.27.3` (required by the new whatsmeow). CI matrix is Go 1.26 and 1.27; `actions/checkout` and `actions/setup-go` are v7.
- Dependencies: whatsmeow `20260211` to `20261030`, go-sqlite3 v1.14.52, tcell v2.13.10, adrg/xdg v0.5.3, beeep v0.11.3, ini v1.67.3, protobuf v1.36.12 and indirect modules. `cbind` moved to `codeberg.org/tslocum/cbind` v0.1.6 (the old host is gone).
- Downloads stream through `DownloadToFile` into a temporary file in the target directory and are renamed into place. A taken file name gets a `_<message-id>` suffix.
- `show` checks the file with `messages.ResolveFileWithin` (symlink-resolved, `filepath.Rel` containment).
- Help text (`/help`) lists the new message commands.

### Fixed
- Keys `d o s u r` were bound to unregistered commands and did nothing.
- QR code now keeps the full 4-module quiet zone, so phones detect it reliably (upstream issue 138).
- Clipboard copy and paste no longer crash the TUI when no clipboard backend exists; the error is shown in red (upstream `02b3fbb`).
- Pre-existing schema migration ignored all `ALTER TABLE` errors; now only "duplicate column name" is tolerated.
- `BuildHistorySyncRequest` is never called with a nil anchor (it would panic); a chat with no stored message reports that instead.
- `@lid` DMs were dropped by history sync, so `/backlog` could never fill them.

### Security
- Incoming revokes are authorised: the stored message must be in the same chat, and the revoker must be one of your own devices, the original sender, or (groups) a current admin. Previously any contact could blank any stored message by ID. `MarkMessageRevoked` is also scoped by `chat_id`.
- `open` passes only images, video, audio, PDF and plain text to the system opener; other types (for example `.exe`, `.bat`, `.lnk`) are saved and the path is printed. On Windows the opener would otherwise run them.
- File names from senders are sanitised for all operating systems (path separators, `<>:"|?*`, control characters, trailing dots and spaces, reserved device names such as `CON` and `NUL`) and confined to the download directory (upstream issue 123).
- `/url` opens only `http` and `https` links with a host.

### Removed
- `release.sh` (cross-built without CGO, updated the upstream Homebrew tap). Replaced by `release.yml`; `make release` now prints the tag command.
- Release artefacts and automation for macOS, Raspberry Pi (armv5) and the Homebrew tap. They are not rebuilt for now (DECISIONS.md D4, ROADMAP row 8).
- Stale agent and doc files replaced: `agents.md` is now `AGENTS.md`; `GEMINI.md` and `.cursorrules` are pointers.

## [1.3.0] - 2026-02-21

History sync with real conversation metadata, relative timestamps and UI fixes (`76e524d`); `session_manager.go` split into three focused files (`bb7d9dd`); dead code and lock-during-I/O in logout fixed (`3c5c04c`); production README and AI assistant config files (`77343f6`); upstream sponsorship removed (`1f0c6f1`).

## [1.2.0] - 2026-02-15

Seven logic fixes (`59ecc92`), all audit findings applied (`eefceb0`, `cb71e81`, `892c09b`). See [docs/archive/audit-report-2026-02-15.md](docs/archive/audit-report-2026-02-15.md).

## [1.1.0] - 2026-02-15

Command registry replaces the 550-line switch, SQLite-only persistence, `AppContext`, 8 new message types, test suite and CI (`24fe406` to `617a95e`).

## Whatsmeow migration, 2025-03-17

Upstream pull request 118 (merged 2025-03-19). Earlier versions of this file labelled it `[Unreleased] - 2023-03-17`, which was wrong: the commit is dated 2025-03-17.

### Changed
- Migrated from the deprecated `go-whatsapp` library to `whatsmeow`.
- Rewrote session management for reliability.
- Improved message handling, error feedback and UI interactions for the new backend.

### Added
- Automatic loading of recent chats after login or session restore.
- Loading of the 20 most recent messages per chat, including text and image captions.
- Loading of WhatsApp contacts for correct name display.
- Backlog command with several history retrieval strategies (read receipt, chat presence, history sync notification).
- Friendlier error messages; more robust session restore and QR login.

### Fixed
- Nil pointer dereferences in message history retrieval.
- Type mismatches in the backlog command.
- Contact name display in messages and chat list.
- Reconnection after disconnect.

### Security
- Moved to the current WhatsApp protocol implementation and improved handling of encrypted sessions.
