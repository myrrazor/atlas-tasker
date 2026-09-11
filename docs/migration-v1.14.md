# Migrating to Atlas Tasker v1.14

v1.14 adds unified `tracker setup` and automatic verified backup. Existing v1.13 workspaces keep their tickets, events, integrations, and backups.

## What setup will not do

- It will not silently rewrite an unmanaged MCP server named `atlas`.
- It will not replace custom text outside the Atlas instruction markers in `AGENTS.md` or `CLAUDE.md`.
- It will not convert a project `origin` remote into a backup target.
- It will not install a user-level backup scheduler. That remains `tracker backup schedule install --yes`.
- `tracker update` still replaces only the tracker binary. Provider config changes wait for an explicit `tracker setup` or `tracker setup repair --yes`.

## Upgrade path

1. Install the v1.14 binary (`tracker update` or a new download).
2. In each workspace, run `tracker setup --plan` and read the repair reasons.
3. Apply with `tracker setup --yes --agents <targets>` when the plan is correct.
4. If a workspace moved or the binary path changed, run `tracker setup repair` (plan) then `tracker setup repair --yes`.
5. Existing manual backups remain restorable through `tracker backup restore-plan` / `restore-apply`.
6. To turn on automatic backup, add an explicit target first, then `tracker setup --backup --backup-target <ID>` or `tracker backup auto enable --target <ID>`.

## Copied or moved workspaces

A copied workspace does not inherit the machine-local replica identity. Use `tracker backup replica view` and, if needed, `tracker backup reconcile --yes` before publishing again. A moved workspace shows `repair_required` on the scheduler and in `tracker setup status`.

## Remaining host work

Real Codex, Claude Code, Cursor, OpenClaw, and Grok sessions, plus a private off-device Git remote, remain operator-run. Synthetic `file://` remotes are disposable and are not an off-device claim.
