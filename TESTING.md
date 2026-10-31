# Testing

How WhatsCLI is tested, and the live-account checklist that gates every release. The checklist is ROADMAP row 15 (Recurring).

_Last updated: 2026-10-31T17:32:04+00:00 (step 34, ui_overlay_test.go)_

## 1. Commands

```bash
make test                       # go test -race ./...
make vet                        # go vet ./...
go build ./...
go test -race -count=1 ./...    # what CI runs (without -v), bypasses the test cache
go test -race -run TestIncomingRevoke ./messages    # one group
```

Requires CGO and a C compiler (`apt install gcc` on Linux). Go directive 1.26.0 with toolchain go1.27.3; an older system Go downloads the toolchain automatically.

Last verified 2026-10-31 at `v1.6.2` (`ad7df87`): build, vet and `go test -race -count=1 ./...` pass for the root package, `config`, `messages` and `qrcode`.

## 2. CI

`.github/workflows/ci.yml` runs on push and pull request to `master`: matrix Go 1.26 and 1.27 on `ubuntu-latest` with `GOTOOLCHAIN=local`, so each leg runs the Go it names (a `go version` step fails the leg otherwise). Steps: install gcc, SPDX header check (`.github/scripts/check-spdx.sh`), `go mod download`, `go mod verify`, `go vet ./...`, `go build -v ./...`, `go test -v -race -count=1 ./...`. Every action is pinned to a commit SHA; Dependabot proposes updates weekly. Releases are a separate workflow (`release.yml`) and do not run tests.

## 3. Test layout

**Table T1: Test files (184 top-level tests at v1.6.5)**

| File | Tests | Covers |
|---|---|---|
| `config/settings_test.go` | 7 | Config defaults, load, XDG paths |
| `messages/storage_test.go` | 20 | SQLite layer, search, pagination, conversations |
| `messages/msg_handler_test.go` | 20 | Message content extraction and handling |
| `messages/media_test.go` | 13 | Schema migration on an old DB, media metadata round trip, sanitisation, `safeJoin`, download traversal, collision, temp cleanup, `firstURL` |
| `messages/media_safety_test.go` | 5 | Windows-safe names, `safeToOpen` allow and deny lists, open gate, `ResolveFileWithin` |
| `messages/cmd_message_test.go` | 10 | Registry, revoke, download validation, `/url`, backlog anchor and guards, backlog wait, opener errors reported |
| `messages/revoke_incoming_test.go` | 5 | Spoof from another chat, 12 authorisation cases, LID chat, group admin, `chat_id` scoping |
| `messages/jid_norm_test.go` | 6 | Canonical chat JID, `sameUser`, history sync with `@lid`, live and history agreement, backlog target |
| `messages/lid_merge_test.go` | 5 | Merge of `@lid` chats into the phone-number chat, open chat moves, unknown mapping unchanged |
| `messages/empty_chats_test.go` | 2 | Empty contact chats skipped and removed |
| `messages/contact_import_test.go` | 3 | `@lid` contact import, no new chat for a mapped LID, bare LID never renames |
| `messages/delete_for_me_test.go` | 6 | Delete for me: row removed, preview and redraw, other chat, last message, LID chat, wrong chat ignored |
| `messages/chat_view_test.go` | 7 | Newest page on open, `/more` paging and "all shown", new and history-synced messages visible |
| `messages/store_upsert_test.go` | 6 | `StoreResult`, media fill only when empty, revoked row untouched, redraw only on change, redelivered live message |
| `messages/local_path_test.go` | 4 | `local_path` migration, scoping and clearing on revoke, repeat download reuse and fallback |
| `messages/partfiles_test.go` | 8 | Part-file name match, sweep, blank or missing folder, same folder once, `Shutdown` cancels download and backlog wait |
| `messages/status_order_test.go` | 7 | Status printed with the redraw (`NewScreenWithStatus`) for revoke, backlog and history sync |
| `messages/session_manager_test.go` | 5 | Session lifecycle |
| `messages/cmd_media_test.go` | 3 | Send media commands |
| `messages/priority_queue_test.go` | 1 | Chat ordering |
| `qrcode/qrcode_test.go` | 1 | Full quiet zone in rendered output |
| `ui_render_test.go` | 10 | UI rendering helpers |
| `ui_image_test.go` | 5 | `renderImage`: output, timeout, child holding stdout, viewer failure, refusals |
| `ui_opener_test.go` | 4 | `startOpener` start failure, no blocking and reaping, clean exit; target passed as one argument |
| `ui_complete_test.go` | 9 | Command list matches the registry; drop-down entries, completed text, echo line and tag escaping; chat search query, matching and entries |
| `version_test.go` | 1 | Version shown: injected tag, `go install` module version, `dev` |
| `ui_scrollbar_test.go` | 4 | Scroll thumb size and position, click-to-offset mapping, bar hit test, chat list time cell with unread count |
| `ui_overlay_test.go` | 3 | Help window lists every command and key; welcome text stays short; top bar hints and state, no battery |
| `messages/display_names_test.go` | 3 | Sender label order (saved name, phone number, `~PushName`, LID), contact saved under the LID, no stores, `getMessages` labels without changing stored rows |
| `ui_helpers_test.go` | 1 | `chatForID` |

## 4. Conventions

- Table-driven tests preferred (`storage_test.go`, `media_safety_test.go` are examples).
- Storage tests use in-memory SQLite: `db, _ := sql.Open("sqlite3", ":memory:")`, then `md.InitWithDB(db)`. Never call `Init()` in tests; it opens the real config path.
- Use `MockUiHandler` (`messages/mock_ui_handler.go`) for backend tests; it implements all 18 `UiMessageHandler` methods.
- No test talks to WhatsApp. Use the seams: `mediaDownloader` (fake downloads), `lidMapperOverride` (fake LID store), `groupInfoOverride` (fake group metadata), `webMessageParser` (history sync without a connected client; a zero `whatsmeow.Client{}` parses incoming messages).
- Every security rule gets a negative test (what must not happen). Authors of the revoke, open and path fixes also ran mutation checks: reverting each control made at least one test fail. Repeat that when you change a control.
- All tests run with `-race`.
- A bug fix includes a test that fails without the fix.

Known gaps: no success-path test for the `cmdRevoke` goroutine (needs a client seam), no test that sends to or receives from a real WhatsApp server, no test for the UI key handlers. The download goroutine is covered through the `mediaDownloader` seam (`local_path_test.go`, `partfiles_test.go`); viewer and opener execution are covered by `ui_image_test.go` and `ui_opener_test.go`. These gaps are why the live checklist exists.

## 5. Live-account checklist (release gate)

Run before every tagged release (ROADMAP row 15). Runs so far are recorded in Table T3; no run has yet covered every step. It checks what unit tests with fakes cannot: real WhatsApp behaviour.

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
| 22 | Chat paging | Open a chat with more than 200 stored messages; run `/more` until it reports that all are shown | Opens with the newest 200; each `/more` adds 200 older ones and prints "Showing N of M stored messages", visible after the redraw; at the end "All M stored messages are shown. Use /backlog to request older ones from WhatsApp." |
| 23 | Group info | In a large group run `/info` (or `i` with a message highlighted), and type a command while it waits | Group name, JID and participant count print; other commands keep working meanwhile; a slow server gives an error after 15 s, no freeze |
| 24 | Delete for me | In "Message yourself", send a message from WhatsCLI or the phone, then "Delete for me" on the phone while WhatsCLI is running | The message disappears from WhatsCLI and the chat preview moves back to the newest remaining message |
| 25 | Repeat download | `d` twice on the same named document, then `o` on it | One file in `download_path` (no `<stem>_<id>` copy); `open` reuses the saved file |
| 26 | Part-file sweep | Quit during a large video download; then plant `.whatscli-123456.part`, `.whatscli-abc.part` and `keep.part` in `download_path` and start WhatsCLI | No `.part` left by the quit; at start only `.whatscli-123456.part` is removed, the other two stay |
| 27 | Command list | On the input line type `/lo`, press `Down`, then `Tab`; type `/sendi` and `Tab` | The list shows `/login` and `/logout` with descriptions; the input becomes `/logout`, then `/sendimage ` |
| 28 | Command echo | Enter `/info` in a chat | `(<date> <time>) > /info` appears before its output |
| 29 | Open at latest | Scroll up in a long chat with `PgUp`, then open another chat | The new chat shows its latest message at the bottom |
| 30 | Scroll bars | With the mouse on, open a long chat; drag the thumb on the message pane's right border up; click the middle of the Chats bar | The thumb shows near the bottom first; dragging scrolls to older messages; the click moves the chat list to its middle without opening a chat |
| 31 | Click near the list end | Click each of the last three visible rows of the Chats list | Each click opens that chat; the list does not move |
| 32 | Sender names | Open a group and a one-to-one chat with messages from people shown before as long LID numbers | Each sender shows the saved contact name, else `+<phone>`; only a sender with a private number shows `~Name` or a LID |
| 33 | Find a chat | Press `Ctrl+f`, type part of a name, press `Down` and `Enter`; then enter `/select zzzq` | The list narrows as you type and the chosen chat opens; `No chat matches "zzzq"` is printed |
| 34 | Welcome and help | Start WhatsCLI; press `F1` (or enter `/help`), scroll, press `Esc` | The pane shows "Welcome to WhatsCLI" with five keys; the top bar shows the version, online or offline, and the main keys, with no battery figure; the help window lists every command and closes on `Esc`, focus back on the input |

**Table T3: Run record (copy one row per run)**

| Date (ISO 8601) | Build or tag | Tester | Steps passed | Steps failed or skipped | Notes |
|---|---|---|---|---|---|
| 2026-10-31 | `chore/catch-up` at `ddbb7f9` (v1.4.0 candidate) | Owner (QR scans, phone deletes) and Claude (headless driver) | 1, 2, 3, 6, 7, 8, 9, 10, 12, 14, 19, 20 | Skipped: 4, 5, 11, 13, 15, 16, 18, 21 (unit tests only). 17: no `@lid` chats in the synced store, so not exercised. 19 on Windows: not run | Run on the owner's own account with test messages only to "Message yourself" and two trusted contacts; system opener stubbed to a log, `show_command` set to `file` (jp2a not installed). Step 1 run offline on a copy of the old database. Found and fixed: media saved as `.jfif`/`.f4v` so `open` refused photos and videos; QR left on screen after login; `/select` did not set the typing target; message keys lost focus after one use; QR countdown ignored the 60 s first code. Step 14 seen on two real group deletes. Backlog answered in a phone-number chat (row 14 partly verified) |
| 2026-10-31 | `v1.5.0-rc1` at `07f62c0` | Owner (QR scan) and Claude (headless driver) | Smoke run only: QR login, chat list, history sync, empty-chat skip, clean quit | Not run: 1 to 21 (full run above). 17: no `@lid` chats arrived, so no merge ran | Smoke run for v1.5.0 on a fresh test store, no messages sent. Login, chat list and full history sync worked (503 chats, 36,527 messages); no chat row without a time, unread count, pin or messages was stored; 141 one-to-one chats showing "New chat" all have a last-message time, so they are real chats whose messages were not synced. Store integrity check ok after quit |
| 2026-10-31 | `v1.6.0-rc1` candidate at `d803e27` (local build) | Owner (QR scan, one phone delete) and Claude (headless driver) | 1; 2 (document download twice, `open`, `show`, `/url`); new checks: chat paging and `/more`, `/info` in a group, Delete for me, part-file sweep | Not run: 3 to 21 except the above. 17: no `@lid` chats arrived | Messages only to "Message yourself". Login and full sync (503 chats, 36,576 messages). A group with 2,067 messages opened with 200; three `/more` showed "Showing 800 of 2067 stored messages" and the status stayed visible. `/info` in a group answered (872 participants) without blocking. The same document downloaded twice gave one file, and `open` reused it. `show` rendered, `/url` reached the opener, no defunct processes. "Delete for me" on the phone removed the message and moved the chat preview back. Quitting during a video download left no `.part` file; a planted `.whatscli-123456.part` was removed at the next start while `.whatscli-abc.part` and `keep.part` stayed; reconnect needed no QR. Store integrity ok |
| 2026-10-31 | `v1.6.2` release binary | Owner (daily use on own account) | Not a checklist run | Steps not tracked one by one | Owner used v1.6.2 after relinking, including the scroll bars and the chat list, and reported it works well. Recorded as feedback, not as a pass of steps 30 and 31 |

## 6. Before tagging

1. `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` pass on the commit to be tagged.
2. The live checklist above passed; the run is recorded in ROADMAP row 15.
3. Secret scan of the repository and the built binary (SECURITY.md section 3).
4. CHANGELOG `[Unreleased]` renamed to the version and dated; ROADMAP §2.1 updated.
5. Release candidate tag (for example `v1.7.0-rc1`, published as a pre-release) run through `release.yml` first, then the real tag.
