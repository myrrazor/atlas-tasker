# JSON Contracts

## Shared envelope rules for new v1.5 surfaces
Success payloads use:
- `format_version`
- `kind`
- `generated_at`
- `payload` or `items`
- optional `warnings`
- optional `pagination`
- optional `reason_codes`
- optional `sync_status`

Error payloads use:
- `format_version`
- `ok: false`
- `generated_at`
- `error`
  - `code`
  - `message`
  - `details` when useful
- optional `warnings`

Warning items use:
- `code`
- `message`
- `scope` when useful

Pagination uses:
- `limit`
- `next_cursor`
- `prev_cursor` when supported
- `total_estimate` when available

## Frozen v1.5 kinds
- `change_list`
- `change_detail`
- `change_status`
- `change_create_result`
- `check_list`
- `check_detail`
- `check_sync_result`
- `permission_profile_list`
- `permission_profile_detail`
- `permissions_effective_detail`
- `permission_violation_report`
- `import_preview`
- `import_apply_result`
- `import_job_list`
- `import_job_detail`
- `export_bundle_create_result`
- `export_bundle_list`
- `export_bundle_detail`
- `export_verify_result`
- `archive_plan`
- `archive_apply_result`
- `archive_list`
- `archive_restore_result`
- `compact_result`
- `dashboard_summary`
- `timeline_detail`

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
