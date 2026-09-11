# Getting Started

This path gets you from a clean checkout to a usable local Atlas workspace.

## 1. Build The CLI

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

If `go build` tries to download a Go toolchain, let it finish or install the pinned Go version from `go.mod`. Prebuilt stable releases are also available; see [installation.md](installation.md).

## 2. Initialize Atlas

Run this inside the repo or workspace you want Atlas to manage:

```bash
./tracker init
```

In a terminal, Atlas asks whether to install coding-agent integrations after initialization. Detected
agents are checked in the picker; choose `none` to skip. Use `./tracker init --skip-integrations` for a
non-interactive bootstrap, or see [coding-agent integrations](guides/agent-integrations.md) for all six
targets and scripted setup.

Atlas writes local state under `.tracker/`. Keep that directory out of public bug reports unless you have redacted it.

## 3. Create A Project And Ticket

```bash
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "getting started"
./tracker ticket move APP-1 ready --actor human:owner --reason "ready to plan"
```

## 4. Inspect The Work Queue

```bash
./tracker board
./tracker queue --actor human:owner
./tracker inspect APP-1 --actor human:owner
```

Use `--json` when another tool needs structured output:

```bash
./tracker inspect APP-1 --actor human:owner --json
```

## 5. Connect Agents And Optional Backup

`tracker setup` and automatic Atlas backup are in the v1.14 implementation candidate. Build from this source to try them; the latest published installer tag is still v1.13.0.

```bash
./tracker setup --plan
./tracker setup --yes --agents generic
```

That refreshes the existing worker skill and, where the client supports it, registers a workspace-bound MCP server. `--yes` is not backup consent. To turn on automatic Atlas checkpoints you first add a target, then either `tracker backup auto enable` or `tracker setup --backup --backup-target <ID>`. The short guide is [setup and backup](guides/setup-and-backup.md).

## 6. Keep Going

- [Quickstart](quickstart.md) gives one copyable flow.
- [Setup and backup](guides/setup-and-backup.md) covers `tracker setup` and automatic Atlas checkpoints.
- [Coding-agent integrations](guides/agent-integrations.md) explains Claude, Codex, Cursor, OpenClaw, Grok, and generic setup.
- [First agent workflow](first-agent-workflow.md) shows the agent run lifecycle.
- [Updating](guides/updating.md) explains check, dry-run, version pinning, and verified replacement.
- [Doctor and repair](guides/doctor-and-repair.md) explains the safe health-check path.
- [Release verification](guides/release-verification.md) explains why a local build is not the same as a hosted release.
