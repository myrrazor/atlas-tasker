# Tutorial 1: Install And Initialize

Needs `curl`, `tar`, and GitHub CLI (`gh`). The one-line installer is the
normal path and still installs the **published** tag (v1.15.0 today):

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
tracker --help
```

Optional, build locally from the checkout (**source-build walkthrough**):

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

For a local source build, use `./tracker` in the commands below until you put
the binary on your PATH. Tutorials 2 and 3 keep that prefix.

Initialize the workspace, then open Home:

```bash
tracker init
tracker
```

`init` writes Atlas-managed MCP entries named `atlas-tasker` for detected
agents unless you pass `--no-agents` / `--skip-integrations`. Detected agents
are configured without a picker. Restart the client. Grok also needs you to
trust this project in its own UI. `--integrations` still opens the older TTY
picker. See [coding-agent integrations](../guides/agent-integrations.md).

Check the workspace health in read-only mode:

```bash
tracker doctor --json
```

`doctor` without `--repair` should be your default first check. Use `--repair` only when you intend to rebuild Atlas projection state.

Next: [create a project and ticket](02-create-project-and-ticket.md).
