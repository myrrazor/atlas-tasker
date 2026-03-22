# atlas-tasker

Atlas Tasker v1 is a local-first, terminal-first, markdown-native issue tracker for AI coding agents.

## Quickstart

```bash
go run ./cmd/tracker init
go run ./cmd/tracker project create APP "App Project"
go run ./cmd/tracker ticket create --project APP --title "First task" --type task
go run ./cmd/tracker board --project APP
```

## Docs

- [Implementation plan](docs/v1-implementation-plan.md)
- [PR breakdown](docs/v1-ticket-pr-breakdown.md)
- [Decision log](docs/v1-decision-log.md)
- [Architecture](docs/architecture.md)
- [Command reference](docs/command-reference.md)
- [Agent rules](AGENTS.md)

## Fixture

Sample workspace fixture for smoke tests:

- `fixtures/app_sample`
- Run `go run ./cmd/tracker reindex` inside that fixture before querying board/search views.
