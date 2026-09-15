# Quickstart

Copy this into a directory named `app` after `tracker` is on your `PATH`.
The directory name is the default project key, so `--project APP` is valid. The
one-line installer is the normal path; it does not initialize this directory.
Run `tracker init` here, then restart detected coding agents.

```bash
mkdir app && cd app
tracker init --no-open
tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "quickstart"
tracker ticket move APP-1 ready --actor human:owner --reason "start work"
tracker board
```

`--no-open` keeps this block non-interactive in CI. In a terminal, omit it so `init`
can open Home. In a repo whose directory name is not `app`, use that generated key
(or `MAIN` when the basename is too short) instead of `APP`. Extra projects are
`tracker project create AUTH "Auth"`.

The default human board is an aligned table with a readable title column, ticket IDs,
status labels, and color accents. It prints every matching ticket. The TUI keeps long
lists navigable with keyboard scrolling and explicit ticket details.

Use `tracker board --style kanban` for optional side-by-side cards. The browser keeps
its Kanban. `--json`, `--md`, `--plain`, and `NO_COLOR` remain available for other consumers.

Useful next commands:

```bash
tracker
tracker inspect APP-1 --actor human:owner
tracker ticket history APP-1 --json
tracker tui --actor human:owner
```

Use explicit actors and reasons for mutations in examples. Atlas records those fields
in the event stream and policy/audit surfaces use them later.

`--no-agents` / `--skip-integrations` skip coding-agent MCP writes. Run plain
`tracker init` in a terminal when you want detected agents registered. See
[coding-agent integrations](guides/agent-integrations.md).
