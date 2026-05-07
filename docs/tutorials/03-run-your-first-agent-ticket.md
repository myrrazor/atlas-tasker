# Tutorial 3: Run Your First Agent Ticket

Register an agent profile:

```bash
./tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner --reason "tutorial builder"
```

Dispatch the ticket:

```bash
printf '\ntracker\n.tracker/write.lock\n.tracker/index.sqlite\n.tracker/index.sqlite-wal\n.tracker/index.sqlite-shm\n' >> .git/info/exclude
git add -A
git commit -m "track atlas tutorial state"
git status --short
```

`git status --short` should be empty. Atlas blocks dispatch when the repo is dirty, because dispatch may create a managed worktree.

```bash
./tracker run dispatch APP-1 --agent builder-1 --actor human:owner --reason "tutorial dispatch"
```

Use the returned run ID in later commands:

```bash
./tracker run start <RUN-ID> --summary "Work started" --actor agent:builder-1 --reason "begin tutorial run"
./tracker run checkpoint <RUN-ID> --title "First checkpoint" --body "Implementation is in progress." --actor agent:builder-1 --reason "checkpoint"
./tracker run evidence add <RUN-ID> --type note --title "Local proof" --body "Smoke test passed locally." --actor agent:builder-1 --reason "evidence"
```

Atlas does not spawn the provider for you here. The profile and run give Codex, Claude Code, or another coding agent a durable record to work against.

Next: [review and approve work](04-review-and-approve-work.md).
