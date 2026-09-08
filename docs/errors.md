# Atlas Tasker Error and Exit Code Policy

## Error codes
- `invalid_input`
- `not_found`
- `conflict`
- `permission_denied`
- `busy`
- `repair_needed`
- `internal`

## Exit codes
- `0` success
- `1` internal error
- `2` invalid input
- `3` not found
- `4` conflict
- `5` permission denied
- `6` busy / lock timeout — a read can hit this too, when the index is stale and another process holds the write lock for the rebuild; retry, or run `tracker reindex`
- `7` repair needed / degraded success requiring follow-up — the index is unreadable, or read-only `doctor` found it stale against the sources

## JSON error envelope
When a command is invoked with `--json` and it fails, stderr emits:

```json
{
  "format_version": "v1",
  "ok": false,
  "error": {
    "code": "invalid_input",
    "message": "...",
    "exit": 2
  }
}
```
