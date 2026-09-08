# Atlas Threat Model

Atlas protects local-first collaboration from accidental or adversarial misuse inside Atlas-owned workflows. It is not an operating-system sandbox, hosted identity provider, encrypted vault, or network security product.

## In Scope

- tampered bundles, sync publications, backups, audit packets, handoffs, approvals, evidence packets, and goal manifests
- unknown, untrusted, revoked, expired, rotated, or malformed signer states
- protected-action bypass through older CLI/shell/TUI/MCP paths
- stale redaction previews used against changed source data
- accidental private-key leakage through sync, export, logs, JSON, markdown, TUI, `TEST_STDOUT.log`, or errors
- symlink-assisted path escape through archive restore/compact or home-directory skill installs
- secrets pasted into ticket bodies/comments (persisted in markdown and the event log)
- world-readable `.tracker` runtime/MCP approval directories on shared multi-user hosts
- restore flows that try to recreate worktrees, runtime dirs, provider state, notifiers, remotes, or MCP approvals

## Out Of Scope

- malicious local users who can read or replace private key files
- compromised host operating systems
- SaaS-grade identity proof
- encrypted storage at rest
- full provider-rule enforcement
- full MCP client safety

## Design Response

Atlas uses Ed25519 signatures for authenticity, governance policies for authority, classification/redaction for disclosure control, audit packets for evidence snapshots, and preview-first restore/redaction flows for dangerous state changes.
