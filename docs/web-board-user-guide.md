# Web Board User Guide

Start the board from an initialized workspace:

```bash
tracker web serve --open
```

Useful options:

```bash
tracker web serve --project APP --actor human:owner --open
tracker web serve --read-only --no-browser
tracker web status
tracker web open
```

`--open` lands on `/board`. The server root `/` is a welcome overview with per-project rollups and the latest ticket changes, `/schedule` is the week timeline for one-time reminders and agent wakeups (the ticket drawer on the board can set or clear a schedule too — see [scheduling](scheduling.md)), and `/settings` shows web preferences read-only.

The board mirrors Atlas workflow rules. If a dependency, reviewer gate, owner gate, or read-only mode blocks an action, the page shows the service error instead of bypassing policy. Dropping a card on the column it is already in is a no-op.

Drag cards between columns when JavaScript is enabled. The same moves are available through card and detail actions for keyboard and non-drag users.

`tracker web open` reuses the last recorded server and fails fast when that server is no longer running. Opening the board in a browser that has no session cookie for it requires the session URL printed by `tracker web serve`.

