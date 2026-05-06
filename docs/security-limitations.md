# Security Limitations

Atlas v1.7 may claim:

- signed artifacts verify against trusted local keys
- governance policies are enforced by Atlas application logic
- redaction policies apply to Atlas-owned structured data
- audit packets can be signed and verified
- restore planning avoids known local side effects

Atlas v1.7 must not claim:

- OS-level sandboxing
- SaaS-grade identity proof
- encrypted-at-rest confidentiality
- protection from malicious local users with filesystem access
- full provider-rule enforcement
- full MCP client safety
- formal data-loss prevention

These limits are product guarantees, not caveats to hide. Operator docs, release notes, CLI help, and JSON wording should avoid overclaiming identity, confidentiality, or provider authority.
