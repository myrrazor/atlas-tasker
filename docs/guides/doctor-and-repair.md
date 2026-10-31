# Doctor And Repair

Use `doctor` in read-only mode first:

```bash
tracker doctor --json
tracker doctor --md
```

Doctor always checks the **machine**: registry, agent
registrations, backup/replica health, and service identity. Workspace checks run
when the current directory is an Atlas root. Outside a workspace it reports
`current_workspace: none` and does not create `.tracker`.

`--repair` is the single repair switch for machine + current workspace when
present: moved-path updates, projection rebuild, and journal replay. Forking a
copied workspace onto a new ID is an explicit repair action, not an implicit
side effect.

Read-only workspace doctor still checks consistency between Atlas markdown, events, and the SQLite projection. It should not mutate workspace state.

Three things it will refuse to call `ok`:

- an index file sqlite cannot read at all (truncated, overwritten, not sqlite) — `repair_needed`, exit 7
- an index that opens fine but was built from different sources than the ones on disk — also exit 7, with both fingerprints in the message: `projection index is stale (index built from events=12 tickets=4, sources now events=13 tickets=5)`
- a ticket file that is cut short or no longer parses — `repair_needed`, exit 7, and the message names the file. `tracker doctor --repair` copies it to `.tracker/quarantine/` first, then restores it from the event log, and leaves it in place if the log has no copy. `tracker reindex` rebuilds an unreadable index but does not repair a damaged ticket. `tracker board` and `tracker search` still exit 0 while a ticket file is damaged; they read the projection and do not scan ticket markdown.

The second case is the one that used to slip through. `doctor` counted events and tickets from the files and printed `doctor ok`, without ever asking whether the index agreed.

## The index heals itself

You should rarely need to act on a stale index, because every command that opens the workspace checks the fingerprint first and rebuilds the index under the write lock when it is missing or behind. That includes `board`, `ticket view`, the TUI, Home / `web serve`, and `mcp serve`. The rebuild reads only markdown and events, takes a fraction of a second on a normal workspace, and announces itself once on stderr:

```
[tracker] index.sqlite was missing or stale; rebuilt it from markdown and events (events=13 tickets=5)
```

The next command is quiet again. A workspace with nothing in it (right after `tracker init`) rebuilds silently. Upgrading from a release that did not stamp the index costs exactly one rebuild the first time you run anything.

What does not heal itself is a byte-corrupt file. Every command except `reindex` and `doctor --repair` stops with exit 7 and points at `doctor --repair`, so a damaged index is never silently thrown away underneath a long-running server.

## A damaged ticket file

A ticket file that no longer parses (for example one cut short mid-frontmatter, which fails with `missing frontmatter end`) no longer stops everything:

- `tracker doctor` exits 7 (`repair_needed`) and names the file: `1 ticket file is damaged: projects/APP/tickets/APP-1.md (missing frontmatter end); run 'tracker doctor --repair' ...`.
- `tracker reindex` still indexes every other ticket, keeps the damaged ticket's last state from the event log on the board, then exits 7 with the same message.
- `tracker doctor --repair` first copies the damaged file byte-for-byte to `.tracker/quarantine/<UTC time>/projects/<KEY>/tickets/<ID>.md`, then rewrites the ticket from its newest snapshot in the event log. `repair_actions` lists `restored APP-1 from event log (backup: .tracker/quarantine/...)`.
- If the event log has no snapshot of that ticket (a file written by hand, never through Atlas), repair still makes the backup but leaves the file exactly as it is and exits 7, naming the file and its backup. Fix or remove it by hand, then rerun `tracker doctor --repair`.

Repair never drops a damaged file. Edits made by hand after the last event are not in the log; compare the restored file with the backup if you made any.

Copies under `.tracker/quarantine/`, including the journal backups below, are git-ignored. `tracker init` writes the rule for a new shared or private workspace. On an existing workspace, run `tracker doctor --repair` once: it refreshes the ignore rules before writing a backup, and a second repair does not add the rule again. Shared mode updates the managed block in the root `.gitignore` (`/.tracker/quarantine/`). Private mode updates `.git/info/exclude`. Unmanaged mode has no root block, so repair ignores `/quarantine/` from `.tracker/.gitignore`. Ticket files stay tracked.

## When To Repair

Use repair when the projection or journaled mutation state needs rebuilding:

```bash
tracker doctor --repair --json
```

`--repair` can rebuild projection state and replay pending journal entries. That is intentional, but it is still a mutation of derived local state. If you are investigating a production-like workspace, capture read-only output first.

`doctor --json` reports an `index` object with or without `--repair` — `stale_before_repair`, `rebuilt`, `stored_fingerprint`, `current_fingerprint` — so a script can tell "repair ran and found nothing" from "repair replaced a stale index".

`tracker reindex` is the narrower tool: it only rebuilds the index, and like `doctor --repair` it removes and recreates a corrupt `index.sqlite` instead of refusing to open it.

## A pending restore whose event id is taken

Before the fix for rough edge 19, a restore whose backup carried a newer event log than the destination's could stop with exit 7 `event append failed after canonical write`, leaving `.tracker/mutations/_workspace-000000000001.json` with `event_id 1 must be greater than 2`. The restored files were already written; only the restore's event was missing.

`tracker doctor --repair` now fixes this. When a pending journal's event never landed but its id is already taken in the log, repair copies the journal entry to `.tracker/quarantine/<UTC time>/mutations/`, appends the same event at the next free id and logical clock, and records `renumbered _workspace #1 to #3 and replayed it (journal backup: …)` in `repair_actions`. A journal is treated as already applied only when the event at its id is the same event (same `event_uid`), so a different event at that id is never mistaken for it.

## The same event id used twice

Each workspace numbers its events per project, so two synced workspaces can each have an `APP #2`; that is normal. One workspace issuing `APP #2` for two different events is not: a restore in a build before this fix could reuse the ids of the events it discarded, and a sync peer that had already pulled the old events then holds both. Restores now record the highest ids issued in `.tracker/event-watermark.json` before replacing the event log and number new events above it.

`tracker doctor` reports such a pair with issue code `event_id_reused`, a `warning: event id APP #2 from workspace … is used by 2 different events` line, and `issues.event_id_reuse` in `--json` output. Both events are kept and remain distinct by `event_uid`; `doctor --repair` does not renumber them, because peers already hold those ids. If a ticket title or field looks stuck after such a pull, resolve the open conflict with `tracker conflict resolve <ID> --resolution use_remote` (or `use_local`).

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
