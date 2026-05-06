# Local-only vs Syncable State

Atlas sync only moves canonical collaboration state.
That rule is strict because worktrees, runtime artifacts, journals, mirror caches, and staging dirs are operational leftovers, not source of truth.

## Syncable canonical state
- projects and project policy
- tickets
- collaborators
- memberships
- mentions
- runs, gates, handoffs, evidence metadata
- changes and checks
- import/export/archive metadata
- public key records, revocation records, and signed-artifact envelopes
- canonical event history
- sync jobs, remotes, and conflicts

## Local-only operational state
- `.tracker/runtime/`
- `.tracker/mutations/`
- `.tracker/*.log`
- `.tracker/sync/mirror/`
- `.tracker/sync/staging/`
- `.tracker/sync/bundles/`
- `.tracker/archives/*` payload dirs
- `.tracker/exports/*` payload artifacts
- `.tracker/security/keys/private/`
- `.tracker/security/trust/`
- linked worktrees under `.atlas-tasker-worktrees/`
- notifier dead letters and delivery logs
- launch manifests and other regenerated runtime files

## Closeout rules in v1.6.1
- sync publication only includes paths on the canonical allowlist
- bundle verification rejects unsafe archive entries and non-syncable payload paths
- replay, reindex, repair, and archive restore do not recreate local-only state
- remote locations with embedded credentials are rejected at save time and redacted on read paths
