# Tutorial 1: Install And Initialize

Build locally from the checkout:

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

Initialize the workspace:

```bash
./tracker init
```

In a terminal, `init` offers to install project guidance for Claude, Codex, Cursor, OpenClaw, Grok,
or a generic agent. Choose detected agents, enter `none` to skip, or run
`./tracker init --skip-integrations` when following this tutorial non-interactively. Integration files
and MCP client registration are separate; see [coding-agent integrations](../guides/agent-integrations.md).

Check the workspace health in read-only mode:

```bash
./tracker doctor --json
```

`doctor` without `--repair` should be your default first check. Use `--repair` only when you intend to rebuild Atlas projection state.

Next: [create a project and ticket](02-create-project-and-ticket.md).
