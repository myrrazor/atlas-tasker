# Tutorial 5: Use The TUI

Launch the full-screen interface:

```bash
./tracker tui --actor human:owner
```

The TUI uses the same service layer as the CLI. A ticket mutation from the TUI should produce the same event metadata as the equivalent command.

Once it opens, press `?` for the built-in help guide. The guide explains the tabs, keybindings, bulk flow, and supported palette commands. You can also press `/`, type `/help`, and press enter to open the same guide from the command palette.

Useful keys:

```text
tab/right      next tab
shift+tab/left previous tab
j/down         move down
k/up           move up
enter          open or submit
/              command palette
?              help
esc            close help or dialog
r              refresh
q              quit
```

Useful fallback commands while learning the UI:

```bash
./tracker board
./tracker queue --actor human:owner
./tracker approvals
./tracker inbox
```

If the TUI looks empty, check whether the workspace has tickets and whether the actor queue is empty:

```bash
./tracker ticket list
./tracker queue --actor human:owner
```

For repair guidance, read [doctor and repair](../guides/doctor-and-repair.md).
