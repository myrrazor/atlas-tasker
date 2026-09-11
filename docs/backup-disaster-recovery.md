# Backup And Disaster Recovery

Backups are Atlas-owned snapshots of canonical Atlas data. They are not machine images and do not recreate local side effects.

PR-707 implements the first concrete backup lane:

- `tracker backup create [--scope workspace|project:<KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup list`
- `tracker backup view <BACKUP-ID>`
- `tracker backup verify <BACKUP-ID|PATH>`
- `tracker backup restore-plan <BACKUP-ID|PATH>`
- `tracker backup restore-apply <BACKUP-ID|PATH> --yes [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup drill [--target <TARGET-ID>]`
- `tracker backup auto status`
- `tracker backup auto enable --target <TARGET-ID>`
- `tracker backup auto disable`
- `tracker backup run --now`
- `tracker backup tick`
- `tracker backup watch`
- `tracker backup target add|list|view|edit|remove`
- `tracker backup replica view`
- `tracker backup replica reset --yes`
- `tracker backup reconcile --yes`
- `tracker backup remote list|verify|restore-plan|restore-apply`
- `tracker backup schedule plan|install|status|remove|repair`
- `tracker backup prune plan|apply`
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

`tracker backup drill` without `--target` is read-only. It verifies every local backup snapshot it can find, reports warning codes such as `no_backups`, `backup_verify_error:<id>`, and `backup_not_verified:<id>`, and includes `side_effect_free=true` in JSON output. With `--target` it verifies the selected disposable remote and builds a restore plan without applying it.

Automatic checkpoints use the same restore-safe file set as `backup create`, plus a `.atlas-checkpoint.json` manifest committed in an Atlas-owned bare repository outside the workspace. They append no canonical events. When automatic backup is enabled for an explicit Git target, each checkpoint is published as a fast-forward update of `refs/atlas/backups/<workspace-id>/<replica-id>` (never `refs/heads/*`, never `--force`). Push success is not `verified` until ls-remote, fetch, commit, tree, and manifest hash match. Remote divergence stays `blocked_remote_diverged` until `backup reconcile` or `replica reset`; ticket writes remain available.

`file://` remotes are disposable local drills (DEC-088), not an off-device claim. Production schemes are `https` and SSH. Target URLs never store credentials. Public GitHub is refused unless `--attest-private` or (`--attest-public` and `--allow-public-github`). Origin is never inferred.

Remote restore fetches into a temporary isolated bare repository, materializes regular blobs only, and reuses `backup restore-plan` / `backup restore-apply`. Wrong workspace requires `--allow-workspace-mismatch`. A denying `backup_restore` governance policy exits 5 and writes nothing. `--yes` is not authorization.

The AT114-507 recovery drill restores tickets, collaborators, memberships, mentions, and archive markdown. Exported-only collector roots (`config.toml`, agents, views, automations, subscriptions, runbooks, imports) stay off the restore allowlist because `tracker init` / `tracker setup` recreate them. Allowlist widening is not invented from the drill.

`tracker setup --backup --backup-target <ID>` enables automatic backup for an already-added target. Scheduler install remains a separate explicit consent; `tracker init` never writes user services.

Release drills should also prove restore into a clean workspace, conflict planning for existing workspaces, interrupted restore repair, and reindex/doctor health after restore. PR-708 owns the full release proof matrix.

## Admin Diagnostics

PR-707 adds three read-only diagnostics:

- `tracker admin security-status` reports key/trust/governance/audit/backup/goal counts plus warnings.
- `tracker admin trust-store` reports local trust-store health without printing private key material.
- `tracker admin recovery-status` reports backup counts, the latest backup, restore-plan count, and recovery warnings.
