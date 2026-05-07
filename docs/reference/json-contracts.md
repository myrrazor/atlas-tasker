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
