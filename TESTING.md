# Testing

How WhatsCLI is tested, and the live-account checklist that gates every release. The checklist is ROADMAP row 15 (Recurring).

_Last updated: 2026-10-31T06:55:05+00:00 (first live run recorded in Table T3; step 9 note on "Message yourself")_

## 1. Commands

```bash
make test                       # go test -race ./...
make vet                        # go vet ./...
go build ./...
go test -race -count=1 ./...    # what CI runs (without -v), bypasses the test cache
go test -race -run TestIncomingRevoke ./messages    # one group
```

Requires CGO and a C compiler (`apt install gcc` on Linux). Go directive 1.26.0 with toolchain go1.27.3; an older system Go downloads the toolchain automatically.

Last verified when this file was written: build, vet and `go test -race -count=1 ./...` pass for the root package, `config`, `messages` and `qrcode`.

## 2. CI

`.github/workflows/ci.yml` runs on push and pull request to `master`: matrix Go 1.26 and 1.27 on `ubuntu-latest`, then `go mod download`, `go mod verify`, `go vet ./...`, `go build -v ./...`, `go test -v -race -count=1 ./...`. [Inference] The 1.26 leg may run 1.27.3 through the `toolchain` line (ROADMAP row 12). Releases are a separate workflow (`release.yml`) and do not run tests.

## 3. Test layout

**Table T1: Test files (104 top-level tests when this file was written)**

| File | Tests | Covers |
|---|---|---|
| `config/settings_test.go` | 7 | Config defaults, load, XDG paths |
| `messages/storage_test.go` | 20 | SQLite layer, search, pagination, conversations |
| `messages/msg_handler_test.go` | 20 | Message content extraction and handling |
| `messages/media_test.go` | 13 | Schema migration on an old DB, media metadata round trip, sanitisation, `safeJoin`, download traversal, collision, temp cleanup, `firstURL` |
| `messages/media_safety_test.go` | 5 | Windows-safe names, `safeToOpen` allow and deny lists, open gate, `ResolveFileWithin` |
| `messages/cmd_message_test.go` | 8 | Registry, revoke, download validation, `/url`, backlog anchor and guards |
| `messages/revoke_incoming_test.go` | 5 | Spoof from another chat, 12 authorisation cases, LID chat, group admin, `chat_id` scoping |
| `messages/jid_norm_test.go` | 6 | Canonical chat JID, `sameUser`, history sync with `@lid`, live and history agreement, backlog target |
| `messages/session_manager_test.go` | 5 | Session lifecycle |
| `messages/cmd_media_test.go` | 3 | Send media commands |
| `messages/priority_queue_test.go` | 1 | Chat ordering |
| `qrcode/qrcode_test.go` | 1 | Full quiet zone in rendered output |
| `ui_render_test.go` | 10 | UI rendering helpers |

## 4. Conventions

- Table-driven tests preferred (`storage_test.go`, `media_safety_test.go` are examples).
- Storage tests use in-memory SQLite: `db, _ := sql.Open("sqlite3", ":memory:")`, then `md.InitWithDB(db)`. Never call `Init()` in tests; it opens the real config path.
- Use `MockUiHandler` (`messages/mock_ui_handler.go`) for backend tests; it implements all 16 `UiMessageHandler` methods.
- No test talks to WhatsApp. Use the seams: `mediaDownloader` (fake downloads), `lidMapperOverride` (fake LID store), `groupInfoOverride` (fake group metadata), `webMessageParser` (history sync without a connected client; a zero `whatsmeow.Client{}` parses incoming messages).
- Every security rule gets a negative test (what must not happen). Authors of the revoke, open and path fixes also ran mutation checks: reverting each control made at least one test fail. Repeat that when you change a control.
- All tests run with `-race`.
- A bug fix includes a test that fails without the fix.

Known gaps: no success-path test for the `runDownload` and `cmdRevoke` goroutines (needs a client seam), no test that sends to or receives from a real WhatsApp server, no test for the UI key handlers or `show` viewer execution. These are why the live checklist exists.

## 5. Live-account checklist (release gate)

Run before every tagged release (ROADMAP row 15). No agent has run this; the first run is still due. It checks what unit tests with fakes cannot: real WhatsApp behaviour.

**Preconditions**

- Use a non-production WhatsApp number linked to a throwaway or test session, never your main account.
- Back up `~/.config/whatscli/session_meta.db` before starting the new build against it.
- Have a second phone or contact who can send you media and delete messages. A Windows machine for step 19 if the Windows build is being released.
- Install `jp2a` (or set `show_command`) for step 2, and `xdg-utils` for steps 2 and 7.
- Record the result of each step in the table at the end and copy the summary into ROADMAP row 15.

**Table T2: Checklist**

| Step | Area | Do | Expect |
|---|---|---|---|
| 1 | Migration | Start the new build against the backed-up `session_meta.db` | Starts without error; old chats display |
| 2 | Media receive | Receive an image, a named PDF, a voice note and a video. Press `Ctrl+w`, highlight each. `d` on each; `o` on each; `s` on the image; `s` on the PDF | `d`: path printed under `download_path`, file opens correctly, no `.whatscli-*.part` left. `o`: system viewer opens the file from `preview_path`. `s` on the image: rendered by `show_command`. `s` on the PDF: "show only works for image messages" |
| 3 | Name collision | Receive two different PDFs with the same name; download both | Second is saved as `name_<msgid>.pdf` |
| 4 | Hostile name | Ask a contact to send a document named `..\..\evil.txt` if their client allows | Saved inside `download_path` as `evil.txt` (no escape) |
| 5 | Old media | Highlight a media message from before the upgrade and press `d` | "this is not a downloadable message" (accepted limitation) |
| 6 | Send media | `/sendimage path`; then `d` on the sent message | It appears in the chat; `d` downloads it |
| 7 | URL | Message containing a link; `u` | Browser opens the right URL |
| 8 | Own revoke | `r` on your own message in a 1:1 chat, in a group, and in an `@lid` chat if you have one. Then `r` on someone else's message | Disappears on the phone, shows `[message revoked]` locally. Someone else's: "you can only revoke (delete for everyone) your own messages" |
| 9 | Revoke from phone | Delete for everyone from the phone, in a chat with another person. "Message yourself" only offers "Delete for me", which is not a revoke | Line updates to `[message revoked]` in WhatsCLI |
| 10 | Backlog | Open a chat with history, press `Ctrl+b`; type during the wait; repeat in a group and in an `@lid` chat | Within about 5 s "Loaded N additional messages", or the "none arrived yet" note and a later refresh if the phone answers; the UI stays responsive |
| 11 | Clipboard | In a session without xclip, xsel or wl-clipboard (for example plain SSH), press `Ctrl+c` and `Ctrl+v` | A red error line, no crash |
| 12 | QR | `/logout`, `/connect`; scan the QR in a dark and a light terminal theme | Detected quickly in both |
| 13 | Revoke from contact | Ask a contact to delete for everyone a message in your 1:1 chat | `[message revoked]` shown |
| 14 | Revoke from own phone | Delete for everyone one of your own messages on the phone, in a 1:1 chat and in a group | WhatsCLI updates both |
| 15 | Group admin revoke | As a non-admin, have a group admin delete another member's message; separately have a member delete their own message | Both update (after one group info request). [Unverified] WhatsApp may send admin revokes with the sender in LID form; the check accepts both |
| 16 | Contact revokes your message | Not possible from official clients in a 1:1 chat | No case to test; only unit tests cover spoofing |
| 17 | `@lid` chats | Note whether any chat JID shown by `/info` or in the DB is `@lid`. Receive a message from a contact who shows as `@lid`. Run backlog in that chat | It lands in the contact's phone-number chat with no duplicate chat; older messages arrive in the same chat. Count how many `@lid` chats exist (input for ROADMAP row 13) and whether the phone answers (row 14) |
| 18 | Old `@lid` chat | If a pre-existing `@lid` chat exists, run backlog in it | The note "This chat is stored under its LID form..." naming the phone-number chat |
| 19 | Open gate | Have a contact send `test.exe` as a document; `o` on it. Repeat on the Windows build if releasing it | Saved, path printed, "Saved but not opened" note; nothing runs |
| 20 | Open allowlisted | `o` on a PDF and on an image | Opens in the system viewer as before |
| 21 | Show with symlink | Make `download_path` (for example `~/Downloads`) a symlink and `s` on an image. Keep a file in `~/Downloads2` | The image renders. `~/Downloads2` is never shown (not reachable from the UI; unit-tested) |

**Table T3: Run record (copy one row per run)**

| Date (ISO 8601) | Build or tag | Tester | Steps passed | Steps failed or skipped | Notes |
|---|---|---|---|---|---|
| 2026-10-31 | `chore/catch-up` at `ddbb7f9` (v1.4.0 candidate) | Owner (QR scans, phone deletes) and Claude (headless driver) | 1, 2, 3, 6, 7, 8, 9, 10, 12, 14, 19, 20 | Skipped: 4, 5, 11, 13, 15, 16, 18, 21 (unit tests only). 17: no `@lid` chats in the synced store, so not exercised. 19 on Windows: not run | Run on the owner's own account with test messages only to "Message yourself" and two trusted contacts; system opener stubbed to a log, `show_command` set to `file` (jp2a not installed). Step 1 run offline on a copy of the old database. Found and fixed: media saved as `.jfif`/`.f4v` so `open` refused photos and videos; QR left on screen after login; `/select` did not set the typing target; message keys lost focus after one use; QR countdown ignored the 60 s first code. Step 14 seen on two real group deletes. Backlog answered in a phone-number chat (row 14 partly verified) |

## 6. Before tagging

1. `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` pass on the commit to be tagged.
2. The live checklist above passed; the run is recorded in ROADMAP row 15.
3. Secret scan of the repository and the built binary (SECURITY.md section 3).
4. CHANGELOG `[Unreleased]` renamed to the version and dated; ROADMAP §2.1 updated.
5. Release candidate tag run through `release.yml` first (ROADMAP row 17), then the real tag.
