# First Agent Workflow

Everyday start is install, `tracker init`, restart the client, then ask for
status or create a ticket. Dispatch, evidence bundles, and review gates are a
later, advanced loop — not the first command you run.

The snippets below assume `tracker` is on your `PATH` (release installer or
`go install`). **Source-build walkthrough:** if you just ran
`go build -o tracker ./cmd/tracker` in this checkout, prefix the same commands
with `./tracker` until that binary is on `PATH`.

## 1. Initialize, Then Ask

```bash
mkdir app && cd app
tracker init
```

`init` creates the workspace, a default project (directory `app` → `APP`),
local checkpoints, and Atlas-managed MCP/skill files for coding agents it finds.
It does not open the six-target picker. Restart the client. Grok also needs you
to trust this project in its own UI before local skills appear.

Then ask the agent:

```text
What's the current status of this project?
```

Or create the first ticket yourself:

```bash
tracker ticket create --project APP --title "Add health check" --type task --actor human:owner --reason "agent workflow demo"
tracker ticket move APP-1 ready --actor human:owner --reason "ready for implementation"
tracker board
```

Atlas does not spawn Claude, Codex, Cursor, OpenClaw, or Grok. A written MCP
entry is `pending_client_restart` until the client reconnects. See
[coding-agent integrations](guides/agent-integrations.md) and
[compatibility](v1.16-client-compatibility.md).

## 2. Install Extra Guidance (optional)

Init already wrote detected targets. To add one later, or to open the picker:

```bash
tracker integrations install codex
# picker (TTY only): tracker integrations install
# after init: tracker init --integrations
```

The other targets are `claude`, `cursor`, `openclaw`, `grok`, and `generic`.
This writes project instructions and an agent-specific skill pack; it does not
by itself prove the client listed the skill or connected MCP.

## 3. Register An Agent Profile

```bash
tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner --reason "register builder"
tracker ticket assign APP-1 agent:builder-1 --actor human:owner --reason "agent work"
```

The profile records routing metadata only. It does not give Atlas control over
the provider.

From here the agent can claim and move with CLI or MCP (`atlas.ticket.claim`,
then `atlas.ticket.move`). That is the everyday loop in [AGENTS.md](../AGENTS.md).

## 4. Advanced: Dispatch A Tracked Run

Dispatch is optional. It requires a clean git workspace because Atlas may
create a managed worktree. Skip this section if you only needed status and a
ticket.

**Source-build note:** if `tracker` is a local binary in the repo root, exclude
it before the cleanliness check:

```bash
printf '\ntracker\n.tracker/write.lock\n.tracker/index.sqlite\n.tracker/index.sqlite-wal\n.tracker/index.sqlite-shm\n' >> .git/info/exclude
git add -A
git commit -m "track atlas tutorial state"
git status --short
```

`git status --short` should print nothing.

```bash
RUN_ID=$(tracker run dispatch APP-1 --agent builder-1 --actor human:owner --reason "start implementation" --json | jq -r '.payload.run_id')
tracker run launch "$RUN_ID" --actor human:owner --reason "prepare launch files"
tracker run open "$RUN_ID" --json
```

These examples use `jq`; if it is not installed, copy `payload.run_id` from the
JSON output and export it as `RUN_ID`.

## 5. Record Progress And Evidence

```bash
tracker run start "$RUN_ID" --summary "Implementation started" --actor agent:builder-1 --reason "begin work"
tracker ticket move APP-1 in_progress --actor agent:builder-1 --reason "implementation started"
tracker run checkpoint "$RUN_ID" --title "First pass" --body "Health check route added locally." --actor agent:builder-1 --reason "status update"
tracker run evidence add "$RUN_ID" --type note --title "Test proof" --body "go test ./... passed" --actor agent:builder-1 --reason "attach test proof"
```

`run start` activates the run record; the ticket itself still moves through
workflow states with `ticket move`.

Evidence can also copy a file into the run evidence bundle with `--artifact <PATH>`.

## 6. Hand Off For Review

```bash
tracker run handoff "$RUN_ID" --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
GATE_ID=$(tracker gate list --run "$RUN_ID" --json | jq -r '.items[0].gate_id')
```

The active run actor or ticket assignee may generate this handoff packet and
open the review handoff gate. That does not approve or complete the work.

If a gate is opened for `agent:reviewer-1`, that reviewer actor must approve or
reject it:

```bash
tracker gate approve "$GATE_ID" --actor agent:reviewer-1 --reason "reviewed evidence"
```

## 7. Finish The Run

```bash
tracker run complete "$RUN_ID" --summary "Implementation and review complete" --actor agent:builder-1 --reason "done"
tracker ticket request-review APP-1 --actor agent:builder-1 --reason "ready for final review"
tracker ticket approve APP-1 --actor human:owner --reason "reviewed"
tracker ticket view APP-1 --json
# The fresh workspace in this tutorial uses open mode, so approval leaves the ticket in_review.
tracker ticket complete APP-1 --actor human:owner --reason "done"
```

Completing a run does not complete the ticket. The ticket has to pass through
`in_review` first, and with no reviewer configured, approval falls to the
assignee, active worker, or `human:owner`.
In `review_gate` mode, `ticket approve` also completes the ticket; inspect the
returned status and do not issue a second `ticket complete`. The final command
above is required because this tutorial uses the default `open` mode.

The run, evidence, handoff, and gate history stay inspectable through the CLI,
TUI, shell, JSON output, and safe MCP read surfaces.
