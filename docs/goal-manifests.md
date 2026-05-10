# Goal Manifests

Goal briefs and manifests are read-only derived outputs for agents. They can be pasted into Codex `/goal`, Claude Code, or another coding agent. Atlas does not spawn Codex, control goals, pause/resume goals, or call Codex app-server APIs.

Atlas implements:

- `tracker goal brief <TICKET-ID|RUN-ID>`
- `tracker goal manifest <TICKET-ID|RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker goal verify <MANIFEST-ID|PATH>`
- `tracker sign goal <MANIFEST-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker verify goal <MANIFEST-ID|PATH>`

`goal brief` is pure output. `goal manifest` writes a derived local artifact under `.tracker/goal/manifests/`, requires actor/reason because it creates an evented handoff-ready artifact, and stores the current policy/trust snapshot hashes with the manifest. Goal files are local-only/derived by default and are not included in backups or sync unless a later explicit export path adds that.

## Markdown Sections

Goal markdown uses the ticket title as the H1, then these sections:

1. Objective
2. Current State
3. Ticket / Run
4. Acceptance Criteria
5. Constraints
6. Required Evidence
7. Required Gates
8. Allowed Actions
9. Do Not Do
10. Context
11. Suggested Commands
12. Done When
13. Verification

JSON output keeps the stable `Goal` section for schema compatibility, but Markdown does not repeat it as `## Goal`. Goal manifests may be signed/verified as `goal_manifest` artifacts. Current goal output is not a redaction boundary: operators must avoid generating or sharing goal briefs from sensitive tickets unless the source ticket has already been redacted or classified for that audience.

## Output Contract

Every manifest includes:

- `format_version`, `manifest_id`, `target_kind`, `target_id`, `generated_at`, `generated_by`, and `reason`
- the stable section list above
- `policy_snapshot_hash`, `trust_snapshot_hash`, and `source_hash`
- optional `signature_envelopes`

The markdown form intentionally stays compact and pasteable. It avoids local private-key details, provider credentials, MCP approvals, runtime directories, and generated backup state. Verification checks the stored manifest payload and signature envelopes; it does not re-interpret the goal against current live policy.
