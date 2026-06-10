# Team Presets

`tracker team` turns a fresh workspace into a working agent team in one command. A preset bundles everything the workflow needs: agent profiles with roles and routing weights, the `standard-build` runbook (implement → review), a separation-of-duties permission profile, and the completion mode that makes the gates real.

## The presets

| Preset | Roster | Completion | Notes |
|---|---|---|---|
| `solo` | builder-1 | `open` | One agent works the board and finishes its own tickets |
| `pair` | builder-1 + reviewer-1 | `review_gate` | Builders are denied `gate_approve` and `ticket_complete` — review is structural, not optional |
| `swarm` | builder-1..3 + qa-1 + owner-delegate-1 | `review_gate` | Builders pull by routing weight with one active run each; QA holds a reviewer role |
| `crossfire` | builder-1 (codex) + reviewer-1 (claude) | `review_gate` | Cross-vendor review: two different models keep each other honest. `--provider claude` flips who builds |

## Apply one

```bash
tracker team list
tracker team show pair
tracker team apply pair --dry-run --actor human:owner --reason "preview"
tracker team apply pair --actor human:owner --reason "team setup"
```

Re-running `apply` is safe: existing agents, runbooks, and profiles are skipped, never overwritten. `--provider claude|codex|mixed` picks the vendor for the roster (presets default to claude; `crossfire` is mixed by definition).

## After applying

1. Install the skill for your agent runtime: `tracker integrations install claude` (or `codex`, or `generic`). The skill teaches agents to bootstrap, claim, build, attach evidence, request review, and acknowledge wake-ups without hand-holding.
2. File tickets and assign them: `tracker ticket assign APP-1 agent:builder-1 --actor human:owner --reason "agent work"`.
3. Wire dependencies with `tracker ticket link` — when a blocker lands, Atlas wakes the assigned agent (`agent.work_available`), and with `tracker agent auto set` it can launch your agent command automatically with the ticket id substituted in.

From there the loop runs itself: builders claim and implement, the review gate routes work to the reviewer, handoffs carry context across sessions, and the owner only shows up for the decisions that genuinely need a human.
