# MCP JSON Contracts

MCP tool responses use structured content with the same `format_version` convention as Atlas CLI JSON.

Success:

```json
{
  "format_version": "v1",
  "kind": "atlas.ticket.view",
  "generated_at": "2026-05-05T12:00:00Z",
  "payload": {}
}
```

Tool error:

```json
{
  "format_version": "v1",
  "ok": false,
  "error": {
    "code": "permission_denied",
    "message": "operation approval expired",
    "exit": 5
  }
}
```

Large result truncation:

```json
{
  "format_version": "v1",
  "kind": "atlas.search",
  "generated_at": "2026-05-05T12:00:00Z",
  "payload": {
    "truncated": true,
    "original_bytes": 300000,
    "max_result_bytes": 131072,
    "hint": "Use filters, cursor, limit, or a narrower Atlas MCP tool call."
  }
}
```

Paged tools accept:

```json
{
  "limit": 25,
  "cursor": "25"
}
```

Paged payloads include:

```json
{
  "items": [],
  "total": 100,
  "next_cursor": "50",
  "truncated": true
}
```

Grouped reads use independent cursors so short sections do not disappear when a longer section advances:

```json
{
  "limit": 10,
  "cursor_by_status": {
    "ready": "10",
    "blocked": "20"
  }
}
```

Dashboard pagination uses the same shape with `cursor_by_section`.

Every tracked mutation requires:

```json
{
  "actor": "human:owner",
  "reason": "why this mutation is being made"
}
```

For example, a workflow-profile claim call uses:

```json
{
  "ticket_id": "APP-12",
  "actor": "agent:builder-1",
  "reason": "start work"
}
```

The same required pair applies to ticket, agent, team, schedule, checkpoint, evidence, and handoff
workflow tools. This includes `atlas.ticket.heartbeat`, `atlas.ticket.priority`,
`atlas.ticket.label.add`, `atlas.ticket.label.remove`, and `atlas.ticket.edit`. Read tools do not
require either field; some accept an optional `actor` as query context.

`atlas.project.create` is the project-container exception. Its complete input is:

```json
{
  "key": "APP",
  "name": "My App"
}
```

It has no actor/reason fields and creates no event. Project key/name validation and the Atlas write
lock still apply.

`atlas.ticket.edit` is a typed patch. For example:

```json
{
  "ticket_id": "APP-12",
  "description": "Use the retried response.",
  "acceptance": ["timeout is covered", "evidence is attached"],
  "labels": ["reliability"],
  "assignee": "",
  "actor": "agent:builder-1",
  "reason": "refine scope and clear assignment"
}
```

The optional edit fields are exactly `title`, `description`, `acceptance`, `priority`, `labels`,
`assignee`, and `reviewer`. Omitted fields are preserved. Explicit empty strings clear description,
assignee, or reviewer; empty arrays clear acceptance or labels. A blank title, invalid priority, or
invalid nonempty actor value is rejected. Status, policy, project, identity, timestamps, lease, and
archive fields are not accepted.

The adapter passes description input unchanged to the existing Markdown storage path. The Markdown
codec still applies its normal boundary-whitespace formatting when it persists the ticket.

Every MCP input schema is closed with `additionalProperties: false`. Unknown arguments, null where
the schema expects a value, and wrong primitive or array-item types are rejected. Tool and argument
names must match `tracker mcp schema --json` exactly; there are no silent aliases.

`atlas.ticket.create` always requires `type`. Its MCP handler can store a template name but does not
apply template defaults; deriving type from a selected template is supported by the CLI
`tracker ticket create` path only.

Every high-impact mutation also requires:

```json
{
  "operation_approval_id": "mcp_approval_...",
  "confirm_text": "execute atlas.change.merge CHG-123"
}
```

The approval target must match the tool target exactly. High-impact tools with extra side-effecting arguments use canonical JSON targets, such as `{"remote_id":"origin","source_workspace_id":"workspace-a"}` for `atlas.sync.pull`.
