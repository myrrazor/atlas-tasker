# Troubleshooting

## `tracker doctor --repair` keeps finding pending work

Start read-only:

```bash
tracker doctor --json
```

Use repair only when you intend to rebuild derived projection state:

```bash
tracker doctor --repair --json
```

The first repair run may replay a journaled event and rebuild the projection. The next read-only doctor run should usually show `repair_pending: 0`.

If it does not:
- inspect `.tracker/mutations/`
- inspect `.tracker/events/*.jsonl`
- run `tracker inspect <ID> --json`
- read [doctor and repair](guides/doctor-and-repair.md)

## Queue and board disagree

Start with:

```bash
tracker inspect APP-1 --json
tracker ticket history APP-1 --json
tracker reindex
```

If the disagreement remains after `reindex`, the issue is probably in the canonical markdown or event stream, not SQLite.

## Notifications are missing

Check:
- `notifications.terminal`
- `notifications.file_enabled`
- `notifications.webhook_url`
- `tracker notify log --json`
- `tracker notify dead-letter --json`

Remember:
- notifier delivery is post-commit best-effort
- notifier failure does not roll back the mutation

## TUI looks empty

Usually one of these is true:
- no tickets exist yet
- actor context is unset
- the selected tab is read-only and waiting for data

Try:

```bash
tracker queue --actor agent:builder-1
TRACKER_ACTOR=agent:builder-1 tracker tui
```

## Git integration does nothing

Atlas only enriches views when the workspace is inside a git repo.

Check:

```bash
tracker git status
```

If it reports no repo, Atlas core still works; git is optional.

## Bulk apply did not run

CLI bulk commands require `--yes`.

In the TUI:
- `b` previews
- `y` applies the last preview

If the current tab has no tickets, bulk preview will do nothing.
