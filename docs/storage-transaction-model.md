# Storage Transaction Model

Atlas v1.3 uses a staged local mutation model.

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
