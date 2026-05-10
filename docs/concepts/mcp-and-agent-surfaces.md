# MCP And Agent Surfaces

Atlas can serve an MCP adapter for coding agents, but MCP is not the only or safest way to use Atlas. The CLI, shell, TUI, JSON output, and Markdown artifacts remain first-class.

Safe default MCP usage should focus on reads:

- queue and next work
- ticket inspect/history
- board, dashboard, timeline
- gates, approvals, inbox
- evidence and handoff views
- change/check/sync status

Atlas MCP docs must not expose private-key operations or trust mutation by default. High-impact MCP tools must respect v1.7 governance and approval protections.

For now, use these docs:

- [MCP overview](../mcp.md)
- [MCP security](../mcp-security.md)
- [MCP tools](../mcp-tools.md)
- [MCP Codex setup](../mcp-codex.md)
- [MCP Claude Code setup](../mcp-claude-code.md)

PR-805 adds the polished public agent workflow guides.
