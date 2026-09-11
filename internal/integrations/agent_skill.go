package integrations

import (
	"fmt"
	"strings"
)

func openclawBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (OpenClaw)

- Read work with `+"`tracker agent available <agent-id> --json`"+` and `+"`tracker agent pending <agent-id> --json`"+`.
- Every write takes `+"`--actor`"+` and `+"`--reason`"+`; `+"`tracker project create`"+` takes neither.
- Claim before editing: `+"`tracker ticket claim <ID> --actor agent:<agent-id> --reason \"start work\"`"+`.
- Exit 4 on a status change means the transition is forbidden, not that the command broke. Read `+"`tracker inspect <ID> --json`"+` before retrying.
- Moving a ticket to its current status is a successful no-op; inspect the ticket before retrying a different transition.
- The `+"`atlas-worker`"+` skill installs under `+"`.agents/skills/`"+`; confirm it loaded with `+"`openclaw skills list`"+`.
- The browser board (`+"`tracker web serve`"+`) is for humans. Agents use the CLI or `+"`tracker mcp serve`"+`.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func openclawGuide(skillDir string) string {
	return strings.TrimSpace(fmt.Sprintf(`# Atlas Tasker OpenClaw Guide

OpenClaw reads `+"`AGENTS.md`"+` into every session and loads skills from four roots. Atlas installs into the repo-local one.

## Where things land

- `+"`AGENTS.md`"+` gets an Atlas block between `+"`atlas-tasker:openclaw`"+` markers. Codex writes its own block with different markers, so both can live in the same file.
- The `+"`atlas-worker`"+` skill goes to `+"`%s`"+`, which OpenClaw picks up as a project-agent skill for this repo only.
- The skill is gated on the `+"`tracker`"+` binary, so it stays out of the prompt in workspaces that do not have Atlas installed.

## Confirm it loaded

~~~bash
openclaw skills list
openclaw skills check
~~~

`+"`check`"+` is the one that explains a skill that is present but not ready — usually a missing binary or a name collision with a higher-precedence root.

## Sharing it across agents

Repo-local is the right default: the skill travels with the repo and only applies where Atlas is. To explicitly copy it into OpenClaw's shared skill root for every agent on the machine, run:

~~~bash
tracker integrations install openclaw --global
~~~

The default install stays inside the repository. The explicit `+"`--global`"+` command also writes the managed skill files to `+"`~/.openclaw/skills/atlas-worker/`"+`.

## The loop

1. `+"`tracker agent available <agent-id> --json`"+`
2. `+"`tracker ticket claim <ID> --actor agent:<agent-id> --reason \"start work\"`"+`
3. `+"`tracker ticket move <ID> in_progress --actor agent:<agent-id> --reason \"start work\"`"+`
4. `+"`tracker ticket comment <ID> --body \"what changed\" --actor agent:<agent-id> --reason \"progress note\"`"+`
5. `+"`tracker ticket request-review <ID> --actor agent:<agent-id> --reason \"ready for review\"`"+`

Read `+"`references/workflow.md`"+` inside the skill for blocker codes, reviewer behavior, and wake-ups.
`, skillDir)) + "\n"
}

func genericBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Generic Agent)

- Start with `+"`tracker agent available <agent-id> --json`"+` and `+"`tracker agent pending <agent-id> --json`"+`.
- Agents may self-dispatch eligible assigned work with `+"`tracker run dispatch <ticket-id> --agent agent:<agent-id> --actor agent:<agent-id> --reason \"start run\"`"+`.
- Claim before editing and request review when done.
- An available entry with action `+"`promote`"+` is a backlog ticket whose blockers are all `+"`done`"+`; run its `+"`ticket move <ID> ready`"+` before claiming.
- Treat `+"`dependency_blocked`"+` as a stop sign until the blocker reaches `+"`done`"+`.
- Moving a ticket to its current status is a successful no-op; inspect the ticket before retrying a different transition.
- Use explicit `+"`--actor`"+` and `+"`--reason`"+` flags for every write.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func genericGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Generic Agent Guide

Use Atlas Tasker as the durable workflow layer. The shortest safe loop is:

1. `+"`tracker agent available <agent-id> --json`"+`
2. `+"`tracker ticket claim <ID> --actor agent:<agent-id> --reason \"start work\"`"+`
3. `+"`tracker ticket move <ID> in_progress --actor agent:<agent-id> --reason \"start work\"`"+`
4. If a run is needed, dispatch yourself with `+"`tracker run dispatch <ID> --agent agent:<agent-id> --actor agent:<agent-id> --reason \"start run\"`"+`.
5. Record evidence, then request review.
6. Check `+"`tracker agent pending <agent-id> --json`"+` when blocked.

Atlas does not poll or launch agents unless an owner enables agent auto mode.

Moving a ticket to its current status is a successful no-op across CLI, MCP, bulk, and web paths.
`) + "\n"
}

// runtime.go has its own providerLabel for launch text; this one reads inside a
// sentence, so "generic" has to come out as something you can say out loud
func skillProviderLabel(provider string) string {
	switch provider {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude Code"
	case "openclaw":
		return "OpenClaw"
	case "cursor":
		return "Cursor"
	case "grok":
		return "Grok"
	default:
		return "generic agent"
	}
}

func cursorBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Cursor)

- Pull actionable work with `+"`tracker agent available <agent-id> --json`"+`.
- Explain blockers with `+"`tracker agent pending <agent-id> --json`"+`.
- Set `+"`TRACKER_ACTOR`"+` to your real Atlas identity, then use `+"`tracker ticket claim <ID> --actor \"$TRACKER_ACTOR\" --reason \"start work\"`"+`.
- Use explicit review commands: `+"`request-review`"+`, `+"`approve`"+`, `+"`complete`"+`.
- Moving a ticket to its current status is a successful no-op; inspect the ticket before retrying a different transition.
- Prefer `+"`tracker mcp serve --tool-profile workflow --workspace <path>`"+` when the session is MCP-first.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func cursorGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Cursor Guide

Cursor loads root `+"`AGENTS.md`"+`. Atlas installs a managed block there and a repo-local skill under `+"`.cursor/skills/atlas-worker/`"+`.

## Recommended loop

1. `+"`tracker agent available builder-1 --json`"+`
2. `+"`tracker ticket claim <ID> --actor agent:builder-1 --reason \"start work\"`"+`
3. `+"`tracker ticket move <ID> in_progress --actor agent:builder-1 --reason \"start work\"`"+`
4. `+"`tracker ticket request-review <ID> --actor agent:builder-1 --reason \"ready for review\"`"+`
5. In open/solo completion mode, `+"`tracker ticket complete <ID> --actor agent:builder-1 --reason \"done\"`"+` can finish from `+"`in_progress`"+` without the three-step review path.

## Notes

- The managed block in `+"`AGENTS.md`"+` uses `+"`atlas-tasker:cursor`"+` markers so Codex/OpenClaw/Grok blocks can coexist.
- Keep custom house rules outside the managed markers.
- Moving a ticket to its current status is a successful no-op across CLI, MCP, bulk, and web paths.
`) + "\n"
}

func grokBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Grok)

- Start with `+"`tracker agent available <agent-id> --json`"+` and `+"`tracker agent pending <agent-id> --json`"+`.
- Claim before editing and request review when done.
- Pass `+"`--actor`"+` and `+"`--reason`"+` on every write.
- Moving a ticket to its current status is a successful no-op; inspect the ticket before retrying a different transition.
- Prefer JSON reads; treat exit 4 as a forbidden workflow edge, not a crash.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func grokGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Grok Guide

Grok-style agents that load root `+"`AGENTS.md`"+` get the managed Atlas block from `+"`tracker integrations install grok`"+`.

## Recommended loop

1. `+"`tracker agent available <agent-id> --json`"+`
2. `+"`tracker ticket claim <ID> --actor agent:<agent-id> --reason \"start work\"`"+`
3. `+"`tracker ticket move <ID> in_progress --actor agent:<agent-id> --reason \"start work\"`"+`
4. Record progress with comments or run evidence.
5. `+"`tracker ticket request-review <ID> --actor agent:<agent-id> --reason \"ready for review\"`"+`

## Notes

- The managed block uses `+"`atlas-tasker:grok`"+` markers so other install targets do not overwrite it.
- Moving a ticket to its current status is a successful no-op across CLI, MCP, bulk, and web paths.
`) + "\n"
}

func atlasWorkerSkill(provider string) string {
	// keep the description free of ": " -- a plain YAML scalar cannot hold one and
	// a skill whose frontmatter will not parse never loads
	label := skillProviderLabel(provider)
	frontmatter := fmt.Sprintf(`---
name: atlas-worker
description: Use inside an Atlas Tasker workspace -- "what should I work on", "pick up the next ticket", "claim APP-12", "why is this blocked", "ready for review", "hand this off", "current status", "show the board". Drives the tracker from %s sessions; finds available work, claims tickets, respects dependency and policy blockers, records evidence, and requests review.`, label)
	if provider == "openclaw" {
		// keeps the skill out of the prompt in workspaces with no tracker binary
		frontmatter += "\nmetadata: { \"openclaw\": { \"requires\": { \"bins\": [\"tracker\"] } } }"
	}
	return strings.TrimSpace(fmt.Sprintf(`%s
---

# Atlas Worker

You are driving Atlas Tasker from %s. Atlas Tasker is the source of truth for ticket state. Prefer MCP tools when this session has them. Prefer JSON reads. Mutate only with an explicit actor and reason. Never bypass dependency or governance blockers.

Moving a ticket to its current status is a successful no-op. Inspect the ticket before choosing a different transition.

<!-- atlas-managed-lifecycle -->
%s
<!-- /atlas-managed-lifecycle -->

## More Detail

Read `+"`references/workflow.md`"+` when you need the full loop, blocker handling, reviewer behavior, or handoff patterns.
`, frontmatter, label, atlasManagedLifecycle())) + "\n"
}

func atlasManagedLifecycle() string {
	return strings.TrimSpace(`
## Bootstrap

If the workspace has no agent profiles yet (` + "`tracker agent list --json`" + ` is empty), set the team up first or ask the human to:

1. ` + "`tracker team list`" + ` shows the ready-made rosters (solo, pair, swarm, crossfire).
2. ` + "`tracker team apply pair --actor human:owner --reason \"team setup\"`" + ` creates builder + reviewer profiles, the standard-build runbook, separation-of-duties permissions, and the review gate in one shot. Use ` + "`--dry-run`" + ` to preview.
3. Your agent id should match one of the roster ids (builder-1, reviewer-1, ...).

## MCP first

- Prefer Atlas MCP tools when this session has them. Call ` + "`atlas.context`" + ` when starting material work. Call ` + "`atlas.status`" + ` before every status report. Use ` + "`atlas.board`" + ` or ` + "`atlas.status`" + ` with a named project to show a board.
- If MCP is unavailable (guidance mode, the server is not registered, or a tool call failed), fall back to the CLI commands in this skill. Do not invent Atlas state from conversational memory. Report a failed read instead of guessing.
- Do not invoke high-impact Atlas operations. Do not tell the user to run low-level tracker internals as a required step.

## Capture

Classify the user request before creating or attaching a ticket.

- Status queries, read-only explanations, casual discussion, and simple questions never create tickets (` + "`no_ticket`" + `).
- Material repository changes attach to an existing ticket or create one only when policy is ` + "`material_work`" + ` (` + "`attach_or_create`" + `). Search first with ` + "`atlas.search`" + ` or ` + "`tracker search`" + `. Use the ticket the user named. Avoid duplicate work.
- Capture policy ` + "`ask`" + ` means ask before creating; a ticket the user named may still be used.
- Capture policy ` + "`never`" + `, mode ` + "`disabled`" + `, or an explicit user request not to track (` + "`tracking_excluded`" + `) disables tracking for that task.
- Read declared and effective managed mode from ` + "`atlas.context`" + `. Completion always follows the workspace policy (` + "`follow_workspace`" + `). Managed mode cannot relax review, approval, or dependencies.

## Start

1. Resolve your configured Atlas actor, usually ` + "`agent:<agent-id>`" + `. A missing actor is a setup error -- stop and say so.
2. Call ` + "`atlas.context`" + ` (or ` + "`tracker agent available <agent-id> --json`" + `) for assigned work, available work, and pending blockers.
3. If nothing is available, call ` + "`atlas.agent.pending`" + ` or ` + "`tracker agent pending <agent-id> --json`" + ` and report the stable blocker reason codes.
4. If you were launched by a wake-up, acknowledge it: ` + "`tracker agent wakeups list <agent-id> --json`" + `, then ` + "`tracker agent wakeups ack <WAKEUP-ID> --actor agent:<agent-id> --reason \"picked up\"`" + `.
5. Claim before substantial edits. Move to ` + "`in_progress`" + ` only through a legal workflow edge. ` + "`backlog -> in_progress`" + ` is forbidden; an entry with action ` + "`promote`" + ` is still in ` + "`backlog`" + ` with every blocker ` + "`done`" + `, and its first suggested command moves it to ` + "`ready`" + `.
6. Do not start work that is ` + "`dependency_blocked`" + `, ` + "`policy_blocked`" + `, ` + "`claimed_by_other`" + `, or ` + "`waiting_for_review`" + `.
7. When a run is needed, dispatch yourself with ` + "`tracker run dispatch <ID> --agent agent:<agent-id> --actor agent:<agent-id> --reason \"start run\"`" + `.

## Work

- Use ` + "`atlas.ticket.inspect`" + ` or ` + "`tracker inspect <ID> --actor agent:<agent-id> --json`" + ` before making workflow decisions.
- Record milestone progress, not every command. Default progress policy is ` + "`milestones`" + ` (tests passing, review requested, a deliverable). Attach durable evidence for tests and reviews (comments, checkpoints, evidence, or handoffs).
- Request review or complete according to the workspace completion mode. In ` + "`review_gate`" + ` or ` + "`dual_gate`" + `, approval is not self-service when a reviewer or separation-of-duties rule applies.
- Self-approval is allowed by default only when no reviewer or governance separation rule is configured; respect stricter workspace policies when they exist.

## Status and completion

- Query Atlas before every status report. Never answer "what is the status" from memory.
- Reconcile Atlas state before final completion messaging. The board and ticket view must already show the new status.
`)
}

func atlasWorkerReference() string {
	return strings.TrimSpace(`# Atlas Worker Reference

## Available Work

`+"`tracker agent available <agent-id> --json`"+` returns tickets the agent can act on now. Entries include an action such as `+"`start`"+`, `+"`continue`"+`, `+"`review`"+`, or `+"`promote`"+` plus suggested commands. `+"`promote`"+` means every blocker is `+"`done`"+` but the ticket is still in `+"`backlog`"+`; the first suggested command is the `+"`ticket move <ID> ready`"+`, and `+"`backlog -> in_progress`"+` is not a legal edge, so run it first.

## Pending Work

`+"`tracker agent pending <agent-id> --json`"+` returns tickets that are relevant but blocked. Stable reason codes include:

- `+"`dependency_blocked`"+`
- `+"`waiting_for_review`"+`
- `+"`waiting_for_owner`"+`
- `+"`not_ready_status`"+`
- `+"`claimed_by_other`"+`
- `+"`policy_blocked`"+`
- `+"`agent_at_capacity`"+`
- `+"`missing_capability`"+`

`+"`not_ready_status`"+` means the ticket is not in a state you can act on: usually backlog that never had blockers, or someone else's `+"`in_progress`"+` work. A backlog ticket whose blockers all landed is not pending; it is listed under available as `+"`promote`"+`.

Only `+"`done`"+` unblocks dependencies. `+"`canceled`"+` does not. `+"`--override-deps`"+` is for `+"`human:owner`"+` only and must include a reason.

Moving a ticket to its current status is a successful no-op across CLI, MCP, bulk, and web paths.

## Worker Loop

Prefer MCP (`+"`atlas.context`"+`, `+"`atlas.status`"+`, claim/move/comment tools) when this session has Atlas MCP. Fall back to the CLI commands below when MCP is unavailable.

1. Call `+"`atlas.context`"+` or read available work.
2. Search before creating a ticket. Use the ticket the user named. Status and read-only questions never create tickets.
3. Claim the ticket before substantial edits.
4. Move it to `+"`in_progress`"+` only through a legal edge.
5. Dispatch yourself if the workflow needs a run snapshot: `+"`tracker run dispatch <ID> --agent agent:<agent-id> --actor agent:<agent-id> --reason \"start run\"`"+`.
6. Implement narrowly. Record milestone progress, not every command.
7. Attach durable evidence with `+"`tracker run evidence add`"+` or ticket comments.
8. Request review with `+"`tracker ticket request-review <ID> --actor agent:<agent-id> --reason \"ready for review\"`"+` according to workspace completion policy.
9. Query Atlas before every status report. Reconcile Atlas state before saying work is complete.

## Reviewer Loop

1. Read available work as the reviewer.
2. Inspect the ticket, history, run evidence, and handoff.
3. Approve or reject with explicit reason.
4. Approve your own implementation only when no reviewer or separation-of-duties policy applies.

## Blocker Loop

When no tickets are available, inspect pending items and wait for the next Atlas wake-up or external scheduler tick. Do not create polling loops inside Atlas unless owner-enabled auto mode exists in the workspace.

## Wake-ups

When a ticket you are assigned to becomes unblocked (its last `+"`blocked_by`"+` dependency reaches `+"`done`"+`), Atlas moves it from `+"`backlog`"+` to `+"`ready`"+` for you (audited as `+"`agent:atlas`"+`), emits an `+"`agent.work_available`"+` event, and records a wake-up. If the owner enabled auto mode (`+"`tracker agent auto set <agent-id> --mode command ...`"+`), your session may have been launched by that wake-up with the ticket id substituted into the command.

1. `+"`tracker agent wakeups list <agent-id> --json`"+` shows pending wake-ups.
2. Acknowledge before working: `+"`tracker agent wakeups ack <WAKEUP-ID> --actor agent:<agent-id> --reason \"picked up\"`"+`.
3. Then run the normal worker loop against the wake-up's ticket. If the wake-up's `+"`metadata.promoted`"+` is `+"`\"false\"`"+`, the automatic move did not go through: when `+"`tracker agent available`"+` still lists the ticket as `+"`promote`"+`, run that entry's first suggested command; when the wake-up itself is `+"`failed`"+` and its error names `+"`tracker doctor --repair`"+`, stop and let a human run that first.

## Team Presets

A fresh workspace becomes a working team with one command -- `+"`tracker team apply <preset> --actor human:owner --reason \"team setup\"`"+`:

- `+"`solo`"+`: one builder, open completion.
- `+"`pair`"+`: builder + reviewer, review gate, builders cannot approve their own work.
- `+"`swarm`"+`: three builders by routing weight, QA, owner delegate.
- `+"`crossfire`"+`: Codex builds, Claude reviews (flip with `+"`--provider claude`"+`).

`+"`tracker team show <preset>`"+` previews the roster; `+"`--dry-run`"+` on apply previews without creating.
`) + "\n"
}

func atlasWorkerOpenAIYAML() string {
	return strings.TrimSpace(`display_name: Atlas Worker
short_description: Work Atlas Tasker tickets safely.
default_prompt: Check my available Atlas work, pick the highest-priority actionable ticket, and follow the Atlas Worker loop.
`) + "\n"
}

func atlasNextCommandTemplate() string {
	return strings.TrimSpace(`# Atlas Next

Find work this agent can do now.

~~~bash
tracker agent available <agent-id> --json
tracker agent pending <agent-id> --json
~~~

Use the first command for actionable work. Use the second only to explain why you are waiting.
`) + "\n"
}

func atlasTakeCommandTemplate() string {
	return strings.TrimSpace(`# Atlas Take

Assign yourself as the active worker only when the user or policy allows it.

~~~bash
tracker ticket claim <ticket-id> --actor agent:<agent-id> --reason "start work"
tracker ticket move <ticket-id> in_progress --actor agent:<agent-id> --reason "start work"
tracker run dispatch <ticket-id> --agent agent:<agent-id> --actor agent:<agent-id> --reason "start run"
~~~

If tracker agent available did not list the ticket, inspect it before changing anything.
`) + "\n"
}

func atlasReviewCommandTemplate() string {
	return strings.TrimSpace(`# Atlas Review

Use this when the user asks this agent to review work.

~~~bash
tracker ticket request-review <ticket-id> --reviewer agent:<agent-id> --actor agent:<worker-id> --reason "ready for review"
tracker agent available <agent-id> --json
tracker ticket approve <ticket-id> --actor agent:<agent-id> --reason "review passed"
~~~

Self-approval is allowed for autonomous tickets. If the workspace requires separation-of-duties, follow the owner override or third-reviewer path.
`) + "\n"
}
