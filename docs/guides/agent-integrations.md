# Coding-Agent Integrations

For the short regular-user path, start with [setup and backup](setup-and-backup.md). This page is the per-target detail.

Install the binary, then in your project run `tracker init`. That
detects the coding agents on the machine and, unless you pass `--no-agents`, writes
Atlas-managed MCP entries pointing at `tracker mcp serve --global --tool-profile workflow` plus the worker
skill. Restart the client. After that, open the same repo in Claude Code, Codex, Cursor,
OpenClaw, or Grok and ask for the board — you do not install a second Atlas package
inside the agent.

Atlas can install project-level instructions and an `atlas-worker` skill for six agent targets. The
integration pack teaches an agent how to read Atlas work, claim a ticket, record evidence, and request
review. It does not install an agent, launch one, or grant it permissions. A written MCP entry is
`written` or `pending_client_restart`, never “connected” just because a config file exists.
Home → Settings → Agents labels that as configured (and pending a client restart when the
file merely matches). User-scoped `atlas-tasker` rows are distinct from project-scoped
workspace servers; a repo `.grok/config.toml` entry is not reported as a broken user
registration. Each row shows the actual written command, including Grok’s
`--tool-name-style portable`. Advanced `tracker setup` can still register a workspace-bound
Atlas MCP server for selected providers.

## Unified setup (advanced / v1.14)

Prefer `tracker init`. `tracker setup` is the workspace-scoped
planner and apply path for agent guidance and MCP registration when you already have a
workspace and want the older one-pass refresh. It inspects detected clients, existing managed blocks, and local setup state, prints a
read-only plan, and applies one provider transaction at a time. Planning never writes. `--yes` is
not backup consent and is not consent for unnamed machine-wide scopes. `--team` may apply `solo`,
`pair`, `swarm`, or `crossfire` and never silently overwrites existing roles.

```bash
tracker setup --plan --json
tracker setup --yes --agents generic
tracker setup status --json
tracker integrations repair generic --yes
tracker integrations disconnect generic --yes
```

The release installer may offer `tracker setup` after an explicit TTY yes. Unattended install never
initializes the current directory and never runs a second integration wizard.

## Choose During Init

In an interactive terminal, `tracker init` initializes the current workspace and then asks:

```text
Set up coding-agent integrations now? [Y/n]
```

Accepting opens a picker. Atlas checks the current `PATH`, the agent's home configuration directory,
and agent-specific files in the workspace. Detected targets are checked by default; `generic` is
always available but is never selected automatically.

The picker accepts:

- Enter to install the checked targets
- target numbers or names, such as `1,3` or `claude,cursor`
- `all` to select all six targets
- `none` to finish without installing an integration
- `q` to cancel the integration step

Use `tracker init --skip-integrations` when initialization must not prompt. Use
`tracker init --integrations` to open the picker immediately after initialization; this form requires
an interactive terminal.

## Install Later

Detection is read-only:

```bash
tracker integrations detect
tracker integrations detect --json
```

Open the picker from an initialized or new workspace:

```bash
tracker integrations install
```

For scripts and other non-interactive callers, name the targets explicitly:

```bash
tracker integrations install codex
tracker integrations install --targets claude,codex,cursor
tracker integrations install --targets claude,codex,cursor,openclaw,grok,generic --json
```

With no target, a non-TTY invocation fails before writing integration files. The install command can
create the normal Atlas initialization artifacts when needed, but target validation happens before
that scaffold in non-interactive use.

## What Each Target Writes

Every target gets a generated guide under `.tracker/integrations/`. Managed instruction blocks use
HTML comments so a later install can update Atlas guidance without replacing house rules outside the
block.

| Target | Instruction file | Skill and command files |
|---|---|---|
| `claude` | `CLAUDE.md` | `.claude/skills/atlas-worker/` and `.claude/commands/atlas-{next,take,review}.md` |
| `codex` | `AGENTS.md` | `.codex/skills/atlas-worker/` plus `.tracker/integrations/commands/atlas-{next,take,review}.md` |
| `cursor` | `AGENTS.md` | `.cursor/skills/atlas-worker/`, including its `commands/` directory |
| `openclaw` | `AGENTS.md` | `.agents/skills/atlas-worker/`, including its `commands/` directory |
| `grok` | `AGENTS.md` | `.tracker/integrations/grok-agent-skill/` |
| `generic` | `AGENTS.md` | `.tracker/integrations/generic-agent-instructions.md` and `.tracker/integrations/generic-agent-skill/`, including its `commands/` directory |

Codex, Cursor, OpenClaw, Grok, and generic targets use distinct managed markers in `AGENTS.md`, so
their Atlas blocks can coexist. Claude uses `CLAUDE.md`. Re-running the same install refreshes Atlas's
managed files. Keep custom instructions outside the markers.

Grok and generic installs use separate skill directories, so installing one cannot rewrite the
other's provider-specific skill. Older `.tracker/integrations/atlas-agent-skill/` content is left
untouched; reinstall the intended target to create its new provider-specific directory.

`--force` replaces the entire target instruction file before writing the Atlas block. Use it only
when replacing existing `AGENTS.md` or `CLAUDE.md` content is intentional.

OpenClaw is repo-local by default. This explicit form also copies the generated skill and command
files to `~/.openclaw/skills/atlas-worker/`:

```bash
tracker integrations install openclaw --global
```

`--global` is rejected for every other target and for a multi-target install.

## Use The Installed Pack

Start the agent in the Atlas workspace so it can read the project instruction and skill files. The
core loop is the same for every target:

```bash
tracker agent available builder-1 --json
tracker agent pending builder-1 --json
tracker inspect APP-1 --actor agent:builder-1 --json
tracker ticket claim APP-1 --actor agent:builder-1 --reason "start work"
tracker ticket move APP-1 in_progress --actor agent:builder-1 --reason "start work"
```

For OpenClaw, `openclaw skills list` and `openclaw skills check` show whether the skill is present and
ready. For other agents, inspect the generated paths above and use that client's normal project-skill
or command discovery UI.

## MCP registration

`tracker setup --yes --agents <targets>` refreshes each target's managed instruction block, the
existing `atlas-worker` skill, and the provider's MCP registration in one transaction. The server
name is derived from the workspace ID (`atlas-` plus twelve hex characters), the profile is always
`workflow`, and high-impact tools stay absent. Atlas never grants workspace trust or MCP approval
on the user's behalf; those stay `pending_workspace_trust` or `pending_mcp_approval`.

Grok cannot load dotted MCP tool names (`atlas.status` becomes an invalid `server__tool`
qualifier). Both `tracker init` (user-scope `grok mcp add`) and `tracker setup --agents grok`
register Atlas with `--tool-name-style portable`, so Grok sees `atlas_status`, `atlas_board`,
and the rest of the catalog with dots turned into one underscore. Claude, Codex, Cursor,
OpenClaw, and generic keep canonical dotted names. JSON and Markdown still use the public
`atlas.status` names.

Project-scoped Grok config (`grok mcp add --scope project`) keeps `--workspace-from-cwd`
and the expected workspace id. The command is the bare `tracker` name only when `PATH`
resolves to the same Atlas executable that is running. A source build that is not on
`PATH` may keep its absolute command only when that path is not under the home
directory — repository-carried config cannot embed a personal path. If this binary
lives under home and is not on `PATH`, setup refreshes the skill, skips project MCP,
and tells you to put this `tracker` on `PATH` and rerun setup. User-scoped Home
registration can still use the absolute command; setup does not invent a user config
path. Home Agents treats a written `tracker` name as configured only while `PATH`
still resolves to that same executable.

Ask for status or the board after restart. The generated `atlas-worker` skill tells the
session to query Atlas and answer with a compact Markdown ticket table, then blockers
and next steps. Large boards stay bounded: use filters or pagination and disclose
shown/total instead of dumping the whole catalog. Browser Kanban and the terminal/TUI
table are unchanged.

Generic setup writes a portable descriptor at `.tracker/integrations/atlas-mcp.json` and reports
`portable_ready` until a conformance host or a real client probes it. It does not claim a universal
unknown client is connected merely because a file exists.

Manual registration remains available when you are not using setup:

```bash
codex mcp add atlas -- /usr/local/bin/tracker mcp serve --workspace /path/to/workspace --tool-profile workflow
```

See [MCP for agents](mcp-for-agents.md), [Codex MCP setup](../mcp-codex.md), and
[Claude Code MCP setup](../mcp-claude-code.md).

## After An Atlas Update

`tracker update` replaces the binary only. Re-run the same `tracker integrations install ...` command
after upgrading when you want to refresh generated guidance and skills. Existing custom content
outside the managed markers remains in place unless `--force` is used.
