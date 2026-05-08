# First Agent Workflow

Atlas treats an agent run as a durable work record, not just a chat transcript. A typical flow is ticket, dispatch, checkpoint, evidence, handoff, review.

## 1. Create Work

```bash
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Add health check" --type task --actor human:owner --reason "agent workflow demo"
./tracker ticket move APP-1 ready --actor human:owner --reason "ready for implementation"
```

## 2. Register An Agent

```bash
./tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner --reason "register builder"
```

The agent profile records routing metadata only. It does not give Atlas control over Codex, Claude Code, or another provider.

## 3. Dispatch The Ticket

Dispatch requires a clean git workspace because Atlas may create a managed worktree. If you are using a source-built `tracker` binary in the repo root, exclude local build/projection files and commit the tutorial state before dispatching:

```bash
printf '\ntracker\n.tracker/write.lock\n.tracker/index.sqlite\n.tracker/index.sqlite-wal\n.tracker/index.sqlite-shm\n' >> .git/info/exclude
git add -A
git commit -m "track atlas tutorial state"
git status --short
```

`git status --short` should print nothing.

```bash
RUN_ID=$(./tracker run dispatch APP-1 --agent builder-1 --actor human:owner --reason "start implementation" --json | jq -r '.payload.run_id')
./tracker run launch "$RUN_ID" --actor human:owner --reason "prepare launch files"
./tracker run open "$RUN_ID" --json
```

These examples use `jq`; if it is not installed, copy `payload.run_id` from the JSON output and export it as `RUN_ID`.

## 4. Record Progress And Evidence

```bash
./tracker run start "$RUN_ID" --summary "Implementation started" --actor agent:builder-1 --reason "begin work"
./tracker run checkpoint "$RUN_ID" --title "First pass" --body "Health check route added locally." --actor agent:builder-1 --reason "status update"
./tracker run evidence add "$RUN_ID" --type note --title "Test proof" --body "go test ./... passed" --actor agent:builder-1 --reason "attach test proof"
```

Evidence can also copy a file into the run evidence bundle with `--artifact <PATH>`.

## 5. Hand Off For Review

```bash
./tracker run handoff "$RUN_ID" --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
GATE_ID=$(./tracker gate list --run "$RUN_ID" --json | jq -r '.items[0].gate_id')
```

If a gate is opened for `agent:reviewer-1`, that reviewer actor must approve or reject it:

```bash
./tracker gate approve "$GATE_ID" --actor agent:reviewer-1 --reason "reviewed evidence"
```

## 6. Finish The Run

```bash
./tracker run complete "$RUN_ID" --summary "Implementation and review complete" --actor agent:builder-1 --reason "done"
./tracker ticket complete APP-1 --actor human:owner --reason "done"
```

The run, evidence, handoff, and gate history stay inspectable through the CLI, TUI, shell, JSON output, and safe MCP read surfaces.
