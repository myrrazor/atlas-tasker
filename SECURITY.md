# Security Policy

Atlas stores local task state, agent evidence, sync artifacts, signatures, governance policy, redaction metadata, audit reports, and backups. Treat workspace data as sensitive.

## Supported Versions

`v1.8.0-rc1` is planned, not shipped. Until hosted release proof passes, security reports should reference the commit SHA, branch, or PR where the issue appears.

## Reporting A Vulnerability

Do not post secrets, private keys, webhook URLs, exploit payloads, or full `.tracker` archives in public issues.

Preferred paths:

1. Use GitHub private vulnerability reporting: `https://github.com/myrrazor/atlas-tasker/security/advisories/new`.
2. If GitHub reports that private vulnerability reporting is unavailable, open a public issue with a minimal title and no sensitive details, then ask the maintainer for a private channel.

Good public issue title:

```text
Potential redaction bypass in export preview flow
```

Bad public issue body:

```text
Here is my full .tracker directory and private key...
```

## What Atlas Claims

Atlas may claim:

- signed artifact verification against trusted local keys
- app-level governance enforcement
- structured redaction for Atlas-owned data
- signed audit packets
- side-effect-safe restore planning

Atlas does not claim OS sandboxing, SaaS-grade identity proof, encrypted-at-rest storage, protection from malicious local filesystem users, formal DLP, full provider-rule enforcement, or full MCP client safety.

## Handling Sensitive Attachments

Before sharing output, redact:

- private keys and trust-store secrets
- tokens, webhook URLs, and API keys
- customer names, private repo names, and private branch names
- full `.tracker/events/` history unless a maintainer explicitly asks for a minimized reproduction
- generated backup archives, sync bundles, and audit packets
