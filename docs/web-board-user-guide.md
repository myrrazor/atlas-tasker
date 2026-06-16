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

The board mirrors Atlas workflow rules. If a dependency, reviewer gate, owner gate, or read-only mode blocks an action, the page shows the service error instead of bypassing policy.

Drag cards between columns when JavaScript is enabled. The same moves are available through card and detail actions for keyboard and non-drag users.

