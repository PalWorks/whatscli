# Implementation Plan: Pending-Items Rectification (2026-07-19)

Scope: rectify every open item from the Consolidated Pending Items Report (2026-07-19),
which aggregates the Phase 4.3 handoff's pre-existing architectural issues (A1-A5),
the Phase 5 master-audit deferrals (B1-B3), the deferred plan item C1, the test-quality
gaps (D1-D3), and the process items (E1-E2). Everything here was re-verified against
the current working tree on 2026-07-19 before planning.

Baseline: branch `phase-1-truth-safety`, HEAD `1f0c6f1`, 58 uncommitted changed/new
files, v1.7.0, 277 tests passing under `-race`, build/vet/gofmt clean.

Verification finding made during planning: **B3 (export unlink-before-close ordering)
is already correct in the current tree.** All three error branches in
`encryptMessagesToPath` (`messages/cmd_export.go:127-146`) call `f.Close()` before
`os.Remove(path)`. B3 is therefore closed as "no change needed" and appears in no
phase below.

---

## Phase Map

### Table: PhaseMap

| Phase | Covers | Theme | Depends on |
|-------|--------|-------|------------|
| H0 | E1 | Protect existing work (commit) | user approval |
| H1 | A1, A2 | Network hardening: timeouts and non-blocking command loop | H0 (recommended) |
| H2 | A3, B1, B2 | Resource lifecycle: container close, signal trap, export read | H1 (shares files) |
| H3 | A4, A5 | Small correctness gaps: backlog waiters, forwarded coverage | none |
| H4 | C1 | Feature: per-message delivery/read markers | H3 (touches event handler) |
| H5 | D1, D2 | Testability seams and coverage re-measure | H1 (login guard) |
| H6 | D3, E2, docs | Documentation, v1.8.0, master audit | all above |

Rationale for the order: H0 first because all five prior release milestones exist
only in the working tree. H1 is the highest-severity code work and restructures the
same functions H2 and H5 touch, so it goes before them. H3 and H4 are independent of
H1/H2 but H4 builds on the event-handler file H3 edits. H6 is the exit gate.

---

## Phase H0: Protect Existing Work (E1)

**Problem.** Phases 1 through 5 (v1.3.1 to v1.7.0) exist only as uncommitted files.
Any accident (disk, reset, tooling) loses five releases of work.

**Action.** Commit the current tree before touching any code. Two options:

1. Single commit: `feat: phases 1-5 (v1.3.1..v1.7.0): truth/safety, media pipeline,
   live correctness, testability, feature growth`. CHANGELOG.md already records the
   per-phase history, so granular history is preserved in-content. Recommended:
   lowest risk, zero archaeology.
2. Five thematic commits, splitting files by phase. Not recommended: many files
   (storage.go, event_handler.go, README.md, ui_*.go) span multiple phases, so any
   retroactive split is partly fictional and risks a broken intermediate commit.

**Resolution (user, 2026-07-19): thematic commits.** Since per-phase splitting is
partly fictional (files span phases), the series is grouped by architectural theme
and ordered by dependency, so breakage between intermediate commits is minimized
and the tip is fully green:

1. build: go.mod, go.sum, CI workflow (deps upgrade; old code may not compile
   against the new whatsmeow until commit 3, accepted for a feature branch)
2. config: config/settings.go
3. messages backend + its tests + mock (self-contained package, builds once 1-2
   exist)
4. UI layer: app_context.go, main.go, ui_*.go + root tests (whole tree green here)
5. docs: README, CHANGELOG, AI-assistant config files, plan/status reports

Exit criteria: `git status` clean; full suite green at the tip (nothing changed
code-wise, verified before committing).

---

## Phase H1: Network Hardening (A1 + A2)

The two items share call sites, so they land as one phase with two mechanisms:

- **Mechanism 1 (A1):** every whatsmeow network call gets a bounded context instead
  of `context.Background()`.
- **Mechanism 2 (A2):** every network call reachable from `execCommand` moves off
  the command-loop goroutine, following the exact pattern already used by
  `cmdRevoke`/`cmdReply`/`cmdMedia` (synchronous cheap guards, then
  `go func(){ network + result handling }()`).

### H1.1 Timeout infrastructure (A1)

New code in `messages/` (suggested file: `netctx.go`):

```go
func netCtx() (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(),
        time.Duration(config.Config.General.NetworkTimeout)*time.Second)
}

func netTransferCtx() (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(),
        time.Duration(config.Config.General.NetworkTransferTimeout)*time.Second)
}
```

Design decision (user, 2026-07-19): config keys, not constants. Two new
`[general]` keys in `config/settings.go`, matching the existing
`notification_timeout` style:

- `network_timeout` (seconds, default 60): request-response operations
  (sends, group metadata, receipts).
- `network_transfer_timeout` (seconds, default 300): bulk transfers, sized
  for ~100MB (the existing cmdMedia cap) on a slow link.

Guard in the helpers: a zero or negative configured value falls back to the
default rather than producing an already-expired context (a user setting 0 to
"disable" must not break every network call). Both keys documented in README's
configuration table.

Call-site inventory (complete, from a fresh audit of every `context.Background()`):

### Table: TimeoutCallSites

| File:line (current) | Call | Tier | Notes |
|---|---|---|---|
| chat_state.go:74 | `client.MarkRead` | netCtx | already backgrounded |
| chat_state.go:125 | `client.GetGroupInfo` | netCtx | runs on event goroutine |
| chat_state.go:190 | `client.SendMessage` (sendText) | netCtx | also moves off loop (H1.2) |
| cmd_chat.go:113 | `client.SendMessage` (backlog request) | netCtx | also moves off loop (H1.3) |
| cmd_chat.go:165 | `client.GetJoinedGroups` | netCtx | already backgrounded |
| cmd_group.go:31,61,85,118,153,188,223 | group ops | netCtx | also move off loop (H1.4) |
| cmd_media.go:115,186 | `client.Upload` / `SendMessage` | netTransferCtx | already backgrounded |
| cmd_message.go (download/revoke/reply/edit sends) | `DownloadMediaWithPath` / `SendMessage` | transfer for download, netCtx for sends | already backgrounded |
| history_sync.go:38,46 | `GetAllContacts` / `GetJoinedGroups` | GetAllContacts is local store (no change); GetJoinedGroups netCtx | already backgrounded |
| event_handler.go:610,623 | `Store.Contacts.GetContact` | none | local DB read, not network |
| connection.go:45,51 | `sqlstore.New` / `GetFirstDevice` | none | local SQLite |
| connection.go:126,317 / cmd_connection.go:42 | `client.Store.Delete` | none | local SQLite |
| connection.go:163 | `client.GetQRChannel` | **none, deliberately** | QR pairing lifetime is user-paced (scan time); whatsmeow manages the channel; a 60s deadline would break first-login |

Explicitly out of scope for timeouts: `client.Connect()` (whatsmeow supplies its own
dial/handshake handling and the auto-reconnect path already recovers), the QR
channel, and all local sqlstore/contact-store calls.

### H1.2 sendText off the command loop (A2)

`messages/chat_state.go` `sendText`: keep all guards synchronous (Status refusal,
client nil/IsConnected, ParseJID), then move `SendMessage` + `AddMessage` + the
isCurrent/NewMessage block into `go func(){}()`, exactly mirroring `cmdReply`.

Accepted tradeoff, stated openly: two rapid consecutive sends may reach the server
out of order because each runs in its own goroutine. This is already true of every
other backgrounded send in the codebase (reply, edit, media); WhatsApp orders by
server receipt. Not introducing a serialized send-queue: it would be new machinery
that no existing command uses. (Decision point for user if strict local ordering
matters.)

### H1.3 cmdBacklog request send off the command loop (A2, new site found in planning)

`messages/cmd_chat.go:113`: the history-sync request `SendMessage` runs on the
command loop. Restructure: move the send into the already-existing waiter goroutine
(send first, then the existing select/wait logic, same goroutine). The stale-signal
drain moves with it. Net effect: one goroutine instead of sync-send + async-wait,
no behavior change beyond unblocking the loop. (If H3.1 lands, the drain hack is
deleted entirely in favor of waiter registration before the send.)

### H1.4 Group operations off the command loop (A2)

All 7 handlers in `messages/cmd_group.go` (create, subject, leave, add, remove,
admin, removeadmin): keep JID parsing/param guards and the nil-client check
synchronous, wrap the network call + success/error print in `go func(){}()`.
Mechanical, identical shape 7 times, using the captured `client` (post-IsConnected
check), matching `cmdReply`'s convention. Note: these handlers today check only
`client != nil`, not `IsConnected()`; upgrade them to the full
`client == nil || !client.IsConnected()` guard for consistency while touching them.

### H1.5 /connect off the command loop with a connecting guard (A2, highest risk)

`cmdLogin` (`messages/cmd_connection.go:13`) calls `sm.login()` synchronously; the
QR pairing loop inside `loginWithQRCode` can hold the command loop for minutes.

Design:
1. New `SessionManager` field `connecting bool`, guarded by `sm.mu` like its
   siblings.
2. The guard lives inside `login()` itself (not cmdLogin) so both callers, manual
   `/connect` and `scheduleReconnect`, share it: on entry, under lock, if
   `connecting` return a sentinel error ("connection attempt already in progress");
   else set it, and `defer` the clear.
3. `cmdLogin` becomes: synchronous guard-free dispatch of
   `go func(){ err := sm.login(); print result }()`. The "already in progress"
   error surfaces via PrintError like any other.
4. `scheduleReconnect` needs no change beyond tolerating the sentinel (a reconnect
   attempt that collides with a manual /connect just counts as a failed attempt and
   retries or exits; the backoff loop already handles error returns).

Risk to manage: `runManager`'s startup already backgrounds `loginWithConnection`
(session_manager.go:132). That startup goroutine must also respect the flag, or a
user typing /connect during startup races it. Route the startup path through the
same guarded `login()` entry point (it currently calls `loginWithConnection`
directly; unify so every login flows through the guard).

### H1 testing

- Existing guard-chain tests (nil client, bad JID, Status refusal) must pass
  unchanged: guards stay synchronous, which those tests rely on.
- New: `TestLogin_RefusesConcurrentAttempt` (set `connecting` true, call the guard
  path, assert the sentinel error, no network needed). This also shrinks D1's
  exemption (see H5).
- New: table test that `netCtx()`/`netTransferCtx()` return contexts with deadlines
  of the expected magnitude.
- Timeout behavior against a real dead network is not unit-testable here; the
  correctness argument is structural (bounded context passed instead of
  Background) and is a named check in the H6 audit.

Exit criteria: build/vet/gofmt clean; full `-race` suite green; zero
`context.Background()` remaining on any network call except the documented QR/Connect
exclusions; zero network calls reachable synchronously from `execCommand` except
documented single-row DB lookups.

Estimated effort: [Inference] 4-6 hours including tests.

---

## Phase H2: Resource Lifecycle (A3 + B1 + B2)

### H2.1 sqlstore.Container lifecycle (A3)

Verified: `(*sqlstore.Container).Close() error` exists in the current whatsmeow.

Leak sites (all confirmed in current tree):
- `login()` (connection.go:80-82): sets `sm.client = nil`, old container survives,
  `getConnection` then creates a fresh one over it.
- `logout()` (connection.go:308): same pattern.
- `cmdReset` (cmd_connection.go:50): `sm.container = nil` with no Close.
- `Shutdown()` (session_manager.go): never closes the container at all.

Design: one helper, called from all four sites.

```go
// closeContainerLocked closes and clears the session store container.
// Caller must hold sm.mu. The client must already be disconnected or
// about to be discarded; Close only tears down the container's SQLite
// handle, which whatsmeow does not use after Disconnect.
func (sm *SessionManager) closeContainerLocked() {
    if sm.container != nil {
        if err := sm.container.Close(); err != nil {
            sm.uiHandler.PrintError(fmt.Errorf("failed to close session store: %v", err))
        }
        sm.container = nil
    }
}
```

Ordering rule at every site: Disconnect (or confirm client discarded) first, then
close container, then (if reconnecting) `getConnection` builds fresh. In `logout()`
and `Shutdown()` the disconnect already precedes; in `login()` insert the close
where `sm.client = nil` happens; in `cmdReset` replace the bare `sm.container = nil`.

Caution flag for implementation: `closeContainerLocked` calls `PrintError`, which
enqueues a UI update; confirm the UI queue path takes no `sm.mu` lock (it does not,
per the Phase 4.3 non-blocking queue design) so calling it under the lock is safe.
If in doubt, collect the error under lock and print after unlock, matching the
"no side effects under mu" house style.

Test: with an in-memory pattern this is hard to assert directly; add a unit test at
the seam level if H5's path seam lands (reset path closes container: assert via a
test double is not possible on a concrete struct, so this is a named manual-review
item in H6's audit instead. State this honestly in the test-coverage map.)

### H2.2 Signal trap in main.go (B1)

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
go func() {
    <-sigCh
    ctx.App.Stop()
}()
```

Placed just before `ctx.App.Run()`. `App.Stop()` makes `Run()` return, and the
existing `ctx.SessionManager.Shutdown()` on the next line already cancels an
in-progress recording (`recordCancel`), disconnects, and closes the DBs, so the
whole graceful path is reused rather than duplicated. Notes:
- Ctrl+C inside the TUI arrives as a tcell key event, not a signal; this trap is
  for external `kill` (SIGTERM) and non-TTY SIGINT.
- SIGKILL remains untrappable by definition; that residue is documented in H6
  (README known-limitations), closing the doc half of B1.

Test: not unit-testable without running the full TUI; smoke-tested manually in H6
(send SIGTERM to a sandboxed instance, assert clean exit and no orphan recorder).

### H2.3 /export chat read off the command loop (B2)

`messages/cmd_export.go:70`: move `sm.db.GetMessages(chatID)` (and its error print)
inside the existing goroutine, first statement. `MessageDatabase` wraps `*sql.DB`,
which is safe for concurrent use; every other DB call from background goroutines
already relies on this. The synchronous part of cmdExport then ends at the
destination-exists check, all cheap local guards.

Test: existing `TestCmdExport_HappyPath_EndToEnd` polls for completion so it is
timing-agnostic; guard tests unaffected. Add nothing new unless the move breaks a
test, in which case fix the test's assumption, not the design.

Exit criteria: build/vet/gofmt clean; suite green; manual SIGTERM smoke test passes;
grep confirms no site sets `sm.client = nil` or discards the manager without the
container helper.

Estimated effort: [Inference] 2-3 hours.

---

## Phase H3: Small Correctness Gaps (A4 + A5)

### H3.1 Per-waiter history-sync signaling (A4)

Current: single `historySyncDone chan struct{}` (buffer 1); two concurrent
`/backlog` waits race for one signal; cmdBacklog additionally has a drain-stale-
signal hack.

Design: waiter registry.

```go
// session_manager.go
syncWaiters []chan struct{}   // guarded by mu

func (sm *SessionManager) addSyncWaiter() <-chan struct{} {
    ch := make(chan struct{})
    sm.mu.Lock()
    sm.syncWaiters = append(sm.syncWaiters, ch)
    sm.mu.Unlock()
    return ch
}

func (sm *SessionManager) notifySyncWaiters() {
    sm.mu.Lock()
    waiters := sm.syncWaiters
    sm.syncWaiters = nil
    sm.mu.Unlock()
    for _, ch := range waiters {
        close(ch)
    }
}
```

`processHistorySync`'s defer calls `notifySyncWaiters()`. `cmdBacklog` registers
its waiter **before** sending the request (this also removes the current
between-send-and-wait race window entirely) and deletes the drain hack. The
`historySyncDone` field is removed. Every waiter wakes on every completed sync;
a waiter woken by an unrelated sync just refreshes slightly early, same
worst-case as today but with no lost wakeups.

Tests: unit test `notifySyncWaiters` wakes N concurrent waiters exactly once;
existing backlog tests updated for the removed field.

### H3.2 extractForwarded coverage (A5)

`messages/event_handler.go` `extractForwarded`: add the two message classes that
`extractMessageContent` renders but `extractForwarded` skips:

```go
if con := msg.GetContactMessage(); con != nil {
    return con.GetContextInfo().GetIsForwarded()
}
if loc := msg.GetLocationMessage(); loc != nil {
    return loc.GetContextInfo().GetIsForwarded()
}
```

Parity rule: exactly the types extractMessageContent handles, no more (no
LiveLocation etc., which the content extractor also does not handle). Extend the
existing table-driven test with forwarded contact and location cases.

Exit criteria: build/vet/gofmt clean; suite green; A4 race verified by a
`-race` test with two concurrent waiters.

Estimated effort: [Inference] 2 hours.

---

## Phase H4: Per-Message Delivery/Read Markers (C1)

The one deferred feature. Live receipts only; historical delivery state is not in
scope (history-sync protos carry a status field but mapping it reliably across
sync types is a separate research task; documented as a limitation).

### Storage

- Migration: `ALTER TABLE messages ADD COLUMN receipt_status INTEGER DEFAULT 0`
  (0 = sent/none, 1 = delivered, 2 = read). Same additive pattern as quoted_id.
- `messageColumns` 20 to 21; AddMessage placeholders and both Scan sites updated
  (the compile-checked shared-const pattern makes a miss impossible to ship).
- New method:

```go
// SetReceiptStatus upgrades receipt_status for the given message IDs.
// Monotonic: a delivered receipt never downgrades a read one (WhatsApp
// receipt ordering differs between Android and iOS).
func (md *MessageDatabase) SetReceiptStatus(ids []string, status int) error
// UPDATE messages SET receipt_status = MAX(receipt_status, ?) WHERE id IN (...)
```

### Event handling

Replace the placeholder `events.Receipt` case in `Handle`:

- `types.ReceiptTypeDelivered` ("") maps to status 1.
- `types.ReceiptTypeRead` and `types.ReceiptTypeReadSelf` map to status 2.
- Other types (sender, retry, played) ignored.
- After the DB write: if `v.Chat.String()` equals currentReceiver (under RLock),
  refresh via `NewScreen(getMessages(...))`, same shape as handleReaction.
- Runs on whatsmeow's event goroutine, consistent with every sibling case.

### Rendering

`ui_render.go`: new pure helper `receiptSuffix(fromMe bool, status int) string`
returning "" for incoming messages or status 0, a dim `✓✓` for delivered, and a
positive-colored `✓✓` for read (reusing `config.Config.Colors.Positive`; no new
config key, decision point if a dedicated color is wanted). Appended in
`getTextMessageString` after the reaction suffix.

### Tests

- storage: round-trip, monotonicity (read then delivered stays read), multi-ID,
  empty-ID-list no-op, migration-upgrade test mirroring the quote-columns one.
- event handler: fake `events.Receipt` for delivered and read paths, wrong-chat
  no-refresh path.
- render: suffix table test (incoming, sent, delivered, read).

Exit criteria: build/vet/gofmt clean; suite green; manual verification note that
live end-to-end receipt flow needs a real account (same documented limit as all
live-network behavior).

Estimated effort: [Inference] 3-4 hours.

---

## Phase H5: Testability Seams and Coverage (D1 + D2)

### H5.1 Path seam for cmdReset (D1a)

Blocker today: `config.GetSessionFilePath()` resolves via adrg/xdg whose own
`init()` reads XDG_CONFIG_HOME before tests can set it, and cmdReset deletes real
files at that path.

Seam: package-level function variable in `messages`:

```go
// sessionFilePath returns the base path for session files. A var, not a
// direct call, so tests can point it at a temp directory (see
// cmd_connection_test.go); production never reassigns it.
var sessionFilePath = config.GetSessionFilePath
```

`cmdReset` and `getConnection` call `sessionFilePath()` instead of the config
function. Tests reassign it to a `t.TempDir()`-based stub (with a `t.Cleanup`
restore), pre-create fake `session.db`/`session_meta.db` files, run cmdReset,
and assert deletion, DB re-init, and state clearing. The `"reset": false`
exemption in `commandTestCoverage` flips to `true`.

### H5.2 Narrowed login exemption (D1b)

With H1.5's connecting guard, the guard branch of login is testable with zero
network: `TestCmdLogin_RefusesWhileConnecting`. The remaining untestable surface
shrinks to "the actual dial/QR path", which stays exempt with an updated reason
comment. `"login"/"connect"` stay `false` but the comment documents exactly what
is and is not covered now. No artificial loginFunc indirection: a seam that exists
only so a test can replace the entire function under test proves nothing and adds
production indirection; rejected deliberately.

### H5.3 Coverage re-measure (D2)

Run `go test -race -cover ./...` (full, not -short) after H1-H4 land; record both
package percentages in CHANGELOG v1.8.0 alongside the v1.6.0 baseline
(messages 41.4%, root 14.2%). Expected direction: up in `messages` (new guard
tests, reset test, receipt tests); no numeric target is promised.

Exit criteria: reset exemption removed; login exemption narrowed with accurate
comment; coverage numbers recorded.

Estimated effort: [Inference] 2 hours.

---

## Phase H6: Documentation, Version, Master Audit (D3 + E2 + B1-doc)

1. **README**: new "Known Limitations" section consolidating: SIGKILL orphan
   (recorder process + temp file, B1 residue), read markers are live-only (H4),
   send ordering under concurrent sends (H1.2 tradeoff), live-network/microphone
   test limits (D3). Config/keybinding docs untouched unless a decision point
   above added a key.
2. **CHANGELOG**: v1.8.0 entry covering H1-H5 with the coverage numbers.
3. **Version bump**: main.go VERSION, README, GEMINI.md, agents.md to v1.8.0.
4. **Doc-sync check**: CLAUDE.md concurrency rules gain one line if the netCtx
   convention is adopted ("network calls use netCtx/netTransferCtx, never
   context.Background()") so future work inherits the rule.
5. **Master audit (E2)**: re-run the four-perspective audit, now against a session
   with fresh limits: concurrency/race auditor, feature-correctness auditor,
   network/resource-robustness auditor, dead-code/doc-sync auditor. Named manual
   checks that automation cannot cover: container-close ordering (H2.1),
   SIGTERM smoke test (H2.2), QR-flow-has-no-timeout confirmation (H1.1).
6. **Full gate**: build, vet, gofmt, `go test -race -count=1 ./...`, isolated-XDG
   runtime smoke test, real `~/.config/whatscli` untouched, nothing committed
   beyond H0 unless asked.

Estimated effort: [Inference] 2-3 hours including audit turnaround.

---

## Decision Points Requiring User Input

### Table: DecisionPoints

| # | Decision | Resolution (user, 2026-07-19) |
|---|----------|-------------------------------|
| 1 | H0 commit: single commit vs thematic commits | RESOLVED: thematic commits (dependency-ordered series; only the tip is guaranteed fully green, noted in each intermediate commit's context) |
| 2 | Timeout values as constants vs config key | RESOLVED: config keys `network_timeout` (60) and `network_transfer_timeout` (300) in `[general]` |
| 3 | Send ordering: accept goroutine-per-send reordering vs serialized send queue | RESOLVED: Option A, goroutine per send (impact guidance reviewed by user) |
| 4 | Read-marker color: reuse Positive vs new `[colors]` key | RESOLVED: reuse Positive |

## Total Estimate

[Inference] 15-20 hours of implementation work across H1-H6, sequenced as listed.
Each phase ends with the standard gate (build/vet/gofmt/full -race suite) plus a
per-phase strict audit before moving on, per the established working protocol.
