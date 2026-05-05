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

Every mutation requires:

```json
{
  "actor": "human:owner",
  "reason": "why this mutation is being made"
}
```

Every high-impact mutation also requires:

```json
{
  "operation_approval_id": "mcp_approval_...",
  "confirm_text": "execute atlas.change.merge CHG-123"
}
```

The approval target must match the tool target exactly. High-impact tools with extra side-effecting arguments use canonical JSON targets, such as `{"remote_id":"origin","source_workspace_id":"workspace-a"}` for `atlas.sync.pull`.
