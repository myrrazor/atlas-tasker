# TUI

`tracker tui --actor <ACTOR>` opens the terminal UI over the current workspace.

The TUI is optimized for scanning queues, boards, ticket details, approvals, inbox items, runs, sync state, and operations dashboards. It uses the same service layer as the CLI.

The Queues tab now uses the shared agent-work view. It shows:

- Available tickets the current actor can start, continue, review, or complete now
- Pending tickets that matter to that actor but are blocked by dependency, review, owner, claim, capacity, or policy reason codes

Display rules:

- `NO_COLOR=1` disables color
- `COLUMNS=<N>` controls width in non-interactive validation
- user-controlled strings are sanitized before display
- JSON remains the better source for exact stored values

If the TUI looks empty, run `tracker queue --actor <ACTOR>` and `tracker doctor --json` first.
