# Tutorial 2: Create A Project And Ticket

**Source-build walkthrough.** Tutorial 1 left `./tracker` in this checkout. If
`tracker` is already on your `PATH`, drop the `./`. Init in a directory named
`app` already created project `APP`; the `project create` below is for a
fresh extra key or a checkout that skipped the default project.

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
