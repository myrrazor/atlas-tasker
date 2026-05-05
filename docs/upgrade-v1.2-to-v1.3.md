# Upgrade From v1.2 to v1.3

Atlas Tasker v1.3 keeps the same workspace layout:
- markdown snapshots under `projects/`
- append-only JSONL events under `.tracker/events/`
- SQLite projection under `.tracker/index.sqlite`

## Upgrade steps

1. install the v1.3 binary
2. run:

```bash
tracker doctor --repair
tracker reindex
```

3. verify the workspace:

```bash
tracker board --pretty
tracker queue --actor agent:builder-1
tracker inspect APP-1 --json
```

## New v1.3 artifacts

These directories may appear after you start using the new features:
- `.tracker/automations/`
- `.tracker/views/`
- `.tracker/subscriptions/`
- `.tracker/mutations/`

## What to check after upgrade

- `doctor --repair` reports `repair_pending: 0` on the second run
- saved views and watcher files are readable if they exist
- queues, board, and TUI agree on the same ticket state
- notification log and dead-letter files are created only when the notifier is enabled

## Safe rollback assumption

The canonical data is still:
- markdown snapshots
- append-only JSONL events

If you need to rebuild the read model after an upgrade:

```bash
rm -f .tracker/index.sqlite
tracker reindex
```
