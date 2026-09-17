# Atlas MCP Adapter

Atlas exposes a local stdio MCP server for coding agents that need structured access to tickets, runs, gates, changes, checks, sync state, and release workflow context.

The MCP adapter is not a second source of truth. It calls the same service layer as the CLI and TUI. Tracked mutations use Atlas permission checks, write locks, actor/reason event metadata, and storage contracts. `atlas.project.create` is the existing container-creation exception: it takes only `key` and `name`, uses the write lock and project validation, and does not create an event.

## Commands

`tracker init` registers detected clients as **`atlas-tasker`** with this exact argv:

```bash
<absolute-tracker> mcp serve --global --tool-profile workflow
```

Grok's Atlas-managed entry also includes `--tool-name-style portable` (`atlas_status`).
Other clients keep canonical dotted names. Payloads keep public `atlas.status` terminology.

```bash
tracker mcp serve --global --tool-profile workflow
tracker mcp tools --json --global --tool-profile workflow
tracker mcp schema --json --global --tool-profile workflow
tracker mcp serve --tool-profile read
tracker mcp serve --workspace /path/to/workspace --tool-profile read
tracker mcp serve --workspace-from-cwd --expected-workspace-id <WORKSPACE-ID> --tool-profile read
tracker mcp serve --workspace /absolute/existing/repo --init-if-missing --tool-profile workflow
tracker mcp schema --json --tool-profile workflow
tracker mcp tools --json --tool-profile admin
tracker mcp approve-operation --operation atlas.change.merge --target CHG-123 --actor human:owner --reason "release merge"
tracker mcp approvals list --json
tracker mcp approvals revoke <APPROVAL-ID>
```

`--global` is the machine-wide server. Atlas-managed client entries include `--tool-profile workflow`. CLI `--global` without an explicit profile also uses workflow. High-impact tools stay hidden. `--global` cannot be combined with `--workspace`,
`--init-if-missing`, or `--workspace-from-cwd`. Reads may omit `workspace_id` only when
the process CWD is a unique Atlas root. Cross-workspace writes always need explicit
`workspace_id`. Explicit IDs go through the registry `Bind` and refuse moved, copied,
replaced, unavailable, corrupt, or disabled workspaces.

Restart the coding agent after init. File presence is not “connected”: status is
`missing`, `installed` (command+args match **and** initialize succeeded),
`pending_restart`, or `unverified`.

Pinned `serve` without `--global` now defaults to `--tool-profile workflow` as well, and still reads
the current directory unless `--workspace` names one. Pass `--tool-profile read` or `--read-only` for
the read catalog. The MCP library still treats empty `Options.Profile` as `read`. Registrations that live outside a
repo — user-scoped `claude mcp add`, a global Codex `mcp_servers` entry — need the pin,
because the client picks the working directory, not you. By default, the workspace must
already contain Atlas state from `tracker init`. Pinned serve is still supported.

`--workspace-from-cwd` plus `--expected-workspace-id` is the portable binding for clients that cannot safely store an absolute machine path (DEC-072, DEC-076). Atlas canonicalizes the current directory, walks only to the nearest real Atlas root, verifies the ID, and refuses nested workspaces, symlink substitution, a replaced directory, and a copied workspace with a stale local registration. It never initializes and never opens a different registered workspace. `--workspace-from-cwd` cannot be combined with `--workspace` or `--init-if-missing`. When `--workspace` is used with `--expected-workspace-id`, the ID is verified on that root.

`--init-if-missing` is an explicit, noninteractive bootstrap. It requires `--workspace` to name an absolute path to an existing directory and requires a write-capable `workflow`, `delivery`, or `admin` profile. It refuses nested Atlas workspaces, `--read-only`, and existing output paths that redirect initialization outside the selected directory. `--expected-workspace-id` is verified before any write; a fresh directory cannot match an expected ID, so that combination creates nothing. It uses the same default-project naming and Home registry path as `tracker init`. It does not open a browser, re-register coding-agent MCP clients, rewrite git mode, or use the client's working directory as a fallback. If `.tracker` already exists, Atlas does not scaffold again: it registers the workspace and creates the first default project only when none exist, leaving config, identity, and existing projects untouched. `schema` and `tools` only describe the adapter and never initialize a workspace.

Stdio framing: Atlas speaks newline-delimited JSON-RPC and also accepts LSP-style `Content-Length` headers on the same stdio pair. Prefer NDJSON when you control the client; header-framed clients no longer crash the session.

## Profiles

- `read`: read and plan/dry-run tools, including `atlas.context`, `atlas.status`, `atlas.backup.status`, goal brief, agent/team reads, and wake-up inspection. Select it with `--tool-profile read` or `--read-only`.
- `workflow` is the CLI default for `mcp serve`, `mcp tools`, and `mcp schema`. It adds project creation and the real agent loop: ticket create/edit/assign/link, priority and label changes, claim/heartbeat/move/comment, request review, approve/reject/complete, agent create/edit, team apply, schedule writes, evidence, handoffs, and wake-up ack.
- `delivery` adds run dispatch, change creation, change/check sync, and provider review/merge tools. The danger flag exposes guarded delivery actions. Run `tracker mcp tools --json --tool-profile delivery` for the live inventory.
- `admin` adds high-impact sync, import, archive, compact, worktree-cleanup, and gate-waiver tools. Those stay hidden without `--dangerously-allow-high-impact-tools`. Run `tracker mcp tools --json --tool-profile admin` (add the danger flag to inspect the guarded catalog).

High-impact tools are hidden unless both the selected profile and server flag allow them. MCP-first agents should start at `workflow`, not `read`.

`atlas.context` and `atlas.status` are read-only. `atlas.backup.status` is the dedicated read-only backup health tool: it never changes targets, disables backup, restores, prunes, or overrides divergence, and it never returns remote URLs or credentials. `atlas.context` returns workspace identity, project inventory, the configured actor, declared and effective managed-mode policy, assigned/available/pending work, active runs, backup health (including automatic local checkpoint state, never remote credentials or URLs), and a state revision. `atlas.status` accepts `workspace`, `project`, `ticket`, `agent`, or `run` scope and returns structured JSON plus deterministic compact Markdown derived from that payload. Pass `format=chat` on `atlas.status` or `atlas.board` when the answer will be pasted into Discord, Grokbot, or another host that renders ANSI code blocks; the `chat` field is a fenced `ansi` block and the MCP text fallback uses it. Default Markdown stays ANSI-free. Unknown projects disambiguate; multiple projects stay a workspace overview unless one is named. Hosts that render MCP Apps also receive a read-only CSP-constrained board document when tool metadata links `_meta.ui.resourceUri` to a `ui://` resource (`text/html;profile=mcp-app`); Markdown remains the universal fallback. An HTML string in `structuredContent` alone is not enough for a conforming Apps host.

Exact profile counts move as tools are added. Run `tracker mcp tools --json --tool-profile <profile>` (add `--global` for the machine server) rather than copying a number from an older page. New v1.15 names are listed in [MCP tools](mcp-tools.md).

## Resources

Subscribe-able JSON resources on the global server:

- `atlas://workspaces`
- `atlas://attention`
- `atlas://workspace/<id>`
- `atlas://workspace/<id>/projects`
- `atlas://workspace/<id>/project/<key>/board`
- `atlas://workspace/<id>/backup`
- `atlas://workspace/<id>/activity`

Notifications are coalesced and carry `_meta` workspace, project, entity, revision, and event. They do not leak secrets or filesystem paths unless `IncludeLocalOnlyPaths` is on.

## Ordinary Workflow Additions

Use the exact names published by `tracker mcp schema`; Atlas does not accept silent aliases:

- `atlas.project.create` takes `key` and `name`. It is a project-container write and therefore takes no `actor` or `reason` and records no event.
- `atlas.ticket.heartbeat` takes `ticket_id`, `actor`, and `reason`. The actor must hold the active lease.
- `atlas.ticket.priority` takes `ticket_id`, `priority`, `actor`, and `reason`.
- `atlas.ticket.label.add` and `atlas.ticket.label.remove` take `ticket_id`, `label`, `actor`, and `reason`.
- `atlas.ticket.edit` takes `ticket_id`, `actor`, `reason`, plus any of `title`, `description`, `acceptance`, `priority`, `labels`, `assignee`, or `reviewer`. Omitted fields stay unchanged. An empty `description`, `assignee`, or `reviewer` string clears that field; an empty `acceptance` or `labels` array clears that list. An empty title is invalid. It cannot change status, policy, project, identity, timestamps, lease, or archive state.

The adapter passes a supplied description unchanged to the existing Markdown storage path; that
codec's normal boundary-whitespace formatting still applies when the ticket is persisted.

`atlas.ticket.create` always requires an explicit `type`; its MCP handler stores a supplied template
name but does not apply template defaults. Template-provided type remains a CLI-only convenience for
`tracker ticket create`. All five tracked tools above require both actor and non-empty reason. Their
schemas use `additionalProperties: false`, so misspelled or unknown arguments fail instead of being
ignored.

## High-Impact Approval Flow

High-impact MCP calls require a one-time approval created outside MCP:

```bash
tracker mcp approve-operation \
  --operation atlas.change.merge \
  --target CHG-123 \
  --actor human:owner \
  --reason "approved release merge" \
  --ttl 10m \
  --json
```

The tool call must pass the returned `operation_approval_id` and exact `confirm_text`, for example:

```json
{
  "change_id": "CHG-123",
  "actor": "human:owner",
  "reason": "approved release merge",
  "operation_approval_id": "mcp_approval_...",
  "confirm_text": "execute atlas.change.merge CHG-123"
}
```

Atlas validates the approval was created outside MCP, matches the operation, target, and actor, has not expired, has not been used, and still passes normal policy immediately before execution.

The approval target is exact. Tools with side-effecting modifiers bind those modifiers into the target string:

- `atlas.sync.pull`: `{"remote_id":"<REMOTE>","source_workspace_id":"<WORKSPACE-OR-EMPTY>"}`
- `atlas.archive.apply`: `{"project":"<PROJECT-OR-EMPTY>","target":"<RETENTION-TARGET>"}`
- `atlas.worktree.cleanup`: `{"force":false,"run_id":"<RUN>"}`

MCP calls are validated against their JSON schema before they reach Atlas services. Unknown arguments and wrong JSON types are rejected at the adapter boundary.

`atlas.ticket.request_review` accepts the same optional reviewer actor as the CLI `--reviewer` flag. Workflow tools that can advance blocked tickets accept `override_deps` only for `human:owner` with a non-empty reason. The call still goes through the same service-layer dependency checks and records the unresolved blockers in the mutation payload.

The adapter validator enforces required fields, rejects unknown arguments, checks primitive JSON types, and checks string-array items. It does not implement every JSON Schema keyword such as `enum`, `pattern`, numeric bounds beyond the simple `limit` shape, or semantic existence checks. Domain validation, permission checks, and policy gates still run in the Atlas service layer.

Approval consumption is single-use and happens after MCP argument/profile/actor/reason validation but before the service action starts. If the provider or service action later fails, the approval remains used; run the plan/dry-run tool again and create a new approval for a retry. This avoids letting an approval be replayed after execution has begun.

## Output Limits

Use these flags to keep model context under control:

```bash
tracker mcp serve \
  --tool-profile read \
  --max-result-bytes 131072 \
  --max-items 50 \
  --max-text-tokens-estimate 4000
```

Paged list tools accept `limit` and `cursor`. Grouped tools keep independent cursors: `atlas.board` accepts `cursor_by_status`, and `atlas.dashboard` accepts `cursor_by_section`. Large results return a truncated summary with a hint to narrow the request.

## More

`tracker integrations install` writes project instructions and skills only. `tracker setup`
does both: it refreshes those Atlas-owned files and registers a workspace-bound `workflow`
stdio server for each selected provider. The server name is derived from the workspace ID,
high-impact tools stay absent, and setup never claims `connected` until a self-probe or a
client-native check succeeds. Manual `tracker mcp serve` registration remains valid. See
[coding-agent integrations](guides/agent-integrations.md).

- [MCP security](mcp-security.md)
- [MCP tool table](mcp-tools.md)
- [Codex setup](mcp-codex.md)
- [Claude Code setup](mcp-claude-code.md)
- [MCP for agents](guides/mcp-for-agents.md)
- [MCP JSON contracts](mcp-json-contracts.md)
