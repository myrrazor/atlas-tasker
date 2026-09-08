# Roadmap

`v1.10.0` is the current stable release. It shipped the local web console (welcome dashboard, the restyled Kanban board, schedule workspace, read-only settings), six-language web chrome, one-time scheduled tickets across CLI, web, and MCP, the security audit batch, the agent-ready CLI work (`--json` on every command agents run, the rewritten `AGENTS.md`, MCP workspace pinning), and the marketing site. What is left is follow-up work, not new subsystems.

## Next

Follow-ups flagged during the v1.10 work:

- `tracker config set web.lang` still validates only `en`, `es`, and `id` even though the `zh`, `ja`, and `ko` catalogs already ship — accept all six
- the schedule workspace renders English-only; bring it into the language catalogs
- richer screenshots and GIFs, plus packaged examples for common agent workflows

## Not planned

Hosted server, SaaS identity, CRDT rewrite, plugin marketplace, mandatory MCP flow, encrypted workspace storage, provider-rule controller, or a hidden scheduler daemon — unattended schedule ticking stays on the owner's explicit cron, launchd, or other trusted runner.
