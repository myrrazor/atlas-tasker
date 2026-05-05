# Claude Code MCP Setup

Claude Code manages MCP servers with `/mcp` and `claude mcp` commands. Use an absolute `tracker` binary path and avoid shell wrappers.

Example:

```bash
claude mcp add --transport stdio --scope user atlas -- /Users/you/bin/tracker mcp serve --tool-profile read
```

For a project-scoped workflow profile:

```bash
claude mcp add --transport stdio --scope project atlas-workflow -- /Users/you/bin/tracker mcp serve --tool-profile workflow --max-items 30
```

Check status in Claude Code:

```text
/mcp
```

Do not configure Atlas MCP through `sh -c`, `npx`, or snippets from untrusted workspaces. Keep high-impact tools out of normal Claude Code sessions; use `tracker mcp approve-operation` only when a human explicitly authorizes a specific operation and target.

See Claude Code’s MCP docs: <https://docs.claude.com/en/docs/claude-code/mcp>.
