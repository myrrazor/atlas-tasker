# Checks Model

## Frozen fields
- `check_id`
- `source`
- `provider`
- `scope`
- `scope_id`
- `name`
- `status`
- `conclusion`
- `summary`
- `url`
- `started_at`
- `completed_at`
- `external_id`
- `updated_at`

## Enums
- source: `local`, `provider`, `manual`
- scope: `run`, `change`, `ticket`
- status: `queued`, `running`, `completed`
- conclusion: `unknown`, `success`, `failure`, `neutral`, `cancelled`, `timed_out`, `skipped`

## Update model
Checks update in place by stable `check_id`. History is preserved through events.

## Aggregate rules
- `checks_pending`: any relevant check is not `completed` or concludes `unknown`
- `checks_failing`: no relevant checks are pending and at least one relevant check concludes `failure`, `timed_out`, or `cancelled`
- `checks_passing`: at least one relevant check exists, none are pending, and all completed checks conclude `success`, `neutral`, or `skipped`
- `checks_unknown`: no relevant checks exist
