# Redaction, Audit, And Backup

Classification labels describe how sensitive Atlas-owned data is. Redaction previews bind a planned redacted output to source state, actor, target, policy version, classification version, and TTL. Redacted writes must validate the preview before writing.

Audit reports are snapshot artifacts. They should explain the source state, policy state, trust state, findings, redaction state, and optional signatures at generation time.

Backups are Atlas-owned snapshots. They intentionally exclude private keys, local trust decisions, redaction previews, runtime/worktree/provider state, remotes, notifiers, and MCP approvals.

Useful commands:

```bash
tracker classify get APP-1
tracker redact preview --scope project:APP --target export --actor human:owner --reason "prepare redacted export"
tracker audit report --scope project:APP --actor human:owner --reason "release audit"
tracker backup create --scope project:APP --actor human:owner --reason "pre-release backup"
tracker backup restore-plan <BACKUP-ID>
```

Run restore plans before restore apply. Restore planning is intended to avoid known local side effects; it is not an OS-level rollback system.
