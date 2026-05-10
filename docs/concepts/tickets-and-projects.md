# Tickets And Projects

Projects group tickets under a short key such as `APP`. Ticket IDs are project-scoped, so the first ticket in `APP` is usually `APP-1`.

Tickets are markdown-native work items with Atlas metadata. They can carry:

- title, description, type, priority, labels, assignee, and reviewer
- status in the local workflow
- acceptance criteria
- protection and sensitivity flags
- policy and permission profile bindings
- relationships such as blocked-by links

Common commands:

```bash
tracker project create APP "Example App"
tracker ticket create --project APP --title "Add health check" --type task --actor human:owner --reason "create work"
tracker ticket move APP-1 ready --actor human:owner --reason "ready for implementation"
tracker inspect APP-1 --actor human:owner
```

The CLI, TUI, shell, and MCP surfaces all route mutations through the same service layer. That keeps events, permission checks, and audit metadata consistent across surfaces.
