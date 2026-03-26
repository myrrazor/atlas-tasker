# Import / Export Format

## ImportJob states
- `previewed`
- `validated`
- `applying`
- `applied`
- `failed`
- `canceled`

## ExportBundle states
- `creating`
- `created`
- `failed`
- `archived`

## Security and integrity rules
- imports validate before canonical writes
- apply runs under the workspace write lock and mutation journal
- preview is deterministic and side-effect free
- versioned manifest is required for Atlas bundles
- checksums are required for verifiable bundle flows
- path traversal attempts are rejected
- malformed CSV input is rejected
- oversized bundle and archive inputs are rejected by policy
- temp-dir staging is required before apply

## Idempotency
- Atlas bundle import: manifest hash
- Jira CSV import: source hash plus mapping config hash
- provider-ref import: external ref identity

Repeated successful identical imports are no-ops unless an explicit replace/update mode is chosen.

## Conflict handling
- existing local ticket ID conflict is reported explicitly
- external ID collisions are reported explicitly
- preview and apply must target the same deterministic item set
- mixed success is recorded as `failed` with `partial_applied=true`, counts, warnings, and conflict logs
