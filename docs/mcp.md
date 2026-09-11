# Atlas MCP Adapter

Atlas exposes a local stdio MCP server for coding agents that need structured access to tickets, runs, gates, changes, checks, sync state, and release workflow context.

The MCP adapter is not a second source of truth. It calls the same service layer as the CLI and TUI. Tracked mutations use Atlas permission checks, write locks, actor/reason event metadata, and storage contracts. `atlas.project.create` is the existing container-creation exception: it takes only `key` and `name`, uses the write lock and project validation, and does not create an event.

## Commands

```bash
tracker mcp serve --tool-profile read
tracker mcp serve --workspace /path/to/workspace --tool-profile read
tracker mcp serve --workspace-from-cwd --expected-workspace-id <WORKSPACE-ID> --tool-profile read
tracker mcp serve --workspace /absolute/existing/repo --init-if-missing --tool-profile workflow
tracker mcp schema --json --tool-profile workflow
tracker mcp tools --json --tool-profile admin
tracker mcp approve-operation --operation atlas.change.merge --target CHG-123 --actor human:owner --reason "release merge"
tracker mcp approvals list --json
tracker mcp approvals revoke <APPROVAL-ID>
```

`serve` reads the current directory unless `--workspace` names one. Registrations that live outside a repo — user-scoped `claude mcp add`, a global Codex `mcp_servers` entry — need it, because the client picks the working directory, not you. By default, the workspace must already contain Atlas state from `tracker init`.

`--workspace-from-cwd` plus `--expected-workspace-id` is the portable binding for clients that cannot safely store an absolute machine path (DEC-072, DEC-076). Atlas canonicalizes the current directory, walks only to the nearest real Atlas root, verifies the ID, and refuses nested workspaces, symlink substitution, a replaced directory, and a copied workspace with a stale local registration. It never initializes and never opens a different registered workspace. `--workspace-from-cwd` cannot be combined with `--workspace` or `--init-if-missing`. When `--workspace` is used with `--expected-workspace-id`, the ID is verified on that root.

`--init-if-missing` is an explicit, noninteractive bootstrap. It requires `--workspace` to name an absolute path to an existing directory and requires a write-capable `workflow`, `delivery`, or `admin` profile. It refuses nested Atlas workspaces, `--read-only`, and existing output paths that redirect initialization outside the selected directory. It creates only normal Atlas workspace files: it does not open an integration picker, register an MCP client, or use the client's working directory as a fallback. If the workspace is already initialized, Atlas opens it without rerunning initialization. `schema` and `tools` only describe the adapter and never initialize a workspace.

Stdio framing: Atlas speaks newline-delimited JSON-RPC and also accepts LSP-style `Content-Length` headers on the same stdio pair. Prefer NDJSON when you control the client; header-framed clients no longer crash the session.

## Profiles

- `read` is the default: 41 read and plan/dry-run tools, including goal brief, agent/team reads, and wake-up inspection.
- `workflow` exposes 73 tools. It adds project creation and the real agent loop: ticket create/edit/assign/link, priority and label changes, claim/heartbeat/move/comment, request review, approve/reject/complete, agent create/edit, team apply, schedule writes, evidence, handoffs, and wake-up ack.
- `delivery` exposes 77 tools normally and 79 with `--dangerously-allow-high-impact-tools`. It adds run dispatch, change creation, change/check sync, and provider review/merge tools.
- `admin` exposes 77 tools normally and the complete 88-tool inventory with `--dangerously-allow-high-impact-tools`. Its high-impact sync, import, archive, compact, worktree-cleanup, and gate-waiver tools remain hidden without that flag.

High-impact tools are hidden unless both the selected profile and server flag allow them. MCP-first agents should start at `workflow`, not `read`.

## Ordinary Workflow Additions

Use the exact names published by `tracker mcp schema`; Atlas does not accept silent aliases:

- `atlas.project.create` takes `key` and `name`. It is a project-container write and therefore takes no `actor` or `reason` and records no event.
- `atlas.ticket.heartbeat` takes `ticket_id`, `actor`, and `reason`. The actor must hold the active lease.
- `atlas.ticket.priority` takes `ticket_id`, `priority`, `actor`, and `reason`.
- `atlas.ticket.label.add` and `atlas.ticket.label.remove` take `ticket_id`, `label`, `actor`, and `reason`.
- `atlas.ticket.edit` takes `ticket_id`, `actor`, `reason`, plus any of `title`, `description`, `acceptance`, `priority`, `labels`, `assignee`, or `reviewer`. Omitted fields stay unchanged. An empty `description`, `assignee`, or `reviewer` string clears that field; an empty `acceptance` or `labels` array clears that list. An empty title is invalid. It cannot change status, policy, project, identity, timestamps, lease, or archive state.

The adapter passes a supplied description unchanged to the existing Markdown storage path; that
codec's normal boundary-whitespace formatting still applies when the ticket is persisted.

`atlas.ticket.create` always requires an explicit `type`; its MCP handler stores a supplied template
name but does not apply template defaults. Template-provided type remains a CLI-only convenience for
`tracker ticket create`. All five tracked tools above require both actor and non-empty reason. Their
schemas use `additionalProperties: false`, so misspelled or unknown arguments fail instead of being
ignored.

## High-Impact Approval Flow

High-impact MCP calls require a one-time approval created outside MCP:

```bash
tracker mcp approve-operation \
  --operation atlas.change.merge \
  --target CHG-123 \
  --actor human:owner \
  --reason "approved release merge" \
  --ttl 10m \
  --json
```

The tool call must pass the returned `operation_approval_id` and exact `confirm_text`, for example:

```json
{
  "change_id": "CHG-123",
  "actor": "human:owner",
  "reason": "approved release merge",
  "operation_approval_id": "mcp_approval_...",
  "confirm_text": "execute atlas.change.merge CHG-123"
}
```

Atlas validates the approval was created outside MCP, matches the operation, target, and actor, has not expired, has not been used, and still passes normal policy immediately before execution.

The approval target is exact. Tools with side-effecting modifiers bind those modifiers into the target string:

- `atlas.sync.pull`: `{"remote_id":"<REMOTE>","source_workspace_id":"<WORKSPACE-OR-EMPTY>"}`
- `atlas.archive.apply`: `{"project":"<PROJECT-OR-EMPTY>","target":"<RETENTION-TARGET>"}`
- `atlas.worktree.cleanup`: `{"force":false,"run_id":"<RUN>"}`

MCP calls are validated against their JSON schema before they reach Atlas services. Unknown arguments and wrong JSON types are rejected at the adapter boundary.

`atlas.ticket.request_review` accepts the same optional reviewer actor as the CLI `--reviewer` flag. Workflow tools that can advance blocked tickets accept `override_deps` only for `human:owner` with a non-empty reason. The call still goes through the same service-layer dependency checks and records the unresolved blockers in the mutation payload.

The adapter validator enforces required fields, rejects unknown arguments, checks primitive JSON types, and checks string-array items. It does not implement every JSON Schema keyword such as `enum`, `pattern`, numeric bounds beyond the simple `limit` shape, or semantic existence checks. Domain validation, permission checks, and policy gates still run in the Atlas service layer.

Approval consumption is single-use and happens after MCP argument/profile/actor/reason validation but before the service action starts. If the provider or service action later fails, the approval remains used; run the plan/dry-run tool again and create a new approval for a retry. This avoids letting an approval be replayed after execution has begun.

## Output Limits

Use these flags to keep model context under control:

```bash
tracker mcp serve \
  --tool-profile read \
  --max-result-bytes 131072 \
  --max-items 50 \
  --max-text-tokens-estimate 4000
```

Paged list tools accept `limit` and `cursor`. Grouped tools keep independent cursors: `atlas.board` accepts `cursor_by_status`, and `atlas.dashboard` accepts `cursor_by_section`. Large results return a truncated summary with a hint to narrow the request.

## More

Installing an Atlas agent integration does not register this MCP server. Use
`tracker integrations install ...` for project instructions and skills, then configure
`tracker mcp serve` separately in clients that should receive structured tools. See
[coding-agent integrations](guides/agent-integrations.md).

- [MCP security](mcp-security.md)
- [MCP tool table](mcp-tools.md)
- [Codex setup](mcp-codex.md)
- [Claude Code setup](mcp-claude-code.md)
- [MCP for agents](guides/mcp-for-agents.md)
- [MCP JSON contracts](mcp-json-contracts.md)
