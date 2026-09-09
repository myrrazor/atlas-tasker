# First Agent Workflow

Atlas treats an agent run as a durable work record, not just a chat transcript. A typical flow is ticket, dispatch, checkpoint, evidence, handoff, review.

## 1. Create Work

```bash
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Add health check" --type task --actor human:owner --reason "agent workflow demo"
./tracker ticket move APP-1 ready --actor human:owner --reason "ready for implementation"
```

## 2. Install Agent Guidance

Choose the agent that will work in this repository:

```bash
./tracker integrations install codex
```

The other targets are `claude`, `cursor`, `openclaw`, `grok`, and `generic`. This writes project
instructions and an agent-specific skill pack; it does not register MCP. See
[coding-agent integrations](guides/agent-integrations.md) for the exact files and multi-target setup.

## 3. Register An Agent

```bash
./tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner --reason "register builder"
```

The agent profile records routing metadata only. It does not give Atlas control over Codex, Claude Code, or another provider.

## 4. Dispatch The Ticket

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

## 5. Record Progress And Evidence

```bash
./tracker run start "$RUN_ID" --summary "Implementation started" --actor agent:builder-1 --reason "begin work"
./tracker ticket move APP-1 in_progress --actor agent:builder-1 --reason "implementation started"
./tracker run checkpoint "$RUN_ID" --title "First pass" --body "Health check route added locally." --actor agent:builder-1 --reason "status update"
./tracker run evidence add "$RUN_ID" --type note --title "Test proof" --body "go test ./... passed" --actor agent:builder-1 --reason "attach test proof"
```

`run start` activates the run record; the ticket itself still moves through workflow states with `ticket move`.

Evidence can also copy a file into the run evidence bundle with `--artifact <PATH>`.

## 6. Hand Off For Review

```bash
./tracker run handoff "$RUN_ID" --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
GATE_ID=$(./tracker gate list --run "$RUN_ID" --json | jq -r '.items[0].gate_id')
```

The active run actor or ticket assignee may generate this handoff packet and open the review handoff gate. That does not approve or complete the work; it only creates the review packet and gate for the next actor.

If a gate is opened for `agent:reviewer-1`, that reviewer actor must approve or reject it:

```bash
./tracker gate approve "$GATE_ID" --actor agent:reviewer-1 --reason "reviewed evidence"
```

## 7. Finish The Run

```bash
./tracker run complete "$RUN_ID" --summary "Implementation and review complete" --actor agent:builder-1 --reason "done"
./tracker ticket request-review APP-1 --actor agent:builder-1 --reason "ready for final review"
./tracker ticket approve APP-1 --actor human:owner --reason "reviewed"
./tracker ticket view APP-1 --json
# The fresh workspace in this tutorial uses open mode, so approval leaves the ticket in_review.
./tracker ticket complete APP-1 --actor human:owner --reason "done"
```

Completing a run does not complete the ticket. The ticket has to pass through `in_review` first, and with no reviewer configured, approval falls to the assignee, active worker, or `human:owner`.
In `review_gate` mode, `ticket approve` also completes the ticket; inspect the returned status and do
not issue a second `ticket complete`. The final command above is required because this tutorial uses
the default `open` mode.

The run, evidence, handoff, and gate history stay inspectable through the CLI, TUI, shell, JSON output, and safe MCP read surfaces.
