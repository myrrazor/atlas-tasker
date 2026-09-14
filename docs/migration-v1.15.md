# Migrating to the v1.15 source candidate

v1.15 is an implementation candidate in this source tree. It is **not** a published
GitHub release. `scripts/install.sh` and `go install ...@latest` still install the
latest published tag. Unstamped builds report `"version": "dev"` from `buildinfo`.

Keep using a published release until you intentionally build this tree.

## What changes for ordinary use

| Before (published / v1.14 candidate) | v1.15 candidate |
|---|---|
| `tracker init` then maybe `tracker setup` | `tracker init` then `tracker` |
| Per-workspace `mcp serve --workspace PATH` (default `--tool-profile read`) | Machine-wide `mcp serve --global --tool-profile workflow`; pinned `--workspace` is still supported |
| `tracker web serve --open` | `tracker` opens Home; `web serve` remains |
| Default pretty board is an ASCII table | Default is a polished table; `--style kanban` is optional |
| Local backup after `backup auto enable` | Init starts local checkpoints; remotes stay explicit |
| Remove the binary to uninstall | `tracker uninstall` / `/uninstall` (software only) |

Existing workspaces keep their IDs, tickets, events, and backup lineage. The registry
loads v1 files and writes v2 pointer metadata.

## Still supported forms

These commands stay. This page does not set a removal date:

```bash
tracker setup --plan
tracker setup --yes --agents generic
tracker mcp serve --workspace /path/to/workspace --tool-profile read
tracker web serve --open
tracker init --skip-integrations
tracker integrations install --targets claude,codex
```

`--skip-integrations` aliases `--no-agents`. `--integrations` still opens the TTY
picker. `--yes` is still not remote-backup consent and still not consent for unnamed
OpenClaw `--global` skill copies.

## Agents

Init writes Atlas-managed MCP entries for detected clients. Restart each client.
`pending_client_restart` means the file matches `tracker mcp serve --global --tool-profile workflow` but the
process has not been observed to initialize. `unverified` means the command or args
drifted. Atlas does not rewrite unmanaged entries whose names merely start with
`atlas`.

## Backup

Local checkpoints stay in `<state>/backups/<workspace-id>/repo.git`. Existing targets
and replica refs (`refs/atlas/backups/<workspace>/<replica>`) are preserved. A push is
still not verified until isolated fetch. Restore is plan-then-apply bound to a stored
plan ID and digest — `ApplyRestorePlan` will not skip that binding.

On macOS, fresh installations use `~/Library/Application Support/Atlas Tasker`.
If a workspace already has backup history or a checkpoint opt-out under
`~/.local/state/atlas-tasker`, Atlas keeps using that location for that workspace.
It does not copy or move backup data. If both locations contain history, Atlas
reports `backup_state_conflict` and stops backup work until the conflict is
resolved. Keep both histories while investigating; `doctor --repair` does not
merge or delete them. An explicit custom state directory or absolute
`XDG_STATE_HOME` remains authoritative.

## Doctor

`tracker doctor` no longer fails with “not an Atlas workspace” when you run it outside
a repo. It reports `current_workspace: none`. `--repair` remains the single repair
switch.

## See also

- [v1.14 migration](migration-v1.14.md) if you are coming from v1.13
- [Home and workspaces](guides/home-and-workspaces.md)
- [Setup and backup](guides/setup-and-backup.md)
- [Uninstall](guides/uninstall.md)
