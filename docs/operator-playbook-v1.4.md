# Operator Playbook v1.4

Primary workflows:
1. register agents and runbooks
2. dispatch work to an eligible agent
3. inspect run/worktree state
4. collect evidence and handoff packets
5. resolve gates from inbox/approvals
6. complete the run and clean up the worktree

Safety posture:
- replay/repair must not create live side effects
- git/worktrees are execution isolation only
- project guidance stays stable; run context lives in runtime artifacts
