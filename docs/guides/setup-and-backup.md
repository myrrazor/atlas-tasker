# Setup and backup

This is the regular-user path after `tracker init`. `tracker setup` and automatic Atlas backup are part of the v1.14 implementation candidate. They are in this source tree; the latest published installer tag is still v1.13.0. This guide does not publish a release and it does not turn on off-device backup by itself.

## Connect your coding agents

If you just installed the binary, start with `tracker init` in the project. That is the
detect-and-pick step: Atlas checks the agents it found, you press Enter, and those agents can
read the board. `tracker setup` is the later one-pass refresh for the workspace you are standing in:

1. Detect the agents installed on this machine.
2. Refresh the existing Atlas worker skill (there is still one skill, not a second one).
3. Register a workspace-bound MCP server where the selected client supports it.
4. Optionally write managed-mode policy.
5. Optionally enable an *already added* backup target and run one first checkpoint plus remote verify.

```bash
tracker setup --plan
tracker setup --yes --agents generic
# or name every client this repo actually uses:
# tracker setup --yes --agents claude,codex,cursor,openclaw,grok,generic --mode managed
```

`--yes` applies the plan. It is not consent for backup, and it is not consent for machine-wide OpenClaw writes. Those stay named and separate.

What setup will not do:

- Rewrite custom text outside the Atlas markers in `AGENTS.md` or `CLAUDE.md`.
- Rewrite an unmanaged MCP server whose name starts with `atlas` (for example a leftover `atlas-legacy` entry).
- Pick your Git `origin` as a backup target.
- Install a user-level backup scheduler.

`tracker update` only replaces the `tracker` binary. Run setup again when you want provider config refreshed.

## Automatic Atlas backup

Atlas can snapshot *Atlas-owned* tickets and events into an isolated Git repository, then optionally publish that snapshot to a remote you name. It never uses your project `origin` unless you type that URL yourself.

```bash
# 1. Add a target. Production remotes are https or SSH.
tracker backup target add --id private \
  --url git@github.com:you/atlas-backups.git \
  --acknowledge-data-boundary --attest-private

# Disposable local drills only (not an off-device backup):
# tracker backup target add --id drill --url file:///tmp/atlas-drill.git \
#   --allow-local-file --acknowledge-data-boundary --attest-private

# 2. Enable automatic checkpoints and run one now.
tracker backup auto enable --target private
tracker backup run --now

# Or fold enable + first verify into setup:
tracker setup --yes --agents generic --backup --backup-target private
```

What this means in practice:

- Local checkpoints live outside your working tree, under `$XDG_STATE_HOME/atlas-tasker/backups/<workspace-id>/` (or `~/.local/state/atlas-tasker/backups/<workspace-id>/`). They do not change your repo's current branch, index, or uncommitted files.
- A push is not "verified" until Atlas fetches the remote commit into an empty temporary repository and checks the tree and manifest. `tracker setup status` and `tracker backup auto status` report verified only for the current target, and never while the replica is blocked.
- If the remote is offline or has diverged, ticket writes still work. Atlas will not force-push. A failed remote publish does not block a later local restore from a checkpoint that already landed locally.
- Restore goes through `tracker backup restore-plan` then `restore-apply`. `--yes` is confirmation, not authorization, when a restore policy is in force.
- The optional user scheduler is `tracker backup schedule install --yes`. Neither `tracker init` nor `tracker setup` installs it. Install writes the unit files only. On Linux, activate them with `systemctl --user daemon-reload` and `systemctl --user enable --now atlas-backup-<workspace-id>.timer`. The timer includes `OnStartupSec` so it also fires after login, not only after a previous run. On macOS, load the LaunchAgent after install (`launchctl load ~/Library/LaunchAgents/com.atlas-tasker.backup.<workspace-id>.plist`).

Public GitHub remotes need an explicit public attestation plus `--allow-public-github`. URLs that embed passwords or tokens are rejected.

See [migration notes](../migration-v1.14.md) if you are upgrading a v1.13 workspace, and the [command reference](../command-reference.md) for every backup flag.
