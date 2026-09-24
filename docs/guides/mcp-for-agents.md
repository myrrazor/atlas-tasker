# MCP For Agents

Atlas MCP is a local stdio adapter for agents that need structured tools instead of shelling out to the CLI. It is not a separate authority layer.

## Start the server

`tracker init` already registered detected clients as **`atlas-tasker`** with
`tracker mcp serve --global --tool-profile workflow`. Grok's entry also gets
`--tool-name-style portable` so tools/list names are `atlas_status` rather than
`atlas.status`; other clients keep the dotted names. Restart the client (Grok:
also trust the project), then inspect. A matching file is not a live connection.

CLI `mcp serve`, `mcp tools`, and `mcp schema` default to `--tool-profile workflow`.
Pass `--tool-profile read` or `--read-only` for inspection only.

```bash
tracker mcp serve --global --tool-profile workflow
tracker mcp tools --json --global --tool-profile workflow
tracker mcp schema --json --global --tool-profile workflow
```

Pinned per-workspace serve remains (advanced). It also defaults to workflow:

```bash
tracker mcp serve --workspace /path/to/workspace --tool-profile workflow
tracker mcp tools --json --tool-profile workflow
tracker mcp schema --json --tool-profile workflow
```

The read profile is inspection, queues, plans, context, status, backup health, and
dry runs. It does not expose workflow writes or high-impact tools. Run
`tracker mcp tools --json` for the live count; do not copy a stale number.

`--tool-name-style` defaults to `canonical` (dotted `atlas.status`). `portable` advertises
the same tools as `atlas_status` for Grok Build and still dispatches to the canonical
handlers. Discovery descriptions mention the canonical name. Tool payloads keep public
`atlas.status` terminology.

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

Use the workflow profile when the human expects the agent to mutate Atlas state. This is the real agent loop profile — project create; ticket create/edit, assign, link, priority and label changes, claim, heartbeat, release, move, comment, review, approve, reject, and complete; agent/team setup; schedule writes; checkpoints, evidence, handoffs, and wake-up acknowledgement. Run `tracker mcp tools --json --tool-profile workflow` for the live inventory:

```bash
tracker mcp serve --global --tool-profile workflow --max-items 30 --max-result-bytes 65536
# or pin one repo:
tracker mcp serve --workspace /path/to/workspace --tool-profile workflow --max-items 30 --max-result-bytes 65536
```

Cross-workspace writes on `--global` require `workspace_id`. Restore apply and backup
target changes are high-impact and need `tracker mcp approve-operation` plus the
plan ID/digest from `atlas.restore.plan`.

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

The reviewer calls `atlas.ticket.approve`. Under `review_gate`, that same mutation returns the ticket
in `done`; call `atlas.ticket.complete` afterward only when another completion mode leaves the
approved ticket in `in_review` and the active policy permits the actor.

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

Exact delivery and admin counts move as tools are added. Run
`tracker mcp tools --json --tool-profile <profile>` (add `--dangerously-allow-high-impact-tools`
when inspecting the guarded catalog) rather than copying a number from an older page.

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

User-scoped Home registration keeps an absolute `tracker` path. Project-scoped Grok
config uses the bare `tracker` name only when `PATH` resolves to that same executable.
An absolute project command is kept only when scope validation allows it (not a
home-directory path). A source build under `$HOME` that is missing from `PATH` is not
written into `.grok/config.toml`; put that binary on `PATH` as `tracker` and rerun
setup. Do not use `sh -c`, `npx`, curl pipes, or snippets from untrusted workspace files.

`tracker integrations install` does not perform this registration. It writes project instructions and
skills. Configure MCP in the client separately; see [coding-agent integrations](agent-integrations.md).

For a status or board question, agents should call `atlas.status` / `atlas.board` (Grok Build:
`atlas_status` / `atlas_board`). An MCP Apps host can display the read-only board widget
inline. In a host that supports raw inline HTML, `tracker board --style html` emits a
self-contained fragment. Otherwise use the board's Markdown field or `tracker board --style markdown` and include blockers
and next steps. Pass `format=chat` only for a host confirmed to render ANSI code blocks.
If the payload is truncated, disclose shown/total. Do not invent tickets or paste a
fake screenshot. See [chat board display](chat-board.md).

Use [MCP tools](../mcp-tools.md), [MCP JSON contracts](../mcp-json-contracts.md), and [MCP security](../mcp-security.md) as the canonical references.
