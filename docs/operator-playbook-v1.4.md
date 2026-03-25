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

TUI operator console:
1. `Detail` is the selected-ticket cockpit: ticket body, git context, runs, evidence, handoffs, and runtime paths
2. `Inbox` shows approval work, derived handoff attention, delivery logs, and dead letters in one place
3. `Ops` keeps agent capacity, dispatch candidates, worktree drift, automation explain, and bulk preview state on one screen
4. the command palette now understands `/run open <RUN-ID>` and `/run launch <RUN-ID> [--refresh]` in addition to the existing ticket and bulk helpers
