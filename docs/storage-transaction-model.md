# Storage Transaction Model

Atlas uses a staged local mutation model.

## Outcome classes
- `committed`: markdown, event, and projection all updated.
- `committed_repair_needed`: canonical data committed, but projection or another post-commit step needs repair.
- `rejected_or_not_committed`: no canonical mutation committed; safe to retry.

## Current write order
1. acquire workspace write lock
2. write markdown snapshot change
3. append event
4. apply event to projection
5. run post-commit side effects (notifications)

Projection and notifier failures are not allowed to silently corrupt canonical state.

## Projection commit and recovery

The SQLite projection is derived from markdown and events. Event application and full/project rebuilds commit atomically in the existing database, so already-open MCP, web, and TUI readers see the new state after commit. Failed rebuilds preserve the prior projection. Schema setup is serialized by an immediate transaction, including concurrent first opens.

Every pooled connection uses a busy timeout matching the workspace writer lock's existing five-second wait. WAL conversion can bypass SQLite's busy handler; that setup step retries within the same wait with the existing 50ms polling interval.

The on-open fingerprint counts event-log lines and ticket files. It reads the full event files and does not detect same-count manual content edits. A later incremental apply cannot advance a watermark over skipped appends; a full replay can restore it. Read-only doctor rejects pending mutation journals, and doctor repair/reindex acquire the workspace write lock before replacing a corrupt derived index. Stop live consumers before explicitly replacing a corrupt or manually deleted database.

See [DEC-052](v1-decision-log.md#dec-052) for the change from file swaps.
