# Upgrading to v1.16

This guide covers the move from v1.15 to [v1.16.0](https://github.com/myrrazor/atlas-tasker/releases/tag/v1.16.0).
Your boards and ticket history stay in place. Unstamped source builds still report `"version": "dev"`.

## What you must re-run

`tracker update` (or replacing the binary by hand) **replaces the executable
only**. It does not refresh managed skills, instruction blocks, or MCP argv.

In **each workspace** you want current:

```bash
tracker init
```

That refreshes Atlas-managed skill files and the user-scoped `atlas-tasker`
entry for detected clients. Then restart those clients.

If you already use the older one-pass planner:

```bash
tracker setup --plan
tracker setup --yes --agents <targets>
```

Setup stays workspace-bound. It does not copy `--global`. `--yes` is still not
remote-backup consent.

To refresh one target without a full init:

```bash
tracker integrations install codex
```

Unmanaged files in client skill roots are left alone. Atlas-managed leftovers
under `.codex/skills/atlas-worker/` and `.tracker/integrations/grok-agent-skill/`
are removed on re-install only when they match known generated files byte for byte.
User-edited copies stay and are reported as collisions.

## First-use sequence (unchanged shape)

1. Install `curl`, `tar`, a SHA-256 tool, and GitHub CLI (`gh`).
2. Install v1.16.0 through the verified release installer.
3. `tracker init` in the project.
4. Restart the coding agent (Grok: also accept project trust).
5. Ask for status or create a ticket.

Ordinary init does **not** open the six-target picker. Detected agents are
configured automatically unless `--no-agents` / `--skip-integrations`. The
picker is `tracker init --integrations` or bare `tracker integrations install`.

## Skill roots that change

| Target | v1.15 write | v1.16 write |
|---|---|---|
| Codex | `.codex/skills/atlas-worker/` | `.agents/skills/atlas-worker/` (`.codex/skills` still lists on Codex 0.144.5; duplicates if both remain) |
| Grok | `.tracker/integrations/grok-agent-skill/` | `.grok/skills/atlas-worker/` (needs Grok project trust before inspect lists it) |
| Cursor | `.cursor/skills/atlas-worker/` | same; detection also accepts `cursor-agent` |
| OpenClaw | `.agents/skills/atlas-worker/` | same shared root with Codex |
| Claude | `.claude/skills/atlas-worker/` | same |

Details: [v1.16 client compatibility](v1.16-client-compatibility.md).

## Browser

Home can create a board under Atlas boards, attach an existing workspace under
an authorized root, or authorize an existing directory through preview +
exact-path confirmation. Kanban covers everyday ticket work including Delete ticket
(an audited soft delete that retains files and history). See
[v1.16 browser management](v1.16-browser-management.md).

Home init still does not write client MCP files. Use `tracker init` for that.

## Still true

- MCP CLI default profile is `workflow`. Pass `--tool-profile read` or
  `--read-only` for inspection. The Go library empty-profile default remains
  `read`.
- Global server name is `atlas-tasker`. Pinned `--workspace` serve is advanced.
- A written config is not a live connection.
- Storage, JSON envelopes, actor/reason, CSRF, and stale-revision rules are
  unchanged.
