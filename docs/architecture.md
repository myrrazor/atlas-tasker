# Atlas Tasker v1 Architecture

## Overview

Atlas Tasker is local-first and terminal-first.

Storage layers:

1. **Markdown snapshots** (`projects/<KEY>/tickets/*.md`): current human-readable ticket state.
2. **Append-only JSONL events** (`.tracker/events/*.jsonl`): mutation history with actor/reason/timestamp.
3. **SQLite projection** (`.tracker/index.sqlite`): fast read model for board/list/search/history.

## Data Flow

1. Mutation command updates markdown snapshot.
2. Mutation command appends exactly one event to JSONL.
3. Mutation command applies that event to SQLite projection.
4. Read commands query projection (board/search and related views) or markdown as defined by command scope.

## Recovery

`tracker reindex` clears/rebuilds projection from markdown + events using the v1 contracts.

- Event stream is authoritative for replay order.
- Markdown snapshots provide current-state fallback during rebuild where needed.

## Workflow and Permissions

Statuses:

- backlog
- ready
- in_progress
- in_review
- blocked
- done
- canceled

Completion gate mode is configured in `.tracker/config.toml`:

- `open`
- `owner_gate`
- `review_gate`

`in_review -> done` enforcement uses `workflow.completion_mode`.

## Link Invariants

`blocks` / `blocked_by` are symmetric and validated in domain helpers.

Parent links enforce:

- no self-link
- no parent-cycle

## Output Modes

Read commands support:

- `--pretty` terminal-focused view
- `--md` markdown output
- `--json` machine-readable output

Renderer stack:

- `lipgloss` for terminal styling
- `glamour` for markdown rendering
