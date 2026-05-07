# JSON Contracts

Atlas JSON output is intended for coding agents, scripts, and tests. Read the canonical contracts here:

- [JSON contracts](../json-contracts.md)
- [MCP JSON contracts](../mcp-json-contracts.md)
- [Canonicalization](../canonicalization.md)
- [Signature format](../signature-format.md)

General shape for public JSON outputs:

- `format_version`
- `kind`
- `generated_at`
- command-specific payload

Signed artifact verification uses one shared result envelope across bundle, sync, backup, audit, approval, handoff, evidence, and goal verification.

Example version payload:

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

Example verification payload:

```json
{
  "format_version": "v1",
  "kind": "signature_verification",
  "generated_at": "2026-05-07T00:00:00Z",
  "result": "trusted_valid",
  "artifact_kind": "bundle",
  "artifact_id": "bundle-123",
  "signer_key_fingerprint": "SHA256:example",
  "warnings": []
}
```

Example list payload:

```json
{
  "kind": "gate_list",
  "generated_at": "2026-05-07T00:00:00Z",
  "items": []
}
```
