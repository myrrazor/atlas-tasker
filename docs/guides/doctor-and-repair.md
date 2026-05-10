# Doctor And Repair

Use `doctor` in read-only mode first:

```bash
tracker doctor --json
tracker doctor --md
```

Read-only doctor checks consistency between Atlas markdown, events, and the SQLite projection. It should not mutate workspace state.

## When To Repair

Use repair when the projection or journaled mutation state needs rebuilding:

```bash
tracker doctor --repair --json
```

`--repair` can rebuild projection state and replay pending journal entries. That is intentional, but it is still a mutation of derived local state. If you are investigating a production-like workspace, capture read-only output first.

## If Repair Still Reports Pending Work

Run the read-only check again:

```bash
tracker doctor --json
```

If pending work remains:

```bash
tracker inspect <TICKET-ID> --actor human:owner --json
tracker ticket history <TICKET-ID> --json
tracker reindex
```

Stop and inspect manually if:

- event JSONL files are malformed
- `.tracker/mutations/` contains repeated failures
- a repair would touch sensitive or redacted state you have not backed up
- a worktree is dirty and cleanup would discard useful uncommitted work

Atlas-managed worktrees and runtime directories are execution aids. They are not the source of truth.
