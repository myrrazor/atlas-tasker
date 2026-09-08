# Atlas Tasker Invariants

These are release-facing rules. If one is broken, Atlas is inconsistent.

## Write-path invariants
- At most one workspace write lock holder may mutate canonical state at a time.
- Event IDs are monotonic per project.
- Successful mutations write UTC timestamps only.
- Reads remain side-effect free on canonical state. The derived SQLite index may be rebuilt on open when it is missing or behind the sources; the markdown and the event log are never touched by a read.

## State convergence invariants
- Canonical state lives in markdown snapshots plus the append-only event log.
- SQLite is a rebuildable projection, never the source of truth.
- After a successful mutation, markdown and the event log must agree on the resulting canonical state.
- Projection drift is repairable without losing canonical history.
- The index carries a fingerprint of the sources it was built from (event-log line count, ticket-file count). Every write and every full rebuild stamps it; a mismatch on open triggers a rebuild, and read-only `doctor` reports it as `repair_needed` instead of `ok`.

## Lease and workflow invariants
- A ticket may have at most one active lease.
- Review/completion policy is resolved deterministically from workspace/project/epic/ticket inputs.
- Board, queue, inspect, shell, and TUI surfaces must agree on the same workspace state.
