# Atlas Tasker

Local-first task orchestration for AI coding agents and humans.

[![CI](https://github.com/myrrazor/atlas-tasker/actions/workflows/ci.yml/badge.svg)](https://github.com/myrrazor/atlas-tasker/actions/workflows/ci.yml)
[![Release status](https://img.shields.io/badge/status-v1.8.0--rc1%20planned-yellow)](docs/release/public-release-gates.md)

Atlas Tasker is a terminal-first, markdown-native issue tracker and orchestration layer. It gives you Jira-like tickets, Kanban views, review gates, agent runs, evidence, handoffs, Git/worktree integration, collaboration sync, signed artifacts, audit packets, backups, goal manifests, and MCP access without requiring a hosted server.

`v1.8.0-rc1` is planned, not shipped. Release sign-off is blocked until the hosted gates in [public release gates](docs/release/public-release-gates.md) pass.

## Why Atlas

Coding agents need more than a TODO list. They need work queues, ownership, review gates, durable evidence, handoffs, and a way for humans to see what happened without scraping terminal scrollback.

Atlas keeps that coordination in your repo:

- tickets and project state live as local markdown plus Atlas metadata
- every mutation goes through the same service layer from CLI, shell, TUI, and MCP
- agent runs can attach checkpoints, evidence, handoffs, changes, checks, and review gates
- collaboration sync, bundles, archives, audits, and backups stay inspectable
- v1.7 security surfaces add signatures, governance, redaction previews, audit packets, and restore planning

## Quickstart

Build from source while the hosted release is still gated:

```bash
go build -o tracker ./cmd/tracker
./tracker init
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "quickstart"
./tracker ticket move APP-1 ready --actor human:owner --reason "start work"
./tracker board
./tracker tui
```

Typical terminal output is intentionally scannable:

```text
Backlog (0)
  - (empty)

Ready (1)
  - APP-1 Ship first feature

In Progress (0)
  - (empty)
```

## Agent Workflows

Atlas works best when humans and agents share the same ticket flow:

```bash
tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner --reason "register builder"
tracker run dispatch APP-1 --agent builder-1 --actor human:owner --reason "start implementation"
tracker run checkpoint <RUN-ID> --title "Implemented first pass" --body "Tests are green locally." --actor agent:builder-1 --reason "status update"
tracker run evidence add <RUN-ID> --type note --title "Test proof" --body "go test ./... passed" --actor agent:builder-1 --reason "attach proof"
tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
```

Goal manifests help coding agents start with the right context:

```bash
tracker goal brief APP-1 --md
tracker goal manifest APP-1 --actor human:owner --reason "prepare Codex goal" --md
```

## Install And Verify

The one-line installer is intended for published releases:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

For release candidates, prefer explicit verification:

```bash
VERSION=v1.8.0-rc1 ./scripts/verify-release.sh ./tracker_1.8.0-rc1_darwin_arm64.tar.gz
VERSION=v1.8.0-rc1 BIN_DIR="$HOME/.local/bin" sh ./scripts/install.sh
```

`scripts/verify-release.sh` verifies release checksums and, by default, GitHub artifact attestations. Set `VERIFY_ATTESTATIONS=0` only for local rehearsals or intentionally unattested artifacts.

## Docs

Current docs:

- [Product positioning](docs/product-positioning.md)
- [Docs architecture](docs/docs-architecture.md)
- [Public release gates](docs/release/public-release-gates.md)
- [Command reference](docs/command-reference.md)
- [Architecture](docs/architecture.md)
- [Operator manual](docs/operator-manual.md)
- [MCP adapter](docs/mcp.md)
- [MCP security](docs/mcp-security.md)
- [Goal manifests](docs/goal-manifests.md)
- [Security limitations](docs/security-limitations.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Release guide](docs/release.md)

The public getting-started docs, tutorials, and examples are part of the v1.8 stack after PR-802.

## Security

Atlas can verify signed artifacts against trusted local keys, enforce app-level governance, apply structured redaction to Atlas-owned data, produce signed audit packets, and plan restores without known local side effects.

Atlas does not claim OS sandboxing, hosted identity, encrypted-at-rest storage, protection from malicious local filesystem users, formal DLP, full provider-rule enforcement, or full MCP client safety. Read [security limitations](docs/security-limitations.md) before using Atlas on sensitive workspaces.

Do not paste private keys, webhook URLs, tokens, full `.tracker` archives, or unredacted logs into public issues.

## Project Status

- Current train: `v1.8.0-rc1` planned
- Local v1.7.1 proof: green
- Hosted prerelease proof: blocked until a release actor publishes and verifies assets
- License: pending owner selection; public reuse terms are not finalized yet

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and [GOVERNANCE.md](GOVERNANCE.md). PRs should include local proof, docs updates for public interfaces, and no secret material in examples or logs.
