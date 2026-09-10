# Claude Code MCP Setup

Claude Code manages MCP servers with `/mcp` and `claude mcp` commands. Use an absolute `tracker` binary path and avoid shell wrappers.

A user-scoped server starts wherever Claude Code happens to be, so pin the workspace with `--workspace`:

```bash
claude mcp add --transport stdio --scope user atlas -- /usr/local/bin/tracker mcp serve --workspace /path/to/workspace --tool-profile read
```

Stdio speaks newline-delimited JSON-RPC and also accepts LSP-style `Content-Length` frames.

For a project-scoped workflow profile, keep the workspace explicit so the target remains clear:

```bash
claude mcp add --transport stdio --scope project atlas-workflow -- /usr/local/bin/tracker mcp serve --workspace /path/to/workspace --tool-profile workflow --max-items 30
```

When the user explicitly wants an existing but uninitialized directory bootstrapped at server
startup, use a separate opt-in registration:

```bash
claude mcp add --transport stdio --scope project atlas-bootstrap -- /usr/local/bin/tracker mcp serve --workspace /absolute/existing/repo --init-if-missing --tool-profile workflow
```

Bootstrap is noninteractive and requires the explicit absolute workspace plus a write-capable
profile. It refuses missing or relative directories, nested Atlas workspaces, `--read-only`, and
redirected initialization outputs. It creates Atlas workspace files only; it does not register
another MCP server or install agent integrations.

Without `--workspace` the server reads whatever workspace it was started in — usually the reason a
tool answers `not_found` for a ticket you can see in the terminal. Without `--init-if-missing`, a
`--workspace` that does not exist or has no `.tracker/` directory fails at startup with the path it
tried.

The workflow profile exposes the exact tools `atlas.project.create`, `atlas.ticket.heartbeat`,
`atlas.ticket.priority`, `atlas.ticket.label.add`, `atlas.ticket.label.remove`, and
`atlas.ticket.edit`. The five ticket tools require `actor` and `reason`. Project creation takes only
`key` and `name` because it is the existing untracked container exception. Unknown fields and
unpublished aliases are rejected; inspect `tracker mcp schema --json --tool-profile workflow` for
the authoritative names.

Check status in Claude Code:

```text
/mcp
```

Do not configure Atlas MCP through `sh -c`, `npx`, or snippets from untrusted workspaces. Keep high-impact tools out of normal Claude Code sessions; use `tracker mcp approve-operation` only when a human explicitly authorizes a specific operation and target.

For the CLI-first Claude Code workflow, read [Claude Code guide](guides/claude-code.md).

`tracker integrations install claude` writes `CLAUDE.md`, `.claude/skills/`, and
`.claude/commands/` files. It does not create this MCP registration. See
[coding-agent integrations](guides/agent-integrations.md).

See Claude Code’s MCP docs: <https://docs.claude.com/en/docs/claude-code/mcp>.
