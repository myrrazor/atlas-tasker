# Quickstart

Copy this into a fresh repo checkout after building `tracker`.

```bash
./tracker init --skip-integrations
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "quickstart"
./tracker ticket move APP-1 ready --actor human:owner --reason "start work"
./tracker board
```

Expected board shape:

```text
Board
+-----------+-------+--------+---------+----------+--------------------+
| Column    | ID    | Type   | Status  | Priority | Title              |
+-----------+-------+--------+---------+----------+--------------------+
| Ready (1) | APP-1 | [task] | [ready] | [medium] | Ship first feature |
+-----------+-------+--------+---------+----------+--------------------+
```

Empty columns are hidden from the table. On a terminal narrower than 54 columns the board falls back to stacked `Backlog (0)` / `Ready (1)` sections instead.

Useful next commands:

```bash
./tracker inspect APP-1 --actor human:owner
./tracker ticket history APP-1 --json
./tracker tui --actor human:owner
```

Use explicit actors and reasons for mutations in examples. Atlas records those fields in the event stream and policy/audit surfaces use them later.

The `--skip-integrations` flag keeps this copyable block non-interactive. Run plain `tracker init` in a
terminal to choose detected agents during setup, or use `tracker integrations install` later. See
[coding-agent integrations](guides/agent-integrations.md).
