# Import / Export Format

## Commands
- `tracker import preview <PATH>`
- `tracker import apply <JOB-ID>`
- `tracker import list`
- `tracker import view <JOB-ID>`
- `tracker export create [--scope workspace]`
- `tracker export list`
- `tracker export view <BUNDLE-ID>`
- `tracker export verify <PATH|BUNDLE-ID>`

## Atlas bundle contents
- export bundles are `tar.gz` archives with:
  - `manifest.json`
  - canonical workspace markdown snapshots from `projects/`
  - selected Atlas state under `.tracker/` including agents, runbooks, runs, gates, handoffs, evidence markdown, changes, checks, permission profiles, imports, retention config, public key records, revocations, and signature envelopes
- signed export bundles also write an adjacent `<bundle>.signatures.json` sidecar for path-based verification after the archive sidecars are copied
- derived runtime state is excluded from the archive:
  - `.tracker/runtime/`
  - `.tracker/archives/`
  - `.tracker/exports/`
  - `.tracker/mutations/`
  - `.tracker/security/keys/private/`
  - `.tracker/security/trust/`
  - `index.sqlite`
  - copied evidence artifacts that are not markdown
- Atlas bundle import is snapshot-first. It restores canonical markdown snapshots into the target workspace, but it deliberately does not copy source `.tracker/events/` files or source `.tracker/imports/` job logs into the live target workspace. That avoids event-log collisions with the target workspace's own audit trail and keeps preview/apply deterministic.

## Structured source formats
- Jira CSV import uses header-based mapping for:
  - `Project Key`
  - `Project Name`
  - `Issue Key`
  - `Summary`
  - `Issue Type`
  - `Status`
  - `Priority`
  - `Parent` / `Epic Link`
  - `Blocks` / `Blocked By`
- GitHub JSON import currently expects an array of row objects with:
  - `project_key`
  - `project_name`
  - `title`
  - `body`
  - `type`
  - `status`
  - `priority`
  - `url`
  - `number`
  - `kind`
- structured imports are create-only in v1.5. Existing Atlas ticket ids are explicit conflicts, not merge targets.

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
- import preview and apply are persistent audited jobs, not ephemeral shell output
- Atlas bundle import rejects absolute paths and `..` traversal anywhere in the archive
- `export verify` validates archive checksum, manifest checksum, unexpected bundle entries, and missing bundle entries

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
- preview conflicts block apply and produce an `import.failed` job state instead of silently skipping conflicted items
