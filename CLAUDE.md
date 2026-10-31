# CLAUDE.md

> Claude Code reads this file automatically. The canonical agent contract is **[AGENTS.md](AGENTS.md)**; read it first. Do not duplicate rules here.
>
> _Last updated: 2026-10-31T15:51:30+00:00 (v1.6.0 audit: rc tags are pre-releases, UTC doc timestamps)_

## Essentials (full rules in AGENTS.md)

- WhatsCLI: Go terminal WhatsApp client (whatsmeow + tview). Module `github.com/PalWorks/whatscli`, Go directive 1.26.0, toolchain go1.27.3, CGO required.
- Before calling work done: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`.
- Never read `sm.client` without `sm.getClient()`; never hold `sm.mu` during DB writes or network calls; never block `runManager`.
- Chats can be `@lid` or `@s.whatsapp.net`: use `canonicalChatJID` and `sameUser` (`messages/jid_norm.go`).
- Security-sensitive: `messages/revoke_incoming.go`, `messages/media.go` (sanitise, open allowlist), `messages/pathcheck.go`. Read SECURITY.md before changing.
- Do not touch `~/.config/whatscli`, do not run against a real account, do not commit or push unless asked.
- Releases are built by `.github/workflows/release.yml` on a `v*` tag; a tag with `-` (for example `v1.6.0-rc1`) is published as a pre-release. There is no `release.sh` any more.
- Docs: timestamp every update in UTC (`date -u --iso-8601=seconds`), label every table, no emojis or em-dashes, keep README, CHANGELOG, ROADMAP and DECISIONS current in the same change.

## Pointers

README.md, ARCHITECTURE.md, DECISIONS.md (local only, gitignored), ROADMAP.md, TESTING.md, SECURITY.md, CONTRIBUTING.md, SKILLS.md, CHANGELOG.md. Old reviews are in `docs/archive/`.
