# Operator Playbook v1.4

Primary workflows:
1. register agents and runbooks
2. dispatch work to an eligible agent
3. inspect run/worktree state and generate run-scoped launch artifacts
4. collect evidence and handoff packets
5. resolve gates from inbox/approvals
6. complete the run and clean up the worktree

Safety posture:
- replay/repair must not create live side effects
- git/worktrees are execution isolation only
- project guidance stays stable; run context lives in runtime artifacts

Launch loop:
1. `tracker run open <RUN-ID> --json` to inspect the canonical runtime, evidence, and worktree paths
2. `tracker run launch <RUN-ID>` to write `brief.md`, `context.json`, `launch.codex.txt`, and `launch.claude.txt`
3. hand the provider-specific launch file to Codex or Claude
4. `tracker run attach <RUN-ID> --provider <codex|claude> --session-ref <session>` once the external session exists
