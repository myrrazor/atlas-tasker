# Classification And Redaction

Classification levels are ordered:

```text
public < internal < confidential < restricted
```

The default is `internal`. Projects, tickets, runs, evidence, handoffs, audits, and backups inherit classification from parents. Higher sensitivity wins. Lower labels cannot downgrade inherited sensitivity.

PR-705 stores explicit labels as markdown frontmatter under `.tracker/classification/labels/` using collision-resistant `class-<slug>-<hash>.md` filenames. Updating a label removes the old slug-only filename if it exists. Labels currently cover workspace, project, ticket, run, evidence, and handoff entities. Legacy `protected` or `sensitive` tickets still contribute `restricted` to the effective ticket level so older workspaces do not lose protections during upgrade.

Governance policies with `classification:<level>` scope match the exact effective classification level. Redaction rules use the classification hierarchy: a rule with `min_level = "restricted"` affects restricted content, while a future `min_level = "confidential"` rule would affect confidential and restricted content.

## Redaction Preview

Redaction preview is mandatory before redacted export, sync, audit, backup, or goal output. `tracker redact preview` creates a local actor-bound preview record under `.tracker/redaction/previews/` with Unix mode `0600`; it is not just a read-only display. A preview is bound to:

- scope
- target
- actor
- policy version hash
- classification version hash
- source state hash
- command target
- TTL

Stale, reused, actor-mismatched, policy-mismatched, classification-mismatched, source-mismatched, target-mismatched, or item-mismatched previews are rejected. Export recomputes the preview items before writing so a locally edited preview file cannot weaken omissions.

PR-705 implements preview-bound redacted workspace exports. The default export policy omits files whose effective classification is `restricted`, including ticket- and run-owned gate/change/check metadata, classification label files for restricted records, and any extra file below a restricted project directory. Redacted exports always omit `.tracker/events/` history because event payloads can contain older full snapshots of restricted records. Rules live under `.tracker/redaction/rules/`; Atlas keeps conservative built-in defaults for targets that do not have custom stored rules, so a goal-only rule does not disable export omission. Export redaction currently supports `omit` only; `mask`, `hash`, and marker replacement fail closed until structured field-level export rewriting lands.

Redacted export bundles, sync publications, audit reports, backups, and goal manifests must carry `redaction_preview_id` in artifact metadata so verification can prove which preview authorized the redacted output. In PR-705, `tracker redact verify` checks redacted export integrity, confirms the preview binding exists, and verifies omitted preview paths are absent from the manifest and archive.

Redaction should operate on structured fields where possible. String replacement is only a fallback for opaque text.
