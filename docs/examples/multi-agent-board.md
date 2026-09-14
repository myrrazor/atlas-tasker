# Shared board: Grok Build, Cursor, and Grok Bot

This is the synthetic Example App board used for the multi-agent marketing captures.
Recreate it with `examples/create-multi-agent-demo.sh`. The
[walkthrough video](../../site/assets/multi-agent-board.mp4) is silent and uses
the local web board only. Session tokens and local paths are cropped out.

Three coding agents share one project. Each owns different tickets. A lease
stops two agents from claiming the same work.

| Agent | Identity | Tickets |
|---|---|---|
| Grok Build | `agent:grok-build` | APP-1 (ready), APP-3 (in progress) |
| Cursor | `agent:cursor` | APP-2 (in progress), APP-7 (backlog) |
| Grok Bot | `agent:grok-bot` | APP-4 (in review), APP-5 (blocked on APP-2) |

## Recreate the board

```bash
TRACKER_BIN=/absolute/path/to/tracker sh examples/create-multi-agent-demo.sh /tmp/atlas-multi-agent
cd /tmp/atlas-multi-agent
tracker board
tracker agent available grok-build
tracker agent available cursor
tracker agent available grok-bot
tracker web serve --no-browser
```

## What the walkthrough shows

1. The Kanban board with assignee chips for all three agents.
2. APP-3 opened as Grok Build's in-progress upgrade test.
3. APP-2 opened as Cursor's in-progress MCP docs ticket.
4. APP-4 opened as Grok Bot's in-review retry ticket.
5. The terminal board and each agent's `available` queue.

Assignees stay on the same markdown tickets. Switching agents does not create a
second board.
