# JSON Output

Atlas JSON is intended for tools and agents. Most JSON-producing commands use a stable envelope:

```json
{
  "format_version": "v1",
  "kind": "tracker_version",
  "version": "v1.9.0-rc1",
  "commit": "abc123",
  "build_date": "2026-05-07T00:00:00Z",
  "go_version": "go1.26.3",
  "platform": "darwin/arm64"
}
```

List commands usually return:

```json
{
  "kind": "gate_list",
  "generated_at": "2026-05-07T00:00:00Z",
  "items": []
}
```

`tracker key list --json` uses the same list envelope and flattens public key fields onto each item so agents can read `items[0].public_key_id` directly:

```json
{
  "format_version": "v1",
  "kind": "key_list",
  "generated_at": "2026-05-07T00:00:00Z",
  "items": [
    {
      "kind": "key_detail",
      "public_key_id": "key_abc123",
      "fingerprint": "sha256:...",
      "owner_kind": "collaborator",
      "owner_id": "alice",
      "status": "active",
      "source": "local",
      "can_sign": true
    }
  ]
}
```

For scripting, prefer fields over pretty text. Do not parse ANSI, Markdown, or TUI output.
