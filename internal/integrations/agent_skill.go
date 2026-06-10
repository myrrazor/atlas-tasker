package integrations

import (
	"fmt"
	"strings"
)

func genericBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Generic Agent)

- Start with `+"`tracker agent available <agent-id> --json`"+` and `+"`tracker agent pending <agent-id> --json`"+`.
- Agents may self-dispatch eligible assigned work with `+"`tracker run dispatch <ticket-id> --agent agent:<agent-id> --actor agent:<agent-id>`"+`.
- Claim before editing and request review when done.
- Treat `+"`dependency_blocked`"+` as a stop sign until the blocker reaches `+"`done`"+`.
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
`) + "\n"
}

func atlasWorkerSkill(provider string) string {
	return strings.TrimSpace(fmt.Sprintf(`---
name: atlas-worker
description: Use when working inside an Atlas Tasker workspace as a %s coding agent: find available tickets, claim work, respect blockers, request review, and record durable evidence.
---

# Atlas Worker

Atlas Tasker is the source of truth for ticket state. Prefer JSON reads, mutate only with explicit actor and reason, and never bypass dependency or governance blockers.

## Bootstrap

If the workspace has no agent profiles yet (`+"`tracker agent list --json`"+` is empty), set the team up first or ask the human to:

1. `+"`tracker team list`"+` shows the ready-made rosters (solo, pair, swarm, crossfire).
2. `+"`tracker team apply pair --actor human:owner --reason \"team setup\"`"+` creates builder + reviewer profiles, the standard-build runbook, separation-of-duties permissions, and the review gate in one shot. Use `+"`--dry-run`"+` to preview.
3. Your agent id should match one of the roster ids (builder-1, reviewer-1, ...).

## Start

1. Resolve your actor, usually `+"`agent:<agent-id>`"+`.
2. Run `+"`tracker agent available <agent-id> --json`"+`.
3. If nothing is available, run `+"`tracker agent pending <agent-id> --json`"+` and report the blocker reason codes.
4. If you were launched by a wake-up, acknowledge it: `+"`tracker agent wakeups list <agent-id> --json`"+`, then `+"`tracker agent wakeups ack <WAKEUP-ID> --actor agent:<agent-id> --reason \"picked up\"`"+`.
5. Before editing, claim the ticket and move it to `+"`in_progress`"+` if it is still ready.
6. When a run is needed, dispatch yourself with `+"`tracker run dispatch <ID> --agent agent:<agent-id> --actor agent:<agent-id> --reason \"start run\"`"+`.

## Work

- Use `+"`tracker inspect <ID> --actor agent:<agent-id> --json`"+` before making workflow decisions.
- Do not work a ticket with `+"`dependency_blocked`"+`, `+"`policy_blocked`"+`, `+"`claimed_by_other`"+`, or `+"`waiting_for_review`"+`.
- Record durable context with comments, checkpoints, evidence, or handoffs.
- Self-approval is allowed by default when no reviewer or governance separation rule is configured; respect stricter workspace policies when they exist.

## More Detail

Read `+"`references/workflow.md`"+` when you need the full loop, blocker handling, reviewer behavior, or handoff patterns.
`, provider)) + "\n"
}

func atlasWorkerReference() string {
	return strings.TrimSpace(`# Atlas Worker Reference

## Available Work

`+"`tracker agent available <agent-id> --json`"+` returns tickets the agent can act on now. Entries include an action such as `+"`start`"+`, `+"`continue`"+`, or `+"`review`"+` plus suggested commands.

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

Only `+"`done`"+` unblocks dependencies. `+"`canceled`"+` does not. `+"`--override-deps`"+` is for `+"`human:owner`"+` only and must include a reason.

## Worker Loop

1. Read available work.
2. Claim the ticket.
3. Move it to `+"`in_progress`"+`.
4. Dispatch yourself if the workflow needs a run snapshot: `+"`tracker run dispatch <ID> --agent agent:<agent-id> --actor agent:<agent-id> --reason \"start run\"`"+`.
5. Implement narrowly.
6. Attach evidence with `+"`tracker run evidence add`"+` or ticket comments.
7. Request review with `+"`tracker ticket request-review <ID> --actor agent:<agent-id> --reason \"ready for review\"`"+`.

## Reviewer Loop

1. Read available work as the reviewer.
2. Inspect the ticket, history, run evidence, and handoff.
3. Approve or reject with explicit reason.
4. Approve your own implementation only when no reviewer or separation-of-duties policy applies.

## Blocker Loop

When no tickets are available, inspect pending items and wait for the next Atlas wake-up or external scheduler tick. Do not create polling loops inside Atlas unless owner-enabled auto mode exists in the workspace.

## Wake-ups

When a ticket you are assigned to becomes unblocked (its last `+"`blocked_by`"+` dependency reaches `+"`done`"+`), Atlas emits an `+"`agent.work_available`"+` event and records a wake-up. If the owner enabled auto mode (`+"`tracker agent auto set <agent-id> --mode command ...`"+`), your session may have been launched by that wake-up with the ticket id substituted into the command.

1. `+"`tracker agent wakeups list <agent-id> --json`"+` shows pending wake-ups.
2. Acknowledge before working: `+"`tracker agent wakeups ack <WAKEUP-ID> --actor agent:<agent-id> --reason \"picked up\"`"+`.
3. Then run the normal worker loop against the wake-up's ticket.

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
