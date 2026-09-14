# Atlas Home, workspaces, and projects

This is the v1.15 source candidate. The latest published installer tag is still whatever
[releases/latest](https://github.com/myrrazor/atlas-tasker/releases/latest) shows.

## Open Home

```bash
tracker
```

No arguments. Atlas starts or reuses one user-level loopback service and, in a terminal,
opens Home. Noninteractive and `--json` print the service URL instead of opening a
browser. You do not need to be inside a repository.

Home binds `127.0.0.1:7432` by default. If another process already owns that port, or an
Atlas instance with a different identity is listening there, the command fails. It does
not hop to a random port.

Home opens with a one-time claim in the URL fragment. A local script clears the fragment
with `history.replaceState`, then sends it to `POST /session/claim`. The server consumes
it once and sets an HttpOnly local session cookie. The claim is absent from request URLs
and JavaScript storage.
`tracker web serve` still uses `?token=`.

## What Home shows

| Route | Role |
|---|---|
| `/` | Home: workspaces, attention, recent activity |
| `/attention` | Cross-workspace attention |
| `/search` | Search |
| `/w/<workspace-id>` | Workspace overview |
| `/w/<workspace-id>/projects/<project-key>` | Project board |
| `/w/<workspace-id>/activity` | Activity |
| `/w/<workspace-id>/backup` | Backup health (local checkpoints vs remote verify) |
| `/settings` | Machine + local settings |
| `/settings/agents` | Agent registration status (`written` / `pending_client_restart` / `unverified`). Atlas-managed argv is `mcp serve --global --tool-profile workflow`. |
| `/settings/workspaces` | Registry: hide, repair, remove pointer |

The browser cannot POST an arbitrary filesystem path. Create board / init from the UI
needs a path grant minted by the CLI or a listing under a configured discovery root.
For an existing directory outside those roots:

```bash
tracker workspaces grant /absolute/path/to/project --purpose init --json
```

Paste the returned grant ID into Home. Use `--purpose register` for an existing Atlas
workspace or `--purpose repair` for a moved path. Grants expire, can be consumed once,
and bind to the directory identity and selected operation.

Single-workspace `tracker web serve --open` remains: `/`, `/board`, `/schedule`,
`/settings`. Prefer Home for new setups. See [the web board](../web-board.md).

## Workspaces and the registry

A workspace is one repository root with one stable `workspace_id`. Canonical records stay
in that tree: `projects/<KEY>/project.md`, `projects/<KEY>/tickets/*.md`,
`.tracker/events/*.jsonl`. The machine registry (`$STATEDIR/registry.json`) stores
**pointers only**: ID, path, display name, device/inode, timestamps, visibility, health.

v1 registries load; v2 writes extra pointer metadata. Health values:

`available`, `unavailable`, `moved`, `copied_identity_conflict`, `replaced_path`,
`permission_denied`, `schema_upgrade_required`, `corrupt_identity`, `disabled`.

Unavailable workspaces keep their last summary and do not block the others.

```bash
tracker doctor --json
tracker doctor --repair
```

`doctor` always checks the machine. Workspace checks run when the current directory is an
Atlas root. Outside a workspace it reports `current_workspace: none` and does not create
`.tracker`. `--repair` is the single repair switch (machine + current workspace when
present).

Repair actions: update a moved path, fork a copied tree onto a new workspace ID, hide /
unhide, or remove the pointer. Removing a pointer does not delete board files or backups.
A copied workspace that kept the original identity needs `fork_copy` so the original is
untouched.

Discovery walks only configured approved project roots. It skips hidden, build,
dependency, git, and cache trees, and unapproved mounts. Those bounds are engineering
safeguards, not a product promise that Atlas cannot see a path you grant explicitly.

## Projects

`tracker init` creates a default project when the workspace has none. The key is the
directory basename, uppercased to `A–Z`/`0–9`, 2–12 characters, or `MAIN`. A directory
named `app` therefore yields `APP`. It never replaces an existing project or workspace
identity.

```bash
tracker project create AUTH "Auth"
```

Each project has its own Kanban. Home's project selector and
`/w/<id>/projects/<key>` are the browser path; `tracker board --project AUTH` is the
terminal path.

## Opt-outs

Machine settings (`$STATEDIR/settings.json`) can disable auto-register, agent install,
the Home service, browser open, default project, local checkpoints, discovery, Home
visibility of hidden workspaces, and Git mode. Init flags: `--no-open`, `--no-register`,
`--no-agents`, `--no-backup`, `--git-mode`. `--skip-integrations` aliases `--no-agents`.
