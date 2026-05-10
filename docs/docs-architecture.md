# Docs Architecture

v1.8 changes Atlas docs from implementation-history-first to user-first. Historical plans remain available, but the primary path should help a new user install Atlas, create a ticket, run an agent workflow, verify release artifacts, and understand security boundaries.

## Public Entry Points

- `README.md`: GitHub landing page, quickstart, major capabilities, release status, security summary, docs links.
- `docs/README.md`: docs landing page and route map.
- `docs/getting-started.md`: first ten minutes.
- `docs/installation.md`: install from release, source, and local dev.
- `docs/quickstart.md`: copyable workspace flow.
- `docs/first-agent-workflow.md`: ticket to run to evidence to review.

## User Docs

- `docs/tutorials/`: step-by-step learning path.
- `docs/concepts/`: model explanations: tickets, runs, gates, evidence, sync, governance, signatures, redaction, backups, MCP.
- `docs/guides/`: task-oriented guides for Codex, Claude Code, MCP, release verification, and operations.
- `docs/reference/`: command, JSON, MCP, file layout, release scripts, and troubleshooting references.
- `docs/examples/`: prompt packs, transcripts, and demo walkthroughs.

## Maintainer Docs

- `docs/v1*.md`: historical planning, decisions, PR breakdowns, and release evidence.
- `docs/release/`: public release gates, launch checklist, release evidence, and release notes.
- `docs/release-proof/`: raw proof artifacts from local or hosted release attempts.

## Linking Rules

- README links to current user docs first and historical plans last.
- Public docs should avoid promising hosted identity, encrypted storage, OS sandboxing, or full MCP client safety.
- Command snippets should include `--actor` and `--reason` when the CLI requires them.
- Docs that describe release readiness must link to `docs/release/public-release-gates.md`.
