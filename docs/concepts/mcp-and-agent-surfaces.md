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

On the v1.15 candidate, `tracker init` writes Atlas-managed MCP entries for detected
clients (`tracker mcp serve --global --tool-profile workflow`) as well as the worker skill. Restart the client.
`tracker integrations install` still writes skills only. Pinned
Pinned `tracker mcp serve --workspace` is still supported and still defaults to
`--tool-profile read`. Global resources are documented in [MCP overview](../mcp.md).
