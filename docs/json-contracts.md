# JSON Contracts

## `tracker doctor --json`
Success payload includes:
- `ok`
- `events_scanned`
- `projects`
- `tickets`
- `repair_ran`
- `config`
- `issue_codes`
- `issues`

## Error payloads
Commands invoked with `--json` emit the standard error envelope on stderr.

## Stability rule
Existing v1/v1.2 JSON fields are preserved unless a documented v1.3 decision log entry explicitly changes them.
