# Atlas MCP Adapter

Atlas exposes a local stdio MCP server for coding agents that need structured access to tickets, runs, gates, changes, checks, sync state, and release workflow context.

The MCP adapter is not a second source of truth. It calls the same service layer as the CLI and TUI, and every mutation still uses Atlas permission checks, write locks, event metadata, and storage contracts.

## Commands

```bash
tracker mcp serve --tool-profile read
tracker mcp schema --json --tool-profile workflow
tracker mcp tools --json --tool-profile admin
tracker mcp approve-operation --operation change.merge --target CHG-123 --actor human:owner --reason "release merge"
tracker mcp approvals list --json
tracker mcp approvals revoke <APPROVAL-ID>
```

## Profiles

- `read` is the default. It exposes read tools and dry-run/plan tools only.
- `workflow` adds safe workflow writes like comments, claims, gate approve/reject, checkpoints, evidence, handoffs, and import preview.
- `delivery` adds delivery operations such as dispatch, change creation, status sync, check sync, and guarded provider review/merge tools.
- `admin` adds high-impact admin operations only when `--dangerously-allow-high-impact-tools` is also present.

High-impact tools are hidden unless both the selected profile and server flag allow them.

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

The adapter validator is intentionally small in this RC. It enforces required fields, rejects unknown arguments, checks primitive JSON types, and checks string-array items. It does not implement every JSON Schema keyword such as `enum`, `pattern`, numeric bounds beyond the simple `limit` shape, or semantic existence checks. Domain validation, permission checks, and policy gates still run in the Atlas service layer.

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

Paged tools accept `limit` and `cursor`. Large results return a truncated summary with a hint to narrow the request.

## More

- [MCP security](mcp-security.md)
- [MCP tool table](mcp-tools.md)
- [Codex setup](mcp-codex.md)
- [Claude Code setup](mcp-claude-code.md)
- [MCP JSON contracts](mcp-json-contracts.md)
