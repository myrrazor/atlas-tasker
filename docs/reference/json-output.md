# JSON Output

Atlas JSON is intended for tools and agents. Most JSON-producing commands use a stable envelope:

```json
{
  "format_version": "v1",
  "kind": "tracker_version",
  "version": "v1.8.0-rc1",
  "commit": "abc123",
  "build_date": "2026-05-07T00:00:00Z",
  "go_version": "go1.26.2",
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

For scripting, prefer fields over pretty text. Do not parse ANSI, Markdown, or TUI output.
