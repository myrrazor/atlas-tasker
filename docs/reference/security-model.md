# Security Model

Atlas security is application-level and local-first.

Atlas may claim:

- signed artifact verification against trusted local keys
- governance checks in Atlas service paths
- structured redaction for Atlas-owned data where redaction routes are implemented
- signed audit packets
- side-effect-safe restore planning

Atlas must not claim OS sandboxing, hosted identity proof, encrypted-at-rest confidentiality, protection from malicious local filesystem users, formal DLP, complete provider-rule enforcement, or full MCP client safety.

Read [../security-limitations.md](../security-limitations.md) before using Atlas with sensitive workspaces.
