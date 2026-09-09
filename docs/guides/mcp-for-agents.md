# MCP For Agents

Atlas MCP is a local stdio adapter for agents that need structured tools instead of shelling out to the CLI. It is not a separate authority layer.

## Start Read-Only

```bash
tracker mcp serve --tool-profile read
tracker mcp tools --json --tool-profile read
tracker mcp schema --json --tool-profile read
```

The read profile includes core read tools and plan/dry-run tools. It does not expose workflow writes or high-impact tools.

Stdio speaks newline-delimited JSON-RPC and also accepts LSP-style `Content-Length` frames.

If the MCP client launches the server from somewhere other than the repo, add `--workspace /path/to/workspace`. Otherwise the tools answer against whatever directory the client started in.

## Workflow Sessions

Use the workflow profile when the human expects the agent to mutate Atlas state. This is the real agent loop profile — ticket create, assign, link, claim, release, move, comment, review, approve, reject, and complete; agent/team setup; schedule writes; checkpoints, evidence, handoffs, and wake-up acknowledgement:

```bash
tracker mcp serve --tool-profile workflow --max-items 30 --max-result-bytes 65536
```

Workflow writes still require actor, reason, permissions, event metadata, and the Atlas write lock.

A typical MCP worker uses this sequence:

1. Read `atlas.agent.available` or `atlas.next`.
2. Inspect the selected ticket with `atlas.ticket.inspect`.
3. Call `atlas.ticket.claim`, then `atlas.ticket.move` to `in_progress`, passing `actor` and `reason` to both.
4. Record progress with `atlas.ticket.comment` or `atlas.run.checkpoint`.
5. Attach proof with `atlas.evidence.add`, then call `atlas.ticket.request_review`.

The workflow profile does not expose `atlas.dispatch.run`; dispatch is a delivery operation because it
creates run/worktree state. Start a separate delivery-profile session when dispatch is part of the
authorized workflow.

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

`tracker integrations install` does not perform this registration. It writes project instructions and
skills. Configure MCP in the client separately; see [coding-agent integrations](agent-integrations.md).

Use [MCP tools](../mcp-tools.md), [MCP JSON contracts](../mcp-json-contracts.md), and [MCP security](../mcp-security.md) as the canonical references.
