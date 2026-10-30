## Summary

What changes and why.

## Checklist

- [ ] `make vet` and `make test` (race detector) pass
- [ ] New commands registered in `messages/cmd_registry.go` and listed in `PrintHelp()`
- [ ] Concurrency rules in AGENTS.md followed (no DB writes under `sm.mu`, `getClient()`, `snapshotPQ()`)
- [ ] CHANGELOG.md `[Unreleased]` updated; DECISIONS.md / ROADMAP.md updated if a decision or follow-up arose
- [ ] Live-account checklist in TESTING.md run if WhatsApp behaviour changed
