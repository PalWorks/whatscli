# Security

Threat notes, controls and reporting for WhatsCLI. Rationale for each control is in the maintainers' private decision log (D9, D14, D15, D17); open hardening work is in [ROADMAP.md](ROADMAP.md) §3.2.

_Last updated: 2026-10-31T01:12:05+05:30 (initial; controls verified against the code at commit f5a9b66)_

## 1. What is trusted and what is not

WhatsCLI is a linked device of one WhatsApp account. Everything that arrives from other people is untrusted: message text, file names, mime types, media, URLs and protocol messages such as revokes. End-to-end encryption protects content in transit; it does not validate what a sender claims inside a message.

**Table SE1: Threats and controls**

| Threat | Control | Where | Tests |
|---|---|---|---|
| Spoofed incoming revoke blanks someone else's stored message (any contact could, before the fix) | A revoke applies only if the stored message is in the same chat and the revoker is one of our own devices, the original sender (devices ignored, LID and phone number equal when mapped), or a current group admin confirmed with `GetGroupInfo` (15 s timeout, errors mean not admin). Otherwise ignored silently. The SQL update is also scoped `WHERE id = ? AND chat_id = ?` | `messages/revoke_incoming.go`, `messages/storage.go` `MarkMessageRevoked`, `messages/event_handler.go` | `revoke_incoming_test.go` (cross-chat spoof, 12 rule cases, LID, admin), `TestMarkMessageRevoked_ScopedToChat` |
| Sender-chosen file runs when opened (Windows opener runs `.exe`, `.bat`, `.lnk`) | `open` hands only allowlisted extensions to the system opener: images, video, audio, `.pdf`, `.txt`. Other types are saved and the path printed with a note. Judged on the final on-disk extension, case-insensitive, so `a.pdf.exe` is refused | `messages/media.go` `safeToOpen`, `messages/cmd_message.go` `deliverDownload` | `media_safety_test.go` (13 allowed, 32 refused, open gate) |
| Path traversal and unsafe names from the sender (`../..`, `..\..`, NTFS streams, `CON`, `NUL`) | `sanitizeFileName`: NUL removed, `\` treated as `/`, `path.Base`, `<>:"|?*` and control characters become `_`, trailing dots and spaces trimmed, reserved device names prefixed `_`, empty rejected (falls back to message ID plus mime extension). `safeJoin` confirms containment with `filepath.Rel`. Existing non-regular files are never overwritten. Files are written to a mode 0600 temp file and renamed | `messages/media.go`, `messages/cmd_message.go` `downloadMedia` | `media_test.go`, `media_safety_test.go` |
| `show` renders a file outside the download or preview directory | `ResolveFileWithin`: file and directories are absolute and symlink-resolved; containment by `filepath.Rel`; sibling directories (`Downloads2`), `..`, the directory itself and blank config values are rejected | `messages/pathcheck.go`, `ui_render.go` `PrintImage` | `TestResolveFileWithin` (13 cases) |
| Malicious link opened by `/url` | Only the first `http` or `https` URL with a host; trailing punctuation trimmed. The opener is exec'd without a shell (Linux, macOS) or through `rundll32` (Windows) | `messages/media.go` `firstURL`, `messages/cmd_message.go` `cmdURL` | `TestFirstURL`, `TestCmdURL` |
| SQL injection | All user data uses placeholders; `addColumnIfMissing` builds SQL only from compile-time constants; search escapes `LIKE` wildcards | `messages/storage.go` | `storage_test.go` |
| Clipboard backend panics | `recover` around read and write; error shown, no crash | `ui_keybindings.go` | manual (TESTING.md step 11) |
| Release pipeline abuse | Top-level `permissions: contents: read`; only the release job has `contents: write`; `GITHUB_TOKEN` only, no other secrets; tag input passed through `env` and validated by regex, never interpolated into scripts; `persist-credentials: false` | `.github/workflows/release.yml` | not run on a real tag yet (ROADMAP row 17) |

## 2. Known limits

- The open allowlist judges the extension only; a viewer exploit in a PDF or image is still possible, as in any client (ROADMAP row 20).
- Each revoke by a non-sender in a group costs one group info request; a non-admin can trigger them (row 18).
- A revoke that arrives before its target is ignored (row 19). A revoke overwrites the stored text and media data and cannot be undone (row 22).
- `show` runs the external viewer named in `show_command` with the file path. The command comes from the user's own config, not from senders.
- Actions in the workflows are pinned to major tags, not commit SHAs (row 12).
- `contact_id` is stored in the form received; do not treat it as a stable identity across phone-number and LID forms (row 21).

## 3. Secrets and local data

- The repository holds no API keys, tokens or secrets. WhatsCLI has no backend of its own and calls no third-party API. Before every release, scan the repository and built binaries for key patterns (`AIza`, `sk-`, `ghp_`, `xox`, `eyJ`, `BEGIN PRIVATE KEY`, webhook URLs). Binaries are built with `-trimpath -ldflags "-s -w"`; no source maps or databases are shipped. Known false positive (checked 2026-10-31T07:03:48+00:00): one `AIza...` string in every binary comes from whatsmeow's WhatsApp protocol token dictionary (`binary/token/token.go`), not from this project; it is public and cannot be removed without breaking the protocol.
- `~/.config/whatscli/session.db` holds the linked-device keys. Anyone who can read it can act as this linked device until it is unlinked from the phone (Settings, Linked Devices). The app does not set file permissions itself (verified: no `chmod` or restrictive mode in the code); [Unverified] the directory mode depends on the `xdg` library and your umask. Keep the directory private and out of backups and sync folders you do not control.
- `session_meta.db` holds your message history in plain text. Same advice.
- `whatscli --debug` writes verbose protocol logs to the terminal. [Inference] They can contain metadata about your chats; do not paste them into public issues without review.
- Never commit `~/.config/whatscli`, `*.db`, `session.gob`, screenshots of chats, or `.env` files. `.gitignore` covers the binary, `dist/`, `release-assets/` and `aur-out/` only; add entries if you create other local files.
- The dev machine's session was never used by automated work; live checks use a non-production number (TESTING.md).

## 4. Authentication

WhatsApp login is a QR scan from the phone (linked device). There are no passwords or tokens stored by this project beyond the whatsmeow session in `session.db`. `/logout` deletes the session data; unlink the device from the phone as well if the machine is lost.

## 5. Reporting a vulnerability

Do not post exploit details in a public issue.

1. Preferred: GitHub private vulnerability reporting on `PalWorks/whatscli` (Security tab, Report a vulnerability). Enabled 2026-10-31T01:20:43+05:30.
2. Otherwise: email support@palworks.ai.

Include the version (`/help` shows it), the operating system, what you did, what happened, and what you expected. There is no bounty and no fixed response time; the project is maintained by one owner.

Vulnerabilities in whatsmeow, tview or other dependencies should be reported to those projects; tell this repository's owner so the dependency can be bumped.

## 6. Checklist for changes touching these areas

- Any new use of data from a sender (names, mime types, text, protocol messages) needs sanitising or authorisation and a negative test.
- Anything that starts another program needs an allowlist or a validated argument, no shell, and an error path that is shown to the user.
- Any new file write: confine to a configured directory, use temp then rename, never overwrite non-regular files.
- Any new CI job: least permissions, no new secrets without owner approval, no untrusted input in `run:` scripts.
- Run `go vet ./...` and `go test -race -count=1 ./...`, then the secret scan above.
