# Codex Guide

Atlas works best with Codex when Atlas owns workflow state and Codex owns code changes.

## Recommended Loop

```bash
tracker agent available builder-1 --json
tracker agent pending builder-1 --json
tracker inspect APP-1 --actor agent:builder-1 --json
tracker ticket claim APP-1 --actor agent:builder-1 --reason "start work"
tracker ticket move APP-1 in_progress --actor agent:builder-1 --reason "start implementation"
```

Use `available` to choose work Codex can do now. Use `pending` to explain blockers without inventing a polling loop. Use JSON reads when Codex needs exact state and Markdown reads when you want a pasteable prompt.

Codex can dispatch itself to eligible assigned work without project membership:

```bash
tracker run dispatch APP-1 --agent agent:builder-1 --actor agent:builder-1 --reason "start Codex run"
```

Cross-agent dispatch still follows collaborator membership, permission profiles, and normal policy checks.

Install the project skill pack with:

```bash
tracker integrations install codex
```

That writes the `atlas-worker` skill under `.agents/skills/`, an Atlas-managed block in `AGENTS.md`,
and generated command prompts under `.tracker/integrations/commands/`. Codex CLI 0.144.5 still
lists leftover `.codex/skills/atlas-worker/` if that copy remains. Re-install deletes that leftover
only when the file is byte-identical to a known generated Atlas skill (current or v1.15 body).
Changed files stay and are reported as collisions. A leftover whose path or ancestor is a symlink
is also a collision; Atlas does not follow those links. That is canonicalization, not a repair of
unsupported Codex. See `internal/integrations/native_skills.go`.

The install does not add an MCP server. `tracker init` writes user-scoped `atlas-tasker`. Use
[Codex MCP setup](../mcp-codex.md) for pinned `--workspace` serve. Codex workspace trust is
client-owned; Atlas never writes a trust bypass.

On Codex CLI 0.144.5, native `skills/list` listed both roots. An ephemeral
`--ignore-user-config` session with explicit fixture MCP completed `status` and
`project.list`. The client canceled `ticket.create`; there is no successful write
proof on that run. Details: [v1.16 client compatibility](../v1.16-client-compatibility.md).

If Atlas creates a wake-up after a blocker reaches `done`, read it with:

```bash
tracker agent wakeups list builder-1 --json
```

## Goal Prompts

```bash
tracker goal brief APP-1 --md
tracker goal brief <RUN-ID> --json
```

Paste the Markdown output into Codex `/goal` when you want a compact objective, constraints, gates, and verification checklist. Atlas only writes a manifest when you explicitly run:

```bash
tracker goal manifest APP-1 --actor human:owner --reason "prepare Codex goal" --md
```

## Evidence And Review

```bash
tracker run evidence add <RUN-ID> \
  --type test_result \
  --title "go test ./..." \
  --body "paste or summarize the passing output" \
  --actor agent:builder-1 \
  --reason "record verification"

tracker ticket request-review APP-1 --actor agent:builder-1 --reason "implementation ready"
```

Do not treat a status move as completion. Completion still follows the active Atlas governance policy.

For GitHub-backed work, `tracker gh create-pr APP-1` opens the pull request and links it as a change, `tracker gh request-review APP-1` moves a draft to ready, and `tracker gh checks APP-1 --json` gives Codex exact CI state. Run `tracker gh status` first to confirm the `gh` CLI is installed and authenticated.

## MCP

Start Codex with the read profile first. Add workflow or delivery profiles only for a short session where the human expects Codex to mutate Atlas state.

Read the MCP setup details in [Codex MCP setup](../mcp-codex.md) and the safety boundary in [MCP security](../mcp-security.md).
