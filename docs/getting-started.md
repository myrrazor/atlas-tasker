# Getting Started

Install the binary, initialize Atlas in the project you care about, restart
the coding agent so it loads Atlas MCP, then ask for ticket status.

The [latest published release](https://github.com/myrrazor/atlas-tasker/releases/latest)
is what the one-line installer installs. See verification results on the release
page. Unstamped source builds report `"version": "dev"`.

## The short path

Needs `curl`, `tar`, and GitHub CLI (`gh`). Attestation uses a local bundle;
`gh` does not need a GitHub login.

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

In the project you want tracked, ask your coding agent:

```text
Initialize Atlas Tasker in this project.
```

That is `tracker init`. Atlas sets up the board, local checkpoints, and
integrations for supported agents installed on your machine. Restart your
coding agent. Grok also needs you to trust this project in its own UI before
it lists local skills.

Then ask normally:

```text
What's the current status of this project?
```

A real Grok Build status capture is in the README. It uses synthetic sample
tickets and was recorded on a v1.15 source build. Grok reads Atlas through MCP
and renders a ticket table in its own terminal interface.

## 1. Install The CLI

Release installer (published tag):

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
tracker version --json
```

Optional, from this source (commands use `./tracker` until it is on `PATH`):

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

If `go build` tries to download a Go toolchain, let it finish or install the
pinned Go version from `go.mod`. See [installation.md](installation.md).

## 2. Initialize, Then Open Home

Use a directory named `app` so the default project key is `APP`. In any other
directory, use the generated key. Long multiword names use a readable word;
names without a valid short key fall back to `MAIN`.

```bash
mkdir app && cd app
tracker init
tracker
```

`init` writes `.tracker/`, local checkpoints, and Atlas-managed MCP entries
named `atlas-tasker` (`mcp serve --global --tool-profile workflow`) for coding
agents it finds. Detected agents are configured without a picker.
`--integrations` opens the older TTY picker. `tracker` opens Atlas Home on
`127.0.0.1:7432` (or prints the URL / JSON). Restart the coding agent so it
loads the new MCP server; until then the registration is
`pending_client_restart`.

Opt out: `--no-agents` / `--skip-integrations`, `--no-backup`, `--no-register`,
`--no-open`, `--git-mode private|unmanaged`. Keep `.tracker/` out of public bug
reports unless you have redacted it.

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

Default `tracker board` is a polished table. `--style kanban` selects
side-by-side cards. `--json` is the machine contract:

```bash
tracker inspect APP-1 --actor human:owner --json
```

From Home, open the project Kanban to create, edit, assign, and archive
tickets. [Browser management](v1.16-browser-management.md) (v1.16).

## 5. Optional Remote Backup

Local checkpoints started at init. A remote is extra, named by you, never
inferred from Git `origin`. See [setup and backup](guides/setup-and-backup.md).

```bash
tracker backup target add --id private \
  --url git@github.com:you/atlas-backups.git \
  --acknowledge-data-boundary --attest-private
tracker backup auto enable --target private
tracker backup run --now
```

A push is not verified until Atlas fetches the remote commit into an empty
temporary repository.

## 6. Keep Going

- [Quickstart](quickstart.md) gives one copyable flow.
- [v1.16 migration](migration-v1.16.md) is for upgraders.
- [Home and workspaces](guides/home-and-workspaces.md) covers Home routes and repair.
- [Setup and backup](guides/setup-and-backup.md) covers checkpoints, remotes, and `tracker setup`.
- [Coding-agent integrations](guides/agent-integrations.md) explains Claude, Codex, Cursor, OpenClaw, Grok, and generic setup.
- [Compatibility](v1.16-client-compatibility.md) is skill roots, trust, and what “connected” actually means.
- [MCP for agents](guides/mcp-for-agents.md) is the global and pinned-workspace MCP path.
- [Uninstall](guides/uninstall.md) removes software and keeps boards.
- [Doctor and repair](guides/doctor-and-repair.md) is the health-check path.
- [Updating](guides/updating.md) explains check, dry-run, version pinning, and verified replacement.
