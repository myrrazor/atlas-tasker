# MCP Tools

Run this to inspect the live table:

```bash
tracker mcp tools --json --tool-profile admin --dangerously-allow-high-impact-tools
```

Columns:

- `Default` means enabled in `--tool-profile read`.
- `Actor` and `Reason` mean required MCP arguments.
- `Approval` means a one-time operation approval is required.
- `Live side effect` means provider, sync, worktree, archive, import, compact, or other non-read side effect.

| Tool | Class | Default | Actor | Reason | Approval | Live side effect |
|---|---:|---:|---:|---:|---:|---:|
| `atlas.queue` | read | yes | no | no | no | no |
| `atlas.next` | read | yes | no | no | no | no |
| `atlas.agent.available` | read | yes | no | no | no | no |
| `atlas.agent.pending` | read | yes | no | no | no | no |
| `atlas.agent.list` | read | yes | no | no | no | no |
| `atlas.agent.view` | read | yes | no | no | no | no |
| `atlas.agent.wakeup.list` | read | yes | no | no | no | no |
| `atlas.agent.wakeup.view` | read | yes | no | no | no | no |
| `atlas.team.list` | read | yes | no | no | no | no |
| `atlas.team.show` | read | yes | no | no | no | no |
| `atlas.goal.brief` | read | yes | no | no | no | no |
| `atlas.search` | read | yes | no | no | no | no |
| `atlas.board` | read | yes | no | no | no | no |
| `atlas.ticket.view` | read | yes | no | no | no | no |
| `atlas.ticket.history` | read | yes | no | no | no | no |
| `atlas.ticket.inspect` | read | yes | no | no | no | no |
| `atlas.schedule.list` | read | yes | no | no | no | no |
| `atlas.schedule.history` | read | yes | no | no | no | no |
| `atlas.dashboard` | read | yes | no | no | no | no |
| `atlas.timeline` | read | yes | no | no | no | no |
| `atlas.run.view` | read | yes | no | no | no | no |
| `atlas.evidence.list` | read | yes | no | no | no | no |
| `atlas.evidence.view` | read | yes | no | no | no | no |
| `atlas.handoff.view` | read | yes | no | no | no | no |
| `atlas.approvals` | read | yes | no | no | no | no |
| `atlas.inbox` | read | yes | no | no | no | no |
| `atlas.change.status` | read | yes | no | no | no | no |
| `atlas.checks.list` | read | yes | no | no | no | no |
| `atlas.sync.status` | read | yes | no | no | no | no |
| `atlas.conflict.list` | read | yes | no | no | no | no |
| `atlas.conflict.view` | read | yes | no | no | no | no |
| `atlas.archive.plan` | read | yes | no | no | no | no |
| `atlas.dispatch.suggest` | read | yes | no | no | no | no |
| `atlas.dispatch.plan` | read | yes | no | no | no | no |
| `atlas.change.merge_plan` | read | yes | no | no | no | no |
| `atlas.sync.pull_plan` | read | yes | no | no | no | no |
| `atlas.bundle.import_plan` | read | yes | no | no | no | no |
| `atlas.archive.apply_plan` | read | yes | no | no | no | no |
| `atlas.import.apply_plan` | read | yes | no | no | no | no |
| `atlas.compact_plan` | read | yes | no | no | no | no |
| `atlas.worktree.cleanup_plan` | read | yes | no | no | no | no |
| `atlas.ticket.comment` | workflow | no | yes | yes | no | no |
| `atlas.ticket.claim` | workflow | no | yes | yes | no | no |
| `atlas.ticket.release` | workflow | no | yes | yes | no | no |
| `atlas.ticket.move` | workflow | no | yes | yes | no | no |
| `atlas.ticket.create` | workflow | no | yes | yes | no | no |
| `atlas.ticket.assign` | workflow | no | yes | yes | no | no |
| `atlas.ticket.link` | workflow | no | yes | yes | no | no |
| `atlas.ticket.unlink` | workflow | no | yes | yes | no | no |
| `atlas.ticket.approve` | workflow | no | yes | yes | no | no |
| `atlas.ticket.reject` | workflow | no | yes | yes | no | no |
| `atlas.ticket.complete` | workflow | no | yes | yes | no | no |
| `atlas.agent.create` | workflow | no | yes | yes | no | no |
| `atlas.agent.edit` | workflow | no | yes | yes | no | no |
| `atlas.agent.enable` | workflow | no | yes | yes | no | no |
| `atlas.agent.disable` | workflow | no | yes | yes | no | no |
| `atlas.agent.wakeup.ack` | workflow | no | yes | yes | no | no |
| `atlas.team.apply` | workflow | no | yes | yes | no | no |
| `atlas.schedule.set` | workflow | no | yes | yes | no | no |
| `atlas.schedule.clear` | workflow | no | yes | yes | no | no |
| `atlas.ticket.request_review` | workflow | no | yes | yes | no | no |
| `atlas.gate.approve` | workflow | no | yes | yes | no | no |
| `atlas.gate.reject` | workflow | no | yes | yes | no | no |
| `atlas.run.checkpoint` | workflow | no | yes | yes | no | no |
| `atlas.evidence.add` | workflow | no | yes | yes | no | no |
| `atlas.handoff.create` | workflow | no | yes | yes | no | no |
| `atlas.import.preview` | workflow | no | yes | yes | no | yes |
| `atlas.dispatch.run` | delivery | no | yes | yes | no | yes |
| `atlas.change.create` | delivery | no | yes | yes | no | yes |
| `atlas.change.sync` | delivery | no | yes | yes | no | yes |
| `atlas.checks.sync` | delivery | no | yes | yes | no | yes |
| `atlas.change.review_request` | high_impact | no | yes | yes | yes | yes |
| `atlas.change.merge` | high_impact | no | yes | yes | yes | yes |
| `atlas.gate.waive` | high_impact | no | yes | yes | yes | yes |
| `atlas.sync.pull` | high_impact | no | yes | yes | yes | yes |
| `atlas.sync.push` | high_impact | no | yes | yes | yes | yes |
| `atlas.bundle.import` | high_impact | no | yes | yes | yes | yes |
| `atlas.import.apply` | high_impact | no | yes | yes | yes | yes |
| `atlas.archive.apply` | high_impact | no | yes | yes | yes | yes |
| `atlas.archive.restore` | high_impact | no | yes | yes | yes | yes |
| `atlas.compact` | high_impact | no | yes | yes | yes | yes |
| `atlas.worktree.cleanup` | high_impact | no | yes | yes | yes | yes |

`atlas.ticket.request_review` accepts `reviewer` for parity with `tracker ticket request-review --reviewer`. `atlas.ticket.move`, `atlas.ticket.request_review`, `atlas.ticket.approve`, and `atlas.ticket.complete` accept `override_deps` for owner-only dependency override. The override requires `actor: "human:owner"` and a non-empty reason, and Atlas records the unresolved blockers in the resulting mutation payload.

MCP-first agent loops should use `--tool-profile workflow`. That profile covers create, assign, link, claim, release, move, comment, request review, approve, reject, complete, agent/team setup, schedules, evidence, handoffs, and wake-up ack without a high-impact approval token. Run dispatch plus change creation and change/check synchronization require `delivery`. Provider review/merge and sync push/pull, bundle/import apply, archive apply/restore, compact, worktree cleanup, and gate waiver stay high-impact.
