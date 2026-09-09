# MCP And Agent Surfaces

Atlas can serve an MCP adapter for coding agents, but MCP is not the only or safest way to use Atlas. The CLI, shell, TUI, JSON output, and Markdown artifacts remain first-class.

The default `read` profile focuses on:

- queue and next work
- ticket inspect/history
- board, dashboard, timeline
- gates, approvals, inbox
- evidence and handoff views
- change/check/sync status

The `workflow` profile adds ticket lifecycle, agent/team, schedule, checkpoint, evidence, handoff, and
wake-up mutations. `delivery` adds dispatch and provider-backed synchronization. High-impact operations
remain separately hidden and require an external one-time approval even when a delivery or admin
profile is selected.

Atlas MCP does not expose private-key operations or trust mutation. High-impact MCP tools respect the
same governance and permission checks as other Atlas surfaces.

Use these docs:

- [MCP overview](../mcp.md)
- [MCP security](../mcp-security.md)
- [MCP tools](../mcp-tools.md)
- [MCP Codex setup](../mcp-codex.md)
- [MCP Claude Code setup](../mcp-claude-code.md)
- [Coding-agent integrations](../guides/agent-integrations.md)

Project integration installation and MCP registration are separate. The integration installer writes
instructions and skills inside the workspace; an MCP client must be configured explicitly to launch
`tracker mcp serve`.
