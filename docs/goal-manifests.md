# Goal Manifests

Goal briefs and manifests are read-only derived outputs for agents. They can be pasted into Codex `/goal`, Claude Code, or another coding agent. Atlas v1.7 does not spawn Codex, control goals, pause/resume goals, or call Codex app-server APIs.

PR-707 implements:

- `tracker goal brief <TICKET-ID|RUN-ID>`
- `tracker goal manifest <TICKET-ID|RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker goal verify <MANIFEST-ID|PATH>`
- `tracker sign goal <MANIFEST-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker verify goal <MANIFEST-ID|PATH>`

`goal brief` is pure output. `goal manifest` writes a derived local artifact under `.tracker/goal/manifests/`, requires actor/reason because it creates an evented handoff-ready artifact, and stores the current policy/trust snapshot hashes with the manifest. Goal files are local-only/derived by default and are not included in backups or sync unless a later explicit export path adds that.

## Markdown Sections

Goal markdown uses this order:

1. Goal
2. Objective
3. Current ticket/run
4. Acceptance criteria
5. Constraints
6. Required gates
7. Evidence needed
8. Allowed actions
9. Do not do
10. Current blockers
11. Context links
12. Done when

JSON output mirrors the same sections with stable fields. Goal briefs and manifests are redaction-aware: restricted ticket or run context is replaced with the configured goal marker, and generated manifests bind the redaction preview id. They may be signed/verified as `goal_manifest` artifacts.

## Output Contract

Every manifest includes:

- `format_version`, `manifest_id`, `target_kind`, `target_id`, `generated_at`, `generated_by`, and `reason`
- the stable section list above
- `policy_snapshot_hash`, `trust_snapshot_hash`, and `source_hash`
- `redaction_preview_id` when restricted context was marker-redacted
- optional `signature_envelopes`

The markdown form intentionally stays compact and pasteable. It avoids local private-key details, provider credentials, MCP approvals, runtime directories, and generated backup state. Verification checks the stored manifest payload and signature envelopes; it does not re-interpret the goal against current live policy.
