# Backup And Disaster Recovery

Backups are Atlas-owned snapshots of canonical Atlas data. They are not machine images and do not recreate local side effects.

PR-707 implements the first concrete backup lane:

- `tracker backup create [--scope workspace|project:<KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup list`
- `tracker backup view <BACKUP-ID>`
- `tracker backup verify <BACKUP-ID|PATH>`
- `tracker backup restore-plan <BACKUP-ID|PATH>`
- `tracker backup restore-apply <BACKUP-ID|PATH> --yes [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup drill`
- `tracker sign backup <BACKUP-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker verify backup <BACKUP-ID|PATH>`

## Backup Scope

Backups may include projects, events, runs, gates, evidence, collaboration metadata, permission profiles, archive metadata, public security records, governance, classification, redaction rules, and audit data. Private keys, local trust decisions, redaction previews, backup snapshots, generated goal files, runtime files, worktree data, remotes, notifiers, provider caches, and MCP approvals are excluded.

Stored backup state is split intentionally:

- `.tracker/backups/manifests/<backup-id>.json` stores the local backup snapshot record and any signatures.
- `.tracker/backups/manifests/<backup-id>.manifest.json` stores the archive manifest.
- `.tracker/backups/snapshots/<backup-id>.tar.gz` stores the canonical Atlas-owned files plus the manifest.

`tracker sign backup` signs the local backup snapshot record after integrity verification. `tracker backup verify` can verify archive integrity by backup id or path. When a copied archive is verified by path without its local snapshot record, Atlas still proves archive integrity but reports `missing_signature` because the v1.7 signature envelope lives on the stored snapshot record.

## Restore Rules

Restore is preview-first. `backup restore-plan` is side-effect free and does not persist a plan or append an event. `backup restore-apply` recomputes the plan under the write lock, requires `--yes`, requires a valid actor and non-empty reason, writes only allowlisted Atlas-owned files, and records `backup.restored` after the mutation lands.

Restore items must be clean relative paths on the canonical Atlas restore allowlist: project markdown, event logs, run/gate/handoff/evidence/change/check markdown, collaboration metadata, permission/retention/archive metadata, public security records, governance policies/packs, classification policies/labels, redaction rules, and audit reports/packets.

Restore must never recreate provider state, worktrees, runtime dirs, launch files, notifiers, remotes, MCP approvals, private keys, redaction previews, backup snapshots, generated goal files, arbitrary repository files, or remote-side state. Restore from untrusted, revoked, malformed, or older-schema backups must produce explicit plan warnings or blocks.

## Drills

`tracker backup drill` is read-only. It verifies every local backup snapshot it can find, reports warning codes such as `no_backups`, `backup_verify_error:<id>`, and `backup_not_verified:<id>`, and includes `side_effect_free=true` in JSON output.

Release drills should also prove restore into a clean workspace, conflict planning for existing workspaces, interrupted restore repair, and reindex/doctor health after restore. PR-708 owns the full release proof matrix.

## Admin Diagnostics

PR-707 adds three read-only diagnostics:

- `tracker admin security-status` reports key/trust/governance/audit/backup/goal counts plus warnings.
- `tracker admin trust-store` reports local trust-store health without printing private key material.
- `tracker admin recovery-status` reports backup counts, the latest backup, restore-plan count, and recovery warnings.
