# Doctor And Repair

Use `doctor` in read-only mode first:

```bash
tracker doctor --json
tracker doctor --md
```

Read-only doctor checks consistency between Atlas markdown, events, and the SQLite projection. It should not mutate workspace state.

Two things it will refuse to call `ok`:

- an index file sqlite cannot read at all (truncated, overwritten, not sqlite) — `repair_needed`, exit 7
- an index that opens fine but was built from different sources than the ones on disk — also exit 7, with both fingerprints in the message: `projection index is stale (index built from events=12 tickets=4, sources now events=13 tickets=5)`

The second case is the one that used to slip through. `doctor` counted events and tickets from the files and printed `doctor ok`, without ever asking whether the index agreed.

## The index heals itself

You should rarely need to act on a stale index, because every command that opens the workspace checks the fingerprint first and rebuilds the index under the write lock when it is missing or behind. That includes `board`, `ticket view`, the TUI, `web serve`, and `mcp serve`. The rebuild reads only markdown and events, takes a fraction of a second on a normal workspace, and announces itself once on stderr:

```
[tracker] index.sqlite was missing or stale; rebuilt it from markdown and events (events=13 tickets=5)
```

The next command is quiet again. A workspace with nothing in it (right after `tracker init`) rebuilds silently. Upgrading from a release that did not stamp the index costs exactly one rebuild the first time you run anything.

What does not heal itself is a byte-corrupt file. Every command except `reindex` and `doctor --repair` stops with exit 7 and points at `doctor --repair`, so a damaged index is never silently thrown away underneath a long-running server.

## When To Repair

Use repair when the projection or journaled mutation state needs rebuilding:

```bash
tracker doctor --repair --json
```

`--repair` can rebuild projection state and replay pending journal entries. That is intentional, but it is still a mutation of derived local state. If you are investigating a production-like workspace, capture read-only output first.

`doctor --json` reports an `index` object with or without `--repair` — `stale_before_repair`, `rebuilt`, `stored_fingerprint`, `current_fingerprint` — so a script can tell "repair ran and found nothing" from "repair replaced a stale index".

`tracker reindex` is the narrower tool: it only rebuilds the index, and like `doctor --repair` it removes and recreates a corrupt `index.sqlite` instead of refusing to open it.

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
