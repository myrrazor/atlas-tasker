# Quickstart

Copy this into a fresh repo checkout after building `tracker`.

```bash
./tracker init
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "quickstart"
./tracker ticket move APP-1 ready --actor human:owner --reason "start work"
./tracker board
```

Expected board shape:

```text
Backlog (0)
  - (empty)

Ready (1)
  - APP-1 [task] [ready] [medium] Ship first feature

In Progress (0)
  - (empty)
```

Useful next commands:

```bash
./tracker inspect APP-1 --actor human:owner
./tracker ticket history APP-1 --json
./tracker tui --actor human:owner
```

Use explicit actors and reasons for mutations in examples. Atlas records those fields in the event stream and policy/audit surfaces use them later.
