# Classification And Redaction

Classification levels are ordered:

```text
public < internal < confidential < restricted
```

The default is `internal`. Projects, tickets, runs, evidence, handoffs, audits, and backups inherit classification from parents. Higher sensitivity wins. Lower labels cannot downgrade inherited sensitivity unless governance policy explicitly allows downgrade.

## Redaction Preview

Redaction preview is mandatory before redacted export, sync, audit, backup, or goal output. `tracker redact preview` creates a local actor-bound preview record; it is not just a read-only display. A preview is bound to:

- scope
- target
- actor
- policy version hash
- classification version hash
- source state hash
- command target
- TTL

Stale, reused, actor-mismatched, policy-mismatched, classification-mismatched, source-mismatched, or target-mismatched previews are rejected.

Redacted export bundles, sync publications, audit reports, backups, and goal manifests must carry `redaction_preview_id` in artifact metadata so verification can prove which preview authorized the redacted output.

Redaction should operate on structured fields where possible. String replacement is only a fallback for opaque text.
