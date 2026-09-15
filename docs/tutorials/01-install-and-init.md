# Tutorial 1: Install And Initialize

The one-line installer is the normal path:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
tracker --help
```

Optional, build locally from the checkout:

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

For a local source build, use `./tracker` in the commands below until you put the binary on your PATH.

Initialize the workspace, then open Home:

```bash
tracker init
tracker
```

`init` writes Atlas-managed MCP entries for detected agents
unless you pass `--no-agents` / `--skip-integrations`. Restart the client. `--integrations`
still opens the older TTY picker. See [coding-agent integrations](../guides/agent-integrations.md).

Check the workspace health in read-only mode:

```bash
tracker doctor --json
```

`doctor` without `--repair` should be your default first check. Use `--repair` only when you intend to rebuild Atlas projection state.

Next: [create a project and ticket](02-create-project-and-ticket.md).
