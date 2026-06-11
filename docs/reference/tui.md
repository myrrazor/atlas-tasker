# TUI

`tracker tui --actor <ACTOR>` opens the terminal UI over the current workspace.

The TUI is optimized for scanning queues, boards, ticket details, approvals, inbox items, runs, sync state, and operations dashboards. It uses the same service layer as the CLI.

Repeated items render as bordered tables where the terminal is wide enough, so board, queue, inbox, saved-view, and ops panels line up with the CLI pretty output.

Help is available inside the TUI:

- `?` opens or closes the expanded help guide
- `esc` closes the help guide or the active dialog
- `/` opens the command palette
- `/help` opens the same expanded help guide from the palette

The Queues tab now uses the shared agent-work view. It shows:

- Available tickets the current actor can start, continue, review, or complete now
- Pending tickets that matter to that actor but are blocked by dependency, review, owner, claim, capacity, or policy reason codes

The Ops tab includes recent agent wake-ups for the current actor. Command-runner configuration stays CLI-first through `tracker agent auto ...` because it can launch local processes.

The help guide documents the main tabs, the keyboard actions, bulk preview/apply flow, and palette examples for:

- `/ticket create|edit|move|claim|release|heartbeat|assign|comment|request-review|approve|reject|complete|link|unlink`
- `/run open|launch`
- `/bulk move|assign|request-review|complete|claim|release`
- `/views run`

Display rules:

- `NO_COLOR=1` disables color
- `COLUMNS=<N>` controls width in non-interactive validation
- table borders fall back to ASCII in plain/non-color contexts
- user-controlled strings are sanitized before display
- JSON remains the better source for exact stored values

If the TUI looks empty, run `tracker queue --actor <ACTOR>` and `tracker doctor --json` first.
