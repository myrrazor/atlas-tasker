# MCP For Agents

Atlas MCP is a local stdio adapter for agents that need structured tools instead of shelling out to the CLI. It is not a separate authority layer.

## Start Read-Only

```bash
tracker mcp serve --tool-profile read
tracker mcp tools --json --tool-profile read
tracker mcp schema --json --tool-profile read
```

The read profile includes 41 core read and plan/dry-run tools. It does not expose workflow writes or high-impact tools.

Stdio speaks newline-delimited JSON-RPC and also accepts LSP-style `Content-Length` frames.

If the MCP client launches the server from somewhere other than the repo, add `--workspace /path/to/workspace`. Otherwise the tools answer against whatever directory the client started in.

An uninitialized workspace still fails closed by default. When the user explicitly authorizes
bootstrap, use an absolute path to an existing directory and a write-capable profile:

```bash
tracker mcp serve --workspace /absolute/existing/repo --init-if-missing --tool-profile workflow
```

This startup path is noninteractive. It refuses a relative or missing directory, a nested Atlas
workspace, `--read-only`, and redirected initialization outputs. It creates normal workspace files
only; it does not register MCP in a client or install agent guidance. If the directory is already
initialized, startup opens it without rerunning initialization.

## Workflow Sessions

Use the 73-tool workflow profile when the human expects the agent to mutate Atlas state. This is the real agent loop profile — project create; ticket create/edit, assign, link, priority and label changes, claim, heartbeat, release, move, comment, review, approve, reject, and complete; agent/team setup; schedule writes; checkpoints, evidence, handoffs, and wake-up acknowledgement:

```bash
tracker mcp serve --tool-profile workflow --max-items 30 --max-result-bytes 65536
```

Tracked workflow writes require actor, non-empty reason, permissions, event metadata, and the Atlas
write lock. `atlas.project.create` is the container exception: it accepts only `key` and `name`, uses
project validation and the write lock, and records no event.

A typical MCP worker uses this sequence:

1. Read `atlas.agent.available` or `atlas.next`.
2. Inspect the selected ticket with `atlas.ticket.inspect`.
3. Call `atlas.ticket.claim`, then `atlas.ticket.move` to `in_progress`, passing `actor` and `reason` to both.
4. During long work, call `atlas.ticket.heartbeat` with the same lease-holding actor and a reason.
5. Record progress with `atlas.ticket.comment` or `atlas.run.checkpoint`.
6. Attach proof with `atlas.evidence.add`, then call `atlas.ticket.request_review`.

The new ordinary tool names are exact: `atlas.project.create`, `atlas.ticket.heartbeat`,
`atlas.ticket.priority`, `atlas.ticket.label.add`, `atlas.ticket.label.remove`, and
`atlas.ticket.edit`. The edit tool accepts optional `title`, `description`, `acceptance`, `priority`,
`labels`, `assignee`, and `reviewer`. Omitted fields stay unchanged; explicit empty values clear the
supported description, acceptance, labels, assignee, or reviewer fields. It cannot change status or
policy. A supplied description goes unchanged from the adapter to the existing Markdown storage;
that codec's normal boundary-whitespace formatting still applies. `atlas.ticket.create` still needs
an explicit `type`; MCP can store a template name but does not apply its defaults. Template-provided
type is a CLI-only `tracker ticket create` behavior.

Schemas use `additionalProperties: false`. Use the names returned by `tracker mcp schema --json`
exactly: unknown fields, wrong types, null edit values, and silent aliases are rejected.

The workflow profile does not expose `atlas.dispatch.run`; dispatch is a delivery operation because it
creates run/worktree state. Start a separate delivery-profile session when dispatch is part of the
authorized workflow.

## High-Impact Sessions

Delivery exposes 77 tools normally and 79 with the danger flag. Admin also exposes 77 normally and
the full 88-tool inventory with the flag.

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
