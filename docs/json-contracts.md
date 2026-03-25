# JSON Contracts

## `tracker doctor --json`
Success payload includes:
- `ok`
- `events_scanned`
- `projects`
- `tickets`
- `repair_ran`
- `repair_actions`
- `repair_pending`
- `config`
- `issue_codes`
- `issues`

## `tracker automation dry-run --json`
Success payload includes:
- `rule`
- `matched`
- `reasons`
- `actions`
- `dry_run`
- `ticket` when `--ticket` is provided
- `event_type`

## `tracker automation explain --json`
Success payload matches `dry-run`, but `dry_run` is `false`.

## Error payloads
Commands invoked with `--json` emit the standard error envelope on stderr.

## Stability rule
Existing v1/v1.2 JSON fields are preserved unless a documented v1.3 decision log entry explicitly changes them.
