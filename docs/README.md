# Atlas Docs

Start here if you are new to Atlas Tasker. This page is the public docs route map. The [latest stable release](https://github.com/myrrazor/atlas-tasker/releases/latest) lists published builds and their hosted verification; older version plans remain in this directory for reference.

## First Ten Minutes

- [Installation](installation.md): source builds, release installs, and verification.
- [Updating](guides/updating.md): check, preview, and safely replace the current binary.
- [Getting started](getting-started.md): `tracker init` then `tracker`.
- [v1.16 migration](migration-v1.16.md): upgrade notes for existing workspaces.
- [Home and workspaces](guides/home-and-workspaces.md): global Home, registry, repair, create/attach.
- [Browser management](v1.16-browser-management.md): Home/Kanban everyday ticket management.
- [Setup and backup](guides/setup-and-backup.md): local checkpoints vs named remotes; advanced `tracker setup`.
- [Uninstall](guides/uninstall.md): software-only removal that keeps boards and backups.
- [Coding-agent integrations](guides/agent-integrations.md): the six project skill targets and their generated files.
- [Client compatibility](v1.16-client-compatibility.md): guidance vs listing vs configured MCP vs live connection.
- [Quickstart](quickstart.md): one copyable terminal flow.
- [First agent workflow](first-agent-workflow.md): init, ask for status, then the advanced dispatch loop.
- [Scheduled work](scheduling.md): one-time human reminders, agent wakeups, ticking, and completion history.

## Tutorials

- [Install and initialize](tutorials/01-install-and-init.md)
- [Create a project and ticket](tutorials/02-create-project-and-ticket.md)
- [Run your first agent ticket](tutorials/03-run-your-first-agent-ticket.md)
- [Review and approve work](tutorials/04-review-and-approve-work.md)
- [Use the TUI](tutorials/05-use-the-tui.md)

## Examples

- [Demo outputs, prompt packs, and transcripts](examples/README.md)

## Concepts

- [Tickets and projects](concepts/tickets-and-projects.md)
- [Runs, evidence, and handoffs](concepts/runs-evidence-handoffs.md)
- [Gates, governance, and signatures](concepts/gates-governance-signatures.md)
- [Sync, bundles, and archives](concepts/sync-bundles-archives.md)
- [Redaction, audit, and backup](concepts/redaction-audit-backup.md)
- [MCP and agent surfaces](concepts/mcp-and-agent-surfaces.md)

## Guides

- [AGENTS.md](../AGENTS.md): the file to hand a coding agent — don'ts, the loop, exit codes, MCP registration
- [Codex](guides/codex.md)
- [Codex `/goal`](guides/codex-goals.md)
- [Claude Code](guides/claude-code.md)
- [Generic agents](guides/generic-agent.md)
- [Coding-agent integrations](guides/agent-integrations.md)
- [MCP for agents](guides/mcp-for-agents.md)
- [MCP](mcp.md): global and pinned serve, tool profiles, resources, and approvals
- [v1.15 migration](migration-v1.15.md): compatibility with v1.14 and published tags
- [v1.9 agent workflow](v1.9-agent-workflow.md)
- [Local web board](web-board.md)
- [Web board security](web-board-security.md)
- [Web board user guide](web-board-user-guide.md)
- [Web i18n notes](i18n-notes.md): the six configurable board languages and the remaining schedule translation gap
- [Doctor and repair](guides/doctor-and-repair.md)
- [Release verification](guides/release-verification.md)
- [Updating](guides/updating.md)
- [Operations](guides/operations.md)

## Reference

- [Commands](reference/commands.md)
- [CLI behavior](reference/cli.md)
- [Slash shell](reference/shell.md)
- [TUI](reference/tui.md)
- [JSON contracts](reference/json-contracts.md)
- [JSON output](reference/json-output.md)
- [Markdown format](reference/markdown-format.md)
- [Config](reference/config.md)
- [Events](reference/events.md)
- [File layout](reference/file-layout.md)
- [Storage layout](reference/storage-layout.md)
- [Security model](reference/security-model.md)
- [Release scripts](reference/release-scripts.md)
- [Troubleshooting index](reference/troubleshooting.md)
- [Known limitations](KNOWN_LIMITATIONS.md)

## Release Status

Atlas Tasker v1.16 adds anonymous verified installation, native client skills, and browser management. The one-line installer selects the latest published release. See [CHANGELOG.md](../CHANGELOG.md) and [v1.16 migration](migration-v1.16.md). Ordinary use is `tracker init` then `tracker`: Home, automatic agent MCP pickup, modern default boards, local checkpoints versus verified remotes, global MCP, repair/migration, and software-only uninstall. v1.14 `tracker setup` remains documented as an advanced path. See [v1.15 migration](migration-v1.15.md) when upgrading an older workspace. See verification results on the [GitHub release page](https://github.com/myrrazor/atlas-tasker/releases/latest). Local RC reports stay local proof. Read [public release gates](release/public-release-gates.md) before treating a development build as release-ready.

## Security Boundary

Atlas can verify signed artifacts against trusted local keys, enforce app-level governance, apply structured redaction to Atlas-owned data, produce signed audit packets, and plan restores without known local side effects.

Atlas does not claim OS sandboxing, SaaS-grade identity proof, encrypted-at-rest confidentiality, protection from malicious local users with filesystem access, formal DLP, full provider-rule enforcement, or full MCP client safety. Read [security limitations](security-limitations.md) before using Atlas on sensitive work.
