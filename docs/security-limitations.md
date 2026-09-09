# Security Limitations

Atlas (current local-first line, including v1.10+) may claim:

- signed artifacts verify against trusted local keys
- governance policies are enforced by Atlas application logic
- redaction policies apply to Atlas-owned structured data
- audit packets can be signed and verified
- restore planning avoids known local side effects
- fail-closed rejection of symlinked inputs for exports, bundles, backups, archive apply/restore, compact, and workspace integration installs (including OpenClaw `--global` home writes)
- init seeds private modes for sensitive `.tracker` paths and `config.toml`, plus a managed `.gitignore` block for local-only derived paths
- soft warnings when ticket text looks like it contains secrets (PEM keys, `ghp_` tokens, and similar); this is guidance, not DLP

Atlas must not claim:

- OS-level sandboxing
- SaaS-grade identity proof
- encrypted-at-rest confidentiality
- protection from malicious local users with filesystem access
- full provider-rule enforcement
- full MCP client safety
- formal data-loss prevention

These limits are product guarantees, not caveats to hide. Operator docs, release notes, CLI help, and JSON wording should avoid overclaiming identity, confidentiality, or provider authority.
