# Roadmap & Backlog: WhatsCLI

> **The single source of truth for forward-looking work.** Anything not shipped lives here. If an item needs more than a paragraph, it has a subsection in §8 or a file linked from §6. Shipped history is in [CHANGELOG.md](CHANGELOG.md), system design in [ARCHITECTURE.md](ARCHITECTURE.md), the verification procedure in [TESTING.md](TESTING.md).
>
> **Start at §0.** It lists every open item, one row each. §2.2 is the ledger: the same open rows plus every closed one.
>
> Timestamps are ISO 8601 to the second, with the offset. "Raised" dates for rows 1 to 25 are 2026-10-31, the date of the independent review (REVIEW-3, an agent report that is not stored in the repository) and of the residual-risk notes written with the fixes; git has no earlier commit for them.
>
> _Last updated: 2026-10-31T17:21:02+00:00 (v1.6.4: find a chat by name)._

## 0. North star: every open item, one row each

Grouped by state (Open, Decision, Waiting, Blocked, Deferred, Recurring), then by priority. `#` is the ledger number and never changes.

**Table RM0: Open items**

| # | Bucket | Priority | State | Size | Impact | Raised | Problem | Solution or next step |
|---|---|---|---|---|---|---|---|---|
| 14 | §3.6 | P2 | **Open** | S | M | 2026-10-31 | Partly verified 2026-10-31: the phone answered a backlog request in a phone-number chat; no `@lid` chat was available to test. [Unverified] The phone may not answer an ON_DEMAND backlog request when the anchor chat is a phone-number JID but its own record is LID. `/backlog` would silently do nothing there. | Verify with checklist step 17 on a live account; if no reply, send the anchor in LID form via `GetLIDForPN`. [§8.14](#814-r2-backlog-anchor-jid-form) |
| 8 | §3.1 | P2 | **Deferred** | M | M | 2026-10-31 | Releases no longer include macOS, Raspberry Pi (armv5) or a Homebrew tap (owner decision D4). macOS and 32-bit Pi users find no asset. | Back when someone asks: macOS = add a `macos-latest` matrix entry; Homebrew = add a tap job with a token scoped to the tap repository; armv5 needs a cross C toolchain. [§8.8](#88-release-targets-dropped) |
| 18 | §3.2 | P3 | **Deferred** | S | L | 2026-10-31 | R3: each revoke by a non-sender in a group costs one group info request, and a non-admin can trigger these by sending fake revokes. | Cache admin lists per group for some minutes if rate limits appear in logs. Accepted for now. |
| 19 | §3.2 | P3 | **Deferred** | S | L | 2026-10-31 | R4: a revoke that arrives before its target (offline queue order) is ignored, so the message stays visible. | Remember pending revokes briefly. Accepted for now. |
| 20 | §3.2 | P3 | **Deferred** | S | L | 2026-10-31 | R5: the open allowlist judges only the extension; a PDF or image viewer exploit is still possible, as in any client. | Also check that media kind matches the extension. Small gain; accepted. |
| 22 | §3.2 | P3 | **Deferred** | M | L | 2026-10-31 | A spoofed or mistaken revoke overwrites text and media data, which cannot be recovered (review option C). | Add a `revoked` flag column and keep the original data; needs another migration. Authorisation (D14) already closes the spoof. |
| 35 | §3.3 | P3 | **Deferred** | S | L | 2026-10-31 | A "Delete for me" made on the phone while WhatsCLI was unlinked is not applied after relinking, because whatsmeow suppresses app-state events during a full sync. | Set `EmitAppStateEventsOnFullSync` and handle the replayed mark-read, pin and archive events too; only if users report ghost messages. |
| 15 | §3.6 | P1 | **Recurring** | M | H | 2026-10-31 | No automated test talks to WhatsApp, so media, revoke, backlog, `@lid`, show and QR behaviour is verified only by unit tests with fakes. Last full run: 2026-10-31 on `ddbb7f9`, 12 steps passed, 5 bugs fixed, 8 skipped; smoke run on `v1.5.0-rc1` 2026-10-31 passed; live run on the `v1.6.0-rc1` candidate 2026-10-31 passed for the steps it covered (1, 2 and the v1.6.0 checks, now steps 22 to 26; TESTING Table T3). | Run the full live-account checklist in [TESTING.md](TESTING.md) on a non-production number before every tagged release. [§8.15](#815-live-account-checklist-as-a-release-gate) |

**Reading the narrow columns.** Bucket is the §3 area: 3.1 Release and CI, 3.2 Security hardening, 3.3 Chat and message behaviour, 3.4 UI, 3.5 Docs and repository hygiene, 3.6 Verification. Size: T trivial, S small, M medium, L large, ? unknown. Impact: H high, M medium, L low. A long-form note for a row is in §8, linked from the end of its solution cell.

### Legend

**Table RM-P: Priority**

| Priority | Means |
|---|---|
| **P0** | A live customer is losing something today |
| **P1** | Costs a support ticket, real money or a release every time it happens |
| **P2** | The product is measurably worse without it |
| **P3** | Worth doing; nothing breaks if it waits |

An empty P0 is good news. Never inflate an item to fill it.

**Table RM-I: Impact**

| Impact | Means |
|---|---|
| **High** | Costs activations, refunds, revenue, or a customer meeting a defect |
| **Medium** | Costs support hours, or blocks something High |
| **Low** | Real, and nobody is bleeding |

Priority, Size and Impact are separate columns because they answer separate questions: how much it hurts, what it costs to do, and what it costs not to do.

**Table RM-S: State**

| State | Means |
|---|---|
| **Open** | Not started or partly done; nothing stops it but a decision to begin |
| **Blocked** | Cannot proceed until someone outside the project acts or changes a policy; the row names who |
| **Waiting** | Ready, and deliberately held until a named date (a measurement window, a staged rollout) |
| **Deferred** | Could proceed, and the owner has chosen not to yet (usually new spend, or no demand); the row says what would bring it back |
| **Decision** | An answer from the owner is needed first; the row carries a recommendation |
| **Recurring** | A procedure that runs again every release or on a date; the row records the last run |
| **Done** | Closed, with the date and the size it turned out to be (ledger only) |

## How to keep this file current

**Table RM-K: Maintenance**

| When | Do this |
|---|---|
| An item closes | Move its ledger row to the Done block with `**Done** <date>` and `Was <size>`, remove it from §0, delete its §8 subsection. Never delete the ledger row |
| An item is found | Add it to §0 and the ledger with the next unused number, and to its §3 area if it needs context |
| The owner answers a Decision | Update the §0 row and the ledger row the same day |
| A state changes | Change it in §0 and the ledger together |
| A release or milestone ships | Update §2.1, close the rows it shipped, update §1 |
| Anything above happens | Update the header's `_Last updated_` line with the timestamp and what changed |

---

## 1. Shipped capabilities

Orientation only; detail is in CHANGELOG.md.

**Table RM1: Shipped capabilities**

| Capability | Since |
|---|---|
| Text, image, video, audio, document and sticker messages; groups; search; SQLite persistence; auto-reconnect; QR login | v1.3.0 (2026-02-21) and earlier |
| Download, open, show, URL and revoke of messages; media metadata storage | v1.4.0 (2026-10-31) |
| Incoming revoke with authorisation; open allowlist; filename sanitisation; `show` path containment | v1.4.0 (2026-10-31) |
| `@lid` chat normalisation; ON_DEMAND backlog | v1.4.0 (2026-10-31) |
| Tag-driven release for linux/amd64, linux/arm64, windows/amd64 | v1.4.0 (2026-10-31) |
| Merge of `@lid` chats into the phone-number chat; no empty "New chat" rows | v1.5.0 (2026-10-31) |
| Chat paging (newest 200, `/more` adds 200); `/info` off the command loop; `i` key is chat info | v1.6.0 (2026-10-31) |
| "Delete for me" from the phone mirrored; `@lid` contacts imported | v1.6.0 (2026-10-31) |
| Repeat downloads reuse the saved file (`messages.local_path`); media backfill with `StoreResult`; part-file sweep at startup; quitting cancels downloads, backlog and revokes | v1.6.0 (2026-10-31) |
| `show` viewer with a 15 s timeout off the UI goroutine; built-in non-blocking opener; status kept after redraws (`NewScreenWithStatus`) | v1.6.0 (2026-10-31) |
| CI: SHA-pinned actions with Dependabot, `GOTOOLCHAIN=local`, SPDX header check; release candidates published as pre-releases | v1.6.0 (2026-10-31) |
| Command drop-down on the input line, timestamped command echo, chats open at their latest message, `go install` builds show their version | v1.6.1 (2026-10-31) |
| Scroll bars with click and drag; stable chat list (no jump on click, starts at the newest chat, names shortened so times stay visible) | v1.6.2 (2026-10-31) |
| Senders shown by saved contact name, else phone number, resolved at display time (row 21) | v1.6.3 (2026-10-31) |
| Find a chat by name: `Ctrl+f` or `/select <name>` with a drop-down of matches | v1.6.4 (2026-10-31) |

## 2. Next up

### 2.1 Status: v1.6.4 released (updated 2026-10-31T17:21:02+00:00)

- `v1.6.4` adds finding a chat by name (`Ctrl+f`, `/select <name>`); checked with unit tests and an offline run of the real UI. `v1.6.4-rc1` kept as a pre-release.
- Next: rows 14 (needs an `@lid` chat on a live account), 35 and the deferred security rows.

Earlier status, kept for the record:

- v1.6.3: shows senders by saved contact name, else phone number (row 21, display time only). On a real account, 6,015 of 6,017 LID senders had a known phone number. `v1.6.3-rc1` kept as a pre-release.
- v1.6.2: adds scroll bars and fixes the chat list jumping when a row near its end was clicked; checked with unit tests and an offline run of the real UI with mouse clicks and drags. `v1.6.2-rc1` kept as a pre-release.
- v1.6.1: adds the owner's usability requests (command drop-down, command echo, open at latest message) and the `go install` version fix; checked with unit tests and an offline run of the real UI on a copy of local data (no WhatsApp connection). `v1.6.1-rc1` kept as a pre-release.
- v1.6.0: `v1.6.0` tagged on `master` after a live run on the `v1.6.0-rc1` candidate (TESTING Table T3); rows 5, 6, 7, 9, 10, 11, 12, 24, 25, 32, 34 and 36 shipped. `v1.6.0-rc1` kept as a pre-release.
- v1.5.0: `v1.5.0` tagged on `master` at `07f62c0` (rows 13 and 33); release assets and checksums verified; `v1.5.0-rc1` kept as a pre-release after a live smoke run (TESTING Table T3).
- v1.4.0 (2026-10-31T07:03:48+00:00): `chore/catch-up` merged to `master` (PR 2) and tagged `v1.4.0`; release assets and checksums verified; `v1.4.0-rc1` kept as a pre-release. `go install github.com/PalWorks/whatscli@v1.4.0` resolves.
- v1.4.0 next step at the time: row 13 (approved), then the open P2 rows.
- Before v1.4.0 (branch `chore/catch-up`, not yet merged or tagged): gates were row 15 (live-account checklist) and row 17 (first workflow run on a release candidate tag); row 13 was approved to follow (owner, 2026-10-31, D20). The concern that `go install github.com/PalWorks/whatscli@latest` would fail while `v1.3.0` was the newest tag is resolved: `@latest` resolves to `v1.6.0` (checked 2026-10-31). The release was tagged `v1.4.0`; the agents' handover had suggested `v1.3.1`.

### 2.2 The ledger: every backlog item, pending first, done at the bottom

**This table is authoritative for state; §3 and §8 are authoritative for reasoning.** Open rows repeat §0 in one line; §0 holds the problem and solution. Closed rows keep only their date and size.

**Table RM2: Ledger**

| # | Item | Section | State | Size | Impact |
|---|---|---|---|---|---|
| 14 | R2 backlog anchor JID form | §3.6 | **Open**: verify live | S | M |
| 8 | Release targets dropped (finding 8) | §3.1 | **Deferred**: owner decision D4 | M | M |
| 18 | R3 admin revoke lookup cost | §3.2 | **Deferred** | S | L |
| 19 | R4 revoke before target | §3.2 | **Deferred** | S | L |
| 20 | R5 extension versus media kind | §3.2 | **Deferred** | S | L |
| 21 | R6 `ContactId` normalisation | §3.3 | **Done** 2026-10-31 (v1.6.3), display time only (`display_names.go`); stored IDs unchanged | S | L |
| 22 | Keep original data on revoke (flag) | §3.2 | **Deferred** | M | L |
| 35 | Delete for me while unlinked not applied | §3.3 | **Deferred** | S | L |
| 15 | Live-account checklist as release gate | §3.6 | **Recurring**: last full run 2026-10-31 (`ddbb7f9`); partial live run on `v1.6.0-rc1` | M | H |
| 17 | First release workflow run unverified | §3.1 | **Done** 2026-10-31 (v1.4.0) | S | H |
| 12 | CI and release hygiene (review finding 12) | §3.1 | **Done** 2026-10-31 (v1.6.0) | S | M |
| 13 | R1 `@lid` duplicate merge | §3.3 | **Done** 2026-10-31 (v1.5.0) | M | M |
| 6 | `show` freezes UI, leaks zombie (finding 6) | §3.4 | **Done** 2026-10-31 (v1.6.0) | S | M |
| 10 | UI update ordering (finding 10) | §3.4 | **Done** 2026-10-31 (v1.6.0) | S | M |
| 5 | `/url` blocks the event loop (finding 5) | §3.4 | **Done** 2026-10-31 (v1.6.0) | S | L |
| 7 | Media metadata never backfilled; spurious redraws (finding 7) | §3.3 | **Done** 2026-10-31 (v1.6.0) | S | L |
| 9 | Repeat download duplicates a named file (finding 9) | §3.3 | **Done** 2026-10-31 (v1.6.0) | S | L |
| 11 | Downloads not cancellable; orphan `.part` files (finding 11) | §3.3 | **Done** 2026-10-31 (v1.6.0) | M | L |
| 23 | Stale `.github/copilot-instructions.md` and PR template | §3.5 | **Done** | T | L |
| 24 | `info` key ignores the message ID | §3.4 | **Done** 2026-10-31 (v1.6.0) | T | L |
| 25 | `loadRecentChats` ignores `@lid` | §3.3 | **Done** 2026-10-31 (v1.6.0) | S | L |
| 31 | Private vulnerability reporting not confirmed enabled | §3.5 | **Done** | T | L |
| 16 | Licence | §3.5 | **Done** (v1.4.0) | S | M |
| 32 | "Delete for me" from the phone not mirrored | §3.3 | **Done** 2026-10-31 (v1.6.0) | S | L |
| 34 | `/more` has nothing to page | §3.4 | **Done** 2026-10-31 (v1.6.0) | S | L |
| 36 | `/info` blocks the command loop | §3.4 | **Done** 2026-10-31 (v1.6.0) | T | L |
| 33 | Empty "New chat" rows from history sync | §3.3 | **Done** 2026-10-31 (v1.5.0) | S | L |
| 1 | Incoming revoke authorisation (finding 1) | §3.2 | **Done** 2026-10-31, Was Medium | M | H |
| 2 | Open allowlist and Windows-safe names (finding 2) | §3.2 | **Done** 2026-10-31, Was Small | S | M |
| 3 | `@lid` chats dropped by history sync (finding 3) | §3.3 | **Done** 2026-10-31, Was Medium | M | M |
| 4 | `show` path containment (finding 4) | §3.2 | **Done** 2026-10-31, Was Small | S | M |
| 26 | Tag-driven release workflow and AUR generator | §3.1 | **Done** 2026-10-31, Was Medium | M | M |
| 27 | Module rename, dependency and toolchain bump | §3.1 | **Done** 2026-10-31, Was Medium | M | H |
| 28 | Download, open, show, url, revoke, backlog; QR and clipboard fixes | §3.3 | **Done** 2026-10-31, Was Large | L | H |
| 29 | Retire `release.sh` | §3.1 | **Done** 2026-10-31, Was Small | S | M |
| 30 | Documentation set and archive | §3.5 | **Done** 2026-10-31, Was Medium | M | M |

---

## 3. Backlog by area

### 3.1 Release and CI

Rows 8, 12, 17 (open: 8, deferred). Releases come only from `.github/workflows/release.yml`; tags with a `-` are published as pre-releases.

### 3.2 Security hardening

Rows 18 to 22. The authorisation, allowlist and containment rules are in SECURITY.md.

### 3.3 Chat and message behaviour

Rows 7, 9, 11, 13, 21, 25, 32, 33, 35 (open: 21, 35, both deferred). Row 13 shipped in v1.5.0 without row 21: `ContactId` is sent back to WhatsApp in read receipts and backlog anchors, so it keeps its stored form (§0 row 21).

### 3.4 UI

Rows 5, 6, 10, 24, 34, 36 (all done). Rows 5, 6 and 10 came from `UiHandler` queuing work with `go QueueUpdateDraw`; row 10 was fixed with `NewScreenWithStatus` (option A); an ordered update channel remains a possible later refactor (review option B).

### 3.5 Docs and repository hygiene

Rows 16, 23, 31.

### 3.6 Verification

Rows 14, 15. No test talks to WhatsApp; this area exists to keep that gap visible.

## 4. Deferred: decided, waiting on something

**Table RM4: Deferred**

| Item | Waiting on | Detail |
|---|---|---|
| macOS, Raspberry Pi, Homebrew releases (8) | Someone asking for them | §8.8 |
| Admin list cache (18) | Rate limit warnings in logs | row 18 |
| Pending-revoke memory (19) | A reported stale message | row 19 |
| Extension versus media kind check (20) | Nothing; low gain | row 20 |
| `ContactId` normalisation (21) | A need to show names at display time (the row 13 merge shipped in v1.5.0 without it) | row 21 |
| Revoked flag, keep original data (22) | A need to undo a revoke | row 22 |
| Delete for me while unlinked (35) | Users reporting ghost messages | row 35 |

## 5. Not doing: decided against, with the reason

**Table RM5: Not doing**

| Item | Why |
|---|---|
| Merging upstream wholesale | Independent fork (D1); port fixes selectively |
| Revoking other people's messages as a group admin from WhatsCLI | Needs an admin check and sender JID handling under LID; own messages only (D11) |
| Local check of the revoke time window | The server decides and its error is shown; the exact window is [Unverified] |
| Opening Office files, HTML, SVG or archives from `open` | They can run macros or scripts (D15); users open them by hand |
| Publishing to the AUR from CI | Would need an SSH key secret (D5) |
| Reaction display, voice recording, sticker sending, reply threading, status viewing | Listed as ideas in the old README "inferred roadmap"; no demand or owner decision recorded. Re-raise with a row number if wanted |

## 6. Where the detail lives

4 entries.

**Table RM6: Detail documents**

| Document | Covers |
|---|---|
| TESTING.md | Test commands, conventions, the live-account checklist (steps 1 to 33) |
| SECURITY.md | Threat notes, controls, how to report |
| ARCHITECTURE.md | Components, data flow, storage, JID handling |
| docs/archive/ | Four historic reports from February 2026, kept unchanged; open findings from them are tracked here |

## 7. Closed, kept for the reasoning

- **Review finding 1, incoming revoke (row 1).** A revoke is untrusted input. WhatsApp does not validate its contents, so every client must check chat and sender. **Rule: never apply a revoke without the same-chat and authorised-revoker checks, and keep `MarkMessageRevoked` scoped by `chat_id`.**
- **Review finding 2, open (row 2).** The OS opener chooses a program by extension and on Windows runs executables. **Rule: only allowlisted extensions reach the opener; unknown types are saved and reported.**
- **Review finding 3, `@lid` (row 3).** The same person can arrive as a phone-number JID or an LID. **Rule: store chats under `canonicalChatJID`; compare people with `sameUser`, never by string equality.**
- **Review finding 4, show path (row 4).** A prefix test on unresolved paths fails on symlinked homes and accepts siblings. **Rule: containment is decided with `ResolveFileWithin` (resolve both sides, `filepath.Rel`).**

Accepted limitations of rows closed in v1.6.0 (detail in CHANGELOG [1.6.0]):

- **Row 7.** Messages stored before media support gain download details only when WhatsApp sends them again (owner accepted old media, D13).
- **Row 9.** One `local_path` is kept per message, so alternating `download` and `open` with different download and preview folders can still create a second copy.
- **Row 11.** The startup sweep has no age threshold; a second instance started during a download would remove the first one's temp file. Add a threshold if anyone runs two instances.
- **Row 25.** Only groups create new chats from the contact list; an `@lid` contact with no name is skipped so a bare LID number never replaces a real name.
- **Row 32.** Deletes made while WhatsCLI was unlinked are not applied (row 35).

## 8. Open rows in full

### 8.8 Release targets dropped

Old `release.sh` built darwin, windows, linux and linux armv5 and updated the upstream Homebrew tap; `release.yml` builds linux/amd64, linux/arm64, windows/amd64 only. Options: (A) add `macos-latest` (arm64) and an Intel macOS runner, a Homebrew formula artifact like the AUR one, and an armv6 or armv7 job with `CC=arm-linux-gnueabihf-gcc`; costs CI minutes and a cross toolchain. (B) Drop on purpose (current state; README and CHANGELOG say so). Reviewer recommendation: A for macOS (native runners make CGO easy), B for 32-bit Pi unless there is demand. Owner chose B for now (D4).

### 8.14 R2 backlog anchor JID form

The anchor uses the chat and sender JIDs as stored. After D16 that is usually the phone-number form. [Unverified] Whether the phone answers when its own record is LID. Options: (A) verify live (checklist step 17); (B) if there is no reply, build the anchor with the LID from `GetLIDForPN`. Recommendation: A first.

### 8.15 Live-account checklist as a release gate

Everything WhatsApp-facing is covered only by fakes. The procedure (33 steps, covering migration, media, revoke both ways, backlog, `@lid`, opener gate, show with symlinks, clipboard, QR, chat paging, group info, Delete for me, repeat download and the part-file sweep) is in TESTING.md. Use a non-production number and a copy of `session_meta.db`. Record each run in TESTING.md Table T3 and summarise it in row 15. Options: (A) run it manually before each tag (current); (B) build a client seam to cover the `cmdRevoke` success path in unit tests (the download goroutine is already covered through the `mediaDownloader` seam) while keeping the live run for protocol behaviour. Recommendation: A now (current practice); B is not started.
