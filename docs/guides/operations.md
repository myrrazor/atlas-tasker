# Operations

Atlas is local-first. Operational discipline is mostly about keeping state inspectable and not confusing local proof with hosted proof.

## Daily Checks

```bash
tracker
tracker doctor --json
tracker board
tracker approvals
tracker inbox
tracker sync status
```

On the v1.15 candidate, `tracker` with no args opens Home (or prints the URL). `doctor` checks the machine even outside a workspace.

## Before High-Impact Work

Prefer plan/read commands first:

```bash
tracker archive plan
tracker backup restore-plan <BACKUP-ID>
tracker governance explain APP-1 --action ticket_complete --actor human:owner --reason "preflight"
```

For mutating commands, pass a real actor and a non-empty reason where supported.

## Evidence Hygiene

- Keep proof attached to runs with `tracker run evidence add`.
- Do not paste private keys, tokens, webhook URLs, or raw `.tracker` archives into issues.
- Use redaction previews before creating redacted exports.
- Keep release proof in `docs/release-proof/` and release decisions in `docs/release/`.
