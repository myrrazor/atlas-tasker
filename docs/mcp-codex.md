# Codex MCP Setup

Use an absolute path to the built `tracker` binary.

CLI setup:

```bash
codex mcp add atlas -- /usr/local/bin/tracker mcp serve --workspace /path/to/workspace --tool-profile read
```

Verify the saved server with `codex mcp list`; inside an interactive Codex session, `/mcp` shows the
active MCP servers.

Stdio speaks newline-delimited JSON-RPC and also accepts LSP-style `Content-Length` frames.

Global config. A globally registered server does not inherit your shell's directory, so name the workspace:

```toml
[mcp_servers.atlas]
command = "/usr/local/bin/tracker"
args = ["mcp", "serve", "--workspace", "/path/to/workspace", "--tool-profile", "read"]
```

Project-scoped config in a trusted project can use the same shape. Keeping `--workspace` explicit makes the target independent of the client working directory:

```toml
[mcp_servers.atlas]
command = "/usr/local/bin/tracker"
args = [
  "mcp",
  "serve",
  "--workspace",
  "/path/to/workspace",
  "--tool-profile",
  "workflow",
  "--max-items",
  "30",
  "--max-result-bytes",
  "65536"
]
```

For delivery/admin sessions, start narrow and explicit:

```toml
[mcp_servers.atlas-delivery]
command = "/usr/local/bin/tracker"
args = ["mcp", "serve", "--workspace", "/path/to/workspace", "--tool-profile", "delivery"]
```

Only add high-impact tools for a short, supervised session:

```toml
[mcp_servers.atlas-admin]
command = "/usr/local/bin/tracker"
args = ["mcp", "serve", "--workspace", "/path/to/workspace", "--tool-profile", "admin", "--dangerously-allow-high-impact-tools"]
```

High-impact calls still require `tracker mcp approve-operation` outside MCP.

`--workspace` must point at a directory that has already been through `tracker init`; anything else fails at startup naming the path it tried, rather than serving an empty board.

For the CLI-first Codex workflow, read [Codex guide](guides/codex.md) and [Codex `/goal` guide](guides/codex-goals.md).

`tracker integrations install codex` is complementary: it writes `AGENTS.md` guidance and the
repo-local `atlas-worker` skill, but it does not create this MCP entry. See
[coding-agent integrations](guides/agent-integrations.md).

See [OpenAI's Codex MCP documentation](https://developers.openai.com/codex/mcp).
