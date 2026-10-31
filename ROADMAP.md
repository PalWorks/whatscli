# Roadmap & Backlog: WhatsCLI

> **The single source of truth for forward-looking work.** Anything not shipped lives here. If an item needs more than a paragraph, it has a subsection in §8 or a file linked from §6. Shipped history is in [CHANGELOG.md](CHANGELOG.md), system design in [ARCHITECTURE.md](ARCHITECTURE.md), the verification procedure in [TESTING.md](TESTING.md).
>
> **Start at §0.** It lists every open item, one row each. §2.2 is the ledger: the same open rows plus every closed one.
>
> Timestamps are ISO 8601 to the second, with the offset. "Raised" dates for rows 1 to 25 are 2026-10-31, the date of the independent review (REVIEW-3, an agent report that is not stored in the repository) and of the residual-risk notes written with the fixes; git has no earlier commit for them.
>
> _Last updated: 2026-10-31T08:03:19+00:00 (v1.5.0: rows 13 and 33 released)._

## 0. North star: every open item, one row each

Grouped by state (Open, Decision, Waiting, Blocked, Deferred, Recurring), then by priority. `#` is the ledger number and never changes.

**Table RM0: Open items**

| # | Bucket | Priority | State | Size | Impact | Raised | Problem | Solution or next step |
|---|---|---|---|---|---|---|---|---|
| 17 | §3.1 | P1 | **Done** | S | H | 2026-10-31 | Done 2026-10-31: `v1.4.0-rc1` then `v1.4.0` built all three archives; checksums verified. The release workflow has never run on a real tag. If `ubuntu-24.04-arm` or the Windows CGO build fails, the first release fails and blocks the `release` job. | Tag a release candidate on a throwaway tag, check the 3 archives and checksums, then the real tag. [§8.17](#817-first-release-run-unverified) |
| 12 | §3.1 | P2 | **Open** | S | M | 2026-10-31 | CI and release hygiene: mutable action tags in a `contents: write` job, Go 1.26 matrix leg may run 1.27.3, AUR `pkgver` invalid for `-rc` tags, release notes pick the wrong previous tag. | One small CI change covering all four plus `go mod tidy`. [§8.12](#812-ci-and-release-hygiene) |
| 13 | §3.3 | P2 | **Done** | M | M | 2026-10-31 | Done 2026-10-31 in v1.5.0 (`messages/lid_merge.go`, tests in `lid_merge_test.go`); dry run on a copy of real data merged 8 duplicates. R1: an `@lid` chat stored before its phone-number mapping was known stays a separate chat from the person's phone-number chat, so one person can appear twice. | Approved (owner, 2026-10-31, D20): merge the `@lid` rows into the phone-number chat once the mapping is known. Start after the live-account checklist (row 15, step 17) has measured how often it occurs. [§8.13](#813-r1-lid-duplicate-merge) |
| 14 | §3.6 | P2 | **Open** | S | M | 2026-10-31 | Partly verified 2026-10-31: the phone answered a backlog request in a phone-number chat; no `@lid` chat was available to test. [Unverified] The phone may not answer an ON_DEMAND backlog request when the anchor chat is a phone-number JID but its own record is LID. `/backlog` would silently do nothing there. | Verify with checklist step 17 on a live account; if no reply, send the anchor in LID form via `GetLIDForPN`. [§8.14](#814-r2-backlog-anchor-jid-form) |
| 6 | §3.4 | P2 | **Open** | S | M | 2026-10-31 | `show` runs the viewer on the UI goroutine: a slow viewer freezes all input and redraws, and every use leaves a zombie process (`Wait` is never called). | Run the viewer off the UI goroutine with `cmd.Output()` under a timeout, then `QueueUpdateDraw` only to write the bytes (review option A). |
| 10 | §3.4 | P2 | **Open** | S | M | 2026-10-31 | "Loaded N additional messages" and "revoked: id" can be wiped by the redraw that follows, because UI updates are queued from separate goroutines. The user cannot tell whether the action worked. | Print status after the redraw inside one queued update, or use the status bar (review option A). Ordered UI channel is a later refactor. |
| 5 | §3.4 | P3 | **Open** | S | L | 2026-10-31 | `/url` runs the system opener synchronously on the event loop and discards its error. A hanging opener or terminal browser stops all commands. | Use `open.Start`, `go cmd.Wait()`, and return the error through `PrintError`; change `OpenFile` to return an error (touches the interface and mock). |
| 7 | §3.3 | P3 | **Open** | S | L | 2026-10-31 | `INSERT OR IGNORE` never fills media metadata on rows stored before the upgrade, and every history chunk redraws the open chat even if nothing new was stored. | Upsert that fills media columns only when empty and never on a revoked row, and return whether a row changed (review option A). Owner accepted old media for now (D13). |
| 9 | §3.3 | P3 | **Open** | S | L | 2026-10-31 | The first repeat download of a named document saves a second copy `<stem>_<id>`; the reuse comment holds only from the third download. | Record the saved path in SQLite (`local_path`) and reuse it, or accept the duplicate and fix the comment. |
| 11 | §3.3 | P3 | **Open** | M | L | 2026-10-31 | Downloads use `context.Background()`. Quitting mid-download leaves `.whatscli-*.part` files in Downloads, and they accumulate. | Root context cancelled in `Shutdown`, used for downloads, backlog and revoke; sweep stale `.part` files at startup. |
| 23 | §3.5 | P3 | **Done** | T | L | 2026-10-31 | `.github/copilot-instructions.md` still says Go 1.24 and 17 methods; `.github/PULL_REQUEST_TEMPLATE.md` is the 2025 whatsmeow-migration text. Copilot users get wrong rules. Done 2026-10-31T01:13:06+05:30. | Replace both with a pointer to AGENTS.md and a generic PR checklist (docs pass was not allowed to edit `.github/`). |
| 24 | §3.4 | P3 | **Open** | T | L | 2026-10-31 | The `i` key sends `info` with the message ID, but `cmdInfo` ignores it and prints chat or group info, not message details. | Either implement message info or relabel the key and README entry as "chat info". |
| 25 | §3.3 | P3 | **Open** | S | L | 2026-10-31 | `loadRecentChats` (contact list import) still accepts only phone-number and group chats, so `@lid` contacts are not imported at start. | Reuse `isHistoryChatServer` and `canonicalChatJID` there, with a test. |
| 31 | §3.5 | P3 | **Done** | T | L | 2026-10-31 | SECURITY.md points reporters to GitHub private vulnerability reporting, which may not be enabled on `PalWorks/whatscli`. A reporter would have no private channel. | Owner enables it in repository Settings, Code security; or names another contact in SECURITY.md. |
| 16 | §3.5 | P2 | **Done** | S | M | 2026-10-31 | Repository had no `LICENSE` file. | Dual licence added: AGPL-3.0-only (`LICENSE`) plus paid commercial (`LICENSE-COMMERCIAL.md`). [§8.16](#816-licence) |
| 32 | §3.3 | P3 | **Open** | S | L | 2026-10-31 | "Delete for me" on the phone (the only delete offered in "Message yourself") is synced to linked devices as app state, but WhatsCLI ignores it, so the message stays visible (live run 2026-10-31). | Handle whatsmeow's `events.DeleteForMe` by removing or marking the stored row, with a test. |
| 33 | §3.3 | P3 | **Done** | S | L | 2026-10-31 | Done 2026-10-31 with option A in v1.5.0 (owner choice); copy of real data: 8,456 to 504 chat rows. History sync stores every contact WhatsApp lists as an empty chat ("New chat", no messages, no time): on a real account 8,173 of 8,456 chat rows, 1,410 of them under a LID with no known phone number. They sit at the bottom of the chat list and bloat it. | Options: (A) skip conversations with no messages and no timestamp in `storeHistorySync`, and delete existing empty "New chat" rows once; (B) keep them but hide empty chats in the list; (C) leave as is. Recommendation A. |
| 8 | §3.1 | P2 | **Deferred** | M | M | 2026-10-31 | Releases no longer include macOS, Raspberry Pi (armv5) or a Homebrew tap (owner decision D4). macOS and 32-bit Pi users find no asset. | Back when someone asks: macOS = add a `macos-latest` matrix entry; Homebrew = add a tap job with a token scoped to the tap repository; armv5 needs a cross C toolchain. [§8.8](#88-release-targets-dropped) |
| 18 | §3.2 | P3 | **Deferred** | S | L | 2026-10-31 | R3: each revoke by a non-sender in a group costs one group info request, and a non-admin can trigger these by sending fake revokes. | Cache admin lists per group for some minutes if rate limits appear in logs. Accepted for now. |
| 19 | §3.2 | P3 | **Deferred** | S | L | 2026-10-31 | R4: a revoke that arrives before its target (offline queue order) is ignored, so the message stays visible. | Remember pending revokes briefly. Accepted for now. |
| 20 | §3.2 | P3 | **Deferred** | S | L | 2026-10-31 | R5: the open allowlist judges only the extension; a PDF or image viewer exploit is still possible, as in any client. | Also check that media kind matches the extension. Small gain; accepted. |
| 21 | §3.3 | P3 | **Deferred** | S | L | 2026-10-31 | Not done with row 13 (2026-10-31): `ContactId` is sent back to WhatsApp as the participant in read receipts (`markChatAsRead`) and backlog anchors (`backlogAnchor`), so rewriting stored LIDs to phone numbers could break those in LID-addressed groups. Normalise only at display time if names need it. R6: `ContactId` is stored in whatever form arrives (phone number or LID); only chat IDs are normalised. Names or search by contact may show an LID number until whatsmeow knows the contact. | Normalise `ContactId` together with the row 13 merge. |
| 22 | §3.2 | P3 | **Deferred** | M | L | 2026-10-31 | A spoofed or mistaken revoke overwrites text and media data, which cannot be recovered (review option C). | Add a `revoked` flag column and keep the original data; needs another migration. Authorisation (D14) already closes the spoof. |
| 15 | §3.6 | P1 | **Recurring** | M | H | 2026-10-31 | No automated test talks to WhatsApp, so media, revoke, backlog, `@lid`, show and QR behaviour is verified only by unit tests with fakes. Last run: 2026-10-31 on `ddbb7f9`, 12 steps passed, 5 bugs fixed, 8 skipped (TESTING Table T3). | Run the full live-account checklist in [TESTING.md](TESTING.md) on a non-production number before every tagged release. [§8.15](#815-live-account-checklist-as-a-release-gate) |

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

## 2. Next up

### 2.1 Status: v1.4.0 released (updated 2026-10-31T07:03:48+00:00)

- `chore/catch-up` merged to `master` (PR 2) and tagged `v1.4.0`; release assets and checksums verified; `v1.4.0-rc1` kept as a pre-release. `go install github.com/PalWorks/whatscli@v1.4.0` resolves.
- Next: row 13 (approved), then the open P2 rows.

Earlier status, kept for the record:

- Code: 6 commits ahead of `master`; build, vet and `go test -race -count=1 ./...` pass (verified when this file was written). Not merged, not tagged.
- Gates before tagging: row 15 (live-account checklist, TESTING.md) and row 17 (first workflow run on a release candidate tag).
- Approved next after the gates: row 13, merge old `@lid` chats (owner, 2026-10-31, D20), once checklist step 17 has measured how often it occurs.
- [Inference] `go install github.com/PalWorks/whatscli@latest` will fail until a new tag exists, because the `v1.3.0` tag predates the module rename; `@master` works after the merge. The README says so.
- Version number is the owner's choice. [Inference] The agents' handover suggested `v1.3.1`; because commands and storage columns were added, `v1.4.0` fits semantic versioning better.

### 2.2 The ledger: every backlog item, pending first, done at the bottom

**This table is authoritative for state; §3 and §8 are authoritative for reasoning.** Open rows repeat §0 in one line; §0 holds the problem and solution. Closed rows keep only their date and size.

**Table RM2: Ledger**

| # | Item | Section | State | Size | Impact |
|---|---|---|---|---|---|
| 17 | First release workflow run unverified | §3.1 | **Done** 2026-10-31 | S | H |
| 12 | CI and release hygiene (review finding 12) | §3.1 | **Open** | S | M |
| 13 | R1 `@lid` duplicate merge | §3.3 | **Done** 2026-10-31 (v1.5.0) | M | M |
| 14 | R2 backlog anchor JID form | §3.6 | **Open**: verify live | S | M |
| 6 | `show` freezes UI, leaks zombie (finding 6) | §3.4 | **Open** | S | M |
| 10 | UI update ordering (finding 10) | §3.4 | **Open** | S | M |
| 5 | `/url` blocks the event loop (finding 5) | §3.4 | **Open** | S | L |
| 7 | Media metadata never backfilled; spurious redraws (finding 7) | §3.3 | **Open** | S | L |
| 9 | Repeat download duplicates a named file (finding 9) | §3.3 | **Open** | S | L |
| 11 | Downloads not cancellable; orphan `.part` files (finding 11) | §3.3 | **Open** | M | L |
| 23 | Stale `.github/copilot-instructions.md` and PR template | §3.5 | **Done** | T | L |
| 24 | `info` key ignores the message ID | §3.4 | **Open** | T | L |
| 25 | `loadRecentChats` ignores `@lid` | §3.3 | **Open** | S | L |
| 31 | Private vulnerability reporting not confirmed enabled | §3.5 | **Done** | T | L |
| 16 | Licence | §3.5 | **Done** | S | M |
| 8 | Release targets dropped (finding 8) | §3.1 | **Deferred**: owner decision D4 | M | M |
| 18 | R3 admin revoke lookup cost | §3.2 | **Deferred** | S | L |
| 19 | R4 revoke before target | §3.2 | **Deferred** | S | L |
| 20 | R5 extension versus media kind | §3.2 | **Deferred** | S | L |
| 21 | R6 `ContactId` normalisation | §3.3 | **Deferred**: display-time only (see §0) | S | L |
| 22 | Keep original data on revoke (flag) | §3.2 | **Deferred** | M | L |
| 15 | Live-account checklist as release gate | §3.6 | **Recurring**: last run 2026-10-31 (`ddbb7f9`) | M | H |
| 32 | "Delete for me" from the phone not mirrored | §3.3 | **Open** | S | L |
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

Rows 8, 12, 17. Releases come only from `.github/workflows/release.yml`.

### 3.2 Security hardening

Rows 18 to 22. The authorisation, allowlist and containment rules are in SECURITY.md.

### 3.3 Chat and message behaviour

Rows 7, 9, 11, 13, 21, 25. Rows 13 (approved, D20) and 21 belong together: merging old `@lid` chats and normalising `ContactId` touch the same stored rows and the in-memory queue.

### 3.4 UI

Rows 5, 6, 10, 24. Rows 5, 6 and 10 all come from `UiHandler` queuing work with `go QueueUpdateDraw`; an ordered update channel would fix 10 everywhere (review option B, larger refactor).

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
| `ContactId` normalisation (21) | The row 13 merge (approved, D20) | row 21 |
| Revoked flag, keep original data (22) | A need to undo a revoke | row 22 |

## 5. Not doing: decided against, with the reason

**Table RM5: Not doing**

| Item | Why |
|---|---|
| Merging upstream wholesale | Independent fork (D1); port fixes selectively |
| Backfilling media for messages stored before the upgrade | Owner accepted old media as non-downloadable (D13); revisit via row 7 if users ask |
| Revoking other people's messages as a group admin from WhatsCLI | Needs an admin check and sender JID handling under LID; own messages only (D11) |
| Local check of the revoke time window | The server decides and its error is shown; the exact window is [Unverified] |
| Opening Office files, HTML, SVG or archives from `open` | They can run macros or scripts (D15); users open them by hand |
| Publishing to the AUR from CI | Would need an SSH key secret (D5) |
| Reaction display, voice recording, sticker sending, reply threading, status viewing | Listed as ideas in the old README "inferred roadmap"; no demand or owner decision recorded. Re-raise with a row number if wanted |

## 6. Where the detail lives

5 entries.

**Table RM6: Detail documents**

| Document | Covers |
|---|---|
| TESTING.md | Test commands, conventions, the live-account checklist (steps 1 to 21) |
| SECURITY.md | Threat notes, controls, how to report |
| ARCHITECTURE.md | Components, data flow, storage, JID handling |
| docs/archive/ | Four historic reports from February 2026, kept unchanged; open findings from them are tracked here |

## 7. Closed, kept for the reasoning

- **Review finding 1, incoming revoke (row 1).** A revoke is untrusted input. WhatsApp does not validate its contents, so every client must check chat and sender. **Rule: never apply a revoke without the same-chat and authorised-revoker checks, and keep `MarkMessageRevoked` scoped by `chat_id`.**
- **Review finding 2, open (row 2).** The OS opener chooses a program by extension and on Windows runs executables. **Rule: only allowlisted extensions reach the opener; unknown types are saved and reported.**
- **Review finding 3, `@lid` (row 3).** The same person can arrive as a phone-number JID or an LID. **Rule: store chats under `canonicalChatJID`; compare people with `sameUser`, never by string equality.**
- **Review finding 4, show path (row 4).** A prefix test on unresolved paths fails on symlinked homes and accepts siblings. **Rule: containment is decided with `ResolveFileWithin` (resolve both sides, `filepath.Rel`).**

## 8. Open rows in full

### 8.8 Release targets dropped

Old `release.sh` built darwin, windows, linux and linux armv5 and updated the upstream Homebrew tap; `release.yml` builds linux/amd64, linux/arm64, windows/amd64 only. Options: (A) add `macos-latest` (arm64) and an Intel macOS runner, a Homebrew formula artifact like the AUR one, and an armv6 or armv7 job with `CC=arm-linux-gnueabihf-gcc`; costs CI minutes and a cross toolchain. (B) Drop on purpose (current state; README and CHANGELOG say so). Reviewer recommendation: A for macOS (native runners make CGO easy), B for 32-bit Pi unless there is demand. Owner chose B for now (D4).

### 8.12 CI and release hygiene

1. Actions are pinned to mutable tags (`checkout@v7`, `setup-go@v7`, `upload-artifact@v7`, `download-artifact@v8`) in a job with `contents: write`. Option: pin to commit SHAs and let Dependabot bump them. Trade-off: supply-chain safety versus churn in pull requests.
2. `go.mod` has `toolchain go1.27.3`. [Inference] With `GOTOOLCHAIN=auto` the `1.26` matrix leg switches to 1.27.3 and 1.26 is never tested. Option: `GOTOOLCHAIN: local` in `ci.yml` plus a `go version` step.
3. The tag regex allows `v1.2.3-rc1`, but `generate-aur-pkgbuilds.sh` sets `pkgver=1.2.3-rc1` and Arch forbids `-` in `pkgver`. Option: map `-` to `_` for `pkgver` while keeping the tag in `source`.
4. Release notes use `git tag --sort=-v:refname | grep -vx "$RELEASE_TAG" | head -n1`, which picks the highest other tag, not the preceding one. Option: `git describe --tags --abbrev=0 "$RELEASE_TAG^"`.
5. `go mod tidy -diff` shows stale `go.sum` lines (harmless). Run `go mod tidy` once.

Reviewer recommendation: do all five in one small CI change.

### 8.13 R1 `@lid` duplicate merge

Chats are normalised to the phone-number JID only for new data (D16). A chat stored under `@lid` earlier stays, and new messages for that person go to the phone-number chat, so the person appears twice. `/backlog` in the old chat prints a note naming the other chat.

**Decided (owner, 2026-10-31, D20): option A.** One-time merge when a mapping becomes known: update `messages.chat_id` to the phone-number JID, merge the `conversations` row (name, last message time, unread, pinned, archived), fix the in-memory priority queue and `convByJID`, and fix `currentReceiver` if the merged chat is open. Option B (leave duplicates, rely on the backlog note) was rejected.

Sequencing: start after the live-account checklist (row 15, step 17) has counted the `@lid` chats on real accounts and shown the mapping behaviour. Do row 21 (`ContactId` normalisation) in the same change. The merge needs tests for: two rows with the same message ID (`INSERT OR IGNORE` semantics, the first write wins), a merged chat that is open, pinned and archived flags, and a mapping that is still unknown (no change).

### 8.14 R2 backlog anchor JID form

The anchor uses the chat and sender JIDs as stored. After D16 that is usually the phone-number form. [Unverified] Whether the phone answers when its own record is LID. Options: (A) verify live (checklist step 17); (B) if there is no reply, build the anchor with the LID from `GetLIDForPN`. Recommendation: A first.

### 8.15 Live-account checklist as a release gate

Everything WhatsApp-facing is covered only by fakes. The procedure (21 steps, covering migration, media, revoke both ways, backlog, `@lid`, opener gate, show with symlinks, clipboard, QR) is in TESTING.md. Use a non-production number and a copy of `session_meta.db`. Record each run here: date, build, who, result. Options: (A) run it manually before each tag (current); (B) build a client seam to cover `runDownload` and `cmdRevoke` success paths in unit tests (the review noted the gap) while keeping the live run for protocol behaviour. Recommendation: A now, B when a second release is planned.

### 8.16 Licence

Done. WhatsCLI is dual licensed under AGPL-3.0-only ([LICENSE](LICENSE)) and a paid commercial licence ([LICENSE-COMMERCIAL.md](LICENSE-COMMERCIAL.md)); contact support@palworks.ai.

### 8.17 First release run unverified

Nothing in `release.yml` or the AUR generator has run on a real tag. Risks: `ubuntu-24.04-arm` is free only for public repositories, a private repository may queue or bill; if the arm64 job fails the `release` job is blocked by `needs: build`; Windows CGO relies on the runner's MinGW gcc; the generated PKGBUILD has not been built with `makepkg`. Options: (A) push a release-candidate tag such as `v1.4.0-rc1` first and delete the release afterwards; (B) use `workflow_dispatch` on an existing tag. Recommendation: A. If arm64 fails, remove its matrix entry and its packaging loop item.
