# Upgrade v1.3 to v1.4

Upgrade goals:
- preserve existing ticket/project/event state
- lazily add v1.4 fields on write
- rebuild projections safely
- keep v1.3.1 replay, locking, and JSON guarantees intact

Checks before upgrade:
- `tracker doctor --json`
- `tracker reindex`
- verify no pending mutation journals

Post-upgrade proving:
- agent registry create/list
- run dispatch into a temp worktree
- evidence and handoff generation
- gate open/approve flow
- cleanup and reindex parity
