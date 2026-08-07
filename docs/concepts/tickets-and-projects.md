# Tickets And Projects

Projects group tickets under a short key such as `APP`. Ticket IDs are project-scoped, so the first ticket in `APP` is usually `APP-1`.

Tickets are markdown-native work items with Atlas metadata. They can carry:

- title, description, type, priority, labels, assignee, and reviewer
- status in the local workflow
- acceptance criteria
- protection and sensitivity flags
- policy and permission profile bindings
- relationships such as blocked-by links
- an optional one-time schedule whose runner is the ticket assignee

Common commands:

```bash
tracker project create APP "Example App"
tracker ticket create --project APP --title "Add health check" --type task --actor human:owner --reason "create work"
tracker ticket move APP-1 ready --actor human:owner --reason "ready for implementation"
tracker inspect APP-1 --actor human:owner
```

A schedule does not create a second owner field. `tracker schedule set` assigns the ticket to its human or agent runner, then records the due instant in ticket frontmatter. Human schedules become notification events when ticked; agent schedules become inspectable wakeups and use the existing agent auto configuration. Completion timestamps remain derived from the immutable workflow event log.

The CLI, TUI, shell, and MCP surfaces all route mutations through the same service layer. That keeps events, permission checks, and audit metadata consistent across surfaces.
