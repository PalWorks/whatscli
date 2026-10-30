# CLAUDE.md

> Claude Code reads this file automatically. The canonical agent contract is **[AGENTS.md](AGENTS.md)**; read it first. Do not duplicate rules here.
>
> _Last updated: 2026-10-31T01:11:58+05:30 (reduced to a pointer plus a short essentials list; stale facts removed)_

## Essentials (full rules in AGENTS.md)

- WhatsCLI: Go terminal WhatsApp client (whatsmeow + tview). Module `github.com/PalWorks/whatscli`, Go directive 1.26.0, toolchain go1.27.3, CGO required.
- Before calling work done: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`.
- Never read `sm.client` without `sm.getClient()`; never hold `sm.mu` during DB writes or network calls; never block `runManager`.
- Chats can be `@lid` or `@s.whatsapp.net`: use `canonicalChatJID` and `sameUser` (`messages/jid_norm.go`).
- Security-sensitive: `messages/revoke_incoming.go`, `messages/media.go` (sanitise, open allowlist), `messages/pathcheck.go`. Read SECURITY.md before changing.
- Do not touch `~/.config/whatscli`, do not run against a real account, do not commit or push unless asked.
- Releases are built by `.github/workflows/release.yml` on a `v*` tag. There is no `release.sh` any more.
- Docs: timestamp every update (`date --iso-8601=seconds`), label every table, no emojis or em-dashes, keep README, CHANGELOG, ROADMAP and DECISIONS current in the same change.

## Pointers

README.md, ARCHITECTURE.md, DECISIONS.md, ROADMAP.md, TESTING.md, SECURITY.md, CONTRIBUTING.md, SKILLS.md, CHANGELOG.md. Old reviews are in `docs/archive/`.
