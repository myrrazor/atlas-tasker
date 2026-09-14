# Setup and backup

Ordinary use is `tracker init` then `tracker`. This page is the
backup contract plus the older `tracker setup` path. Neither the installer nor a
source build turns on off-device backup by itself.

The one-line installer is the normal path. See verification results on the
[GitHub release page](https://github.com/myrrazor/atlas-tasker/releases/latest).

## After install

```bash
tracker init
tracker
```

Init detects installed coding agents and writes Atlas-managed MCP entries pointing at
`tracker mcp serve --global --tool-profile workflow`, unless `--no-agents` / `--skip-integrations`. It also
starts **local checkpoints** unless `--no-backup`. Restart the client; `written` is not
the same as a live MCP session.

`--integrations` still opens the TTY picker. `tracker integrations install` writes skills
later. Details: [coding-agent integrations](agent-integrations.md).

## Local checkpoints vs a named remote

Local checkpoints live outside your working tree, under
`$XDG_STATE_HOME/atlas-tasker/backups/<workspace-id>/` (or
`~/.local/state/atlas-tasker/backups/<workspace-id>/`). They use an isolated bare Git
repo and replica refs `refs/atlas/backups/<workspace>/<replica>`. They do not change
your repo's current branch, index, remotes, or uncommitted files. Coalescing stays
about 30s quiet / 5min max / 100 pending events. Editing stays available offline.

A remote is extra. Atlas never selects Git `origin`.

```bash
# 1. Add a target. Production remotes are https or SSH.
tracker backup target add --id private \
  --url git@github.com:you/atlas-backups.git \
  --acknowledge-data-boundary --attest-private

# Disposable local drills only (not an off-device backup):
# tracker backup target add --id drill --url file:///tmp/atlas-drill.git \
#   --allow-local-file --acknowledge-data-boundary --attest-private

# 2. Enable automatic publish of local checkpoints and run one now.
tracker backup auto enable --target private
tracker backup run --now
```

What this means:

- A push is not "verified" until Atlas fetches the remote commit into an empty
  temporary repository and checks the commit, workspace, manifest, tree, and files.
  `tracker backup auto status` reports verified only for the current target, and never
  while the replica is blocked.
- If the remote is offline or has diverged, ticket writes still work. Atlas will not
  force-push.
- Restore is `tracker backup restore-plan` then `restore-apply`. Apply refuses unless
  the stored plan ID and digest still match. `--yes` is confirmation, not authorization,
  when a restore policy is in force.
- `file://` still needs `--allow-local-file` (DEC-088) and is not off-device proof.
- Public GitHub remotes need an explicit public attestation plus `--allow-public-github`.
  URLs that embed passwords or tokens are rejected.

The optional user scheduler is `tracker backup schedule install --yes`. Neither
`tracker init` nor `tracker setup` installs it against the real host. Software
uninstall does not delete backup repositories; see [uninstall](uninstall.md).

## Advanced: `tracker setup` (v1.14 path)

Keep this if you already scripted it. It is a later one-pass refresh for the workspace
you are standing in, not the default onboarding:

```bash
tracker setup --plan
tracker setup --yes --agents generic
# tracker setup --yes --agents claude,codex,cursor,openclaw,grok,generic --mode managed
```

`--yes` applies the plan. It is not consent for backup, and it is not consent for
machine-wide OpenClaw writes.

What setup will not do:

- Rewrite custom text outside the Atlas markers in `AGENTS.md` or `CLAUDE.md`.
- Rewrite an unmanaged MCP server whose name starts with `atlas` (for example a leftover
  `atlas-legacy` entry).
- Pick your Git `origin` as a backup target.
- Install a user-level backup scheduler.

`tracker update` only replaces the `tracker` binary. Run setup again when you want
provider config refreshed. Prefer `tracker init` / `tracker doctor --repair`
for machine agent entries and registry health.

See [v1.15 migration](../migration-v1.15.md) and the [command reference](../command-reference.md).
