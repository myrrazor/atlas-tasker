# File Layout

Atlas writes local state under `.tracker/`. Important areas include:

- `.tracker/projects/`: project and ticket markdown
- `.tracker/events/`: append-only event JSONL
- `.tracker/index.sqlite`: derived SQLite projection
- `.tracker/runtime/`: run launch artifacts
- `.tracker/evidence/`: copied evidence artifacts
- `.tracker/handoffs/`: immutable handoff snapshots
- `.tracker/security/`: public keys, signatures, and private key material
- `.tracker/redaction/`: local redaction previews and rules
- `.tracker/backups/`: local backup records

Private keys, local trust decisions, redaction previews, backup snapshots, generated goal files, runtime/worktree/provider state, remotes, notifiers, and MCP approvals are local-only unless a future explicit export flow says otherwise.

Do not attach raw `.tracker` directories to public issues.
