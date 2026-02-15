# whatscli

A command line interface for WhatsApp, based on [whatsmeow](https://github.com/tulir/whatsmeow) and [tview](https://github.com/rivo/tview)

![whatscli-screenshot](/doc/screenshot.png?raw=true "WhatsCLI")

## Features

Things that work.

- Sending and receiving WhatsApp messages in a command line app
- Connects through the WhatsApp Web multi-device API without a browser
- Uses QR code for simple setup
- **Rich message types**: text, images, video, audio, documents, stickers, contacts, locations, and reactions
- Allows downloading and opening image/video/audio/document attachments
- Allows sending images, video, audio, and documents
- Allows color customization
- Allows basic group management (create, add/remove members, sync)
- Supports desktop notifications and terminal bell
- **SQLite-backed persistence**: chats and messages are saved across restarts
- **Auto-reconnect**: automatic reconnection with exponential backoff on disconnect
- **Search**: full-text search across messages with `/search`
- **Pagination**: load older messages with `/more` and `/backlog`
- Binaries for Windows, Mac, Linux and Raspberry Pi

### Caveats

Here are some things you might expect to work that don't. Plus some other things I should mention.

- No automation of messages, no sending of messages through shell commands
- Facebook obviously doesn't endorse or like these kinds of apps and they're likely to break when Facebook changes stuff in their web app

## Similar Apps

Similar but more features:
- [Nchat](https://github.com/d99kris/nchat)

## Installation

How to get it running and how to use it

### Latest Release

Always fresh, always up to date.

- Download a release
- Put the binary in your PATH (optional)
- Run with `whatscli` (or double-click)
- Scan the QR code with WhatsApp on your phone (resize shell or change font size to see whole code)

### Package Managers

Some ways to install via package managers are supported but the installed version might be out of date.

#### MacOS (homebrew)

- `brew install normen/tap/whatscli`

#### Arch Linux (AUR)

- `https://aur.archlinux.org/packages/whatscli/`

## Usage

Most information, all commands and key bindings are available through the in-app help, simply type `/help` and/or `/commands`.

### Login

When starting up, whatscli will immediately try to connect to the WhatsApp server to log in. Keep your phone ready to scan the appearing QR code in WhatsApp on your Phone. If you don't manage to scan the code quick enough just restart the application. If you can not see the whole QR code, reduce the font size of your terminal or increase the window size.

After scanning the QR code the chats should be populated. After you have done this once, whatscli will be able to log into WhatsApp automatically on start. To log out of WhatsApp completely type `/logout`.

### Messaging / Commands

Select a chat on the left and start typing in the input field at the bottom to send messages. Switch between the chat list and the input field with `<Tab>`.

For issuing commands the same input field is used. By default commands are prefixed with `/`. You can for example use the `/sendimage /path/to/file.jpg` command to send images, see `/help` for more commands.

When paths are given for commands you don't need to surround the path in quotes, even if it contains spaces. Also don't prefix spaces with backslashes (as the copy-paste function of MacOS does for example).

### Messages

When pressing `Ctrl-w` (default mapping) you enter "message selection mode" which allows selecting a single message and performing operations on them. For example pressing `o` while a message is selected allows opening any attachments through an external application.

#### Image display

You can display images in whatscli using external programs that convert the image to UTF characters. I found that `jp2a` works well for jpeg images, it is available through package managers on most systems. However the "image quality" leaves a lot to be desired. The [PIXterm](https://github.com/eliukblau/pixterm) app allows displaying true-color versions of the images which are quite recognizable already.

To configure the used command and its parameters edit the `show_command` parameter in `whatscli.config`, see `/help` for the config file location.

#### Copy-Pasting User IDs

Some commands such as the `/add` and `/remove` require a "user id" as their input. You can copy the user ID of a selected chat or a selected message to the clipboard with `Ctrl-c` (default mapping) and easily append them to the current input using `Ctrl-v`.

### Notifications

The app supports basic desktop notifications through the `gen2brain/beeep` library, to enable it set `enable_notifications = true` in `whatscli.config`. Set `use_terminal_bell = true` to ring your terminal's bell instead of sending a desktop notification.

### Configuration

Most key bindings, colors and other options can be configured in the `whatscli.config` file, the `/help` command shows its location.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for architecture details and how to add new commands.

### Building

Using a recent version of Go, building should be straightforward. Either use `go build`, `go run` etc. or use the included Makefile.

### Testing

```bash
make test    # runs go test -v ./...
make vet     # runs go vet ./...
```

### Structure Overview

The application is split into two major packages:

**Root package (`main`)** — TUI layer using tview:

| File | Purpose |
|------|---------|
| `main.go` | Entry point, initializes config, session, and TUI |
| `app_context.go` | `AppContext` struct holding all shared UI state |
| `ui_layout.go` | Layout construction (flex containers, tables, input) |
| `ui_handler.go` | `UiHandler` implementing `UiMessageHandler` interface |
| `ui_keybindings.go` | Key binding setup and input handling |
| `ui_render.go` | Message and chat list rendering |
| `ui_helpers.go` | Shared UI utility functions |

**`messages/` package** — Business logic and WhatsApp connection:

| File | Purpose |
|------|---------|
| `session_manager.go` | Connection lifecycle, event handling, auto-reconnect |
| `storage.go` | SQLite-backed message and conversation persistence |
| `messages.go` | Shared interfaces and data structures |
| `cmd_registry.go` | Command registration and dispatch |
| `cmd_basic.go` | Core commands: `/select`, `/read`, `/info`, `/more` |
| `cmd_chat.go` | `/backlog` — history sync and message loading |
| `cmd_search.go` | `/search` — full-text message search |
| `cmd_media.go` | `/sendimage`, `/sendvideo`, `/sendaudio`, `/upload` |
| `cmd_group.go` | `/create`, `/add`, `/remove`, `/sync-groups` |

**`config/` package** — INI-based configuration with defaults.

**`qrcode/` package** — ANSI QR code rendering for terminal display.

## License

This software is released under MIT license. Remember that this gives you all freedom except for slapping your name on it.
