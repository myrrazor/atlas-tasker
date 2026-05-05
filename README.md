# atlas-tasker

Atlas Tasker is a local-first, terminal-first, markdown-native issue tracker for AI coding agents.

## Install

One-line install for published releases:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

Source install for local development:

```bash
go build -o tracker ./cmd/tracker
./tracker init
```

## Quickstart

```bash
tracker init
tracker project create APP "App Project"
tracker ticket create --project APP --title "First task" --type task --actor human:owner
tracker queue --actor agent:builder-1
tracker tui --actor agent:builder-1
```

## Docs

- [v1.3 implementation plan](docs/v1.3-implementation-plan.md)
- [v1.3 PR breakdown](docs/v1.3-pr-breakdown.md)
- [v1.3 decision log](docs/v1.3-decision-log.md)
- [v1.2 implementation plan](docs/v1.2-implementation-plan.md)
- [v1.2 PR breakdown](docs/v1.2-ticket-breakdown.md)
- [v1.2 decision log](docs/v1.2-decision-log.md)
- [Implementation plan](docs/v1-implementation-plan.md)
- [PR breakdown](docs/v1-ticket-pr-breakdown.md)
- [Decision log](docs/v1-decision-log.md)
- [Architecture](docs/architecture.md)
- [Command reference](docs/command-reference.md)
- [MCP adapter](docs/mcp.md)
- [MCP security](docs/mcp-security.md)
- [Operator manual](docs/operator-manual.md)
- [Upgrade guide](docs/upgrade-v1.2-to-v1.3.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Release guide](docs/release.md)
- [Agent rules](AGENTS.md)

## Fixture

Sample workspace fixture for smoke tests:

- `fixtures/app_sample`
- Run `tracker doctor --repair` and `tracker reindex` inside that fixture before querying board/search views.
