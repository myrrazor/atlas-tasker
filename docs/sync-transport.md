# Sync Transport

v1.6 adds Atlas-owned sync transport without changing the canonical storage architecture.

Rules:
- canonical state is published and exchanged through `.tracker/sync/`
- git transport uses the ref namespace `refs/atlas-tasker/sync/<workspace-id>`
- path remotes use the same logical publication layout as git transport
- bundle exchange is a first-class fallback, not a separate product tier
- sync transport never publishes local-only operational state like worktrees or runtime artifacts
- sync is blocked until deterministic UID stamping is complete locally

Lock model:
- `sync fetch` and `bundle verify` are transport-only
- `sync pull`, `bundle import`, and `conflict resolve` take the workspace sync-apply lock and canonical write lock
- `sync push` and `bundle create` take a short-lived snapshot lock to freeze a consistent manifest
