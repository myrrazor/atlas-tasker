# Tutorial 2: Create A Project And Ticket

Create a project key:

```bash
./tracker project create APP "Example App"
```

Create a ticket:

```bash
./tracker ticket create --project APP --title "Add health check" --type task --actor human:owner --reason "tutorial ticket"
```

Move it into the ready column:

```bash
./tracker ticket move APP-1 ready --actor human:owner --reason "ready for work"
```

Inspect the result:

```bash
./tracker board
./tracker inspect APP-1 --actor human:owner
```

Next: [run your first agent ticket](03-run-your-first-agent-ticket.md).
