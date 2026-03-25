# Upgrade v1.3 to v1.4

Upgrade goals:
- preserve existing ticket/project/event state
- lazily add v1.4 fields on write
- rebuild projections safely
- keep v1.3.1 replay, locking, and JSON guarantees intact

Recommended order:
1. run `tracker doctor --json`
2. run `tracker reindex`
3. verify no pending mutation journals
4. install v1.4 or build from the v1.4 branch
5. run `tracker doctor --repair --json`
6. create one disposable agent and run to prove orchestration state works in the upgraded workspace

Checks before upgrade:
- `tracker doctor --json`
- `tracker reindex`
- verify no pending mutation journals

Post-upgrade proving:
- agent registry create/list
- run dispatch into a temp worktree
- evidence and handoff generation
- gate open/approve flow
- cleanup and reindex parity

Suggested proving script:

```bash
git init -b main
git config user.email atlas@example.com
git config user.name Atlas
printf '# atlas\n' > README.md
git add README.md
git commit -m init

tracker init
tracker project create APP "App Project"
tracker ticket create --project APP --title "Upgrade smoke" --type task --actor human:owner
tracker ticket move APP-1 ready --actor human:owner
tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner
tracker run dispatch APP-1 --agent builder-1 --actor human:owner
tracker run launch <RUN-ID> --actor human:owner
tracker ticket move APP-1 in_progress --actor human:owner
tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor human:owner
tracker approvals --json
tracker gate approve <GATE-ID> --actor agent:reviewer-1 --reason "upgrade smoke"
tracker run complete <RUN-ID> --actor human:owner
tracker ticket request-review APP-1 --actor agent:builder-1
tracker ticket approve APP-1 --actor agent:reviewer-1
tracker ticket complete APP-1 --actor human:owner
tracker run cleanup <RUN-ID> --actor human:owner
tracker reindex
```

Success criteria:
- `run view <RUN-ID>` still shows `cleaned_up`
- `evidence list <RUN-ID>` and `handoff view <HANDOFF-ID>` still work after `reindex`
- `inspect APP-1 --json` shows `done`
