# Atlas Docs

Start here if you are new to Atlas Tasker. The older version plans are still in this directory, but this page is the public docs route map for `v1.8.0-rc1`.

## First Ten Minutes

- [Installation](installation.md): source builds, release installs, and verification.
- [Getting started](getting-started.md): the shortest path from empty repo to first ticket.
- [Quickstart](quickstart.md): one copyable terminal flow.
- [First agent workflow](first-agent-workflow.md): register an agent, dispatch work, attach evidence, and hand off for review.

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

- [Codex](guides/codex.md)
- [Codex `/goal`](guides/codex-goals.md)
- [Claude Code](guides/claude-code.md)
- [Generic agents](guides/generic-agent.md)
- [MCP for agents](guides/mcp-for-agents.md)
- [v1.9 agent workflow](v1.9-agent-workflow.md)
- [Doctor and repair](guides/doctor-and-repair.md)
- [Release verification](guides/release-verification.md)
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

## Release Status

`v1.8.0-rc1` is planned, not shipped. Local proof and hosted release proof are separate gates. Read [public release gates](release/public-release-gates.md) before treating any build as release-ready.

## Security Boundary

Atlas can verify signed artifacts against trusted local keys, enforce app-level governance, apply structured redaction to Atlas-owned data, produce signed audit packets, and plan restores without known local side effects.

Atlas does not claim OS sandboxing, SaaS-grade identity proof, encrypted-at-rest confidentiality, protection from malicious local users with filesystem access, formal DLP, full provider-rule enforcement, or full MCP client safety. Read [security limitations](security-limitations.md) before using Atlas on sensitive work.
