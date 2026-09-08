# Welcome screen brief

Mode: audit → shape → revamp → harden.

The local workspace owner arrives wanting to know which project needs attention. The page first shows a real project/status comparison, then recent ticket changes, then routes into the selected project board.

## Hierarchy

1. Welcome and workspace context.
2. Project name, active/backlog/done counts, and blocked count.
3. Latest ticket changes.
4. New-project action.
5. Read-only web configuration.

At compact widths the ledger stays first and the activity rail stacks below it. At expanded widths the activity rail remains visible beside the ledger. The table uses row separators only; modal elevation belongs only to project creation.

## State coverage

| State | Behavior |
|---|---|
| Empty | Explains browser and CLI project creation |
| Error | Exact error text, preserved project fields, dialog reopened |
| Read only | No creation affordance; overview and settings remain available |
| Success | Redirect to `/` with inline confirmation |
| No JavaScript | New-project link opens an ordinary `dialog[open]` form |
| Long content | Names wrap; counts wrap as linked segments |

## Acceptance

- Rollups match per-project board data, with canceled tickets excluded from Done.
- Feed includes created, moved, commented, and updated ticket events only.
- Every project/feed link carries the correct `project` query.
- Keyboard focus, native dialog escape/cancel, reduced motion, and compact reflow remain usable.
- CSP stays self-only and no CDN or inline style/script is introduced.
