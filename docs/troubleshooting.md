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

## Config not found

Atlas looks for workspace state under `.tracker/`. If commands say config or workspace state is missing, start with:

```bash
pwd
tracker doctor --json
tracker init
```

Run `tracker init` only in the repo or directory that should own the task workspace.

## Projection corruption

SQLite projection state is derived. Rebuild it before editing files by hand:

```bash
tracker doctor --json
tracker reindex
tracker doctor --repair --json
```

If repair still reports pending work, inspect `.tracker/mutations/` and `.tracker/events/*.jsonl` before deleting anything.

## Invalid event log

An invalid event log is source-of-truth damage, not just a projection issue.

```bash
tracker doctor --json
tracker ticket history APP-1 --json
tracker audit report --scope workspace --actor human:owner --reason "inspect invalid event log" --json
```

Preserve the workspace as-is for debugging. Do not run archive, compact, or manual cleanup until the bad event and last good backup are identified.

## Git integration does nothing

Atlas only enriches views when the workspace is inside a git repo.

Check:

```bash
tracker git status
```

If it reports no repo, Atlas core still works; git is optional.

## Worktree drift

Managed run worktrees can drift if branches are edited outside Atlas.

```bash
tracker worktree list --json
tracker worktree view <RUN-ID> --json
tracker run open <RUN-ID> --json
```

Record any recovery action as run evidence before cleanup. Use `tracker run cleanup <RUN-ID> --actor <ACTOR> --reason <TEXT>` only after the run is finished or intentionally abandoned.

## Sync conflict

Sync conflicts are expected when two workspaces edit the same entity.

```bash
tracker conflict list --json
tracker conflict view <CONFLICT-ID> --json
tracker conflict resolve <CONFLICT-ID> --resolution use_local --actor human:owner --reason "resolve sync conflict"
```

Use `use_remote` only when the remote version is the desired source of truth.

## Signature verification failure

Start with the shared verification envelope, then inspect trust state:

```bash
tracker verify bundle <BUNDLE-REF> --json
tracker trust inspect --json
tracker key list --json
```

`valid_untrusted`, `valid_unknown_key`, `valid_revoked_key`, and `valid_expired_key` are not the same as `trusted_valid`. Protected imports should remain blocked until the signer and policy state are understood.

## Bulk apply did not run

CLI bulk commands require `--yes`.

In the TUI:
- `b` previews
- `y` applies the last preview

If the current tab has no tickets, bulk preview will do nothing.
