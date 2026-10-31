<p align="center">
  <img src="/doc/screenshot.png?raw=true" alt="WhatsCLI Screenshot" width="700">
</p>

<h1 align="center">WhatsCLI</h1>

<p align="center">
  <strong>A keyboard-driven WhatsApp client for your terminal, written in Go</strong>
</p>

<p align="center">
  <a href="https://github.com/PalWorks/whatscli/actions/workflows/ci.yml"><img src="https://github.com/PalWorks/whatscli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/PalWorks/whatscli/releases/latest"><img src="https://img.shields.io/github/v/release/PalWorks/whatscli?color=blue" alt="Latest Release"></a>
  <a href="https://goreportcard.com/report/github.com/PalWorks/whatscli"><img src="https://goreportcard.com/badge/github.com/PalWorks/whatscli" alt="Go Report Card"></a>
</p>

<p align="center">
  Read and send WhatsApp messages from a terminal, with no browser. Connects as a linked device through <a href="https://github.com/tulir/whatsmeow">whatsmeow</a> and draws its interface with <a href="https://github.com/rivo/tview">tview</a>.
</p>

_Last updated: 2026-10-31T17:01:57+00:00 (sender names)_

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Command Reference](#command-reference)
- [Key Bindings](#key-bindings)
- [Project Structure](#project-structure)
- [Testing](#testing)
- [Deployment and Releases](#deployment-and-releases)
- [Security](#security)
- [Roadmap](#roadmap)
- [Documentation Map and Agent Notes](#documentation-map-and-agent-notes)
- [Contributing](#contributing)
- [License](#license)
- [Acknowledgements](#acknowledgements)

---

## Overview

WhatsCLI links to your WhatsApp account the same way WhatsApp Web does (QR scan, then it reconnects on its own) and gives you chats, groups, media, search and message history in a terminal. It is for developers, administrators and keyboard users who want WhatsApp without a browser or desktop app.

> **Fork notice.** This repository is an independent fork of [normen/whatscli](https://github.com/normen/whatscli). Upstream is semi-dormant (last push 2026-08-08, v1.1.6). Fixes are ported selectively, never merged wholesale.

### Caveats

- A personal client, not an automation framework: there is no scripting or bot API.
- WhatsApp does not endorse third-party clients; a protocol change can break features until whatsmeow is updated.
- No automated test talks to WhatsApp. Media, revoke, backlog and `@lid` behaviour is covered by unit tests with fakes and by a manual live-account checklist ([TESTING.md](TESTING.md)); its runs are recorded in TESTING.md Table T3 (the latest, on the `v1.6.0-rc1` candidate, covered part of it).
- Messages and your linked-device session are stored unencrypted on disk (see [Security](#security)).

---

## Features

**Table F1: Features**

| Category | Capabilities |
|---|---|
| Messaging | Send text, images, video, audio and documents. Receive text, images, video, audio, documents, stickers, contacts, locations and reactions (shown as message content) |
| Media | Download (`d`), download and open (`o`), show images in the terminal (`s`), open the first link (`u`). Media you send is stored and can be downloaded again. A repeat download reuses the file saved before; temp files left by a download cut short by quitting are removed at the next start |
| Safety on media | Only images, video, audio, PDF and plain text are opened automatically; other types are saved and the path is printed. Sender file names are sanitised and confined to the download folder |
| Delete for everyone | `r` or `/revoke` on your own messages; deletions from your phone, a contact or a group admin update the local copy (checked, see [Security](#security)) |
| Delete for me | "Delete for me" done on your phone while WhatsCLI is linked removes the message here too and updates the chat preview |
| History | A chat opens with its newest 200 messages; `/more` shows 200 older ones from local storage; `/backlog` asks your phone for older messages of the open chat |
| Search | `/search` over messages, `/search-contact` over contacts |
| Groups | Create, rename, leave, add, remove, promote, demote, sync metadata |
| Persistence | SQLite with WAL; chats and messages survive restarts |
| Connectivity | Auto-reconnect with exponential backoff (2 s to 30 s, up to 5 attempts); manual `/connect`, `/disconnect` |
| Identity | Chats that WhatsApp addresses by `@lid` are stored under the phone-number chat when the mapping is known |
| Notifications | Desktop notification or terminal bell, configurable |
| QR login | ANSI QR code with full quiet zone, countdown and refresh |
| Customisation | INI file for colours, key bindings, sidebar width, download and preview paths, image viewer command |
| Clipboard | Copy and paste user IDs for group commands; a missing clipboard backend shows an error instead of crashing |

---

## Architecture

Two packages with a clean boundary: the TUI in the root package and everything WhatsApp-related in `messages/`.

```
 main package (tview)  --Command on CommandChannel-->  messages: SessionManager.runManager()
   AppContext, UiHandler  <--UiMessageHandler (18 methods)--   eventHandler, history_sync, commandRegistry,
                                                              MessageDatabase (SQLite)  -->  whatsmeow  -->  WhatsApp
```

All persistence goes through `MessageDatabase`; the WhatsApp session is kept separately by whatsmeow. Full design, concurrency model, storage schema and JID handling: [ARCHITECTURE.md](ARCHITECTURE.md).

---

## Tech Stack

**Table T1: Stack**

| Component | Technology |
|---|---|
| Language | Go (directive 1.26.0, toolchain go1.27.3), CGO required |
| WhatsApp protocol | [whatsmeow](https://github.com/tulir/whatsmeow) (multi-device) |
| Terminal UI | [tview](https://github.com/rivo/tview), [tcell](https://github.com/gdamore/tcell) |
| Database | SQLite via [go-sqlite3](https://github.com/mattn/go-sqlite3) |
| QR code | [go-qrcode](https://github.com/skip2/go-qrcode) with an ANSI renderer |
| Config | INI via [go-ini](https://github.com/go-ini/ini), paths via [xdg](https://github.com/adrg/xdg) |
| Notifications | [beeep](https://github.com/gen2brain/beeep) |
| Clipboard | [clipboard](https://github.com/atotto/clipboard) |
| Key bindings | [cbind](https://codeberg.org/tslocum/cbind) |
| System opener | Built in (`ui_opener.go`): `xdg-open` on Linux, `open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows, no shell |
| CI and release | GitHub Actions with actions pinned to commit SHAs (Dependabot, weekly); Go 1.26 and 1.27 test matrix; tag-driven release |

---

## Installation

### Prebuilt binaries

Each release on [GitHub Releases](https://github.com/PalWorks/whatscli/releases) has:

**Table I1: Release assets (names from `.github/workflows/release.yml`, checked against the v1.6.2 release)**

| Platform | Asset (v1.6.2) | Contains |
|---|---|---|
| Linux x86_64 | `whatscli-v1.6.2-linux-amd64.tar.gz` | `whatscli` |
| Linux arm64 | `whatscli-v1.6.2-linux-arm64.tar.gz` | `whatscli` |
| Windows x86_64 | `whatscli-v1.6.2-windows-amd64.zip` | `whatscli.exe` |
| Checksums | `whatscli-v1.6.2-checksums.txt` | SHA256 of the three archives |

Other versions follow the same pattern with their tag. The latest release is v1.6.2; tags with a `-` (such as `v1.6.2-rc1`) are pre-releases and never marked Latest.

```bash
# with the GitHub CLI
gh release download v1.6.2 --repo PalWorks/whatscli \
  --pattern 'whatscli-v1.6.2-linux-amd64.tar.gz' --pattern 'whatscli-v1.6.2-checksums.txt'
# or with curl
curl -LO https://github.com/PalWorks/whatscli/releases/download/v1.6.2/whatscli-v1.6.2-linux-amd64.tar.gz
curl -LO https://github.com/PalWorks/whatscli/releases/download/v1.6.2/whatscli-v1.6.2-checksums.txt

sha256sum -c --ignore-missing whatscli-v1.6.2-checksums.txt
tar -xzf whatscli-v1.6.2-linux-amd64.tar.gz      # extracts ./whatscli
./whatscli
```

macOS, Raspberry Pi (armv5) and Homebrew are not released for now (ROADMAP row 8); macOS users build from source. The Windows build has not been run through the live-account checklist (TESTING.md Table T3).

### With go install

```bash
go install github.com/PalWorks/whatscli@latest
```

Needs a C compiler (SQLite is built through CGO). `@latest` resolves to the newest release tag; the Go module proxy can take a short while to pick up a new tag (on 2026-10-31 it still returned `v1.6.1` minutes after `v1.6.2` was published). From v1.6.1 a binary built this way shows the module version it was installed at (for example `v1.6.1`) in the top bar and `/help`; release binaries and `make install` show the tag they were built from.

### From source

```bash
git clone https://github.com/PalWorks/whatscli.git
cd whatscli
make build        # ./whatscli, version from git describe
make install      # into $(go env GOPATH)/bin, version from git describe
make run          # go run .
```

**Prerequisites:** Go 1.21 or newer with the default `GOTOOLCHAIN=auto`: `go.mod` asks for `go 1.26.0` and `toolchain go1.27.3`, so an older Go downloads go1.27.3 by itself. Go 1.26 also builds the code with `GOTOOLCHAIN=local` (CI tests 1.26 and 1.27). Plus gcc or clang; Linux: `sudo apt install gcc`.

### Optional helpers

- `jp2a` or another command for inline images (`show_command`).
- `xclip`, `xsel` or `wl-clipboard` for copy and paste on Linux.
- `xdg-utils` (`xdg-open`) for `open` and `url` on Linux.

### Debug mode

```bash
whatscli --debug     # verbose WhatsApp protocol logging
```

---

## Configuration

INI file, created with defaults on first run: `~/.config/whatscli/whatscli.config` (`/help` prints the exact path).

<details>
<summary><strong>[general]</strong></summary>

**Table C1: [general]**

| Key | Default | Description |
|---|---|---|
| `download_path` | `~/Downloads` | Where `d` saves attachments |
| `preview_path` | `~/Downloads` | Where `o` and `s` save files before opening or showing them |
| `cmd_prefix` | `/` | Prefix for slash commands |
| `show_command` | `jp2a --color` | Command that renders an image in the terminal |
| `enable_notifications` | `false` | Desktop notifications |
| `use_terminal_bell` | `false` | Ring the bell instead |
| `notification_timeout` | `60` | Seconds between repeated notifications |
| `backlog_msg_quantity` | `10` | Messages requested per `/backlog` (50 if not positive) |

</details>

<details>
<summary><strong>[keymap]</strong></summary>

**Table C2: [keymap]**

| Key | Default | Action |
|---|---|---|
| `switch_panels` | `Tab` | Cycle panels |
| `focus_messages` | `Ctrl+w` | Enter message selection |
| `focus_input` | `Ctrl+Space` | Focus input |
| `focus_chats` | `Ctrl+e` | Focus chat list |
| `command_backlog` | `Ctrl+b` | `/backlog` |
| `command_read` | `Ctrl+n` | `/read` |
| `command_connect` | `Ctrl+r` | `/connect` |
| `command_quit` | `Ctrl+q` | Quit |
| `command_help` | `Ctrl+?` | Help |
| `copyuser` | `Ctrl+c` | Copy user ID |
| `pasteuser` | `Ctrl+v` | Paste user ID |
| `message_download` | `d` | Download |
| `message_open` | `o` | Download and open |
| `message_show` | `s` | Show image |
| `message_url` | `u` | Open link |
| `message_info` | `i` | Chat info (the open chat or group) |
| `message_revoke` | `r` | Delete for everyone (own messages) |

</details>

<details>
<summary><strong>[ui]</strong> and <strong>[colors]</strong></summary>

**Table C3: [ui] and [colors]**

| Section | Key | Default |
|---|---|---|
| `[ui]` | `chat_sidebar_width` | `30` |
| `[colors]` | `background` | `black` |
| `[colors]` | `text` | `white` |
| `[colors]` | `forwarded_text` | `purple` |
| `[colors]` | `list_header` | `yellow` |
| `[colors]` | `list_contact` | `green` |
| `[colors]` | `list_group` | `blue` |
| `[colors]` | `chat_contact` | `green` |
| `[colors]` | `chat_me` | `blue` |
| `[colors]` | `borders` | `white` |
| `[colors]` | `input_background` | `blue` |
| `[colors]` | `input_text` | `white` |
| `[colors]` | `unread_count` | `yellow` |
| `[colors]` | `positive` | `green` |
| `[colors]` | `negative` | `red` |

`/colorlist` shows the available colour names.

</details>

---

## Usage

### First login

```bash
whatscli
```

A QR code appears in the terminal. On your phone: WhatsApp, Settings, Linked Devices, Link a Device, scan. If the QR does not fit, enlarge the window or shrink the font. After the first scan WhatsCLI reconnects by itself.

### Moving around

- `Tab` cycles panels; select a chat with the arrow keys and Enter; type and press Enter to send. A chat opens at its latest message; scrolling up stays where you put it until you open another chat.
- Type `/` on the input line to see the commands with a short description; keep typing to narrow the list (`/lo` shows `/login` and `/logout`), move with `Up` and `Down`, pick with `Tab` or `Enter`. Every command you enter is echoed in the message pane with its time, for example `(31-10-26 21:28:40) > /logout` (a command that redraws the chat, such as `/select`, replaces the echo with the chat).
- The Status box lists the WhatsApp Status updates (stories) of your contacts that arrived while WhatsCLI was linked, not your own status. History sync does not include them, so it stays empty until a contact posts one.
- `Ctrl+w` enters message selection; `Up` and `Down` (or `k` and `j`, `g`, `G`, `Ctrl+u`, `Ctrl+d`) move the highlight. With a message highlighted press `d`, `o`, `s`, `u`, `i` or `r`.
- `Ctrl+p` toggles mouse support (off lets the terminal select text). With the mouse on, click a chat to open it, use the wheel to scroll, and click or drag the scroll bar (a thick line on the right border of a panel whose content is longer than the panel).
- Message senders show the name saved in your phone's contacts, else their phone number (`+91...`). A sender whose phone number WhatsApp keeps private shows their WhatsApp name as `~Name`.
- In the chat lists, the right column shows the unread count, if any, and the time of the last message; long names are shortened to keep it visible.

### Working with attachments

```text
Ctrl+w, highlight a PDF, press d   ->  [ -> /home/you/Downloads/report.pdf ]
highlight a photo, press s         ->  rendered by show_command (images only)
highlight an .exe, press o         ->  Saved but not opened: .exe files can run code or macros; only images,
                                       video, audio, PDF and plain text are opened automatically. ...
```

If a different file already has the same name, the new one is saved as `name_<message-id>.ext`. Messages stored before the upgrade have no download data and report "this is not a downloadable message".

### Sending media

```bash
/sendimage /path/to/photo.jpg
/sendvideo /path/to/clip.mp4
/sendaudio /path/to/voice.ogg
/upload /path/to/document.pdf
```

### Older messages

```text
/more      older messages already in local storage
/backlog   ask the phone for older messages of this chat (about 5 s wait, refreshes when they arrive)
```

### Images in the terminal

```ini
# whatscli.config [general]
show_command = jp2a --color
```

---

## Command Reference

Commands use the configurable prefix (default `/`). The `<msg-id>` commands are normally triggered by the keys above, which send the highlighted message's ID.

**Table CR1: Application and connection**

| Command | Description |
|---|---|
| `/help` | Help, key bindings, config path, version |
| `/connect` (alias of `/login`) | (Re)connect |
| `/disconnect` | Disconnect |
| `/logout` | Log out and delete session data |
| `/reset` | Delete the WhatsApp session database and the local message database |
| `/quit` | Exit |
| `/colorlist` | Show colour names |

**Table CR2: Chats and messages**

| Command | Description |
|---|---|
| `/select <chat-id>` | Open a chat |
| `/send <chat-id> <text>` | Send text to a chat |
| `/more` | Show 200 older messages from local storage |
| `/backlog` | Request older messages from the phone |
| `/read` | Mark the chat as read |
| `/info` | Show chat or group info |
| `/download <msg-id>` | Save an attachment to `download_path` |
| `/open <msg-id>` | Save to `preview_path` and open (allowlisted types only) |
| `/show <msg-id>` | Save and render an image |
| `/url <msg-id>` | Open the first http or https link |
| `/revoke <msg-id>` | Delete your own message for everyone |
| `/search <keyword>` | Search messages |
| `/search-contact <name>` | Search contacts |

**Table CR3: Media**

| Command | Description |
|---|---|
| `/sendimage <path>` | Send an image |
| `/sendvideo <path>` | Send a video |
| `/sendaudio <path>` | Send audio |
| `/upload <path>` | Send a file as a document |

**Table CR4: Groups**

| Command | Description |
|---|---|
| `/create <numbers> <subject>` | Create a group |
| `/subject <text>` | Change the subject |
| `/leave` | Leave the group |
| `/add <userid>` | Add a member |
| `/remove <userid>` | Remove a member |
| `/admin <userid>` (alias `/promote`) | Promote |
| `/removeadmin <userid>` (alias `/demote`) | Demote |
| `/sync-groups` | Sync group metadata |

---

## Key Bindings

Configurable under `[keymap]`.

**Table K1: Global keys**

| Key | Action |
|---|---|
| `Tab` | Switch panel |
| `Ctrl+w` | Message selection |
| `Ctrl+Space` | Focus input |
| `Ctrl+e` | Focus chat list |
| `Ctrl+b` | Backlog |
| `Ctrl+n` | Mark read |
| `Ctrl+r` | Reconnect |
| `Ctrl+q` | Quit |
| `Ctrl+?` | Help |
| `Ctrl+p` | Toggle mouse |
| `Ctrl+c` / `Ctrl+v` | Copy and paste user ID |

**Table K2: Keys with a message highlighted**

| Key | Action |
|---|---|
| `d` | Download |
| `o` | Download and open |
| `s` | Show image |
| `u` | Open link |
| `i` | Chat info |
| `r` | Revoke (own messages) |
| `k` `j` `g` `G` `Ctrl+u` `Ctrl+d` | Move the highlight |

---

## Project Structure

```
whatscli/
├── main.go, app_context.go          # entry point, shared UI state
├── ui_layout.go, ui_handler.go      # layout, UiMessageHandler implementation
├── ui_keybindings.go, ui_render.go, ui_helpers.go, ui_opener.go
├── ui_*_test.go
├── messages/                        # business logic (no TUI dependency)
│   ├── session_manager.go, connection.go, event_handler.go, history_sync.go, chat_state.go
│   ├── chat_view.go, delete_for_me.go, lid_merge.go, partfiles.go
│   ├── storage.go, priority_queue.go, messages.go, mock_ui_handler.go
│   ├── media.go, jid_norm.go, revoke_incoming.go, pathcheck.go
│   ├── cmd_registry.go and cmd_basic, cmd_connection, cmd_chat, cmd_group, cmd_media,
│   │   cmd_message, cmd_search .go
│   └── *_test.go
├── config/                          # INI config, XDG paths
├── qrcode/                          # ANSI QR renderer
├── doc/screenshot.png
├── docs/archive/                    # historic reviews (Feb 2026), unchanged
├── .github/workflows/ci.yml, release.yml; .github/dependabot.yml
├── .github/scripts/check-spdx.sh, previous-tag.sh, generate-aur-pkgbuilds.sh
├── Makefile, go.mod, go.sum, LICENSE, LICENSE-COMMERCIAL.md
└── AGENTS.md ARCHITECTURE.md CHANGELOG.md CONTRIBUTING.md
    ROADMAP.md SECURITY.md SKILLS.md TESTING.md (CLAUDE.md, GEMINI.md, .cursorrules point to AGENTS.md)
```

---

## Testing

```bash
make test    # go test -race ./...
make vet     # go vet ./...
```

178 top-level unit tests across the packages, using in-memory SQLite, a mock UI and fakes for downloads, LID mapping and group metadata. CI checks SPDX headers, then runs vet, build and `go test -race` on Go 1.26 and 1.27. Because nothing tests against real WhatsApp, a 26-step live-account checklist gates each release. Details: [TESTING.md](TESTING.md).

---

## Deployment and Releases

Releases are built by `.github/workflows/release.yml`. `release.sh` and the Homebrew update are gone.

1. Merge to `master` with CI green; run the live checklist ([TESTING.md](TESTING.md)).
2. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. The workflow validates the tag, builds natively with CGO (linux/amd64, linux/arm64, windows/amd64) with the version injected, packages archives, writes SHA256 checksums, and creates the GitHub release with notes from `git log` since the previous tag (`.github/scripts/previous-tag.sh`: the previous final tag for a final release, the previous tag of any kind for a release candidate). A tag with a `-` (for example `v1.6.0-rc1`) becomes a pre-release and is never marked Latest. It also generates AUR `PKGBUILD` and `.SRCINFO` as a workflow artifact (not published).
4. Re-run for an existing tag: Actions, Release, Run workflow, enter the tag.
5. Download a binary, run it, confirm the version in the top bar.

`make release` only prints the tagging command.

### Where your data lives

```
~/.config/whatscli/
├── whatscli.config     # settings
├── session.db          # WhatsApp linked-device session (whatsmeow)
└── session_meta.db     # chats and messages (WhatsCLI)
```

---

## Security

Short version: incoming "delete for everyone" requests are checked before they change anything; `open` runs only a short list of safe file types; sender file names cannot escape the download folder; `/url` opens only http and https links. Your session and message history are stored unencrypted in `~/.config/whatscli`; keep it private. Threat notes and how to report a vulnerability: [SECURITY.md](SECURITY.md).

---

## Roadmap

The full backlog, with priorities and owner decisions, is in [ROADMAP.md](ROADMAP.md). Headlines:

- Shipped in v1.6.0: chat paging and `/more`, non-blocking `/info`, `show` and `/url`, Delete for me, repeat downloads reuse the saved file, part-file sweep, SHA-pinned CI.
- Shipped in v1.6.1: command drop-down on the input line, timestamped command echo, chats open at their latest message.
- Shipped in v1.6.2: scroll bars with click and drag, a chat list that no longer jumps when clicked and keeps times visible.
- Open: row 14, whether the phone answers `/backlog` in a chat whose own record is LID (needs an `@lid` chat on a live account).
- Release gate: the live-account checklist before every tag (row 15, recurring).
- Deferred: macOS, Raspberry Pi and Homebrew releases (row 8), Delete for me made while unlinked (row 35), security hardening rows 18 to 22, `

---

## Documentation Map and Agent Notes

**Table D1: Documents**

| File | Purpose |
|---|---|
| [AGENTS.md](AGENTS.md) | Canonical contract for AI agents; `CLAUDE.md`, `GEMINI.md`, `.cursorrules` point to it |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Components, data flow, storage, concurrency, JIDs |
| [ROADMAP.md](ROADMAP.md) | Everything not yet done |
| [CHANGELOG.md](CHANGELOG.md) | What shipped |
| [TESTING.md](TESTING.md) | Tests and the live checklist |
| [SECURITY.md](SECURITY.md) | Threats, controls, reporting |
| [CONTRIBUTING.md](CONTRIBUTING.md), [SKILLS.md](SKILLS.md) | How to contribute, patterns with code |
| [docs/archive/](docs/archive/) | February 2026 audit and review reports, kept unchanged for history |

Agents: read AGENTS.md first, keep these documents current in the same change as the code, and do not run against a real WhatsApp account.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for adding a command, tests and style.

```bash
git checkout -b feature/my-change
make test && make vet      # both must pass
git commit -m "feat: describe the change"
# open a pull request against PalWorks/whatscli
```

---

## License

WhatsCLI is dual licensed (updated 2026-10-30T19:50:43+00:00):

- [GNU AGPL-3.0-only](LICENSE) for open source use.
- A paid commercial license for use without the AGPL obligations. See [LICENSE-COMMERCIAL.md](LICENSE-COMMERCIAL.md) or write to support@palworks.ai.

---

## Acknowledgements

- [whatsmeow](https://github.com/tulir/whatsmeow), the WhatsApp multi-device library.
- [tview](https://github.com/rivo/tview) and [tcell](https://github.com/gdamore/tcell), the terminal UI.
- [go-sqlite3](https://github.com/mattn/go-sqlite3), [beeep](https://github.com/gen2brain/beeep), [go-qrcode](https://github.com/skip2/go-qrcode), [cbind](https://codeberg.org/tslocum/cbind).
- [normen/whatscli](https://github.com/normen/whatscli), the original project this fork started from.
- Similar project with more features: [nchat](https://github.com/d99kris/nchat) (WhatsApp, Telegram and more).
