# Upgrade v1.5 to v1.6

v1.6 is a non-destructive upgrade. The main change is collaboration metadata: deterministic sync-safe IDs, workspace identity, sync jobs, conflicts, collaborator records, and canonical mentions.

## What changes on disk

v1.6 keeps the existing workspace layout and adds collaboration state under `.tracker/sync/` plus workspace identity metadata:

- `.tracker/workspace.json`
- `.tracker/sync/remotes/`
- `.tracker/sync/jobs/`
- `.tracker/sync/conflicts/`
- `.tracker/sync/bundles/`
- `.tracker/sync/mirror/`
- `.tracker/sync/staging/`

Existing markdown snapshots, event logs, projection rebuild behavior, archive records, and import/export metadata all stay in place.

## Deterministic migration rules

- legacy v1.5 entities do not need destructive rewrites
- v1.6 derives deterministic UIDs from stable pre-v1.6 identity anchors
- the first sync-capable write stamps those UIDs locally under the workspace write lock
- independently upgraded replicas derive the same UIDs for the same legacy entities
- local-only operational state still stays local and is never published or reconstructed by sync

In practice:

1. install the v1.6 binary
2. open the workspace with `tracker init` if needed
3. run `tracker sync status --json`
4. perform the first sync-capable write, usually `tracker bundle create --actor human:owner` or `tracker sync push --remote <ID> --actor human:owner`
5. verify `migration_complete` becomes `true`

## Recommended upgrade flow

1. Back up the workspace or clone it into a temporary rehearsal copy.
2. Run `tracker doctor --repair --json` before the upgrade and fix any pre-existing corruption first.
3. Install the v1.6 binary.
4. Run `tracker reindex`.
5. Run `tracker sync status --json` and confirm the workspace has a `workspace_id`.
6. Run a first sync-capable write to stamp deterministic UIDs locally.
7. If this workspace will collaborate, add a remote and run a controlled two-workspace rehearsal before using it for real work.
8. Run `tracker doctor --repair --json` again after the first sync.

## What to verify after upgrade

- deterministic UID stamping completed
- `workspace_id` is present
- the same legacy ticket does not duplicate across independently upgraded workspaces
- collaborators, memberships, mentions, and sync jobs render correctly
- archive/restore/compact and doctor/reindex still behave safely after synced history exists
- `bundle verify` reports integrity truthfully without implying signer authenticity

## Suggested rehearsal

Before calling the upgrade done, prove one full collaboration loop:

1. workspace A publishes through a git sync remote
2. workspace B pulls from A
3. workspace B makes a conflicting edit and publishes it
4. workspace A resolves the conflict explicitly
5. workspace C receives transitive state and publishes a new change
6. workspace A converges with workspace C
7. a clean workspace imports a bundle from the upgraded state
8. archive, compact, restore, reindex, and doctor still pass after the synced history exists
