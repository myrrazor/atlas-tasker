# Backup And Disaster Recovery

Backups are Atlas-owned snapshots of canonical Atlas data. They are not machine images and do not recreate local side effects.

## Backup Scope

Backups may include projects, events, runs, gates, evidence, collaboration metadata, public security records, governance, classification, and audit data. Private keys are excluded by default.

## Restore Rules

Restore is preview-first. `backup restore-plan` is side-effect free. `backup restore-apply` must use the write lock and mutation journal.

Restore items must be clean relative paths on the canonical Atlas restore allowlist: project markdown, event logs, run/gate/handoff/evidence/change/check markdown, collaboration metadata, permission/retention/archive metadata, public security records, governance policies/packs, classification policies/labels, redaction rules, and audit reports/packets.

Restore must never recreate provider state, worktrees, runtime dirs, launch files, notifiers, remotes, MCP approvals, private keys, redaction previews, backup snapshots, generated goal files, arbitrary repository files, or remote-side state. Restore from untrusted, revoked, malformed, or older-schema backups must produce explicit plan warnings or blocks.

## Drills

Recovery drills should prove signed backup verification, restore into a clean workspace, conflict planning for existing workspaces, interrupted restore repair, and reindex/doctor health after restore.
