# MCP For Agents

Atlas MCP is a local stdio adapter for agents that need structured tools instead of shelling out to the CLI. It is not a separate authority layer.

## Start Read-Only

```bash
tracker mcp serve --tool-profile read
tracker mcp tools --json --tool-profile read
tracker mcp schema --json --tool-profile read
```

The read profile includes core read tools and plan/dry-run tools. It does not expose workflow writes or high-impact tools.

## Workflow Sessions

Use the workflow profile only when the human expects the agent to mutate Atlas state:

```bash
tracker mcp serve --tool-profile workflow --max-items 30 --max-result-bytes 65536
```

Workflow writes still require actor, reason, permissions, event metadata, and the Atlas write lock.

## High-Impact Sessions

High-impact tools are hidden unless the selected profile includes them and the server was started with:

```bash
tracker mcp serve --tool-profile admin --dangerously-allow-high-impact-tools
```

Execution still requires a one-time approval created outside MCP:

```bash
tracker mcp approve-operation \
  --operation atlas.change.merge \
  --target CHG-123 \
  --actor human:owner \
  --reason "approved release merge" \
  --ttl 10m \
  --json
```

The approval is a transport safety gate, not an authorization override. Atlas still evaluates service-layer permissions and governance.

## Keep Config Boring

Pin MCP config to an absolute `tracker` binary path. Do not use `sh -c`, `npx`, curl pipes, or snippets from untrusted workspace files.

Use [MCP tools](../mcp-tools.md), [MCP JSON contracts](../mcp-json-contracts.md), and [MCP security](../mcp-security.md) as the canonical references.
