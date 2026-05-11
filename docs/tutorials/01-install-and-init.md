# Tutorial 1: Install And Initialize

Build locally while hosted release assets are pending:

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

Initialize the workspace:

```bash
./tracker init
```

Check the workspace health in read-only mode:

```bash
./tracker doctor --json
```

`doctor` without `--repair` should be your default first check. Use `--repair` only when you intend to rebuild Atlas projection state.

Next: [create a project and ticket](02-create-project-and-ticket.md).
