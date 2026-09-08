# Driving Atlas Tasker

Atlas Tasker (`tracker`) is a local-first issue tracker that lives in the repo: tickets are
markdown files at `projects/<KEY>/tickets/<ID>.md`, every change is an append-only event under
`.tracker/`, and the SQLite file next to them is a rebuildable index, never the source of
truth.

This file is for agents **using** the tracker. If you are contributing to Atlas itself, read
[CONTRIBUTING.md](CONTRIBUTING.md); the archived v1 contributor guide is at
[docs/v1-agents-archive.md](docs/v1-agents-archive.md).

## Read this part first

- **Pass `--actor` and `--reason` on every write.** They land in the event log, some policies
  reject writes without them, and omitting `--actor` silently attributes your work to
  `human:owner`. `--actor` is `human:<name>` or `agent:<agent-id>`; a bare `agent:` is exit 2.
- **`tracker project create` takes neither.** Projects are containers, not tracked
  mutations. Passing `--actor` there is an unknown-flag error.
- **Exit 4 on a status change is the workflow working, not a crash.** Only some status edges
  exist. `backlog -> in_progress` is a forbidden transition and always will be; go through
  `ready`. Retrying the same move gets the same 4.
- **Do not edit `.tracker/` or the ticket markdown by hand.** Writes go through the CLI or MCP so
  the event log, the index, and the lease state stay in agreement. If you already hand-edited
  something, `tracker doctor --repair` rebuilds the index from markdown and events.
- **Claim before you edit code.** A lease is how two agents avoid the same ticket. Claiming a
  ticket someone else holds is exit 4, not a queue.
- **The browser board is for humans.** `tracker web serve` mints a random session token per
  process and never persists it; there is no API key and no headless mode. Agents use the CLI
  or MCP. Do not scrape the board.
- **`--json` goes to stdout and is the whole of stdout.** Notifications, warnings and errors
  go to stderr. Parse stdout, never the pretty or markdown output.

## The loop

```bash
# what can I work on right now
tracker agent available builder-1 --json

# take it
tracker ticket claim APP-12 --actor agent:builder-1 --reason "start work"
tracker ticket move APP-12 in_progress --actor agent:builder-1 --reason "start work"

# leave a durable trace as you go
tracker ticket comment APP-12 --body "swapped the retry to exponential backoff" \
  --actor agent:builder-1 --reason "progress note"

# hand it over
tracker ticket request-review APP-12 --actor agent:builder-1 --reason "ready for review"

# and as the reviewer
tracker ticket approve APP-12 --actor agent:reviewer-1 --reason "review passed"
tracker ticket complete APP-12 --actor agent:reviewer-1 --reason "merged"
```

Every one of those takes `--json`. `tracker agent available` already hands you the exact next
commands:

```console
$ tracker agent available builder-1 --json
{
  "format_version": "v1",
  "kind": "agent_work",
  "payload": {
    "actor": "agent:builder-1",
    "available": [
      {
        "action": "start",
        "reason": "ready for this agent",
        "suggested_commands": [
          "tracker ticket claim APP-12 --actor agent:builder-1 --reason \"start work\"",
          "tracker ticket move APP-12 in_progress --actor agent:builder-1 --reason \"start work\""
        ],
        "ticket": { "id": "APP-12", "status": "ready", "...": "..." }
      }
    ]
  }
}
```

Nothing available? `tracker agent pending builder-1 --json` says why, with stable reason codes:
`dependency_blocked`, `waiting_for_review`, `waiting_for_owner`, `not_ready_status`,
`claimed_by_other`, `policy_blocked`, `agent_at_capacity`, `missing_capability`. Only a
dependency reaching `done` clears `dependency_blocked` — `canceled` does not.

### Reading state

```bash
tracker board --json                       # {"columns": {"ready": [...], ...}}
tracker queue --actor agent:builder-1 --json
tracker ticket view APP-12 --json
tracker inspect APP-12 --actor agent:builder-1 --json   # policy + lease + queue + history
tracker ticket history APP-12 --json
tracker search 'project=APP status=ready text~retry' --json
```

`tracker inspect` is the one to reach for when the queue and the ticket disagree. It answers
"why can't I move this" in a single call.

### Status edges

```
backlog     -> ready, blocked, canceled
ready       -> in_progress, blocked, canceled
in_progress -> in_review, ready, blocked, canceled
in_review   -> done, in_progress, blocked
blocked     -> ready, in_progress, canceled
done        -> (terminal)
canceled    -> (terminal)
```

`in_review` and `done` are reached through `request-review` / `approve` / `complete` rather
than `move`, because those commands also run the completion policy and any gates.

## Exit codes

| Code | Meaning | What it usually is |
|---|---|---|
| 0 | ok | |
| 1 | internal | a real bug, or a missing required flag |
| 2 | invalid_input | bad status/type/priority/actor, malformed query |
| 3 | not_found | no such ticket, project, agent, or view |
| 4 | conflict | forbidden transition, ticket already claimed, already exists |
| 5 | permission_denied | policy, separation of duties, or the wrong reviewer |
| 6 | busy | another writer holds the workspace lock |
| 7 | repair_needed | index is unreadable; run `tracker doctor --repair` |

Under `--json` the error is machine-readable too — on **stderr**, with stdout left empty:

```json
{
  "format_version": "v1",
  "ok": false,
  "error": { "code": "not_found", "message": "ticket APP-99 not found", "exit": 3 }
}
```

Branch on `error.code` or the exit status, not on the message text.

## Nothing prompts

Every command is non-interactive. A missing required flag fails immediately naming the flag
(`required flag(s) "body" not set`) instead of waiting on stdin. If a command appears to hang,
it is waiting on the workspace write lock for a few seconds, and that ends in exit 6 — not a
prompt.

## MCP

For clients without a shell. The tools call the same services as the CLI, so nothing here is a
second source of truth.

```bash
# Claude Code, user scope
claude mcp add --transport stdio --scope user atlas -- /usr/local/bin/tracker mcp serve --workspace /path/to/repo --tool-profile read

# Codex
codex mcp add atlas -- /usr/local/bin/tracker mcp serve --workspace /path/to/repo --tool-profile read

# OpenClaw (--cwd is its own way of pinning the directory)
openclaw mcp add atlas --command /usr/local/bin/tracker --arg mcp --arg serve --cwd /path/to/repo
```

`--workspace` is what stops the server from answering against whatever directory the client
happened to start in. Profiles go `read` (default) -> `workflow` -> `delivery` -> `admin`;
start at `read` and widen only when the human asks for writes. High-impact tools stay hidden
unless the server was started with `--dangerously-allow-high-impact-tools` **and** a human
created a one-time approval with `tracker mcp approve-operation`.

`tracker mcp tools --json --tool-profile read` lists what a profile actually exposes.
Full detail: [docs/mcp.md](docs/mcp.md).

## Installing the skill

`tracker integrations install <codex|claude|openclaw|generic>` writes the `atlas-worker` skill
and an instruction block into the right place for that agent — `.codex/skills/`,
`.claude/skills/`, `.agents/skills/`, or `.tracker/integrations/` respectively. It only ever
writes inside the workspace; the shared per-machine copy is yours to install.

Re-running is safe: only the Atlas-managed block between the `atlas-tasker` markers changes,
and `--force` (whole-file replace) is opt-in.

## More

- [README](README.md) — what Atlas is, install, the tour
- [docs/command-reference.md](docs/command-reference.md) — every command and flag
- [docs/reference/json-output.md](docs/reference/json-output.md) — envelope shapes
- [docs/first-agent-workflow.md](docs/first-agent-workflow.md) — the same loop, narrated
- [docs/guides/claude-code.md](docs/guides/claude-code.md), [docs/guides/codex.md](docs/guides/codex.md), [docs/guides/generic-agent.md](docs/guides/generic-agent.md)
