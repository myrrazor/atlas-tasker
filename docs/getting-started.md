# Getting Started

Install the binary, initialize Atlas in the project you care about, restart
the coding agent so it loads Atlas MCP, then ask for ticket status.

The [latest published release](https://github.com/myrrazor/atlas-tasker/releases/latest)
is what the installer installs. Home, global MCP, and software-only uninstall are
the v1.15 source candidate; unstamped builds report `"version": "dev"`.

## The short path

Install Atlas once:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

In the project you want tracked, ask your coding agent:

```text
Initialize Atlas Tasker in this project.
```

With v1.15, Atlas sets up the board, local checkpoints, and integrations for
supported agents installed on your machine. Restart your coding agent to load
the integration.

Then ask normally:

```text
What's the current status of this project?
```

A real Grok Build status capture is in the README. It uses synthetic sample
tickets and a v1.15 source build. Grok reads Atlas through MCP and renders a
ticket table in its own terminal interface.

## 1. Install The CLI

Release installer (published tag):

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
tracker version --json
```

From this source (v1.15 candidate):

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

If `go build` tries to download a Go toolchain, let it finish or install the pinned
Go version from `go.mod`. See [installation.md](installation.md).

## 2. Initialize, Then Open Home

Use a directory named `app` so the default project key is `APP`. In any other
directory, use the generated key. Long multiword names use a readable word; names
without a valid short key fall back to `MAIN`.

```bash
mkdir app && cd app
tracker init
tracker
```

`init` writes `.tracker/`, local checkpoints, and Atlas-managed MCP entries for
coding agents it finds. `tracker` opens Atlas Home on `127.0.0.1:7432` (or prints
the URL / JSON). Restart the coding agent so it loads the new MCP server; until
then the registration is `pending_client_restart`.

Opt out: `--no-agents` / `--skip-integrations`, `--no-backup`, `--no-register`,
`--no-open`, `--git-mode private|unmanaged`. `--integrations` opens the older TTY
picker. Keep `.tracker/` out of public bug reports unless you have redacted it.

## 3. Create A Ticket

```bash
tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "getting started"
tracker ticket move APP-1 ready --actor human:owner --reason "ready to plan"
```

Extra projects are still `tracker project create AUTH "Auth"`.

## 4. Inspect The Board

```bash
tracker board
tracker queue --actor human:owner
tracker inspect APP-1 --actor human:owner
```

Default `tracker board` is a polished table. `--style kanban` selects side-by-side cards.
`--json` is the machine contract:

```bash
tracker inspect APP-1 --actor human:owner --json
```

## 5. Optional Remote Backup

Local checkpoints started at init. A remote is extra, named by you, never inferred
from Git `origin`. See [setup and backup](guides/setup-and-backup.md).

```bash
tracker backup target add --id private \
  --url git@github.com:you/atlas-backups.git \
  --acknowledge-data-boundary --attest-private
tracker backup auto enable --target private
tracker backup run --now
```

A push is not verified until Atlas fetches the remote commit into an empty temporary
repository.

## 6. Keep Going

- [Quickstart](quickstart.md) gives one copyable flow.
- [Home and workspaces](guides/home-and-workspaces.md) covers Home routes and repair.
- [Setup and backup](guides/setup-and-backup.md) covers checkpoints, remotes, and `tracker setup`.
- [Coding-agent integrations](guides/agent-integrations.md) explains Claude, Codex, Cursor, OpenClaw, Grok, and generic setup.
- [MCP for agents](guides/mcp-for-agents.md) is the global and pinned-workspace MCP path.
- [Uninstall](guides/uninstall.md) removes software and keeps boards.
- [Doctor and repair](guides/doctor-and-repair.md) is the health-check path.
- [Updating](guides/updating.md) explains check, dry-run, version pinning, and verified replacement.
