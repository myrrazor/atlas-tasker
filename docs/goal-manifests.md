# Goal Manifests

Goal briefs and manifests are read-only derived outputs for agents. They can be pasted into Codex `/goal`, Claude Code, or another coding agent. Atlas v1.7 does not spawn Codex, control goals, pause/resume goals, or call Codex app-server APIs.

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

JSON output mirrors the same sections with stable fields. Goal manifests are redaction-aware and may be signed/verified as `goal_manifest` artifacts.
