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
| `atlas.search` | read | yes | no | no | no | no |
| `atlas.board` | read | yes | no | no | no | no |
| `atlas.ticket.view` | read | yes | no | no | no | no |
| `atlas.ticket.history` | read | yes | no | no | no | no |
| `atlas.ticket.inspect` | read | yes | no | no | no | no |
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
| `atlas.ticket.complete` | high_impact | no | yes | yes | yes | yes |
| `atlas.sync.pull` | high_impact | no | yes | yes | yes | yes |
| `atlas.sync.push` | high_impact | no | yes | yes | yes | yes |
| `atlas.bundle.import` | high_impact | no | yes | yes | yes | yes |
| `atlas.import.apply` | high_impact | no | yes | yes | yes | yes |
| `atlas.archive.apply` | high_impact | no | yes | yes | yes | yes |
| `atlas.archive.restore` | high_impact | no | yes | yes | yes | yes |
| `atlas.compact` | high_impact | no | yes | yes | yes | yes |
| `atlas.worktree.cleanup` | high_impact | no | yes | yes | yes | yes |
