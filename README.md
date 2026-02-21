<p align="center">
  <img src="/doc/screenshot.png?raw=true" alt="WhatsCLI Screenshot" width="700">
</p>

<h1 align="center">WhatsCLI</h1>

<p align="center">
  <strong>A full-featured command-line WhatsApp client built with Go</strong>
</p>

<p align="center">
  <a href="https://github.com/PalWorks/whatscli/actions/workflows/ci.yml"><img src="https://github.com/PalWorks/whatscli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/PalWorks/whatscli/releases/latest"><img src="https://img.shields.io/github/v/release/PalWorks/whatscli?color=blue" alt="Latest Release"></a>
  <a href="https://github.com/PalWorks/whatscli/blob/master/LICENSE"><img src="https://img.shields.io/github/license/PalWorks/whatscli" alt="License: MIT"></a>
  <a href="https://goreportcard.com/report/github.com/normen/whatscli"><img src="https://goreportcard.com/badge/github.com/normen/whatscli" alt="Go Report Card"></a>
</p>

<p align="center">
  Send and receive WhatsApp messages from your terminal — no browser required.<br>
  Connects directly via the WhatsApp Web multi-device API using <a href="https://github.com/tulir/whatsmeow">whatsmeow</a> and renders a responsive TUI with <a href="https://github.com/rivo/tview">tview</a>.
</p>

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
- [Deployment & Releases](#deployment--releases)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Similar Projects](#similar-projects)
- [License](#license)
- [Acknowledgements](#acknowledgements)

---

## Overview

> **Fork Notice:** This is an actively maintained fork of [normen/whatscli](https://github.com/normen/whatscli) with significant enhancements including SQLite-backed persistence, auto-reconnect, full-text search, modular command architecture, and comprehensive test coverage.

**WhatsCLI** is a terminal-based WhatsApp client that connects directly to WhatsApp's multi-device network — no browser, Electron, or phone tethering needed after the initial QR scan. It's designed for developers, sysadmins, and power users who live in the terminal and want a fast, keyboard-driven WhatsApp experience.

### Why WhatsCLI?

- **Zero-browser overhead** — runs entirely in your terminal
- **Persistent sessions** — SQLite-backed storage means your chats and messages survive restarts
- **Cross-platform** — pre-built binaries for macOS, Linux, Windows, and Raspberry Pi
- **Keyboard-first UX** — fully navigable with customizable key bindings
- **Auto-reconnect** — reconnects automatically with exponential backoff when your connection drops

### Caveats

- This is a personal messaging client, not an automation framework — there is no scripting/bot API
- WhatsApp does not officially endorse third-party clients; protocol changes may temporarily break functionality

---

## Features

| Category | Capabilities |
|----------|-------------|
| **Messaging** | Send & receive text, images, video, audio, documents, stickers, contacts, locations, and reactions |
| **Media** | Download, open, and preview attachments; send files via `/upload`, `/sendimage`, `/sendvideo`, `/sendaudio` |
| **Search** | Full-text search across all messages (`/search`) and contact search (`/search-contact`) |
| **Groups** | Create groups, add/remove members, promote/demote admins, set subjects, sync group metadata |
| **Persistence** | SQLite-backed message & conversation storage with WAL mode for performance |
| **History** | Load older messages with `/more` and `/backlog`; paginated message retrieval |
| **Connectivity** | Auto-reconnect with exponential backoff (2s → 30s, 5 attempts), manual `/connect` and `/disconnect` |
| **Notifications** | Desktop notifications via `beeep` or terminal bell — configurable per preference |
| **QR Login** | ANSI QR code rendered directly in-terminal with countdown timer and auto-refresh |
| **Customization** | INI-based config for colors, key bindings, sidebar width, download paths, and image preview commands |
| **Image Display** | External image-to-terminal rendering via configurable commands (`jp2a`, `pixterm`, etc.) |
| **Clipboard** | Copy/paste user IDs for group management commands |

---

## Architecture

WhatsCLI follows a clean separation between presentation and business logic:

```
┌──────────────────────────────────────────────────────────┐
│                    main package (TUI)                     │
│                                                          │
│  ┌──────────┐  ┌────────────┐  ┌───────────────────────┐│
│  │ main.go  │  │ ui_layout  │  │  ui_keybindings       ││
│  │ (entry)  │  │ (grid/flex)│  │  (input capture)      ││
│  └─────┬────┘  └──────┬─────┘  └──────────┬────────────┘│
│        │               │                   │             │
│  ┌─────▼───────────────▼───────────────────▼────────┐   │
│  │            AppContext (shared state)              │   │
│  └──────────────────────┬───────────────────────────┘   │
│                         │                                │
│  ┌──────────────────────▼────────────────────────────┐  │
│  │  UiHandler (implements UiMessageHandler interface)│  │
│  └──────────────────────┬────────────────────────────┘  │
└─────────────────────────┼────────────────────────────────┘
                          │  channels
┌─────────────────────────▼────────────────────────────────┐
│                  messages package                         │
│                                                          │
│  ┌────────────────┐  ┌───────────────┐  ┌─────────────┐ │
│  │ SessionManager │  │ EventHandler  │  │ Connection  │ │
│  │ (event loop)   │  │ (WA events)   │  │ (login/QR)  │ │
│  └───────┬────────┘  └───────────────┘  └─────────────┘ │
│          │                                               │
│  ┌───────▼────────┐  ┌───────────────┐  ┌─────────────┐ │
│  │ Command        │  │ MessageDB     │  │ History     │ │
│  │ Registry       │  │ (SQLite)      │  │ Sync        │ │
│  └────────────────┘  └───────────────┘  └─────────────┘ │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │  cmd_basic │ cmd_chat │ cmd_media │ cmd_group │ …  │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘

┌──────────────┐  ┌──────────────┐
│ config/      │  │ qrcode/      │
│ (INI parser) │  │ (ANSI render)│
└──────────────┘  └──────────────┘
```

**Data flow:** The TUI layer sends user commands through a `CommandChannel` to the `SessionManager`'s event loop. WhatsApp events flow back through the `UiMessageHandler` interface, keeping the two layers fully decoupled. All message persistence goes through the `MessageDatabase` (SQLite with WAL mode).

---

## Tech Stack

| Component | Technology |
|-----------|-----------|
| **Language** | Go 1.24 |
| **WhatsApp Protocol** | [whatsmeow](https://github.com/tulir/whatsmeow) (multi-device Web API) |
| **Terminal UI** | [tview](https://github.com/rivo/tview) + [tcell](https://github.com/gdamore/tcell) |
| **Database** | SQLite 3 via [go-sqlite3](https://github.com/mattn/go-sqlite3) (CGO) |
| **QR Code** | [go-qrcode](https://github.com/skip2/go-qrcode) with custom ANSI renderer |
| **Configuration** | INI format via [go-ini](https://github.com/go-ini/ini) with [XDG](https://github.com/adrg/xdg) paths |
| **Notifications** | [beeep](https://github.com/gen2brain/beeep) (cross-platform desktop notifications) |
| **Clipboard** | [clipboard](https://github.com/atotto/clipboard) |
| **Key Bindings** | [cbind](https://code.rocketnine.space/tslocum/cbind) |
| **CI/CD** | GitHub Actions (Go 1.22 + 1.24 matrix) |

---

## Installation

### Build from Source (Recommended)

**Prerequisites:** Go 1.22+ and a C compiler (GCC/Clang) for the SQLite CGO dependency.

```bash
git clone https://github.com/PalWorks/whatscli.git
cd whatscli
go build -o whatscli .
./whatscli
```

Or using the Makefile:

```bash
make build    # compile
make install  # install to $GOPATH/bin
make run      # build and run
```

### Debug Mode

Enable verbose WhatsApp protocol logging with the `--debug` flag:

```bash
whatscli --debug
```

---

## Configuration

WhatsCLI uses an INI config file located at the XDG config path:

```
~/.config/whatscli/whatscli.config
```

The file is auto-created on first run with sensible defaults. Run `/help` inside the app to see the exact path on your system.

### Configuration Sections

<details>
<summary><strong>[general]</strong> — Core application settings</summary>

| Key | Default | Description |
|-----|---------|-------------|
| `download_path` | `~/Downloads` | Where attachments are saved |
| `preview_path` | `~/Downloads` | Where previews are rendered |
| `cmd_prefix` | `/` | Prefix for slash commands |
| `show_command` | `jp2a --color` | External command for in-terminal image display |
| `enable_notifications` | `false` | Enable desktop notifications |
| `use_terminal_bell` | `false` | Ring terminal bell instead of desktop notification |
| `notification_timeout` | `60` | Seconds between repeated notifications |
| `backlog_msg_quantity` | `10` | Number of messages to load per backlog request |

</details>

<details>
<summary><strong>[keymap]</strong> — Keyboard shortcuts</summary>

| Key | Default | Action |
|-----|---------|--------|
| `switch_panels` | `Tab` | Cycle between panels |
| `focus_messages` | `Ctrl+w` | Enter message selection mode |
| `focus_input` | `Ctrl+Space` | Focus the input field |
| `focus_chats` | `Ctrl+e` | Focus the chat list |
| `command_backlog` | `Ctrl+b` | Load message backlog |
| `command_read` | `Ctrl+n` | Mark chat as read |
| `command_connect` | `Ctrl+r` | Reconnect |
| `command_quit` | `Ctrl+q` | Quit application |
| `command_help` | `Ctrl+?` | Show help |
| `copyuser` | `Ctrl+c` | Copy user ID to clipboard |
| `pasteuser` | `Ctrl+v` | Paste user ID from clipboard |
| `message_download` | `d` | Download attachment |
| `message_open` | `o` | Download & open attachment |
| `message_show` | `s` | Show image in terminal |
| `message_url` | `u` | Open URL in browser |
| `message_info` | `i` | Show message info |
| `message_revoke` | `r` | Revoke message |

</details>

<details>
<summary><strong>[ui]</strong> — Layout settings</summary>

| Key | Default | Description |
|-----|---------|-------------|
| `chat_sidebar_width` | `30` | Width of the chat list sidebar in columns |

</details>

<details>
<summary><strong>[colors]</strong> — Theme customization</summary>

| Key | Default | Description |
|-----|---------|-------------|
| `background` | `black` | Main background color |
| `text` | `white` | Default text color |
| `forwarded_text` | `purple` | Forwarded message indicator |
| `list_header` | `yellow` | Chat list section headers |
| `list_contact` | `green` | Contact names in chat list |
| `list_group` | `blue` | Group names in chat list |
| `chat_contact` | `green` | Contact names in message view |
| `chat_me` | `blue` | Your name in message view |
| `borders` | `white` | UI border color |
| `input_background` | `blue` | Input field background |
| `input_text` | `white` | Input field text |
| `unread_count` | `yellow` | Unread message count badge |
| `positive` | `green` | Positive status indicators |
| `negative` | `red` | Error/offline indicators |

Use `/colorlist` in-app to see all available color names.

</details>

---

## Usage

### Quick Start

```bash
# 1. Build from source
git clone https://github.com/PalWorks/whatscli.git
cd whatscli && make build

# 2. Launch
./whatscli

# 3. Scan the QR code with WhatsApp on your phone
#    (WhatsApp → Settings → Linked Devices → Link a Device)

# 4. Start chatting!
#    Select a chat with Tab → Arrow keys → Enter
#    Type your message and press Enter to send
```

### First Login

On first launch, WhatsCLI displays a QR code directly in your terminal. Scan it with WhatsApp on your phone (**Settings → Linked Devices → Link a Device**). If the QR code doesn't fit, reduce your terminal font size or increase the window dimensions.

After the initial scan, WhatsCLI reconnects automatically on subsequent launches — no re-scanning needed.

### Navigating the UI

```
┌─────────────────────────────────────────────────────────┐
│  WhatsCLI v1.3.0  Type /help or press Ctrl+? for help   │
├──────────────┬──────────────────────────────────────────┤
│              │                                          │
│  Chat List   │          Message View                    │
│  (sidebar)   │                                          │
│              │  [12:01] Alice: Hey!                      │
│  > Alice  2  │  [12:02] You: Hi there                   │
│    Bob       │  [12:05] Alice: Check this out            │
│    Team Chat │                                          │
│              │                                          │
├──────────────┼──────────────────────────────────────────┤
│  Status      │  > Type a message or /command...          │
└──────────────┴──────────────────────────────────────────┘
```

- **`Tab`** — Switch between chat list, status panel, and input field
- **`Ctrl+w`** — Enter message selection mode (select individual messages)
- **`Up`/`Down`** — Scroll messages or navigate the chat list
- **`Ctrl+p`** — Toggle mouse support

### Sending Media

```bash
/sendimage /path/to/photo.jpg       # Send an image
/sendvideo /path/to/clip.mp4        # Send a video
/sendaudio /path/to/voice.ogg       # Send audio
/upload /path/to/document.pdf       # Send any file as document
```

> **Tip:** Paths with spaces work without quoting — just type the path directly.

### Viewing Images in Terminal

Configure an external program to render images as UTF characters:

```ini
# In whatscli.config [general] section:
show_command = jp2a --color           # Basic ASCII art (most compatible)
show_command = pixterm -s 2           # True-color rendering (recommended)
```

Install the viewer: `sudo apt install jp2a` or see [PIXterm](https://github.com/eliukblau/pixterm) for high-quality rendering.

---

## Command Reference

All commands use the configurable prefix (default: `/`).

### Application & Connection

| Command | Description |
|---------|-------------|
| `/help` | Show help and key bindings |
| `/connect` | (Re)connect to WhatsApp |
| `/disconnect` | Disconnect from WhatsApp |
| `/logout` | Log out and delete session data |
| `/quit` | Exit WhatsCLI |
| `/colorlist` | Display available color names |

### Chat & Messages

| Command | Description |
|---------|-------------|
| `/more` | Load more messages from local storage |
| `/backlog` | Sync and load historical messages from WhatsApp |
| `/read` | Mark the current chat as read |
| `/info` | Show message details (when a message is selected) |
| `/search <keyword>` | Full-text search across all messages |
| `/search-contact <name>` | Search for contacts by name |

### Media

| Command | Description |
|---------|-------------|
| `/sendimage <path>` | Send an image |
| `/sendvideo <path>` | Send a video |
| `/sendaudio <path>` | Send an audio file |
| `/upload <path>` | Send any file as a document |

### Group Management

| Command | Description |
|---------|-------------|
| `/create <numbers> <subject>` | Create a new group |
| `/subject <text>` | Change group subject |
| `/leave` | Leave the current group |
| `/add <userid>` | Add a member to a group |
| `/remove <userid>` | Remove a member from a group |
| `/admin <userid>` | Promote a member to admin |
| `/removeadmin <userid>` | Demote an admin |
| `/sync-groups` | Sync group metadata from WhatsApp |

---

## Key Bindings

All key bindings are configurable in `whatscli.config` under the `[keymap]` section.

| Default Binding | Context | Action |
|----------------|---------|--------|
| `Tab` | Global | Switch between panels |
| `Ctrl+w` | Global | Enter message selection mode |
| `Ctrl+Space` | Global | Focus input field |
| `Ctrl+e` | Global | Focus chat list |
| `Ctrl+b` | Global | Load backlog |
| `Ctrl+n` | Global | Mark chat as read |
| `Ctrl+r` | Global | Reconnect |
| `Ctrl+q` | Global | Quit |
| `Ctrl+?` | Global | Show help |
| `Ctrl+p` | Global | Toggle mouse |
| `Ctrl+c` | Global | Copy user ID |
| `Ctrl+v` | Global | Paste user ID |
| `d` | Message selected | Download attachment |
| `o` | Message selected | Download & open |
| `s` | Message selected | Show image in terminal |
| `u` | Message selected | Open URL |
| `i` | Message selected | Show info |
| `r` | Message selected | Revoke message |

---

## Project Structure

```
whatscli/
├── main.go                  # Entry point — initializes config, session, and TUI grid
├── app_context.go           # AppContext struct — centralized shared UI state
├── ui_layout.go             # Layout construction (flex containers, tables, sidebar)
├── ui_handler.go            # UiHandler — implements UiMessageHandler interface
├── ui_keybindings.go        # Key binding setup, input capture, and shortcuts
├── ui_render.go             # Message and chat list rendering logic
├── ui_render_test.go        # UI rendering tests
├── ui_helpers.go            # Shared UI utilities (help text, status bar, error printing)
│
├── messages/                # Business logic package (no TUI dependency)
│   ├── messages.go          # Shared interfaces (UiMessageHandler) and data structs
│   ├── session_manager.go   # SessionManager — event loop, command dispatch, lifecycle
│   ├── connection.go        # WhatsApp connection, login, QR code flow, auto-reconnect
│   ├── event_handler.go     # WhatsApp event processing (messages, receipts, presence)
│   ├── history_sync.go      # History synchronization with WhatsApp servers
│   ├── chat_state.go        # Chat state management (current chat, conversations)
│   ├── storage.go           # MessageDatabase — SQLite persistence layer
│   ├── cmd_registry.go      # Command registry — maps names to handler functions
│   ├── cmd_basic.go         # Core commands: /help, /quit, /more, /info, /read
│   ├── cmd_chat.go          # Chat commands: /send, /select, /backlog, /sync-groups
│   ├── cmd_connection.go    # Connection commands: /connect, /disconnect, /logout
│   ├── cmd_media.go         # Media commands: /sendimage, /sendvideo, /sendaudio, /upload
│   ├── cmd_group.go         # Group commands: /create, /add, /remove, /admin, /leave
│   ├── cmd_search.go        # Search commands: /search, /search-contact
│   ├── priority_queue.go    # Heap-based priority queue for conversation ordering
│   ├── mock_ui_handler.go   # Mock UiHandler for testing
│   ├── *_test.go            # Unit tests (storage, commands, message handling, queue)
│
├── config/                  # Configuration package
│   ├── settings.go          # INI config loading with XDG paths and defaults
│   └── settings_test.go     # Config tests
│
├── qrcode/                  # QR code rendering
│   └── qrcode.go            # ANSI QR code generator for terminal display
│
├── doc/                     # Documentation assets
│   └── screenshot.png       # Application screenshot
│
├── .github/
│   ├── workflows/ci.yml     # CI pipeline (build + test on Go 1.22 & 1.24)
│   ├── FUNDING.yml          # Sponsorship configuration
│   └── PULL_REQUEST_TEMPLATE.md
│
├── Makefile                 # Build, test, vet, install, and release targets
├── release.sh               # Cross-platform release script (Linux, macOS, Windows, RPi)
├── go.mod / go.sum          # Go module definitions
├── CONTRIBUTING.md          # Contributor guide with command authoring walkthrough
├── CHANGELOG.md             # Version history
└── .gitignore
```

---

## Testing

WhatsCLI uses Go's standard testing framework with race detection enabled.

```bash
# Run all tests with race detector
make test

# Run go vet for static analysis
make vet

# Build to verify compilation
go build ./...
```

### Test Strategy

- **Unit tests** cover the SQLite storage layer (`storage_test.go`), message handling (`msg_handler_test.go`), media commands (`cmd_media_test.go`), session lifecycle (`session_manager_test.go`), priority queue logic (`priority_queue_test.go`), rendering (`ui_render_test.go`), and configuration (`settings_test.go`)
- Tests use **in-memory SQLite** (`:memory:`) via `InitWithDB()` for fast, isolated execution
- A `MockUiHandler` in `mock_ui_handler.go` stubs the UI interface for backend tests
- **Race detection** (`-race`) is enabled by default in the Makefile and CI

### CI Pipeline

Every push and PR to `master` triggers the GitHub Actions CI workflow:

1. **Matrix build** across Go 1.22 and 1.24
2. `go mod download` + `go mod verify`
3. `go vet ./...`
4. `go build -v ./...`
5. `go test -v -race -count=1 ./...`

---

## Deployment & Releases

### Release Process

Releases are cut using the `release.sh` script, which:

1. Reads the version from `main.go` (`VERSION` constant)
2. Cross-compiles binaries for **4 platforms**:
   - `darwin/amd64` (macOS)
   - `linux/amd64`
   - `windows/amd64`
   - `linux/arm` ARMv5 (Raspberry Pi)
3. Packages each binary into a zip archive
4. Creates a GitHub Release with auto-generated changelogs
5. Updates the Homebrew tap formula with the new SHA256 and URL

```bash
# Release using version from main.go
make release

# Or specify a version manually
./release.sh v1.4.0
```

### Session Data Location

WhatsCLI stores session and message data in the XDG config directory:

```
~/.config/whatscli/
├── whatscli.config           # User configuration (INI)
├── session.db                # WhatsApp device session (whatsmeow SQLite store)
└── session_meta.db           # Message & conversation metadata (app SQLite store)
```

---

## Roadmap

> **Note:** This roadmap is inferred from the codebase and recent development activity. Features are subject to change.

- [ ] End-to-end encrypted message backup/export
- [ ] Reaction display in message view
- [ ] Voice message recording from terminal
- [ ] Sticker sending support
- [ ] Message reply threading in the UI
- [ ] Configurable message display limits
- [ ] Status/story viewing
- [ ] Contact vCard display improvements

---

## Contributing

Contributions are welcome! WhatsCLI uses a **command registry pattern** that makes adding new slash commands straightforward.

See [**CONTRIBUTING.md**](CONTRIBUTING.md) for:
- Step-by-step guide to adding new commands
- Architecture overview and key resources
- Testing conventions and code style guidelines

### Quick Contribution Workflow

```bash
# Fork and clone
git clone https://github.com/<you>/whatscli.git
cd whatscli

# Create a feature branch
git checkout -b feature/my-command

# Make changes, add tests
make test
make vet

# Commit and push
git add -A
git commit -m "feat: add /mycommand for XYZ"
git push origin feature/my-command

# Open a Pull Request against PalWorks/whatscli
```

---

## Similar Projects

| Project | Description |
|---------|-------------|
| [Nchat](https://github.com/d99kris/nchat) | Terminal-based chat client supporting WhatsApp and Telegram with more features |

---

## License

This project is licensed under the [MIT License](LICENSE).

> You have the freedom to use, modify, and distribute this software — just don't slap your name on it as the original author.

---

## Acknowledgements

- **[whatsmeow](https://github.com/tulir/whatsmeow)** — The Go library powering the WhatsApp multi-device protocol implementation
- **[tview](https://github.com/rivo/tview)** — Rich interactive TUI framework for Go
- **[tcell](https://github.com/gdamore/tcell)** — Low-level terminal cell library
- **[go-sqlite3](https://github.com/mattn/go-sqlite3)** — SQLite3 driver for Go's `database/sql`
- **[beeep](https://github.com/gen2brain/beeep)** — Cross-platform desktop notification library
- **[go-qrcode](https://github.com/skip2/go-qrcode)** — QR code generator used for terminal-rendered login codes
- **[cbind](https://code.rocketnine.space/tslocum/cbind)** — Key binding configuration library


