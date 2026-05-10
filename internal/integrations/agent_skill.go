package integrations

import (
	"fmt"
	"strings"
)

func genericBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Generic Agent)

- Start with `+"`tracker agent available <agent-id> --json`"+` and `+"`tracker agent pending <agent-id> --json`"+`.
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
4. Record evidence, then request review.
5. Check `+"`tracker agent pending <agent-id> --json`"+` when blocked.

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

## Start

1. Resolve your actor, usually `+"`agent:<agent-id>`"+`.
2. Run `+"`tracker agent available <agent-id> --json`"+`.
3. If nothing is available, run `+"`tracker agent pending <agent-id> --json`"+` and report the blocker reason codes.
4. Before editing, claim the ticket and move it to `+"`in_progress`"+` if it is still ready.

## Work

- Use `+"`tracker inspect <ID> --actor agent:<agent-id> --json`"+` before making workflow decisions.
- Do not work a ticket with `+"`dependency_blocked`"+`, `+"`policy_blocked`"+`, `+"`claimed_by_other`"+`, or `+"`waiting_for_review`"+`.
- Record durable context with comments, checkpoints, evidence, or handoffs.
- Request review instead of marking protected work done yourself.

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

Only `+"`done`"+` unblocks dependencies. `+"`canceled`"+` does not.

## Worker Loop

1. Read available work.
2. Claim the ticket.
3. Move it to `+"`in_progress`"+`.
4. Implement narrowly.
5. Attach evidence with `+"`tracker run evidence add`"+` or ticket comments.
6. Request review with `+"`tracker ticket request-review <ID> --actor agent:<agent-id> --reason \"ready for review\"`"+`.

## Reviewer Loop

1. Read available work as the reviewer.
2. Inspect the ticket, history, run evidence, and handoff.
3. Approve or reject with explicit reason.
4. Do not approve your own implementation unless the workspace has a documented owner override path.

## Blocker Loop

When no tickets are available, inspect pending items and wait for the next Atlas wake-up or external scheduler tick. Do not create polling loops inside Atlas unless owner-enabled auto mode exists in the workspace.
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

Do not approve a ticket you implemented yourself. If the workspace blocks self-approval, follow the owner override or third-reviewer path.
`) + "\n"
}
