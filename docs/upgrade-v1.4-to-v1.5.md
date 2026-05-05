# Upgrade v1.4 to v1.5

Upgrade goals:
- preserve existing ticket, project, orchestration, change, and event state
- lazily add v1.5 fields on write
- rebuild projections safely
- keep provider boundaries and side-effect suppression intact

## Recommended order

1. run `tracker doctor --json`
2. run `tracker reindex`
3. verify there are no pending mutation journals
4. install v1.5 or build from the v1.5 branch
5. run `tracker doctor --repair --json`
6. run one disposable delivery flow that proves change, archive, and dashboard/timeline surfaces work in the upgraded workspace

## Checks before upgrade

- `tracker doctor --json`
- `tracker reindex`
- verify there are no pending mutation journals
- verify the current workspace has no unexpected dirty source-repo state if you rely on worktree-backed runs

## Post-upgrade proving

- change create/status still work against upgraded runs
- permission and review surfaces still explain blocked actions
- export, archive, and compact operate only on the allowed v1.5 targets
- dashboard and timeline read surfaces match inspect and history
- reindex still preserves upgraded state without recreating side effects

## Suggested proving script

```bash
git init -b main
git config user.email atlas@example.com
git config user.name Atlas
printf '# atlas\n' > README.md
git add README.md
git commit -m init

tracker init
tracker project create APP "App Project"
tracker ticket create --project APP --title "Upgrade smoke" --type task --reviewer agent:reviewer-1 --actor human:owner
tracker ticket move APP-1 ready --actor human:owner
tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner
tracker run dispatch APP-1 --agent builder-1 --actor human:owner
tracker run launch <RUN-ID> --actor human:owner
tracker run start <RUN-ID> --actor human:owner
tracker ticket move APP-1 in_progress --actor human:owner
tracker run checkpoint <RUN-ID> --title "Upgrade checkpoint" --body "runtime + worktree ready" --actor human:owner
tracker run evidence add <RUN-ID> --type note --title "Upgrade evidence" --body "v1.5 proof" --actor human:owner
tracker change create <RUN-ID> --actor human:owner
tracker change status <CHANGE-ID>
tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor human:owner
tracker gate approve <GATE-ID> --actor agent:reviewer-1 --reason "upgrade smoke"
tracker run complete <RUN-ID> --actor human:owner --summary "upgrade smoke complete"
tracker ticket request-review APP-1 --actor agent:builder-1
tracker ticket approve APP-1 --actor agent:reviewer-1
tracker ticket complete APP-1 --actor human:owner
tracker archive plan --target runtime --project APP
tracker archive apply --target runtime --project APP --yes --actor human:owner
tracker archive restore <ARCHIVE-ID> --actor human:owner
tracker dashboard --json
tracker timeline APP-1 --json
tracker run cleanup <RUN-ID> --actor human:owner
tracker reindex
```

## Success criteria

- `change status <CHANGE-ID>` remains readable after `reindex`
- `archive list --target runtime` and `archive restore <ARCHIVE-ID>` work in the upgraded workspace
- `dashboard --json` and `timeline APP-1 --json` agree with `inspect APP-1 --json`
- `run view <RUN-ID>` still shows the upgraded run state after `reindex`
- `inspect APP-1 --json` shows `done`

## Rollback notes

v1.5 stays lazy-on-write and non-destructive, so rollback is operationally simple if no new v1.5-only commands were used. If you have already created v1.5-only retention/archive records, downgrade only after preserving the workspace copy you upgraded from. The safe rule is: if you need to go backward, restore the pre-upgrade workspace snapshot rather than trying to teach an older binary about newer metadata.
