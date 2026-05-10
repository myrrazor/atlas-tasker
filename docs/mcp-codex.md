# Codex MCP Setup

Use an absolute path to the built `tracker` binary.

CLI setup:

```bash
codex mcp add atlas -- /Users/you/bin/tracker mcp serve --tool-profile read
```

Global config:

```toml
[mcp_servers.atlas]
command = "/Users/you/bin/tracker"
args = ["mcp", "serve", "--tool-profile", "read"]
```

Project-scoped config in a trusted project can use the same shape:

```toml
[mcp_servers.atlas]
command = "/Users/you/bin/tracker"
args = [
  "mcp",
  "serve",
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
command = "/Users/you/bin/tracker"
args = ["mcp", "serve", "--tool-profile", "delivery"]
```

Only add high-impact tools for a short, supervised session:

```toml
[mcp_servers.atlas-admin]
command = "/Users/you/bin/tracker"
args = ["mcp", "serve", "--tool-profile", "admin", "--dangerously-allow-high-impact-tools"]
```

High-impact calls still require `tracker mcp approve-operation` outside MCP.

For the CLI-first Codex workflow, read [Codex guide](guides/codex.md) and [Codex `/goal` guide](guides/codex-goals.md).

See OpenAI’s Codex MCP docs: <https://developers.openai.com/codex/mcp>.
