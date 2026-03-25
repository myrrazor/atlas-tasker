# JSON Contracts

## `tracker doctor --json`
Success payload includes:
- `format_version` (`v1`)
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
- `format_version` (`v1`)
- `rule`
- `matched`
- `reasons`
- `actions`
- `dry_run`
- `ticket` when `--ticket` is provided
- `event_type`

## `tracker automation explain --json`
Success payload matches `dry-run`, but `dry_run` is `false` and still includes `format_version`.

## `tracker notify log --json`
Success payload is an object with `format_version` and `items`, where each item is a delivery record with:
- `attempt`
- `delivered`
- `error` when delivery failed
- `event`
- `event_summary`
- `recipients`
- `targets`
- `sink`
- `timestamp`

## `tracker notify dead-letter --json`
Success payload matches `notify log`, but only includes final failed deliveries.

## `tracker watch list --json`
Success payload is an object with `format_version` and `items`, where each item is a watcher entry with:
- `subscription`
  - `actor`
  - `target_kind`
  - `target`
  - `event_types`
- `active`
- `inactive_reason` when the watched target no longer resolves

## `tracker views run <NAME> --json`
Success payload includes:
- `format_version` (`v1`)
- `view`
- one of:
  - `board`
  - `queue`
  - `next`
  - `tickets`
- `actor` when the saved view resolves through actor-aware queue or next logic

## `tracker bulk * --json`
Success payload includes:
- `format_version` (`v1`)
- `batch_id`
- `preview`
  - `kind`
  - `actor`
  - `assignee` when the batch assigns
  - `status` when the batch moves
  - `ticket_ids`
  - `ticket_count`
  - `dry_run`
- `summary`
  - `succeeded`
  - `failed`
  - `skipped`
  - `total`
- `results`
  - `ticket_id`
  - `ok`
  - `dry_run`
  - `reason`
  - `ticket` when the operation succeeded
  - `code` and `error` when a single ticket failed validation or mutation

## Error payloads
Commands invoked with `--json` emit the standard error envelope on stderr.

## Stability rule
Existing v1/v1.2 JSON fields are preserved unless a documented v1.3 decision log entry explicitly changes them.
