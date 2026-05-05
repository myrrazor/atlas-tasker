# Atlas Tasker v1.4 Command Reference

## Top-Level

- `tracker init`
- `tracker help`
- `tracker doctor [--repair]`
- `tracker reindex`
- `tracker inspect <ID> [--actor <ACTOR>]`
- `tracker automation list`
- `tracker automation view <NAME>`
- `tracker automation create <NAME> [flags]`
- `tracker automation edit <NAME> [flags]`
- `tracker automation delete <NAME>`
- `tracker automation dry-run <NAME> [--ticket <ID>] [--event-type <TYPE>] [--actor <ACTOR>]`
- `tracker automation explain <NAME> [--ticket <ID>] [--event-type <TYPE>] [--actor <ACTOR>]`
- `tracker notify send --event-type <TYPE> [--ticket <ID>] [--project <KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker notify log [--limit <N>]`
- `tracker notify dead-letter [--limit <N>]`
- `tracker git status`
- `tracker git branch-name <ID>`
- `tracker git refs <ID>`
- `tracker git commit <ID> --message <TEXT>`
- `tracker views list`
- `tracker views view <NAME>`
- `tracker views save <NAME> --kind <board|search|queue|next> [flags]`
- `tracker views delete <NAME>`
- `tracker views run <NAME> [--actor <ACTOR>]`
- `tracker watch list [--actor <ACTOR>]`
- `tracker watch ticket <ID> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch project <KEY> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch view <NAME> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker unwatch ticket <ID> [--actor <ACTOR>]`
- `tracker unwatch project <KEY> [--actor <ACTOR>]`
- `tracker unwatch view <NAME> [--actor <ACTOR>]`
- `tracker bulk move <STATUS> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk assign <ACTOR> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk request-review [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk complete [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk claim [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk release [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker templates list`
- `tracker templates view <NAME>`
- `tracker integrations install codex [--force]`
- `tracker integrations install claude [--force]`
- `tracker tui [--actor <ACTOR>]`
- `tracker config get [KEY]`
- `tracker config set <KEY> <VALUE>`
- `tracker run list [--ticket <ID>] [--agent <AGENT-ID>] [--status <STATUS>]`
- `tracker run view <RUN-ID>`
- `tracker run dispatch <TICKET-ID> --agent <AGENT-ID> [--kind <work|review|qa|release>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run start <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run attach <RUN-ID> --provider <PROVIDER> --session-ref <REF> [--replace] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run open <RUN-ID>`
- `tracker run launch <RUN-ID> [--refresh] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run complete <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run fail <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run abort <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run cleanup <RUN-ID> [--force] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker worktree list`
- `tracker worktree view <RUN-ID>`
- `tracker worktree repair`
- `tracker worktree prune`
- `tracker dispatch suggest <TICKET-ID>`
- `tracker dispatch queue`
- `tracker dispatch run <TICKET-ID> [--agent <AGENT-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker dispatch bulk [--ticket <ID>]... [--view <NAME>] [--agent <AGENT-ID>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker approvals`
- `tracker gate list [--ticket <ID>] [--run <RUN-ID>] [--state <STATE>]`
- `tracker gate view <GATE-ID>`
- `tracker gate approve <GATE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker gate reject <GATE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker gate waive <GATE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker inbox`
- `tracker inbox view <ITEM-ID>`
- `tracker change list [--ticket <ID>]`
- `tracker change view <CHANGE-ID>`
- `tracker change create <RUN-ID>`
- `tracker change status <CHANGE-ID>`
- `tracker change sync <CHANGE-ID>`
- `tracker change review-request <CHANGE-ID>`
- `tracker change merge <CHANGE-ID>`
- `tracker change link <TICKET-ID> [flags]`
- `tracker change import-url <TICKET-ID> --url <URL>`
- `tracker change unlink <TICKET-ID> <CHANGE-ID>`
- `tracker checks list [--scope <run|change|ticket>] [--id <SCOPE-ID>]`
- `tracker checks view <CHECK-ID>`
- `tracker checks record --scope <run|change|ticket> --id <SCOPE-ID> --name <NAME> [flags]`
- `tracker checks sync <CHANGE-ID>`
- `tracker permission-profile list`
- `tracker permission-profile view <PROFILE-ID>`
- `tracker permission-profile create <PROFILE-ID>`
- `tracker permission-profile edit <PROFILE-ID>`
- `tracker permission-profile bind <PROFILE-ID>`
- `tracker permission-profile unbind <PROFILE-ID>`
- `tracker permissions view <TARGET>`
- `tracker import preview <PATH>`
- `tracker import apply <JOB-ID>`
- `tracker import list`
- `tracker import view <JOB-ID>`
- `tracker export create [--scope <SCOPE>]`
- `tracker export list`
- `tracker export view <BUNDLE-ID>`
- `tracker export verify <PATH|BUNDLE-ID>`
- `tracker evidence list <RUN-ID>`
- `tracker evidence view <EVIDENCE-ID>`
- `tracker handoff view <HANDOFF-ID>`
- `tracker handoff export <HANDOFF-ID>`

## Agents

- `tracker agent list`
- `tracker agent view <AGENT-ID>`
- `tracker agent create <AGENT-ID> --name <NAME> --provider <PROVIDER> [flags]`
- `tracker agent edit <AGENT-ID> [flags]`
- `tracker agent enable <AGENT-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker agent disable <AGENT-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker agent eligible <TICKET-ID>`

Behavior:
- agent profiles live under `.tracker/agents/`
- eligibility is deterministic and returns the same ranking order used later by dispatch
- disabled agents and capability mismatches are reported explicitly in JSON mode

## Runs

- `tracker run list [--ticket <ID>] [--agent <AGENT-ID>] [--status <STATUS>]`
- `tracker run view <RUN-ID>`
- `tracker run dispatch <TICKET-ID> --agent <AGENT-ID> [--kind <work|review|qa|release>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run start <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run attach <RUN-ID> --provider <PROVIDER> --session-ref <REF> [--replace] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run open <RUN-ID>`
- `tracker run launch <RUN-ID> [--refresh] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run checkpoint <RUN-ID> [--title <TEXT>] [--body <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run evidence add <RUN-ID> --type <TYPE> [--title <TEXT>] [--body <TEXT>] [--artifact <PATH>] [--supersedes <EVIDENCE-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run handoff <RUN-ID> [--open-question <TEXT>]... [--risk <TEXT>]... [--next-actor <ACTOR>] [--next-gate <KIND>] [--next-status <STATUS>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run complete <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run fail <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run abort <RUN-ID> [--summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run cleanup <RUN-ID> [--force] [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- dispatch creates a run snapshot first, then the managed worktree and runtime directory
- one active run per ticket is the default; parallel dispatch requires `allow_parallel_runs=true`
- `run attach` is idempotent for the same provider/session pair
- `run open` is read-only and only reports the canonical runtime, evidence, and worktree paths
- `run launch` writes `brief.md`, `context.json`, `launch.codex.txt`, and `launch.claude.txt` under `.tracker/runtime/<run-id>/`
- `run launch` is idempotent by default; `--refresh` rewrites stale runtime artifacts
- cleanup is explicit and only allowed after `completed`, `failed`, or `aborted`
- checkpoints and evidence mutate only the run snapshot/evidence bundle; they do not change ticket status by themselves
- handoff packets are immutable markdown snapshots stored under `.tracker/handoffs/`

## Worktrees

- `tracker worktree list`
- `tracker worktree view <RUN-ID>`
- `tracker worktree repair`
- `tracker worktree prune`

Rules:

- managed worktrees are execution isolation only, never source of truth
- `reindex` and `doctor --repair` will not recreate missing worktrees or runtime artifacts
- dirty worktrees require `run cleanup --force`
- the clean-main check ignores Atlas-managed workspace files and only blocks on non-Atlas repo changes

## Dispatch

- `tracker dispatch suggest <TICKET-ID>`
- `tracker dispatch queue`
- `tracker dispatch run <TICKET-ID> [--agent <AGENT-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker dispatch bulk [--ticket <ID>]... [--view <NAME>] [--agent <AGENT-ID>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- dispatch suggestion and queue surfaces reuse the same eligibility and runbook resolution logic used by live dispatch
- `dispatch run` auto-routes only when exactly one agent is eligible; otherwise it requires `--agent`
- bulk dispatch preserves the exact saved-view order and still re-checks eligibility at apply time
- runbook resolution order is ticket override, agent default, project mapping, then built-in default

## Approvals

- `tracker approvals`
- `tracker gate list [--ticket <ID>] [--run <RUN-ID>] [--state <STATE>]`
- `tracker gate view <GATE-ID>`
- `tracker gate approve <GATE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker gate reject <GATE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker gate waive <GATE-ID> [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- `run handoff` opens any required runbook gates for the run and can also open an explicit `--next-gate`
- rejecting a run-scoped gate sends the run back to `active`
- approving or waiving the last open run-scoped gate relaxes the run back to `handoff_ready`
- open gates block dispatch, `run complete`, and `ticket complete`

## Inbox

- `tracker inbox`
- `tracker inbox view <ITEM-ID>`

Rules:

- inbox items are derived, not stored
- open gates surface as `gate:<gate-id>` items
- handoff-ready runs surface as `handoff:<handoff-id>` items

## Changes

- `tracker change list [--ticket <ID>]`
- `tracker change view <CHANGE-ID>`
- `tracker change create <RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker change status <CHANGE-ID>`
- `tracker change sync <CHANGE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker change review-request <CHANGE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker change merge <CHANGE-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker change link <TICKET-ID> [--change-id <CHANGE-ID>] [--provider <local|github>] [--status <STATUS>] [--run <RUN-ID>] [--branch <NAME>] [--base <NAME>] [--head <REF>] [--url <URL>] [--external-id <ID>] [--checks-status <STATE>] [--reviewer <ACTOR>]... [--review-summary <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker change import-url <TICKET-ID> --url <URL> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker change unlink <TICKET-ID> <CHANGE-ID> [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- `change create` derives the linked change from the run branch/worktree and keeps the local change snapshot canonical
- `change status` is read-only and observes local/provider state without mutating the stored change
- `change sync` is the explicit live operation that reconciles provider-backed status into the stored change snapshot
- `change review-request` is the explicit provider-write path for moving a draft GitHub pull request into review and recording the requested review target locally
- `change merge` is the explicit provider-write path for merging a GitHub pull request after readiness and gate checks pass
- `change link` creates a new local change id when `--change-id` is omitted
- `change import-url` currently accepts GitHub pull request URLs only; lightweight GitHub issue reference import remains part of the import/export slice
- ticket snapshots store the active linked change ids and a deterministic change-readiness rollup
- linked changes appear in `ticket view`, `run view`, and `handoff view`
- `change view` includes the current local changed-file summary for the associated run worktree when available
- passive read surfaces like `ticket view` and `inspect` do not call providers; provider reads and writes stay on explicit `change status|sync|review-request|merge` and `checks sync` commands
- unlink removes the active ticket link but keeps the change snapshot and event history intact

## Checks

- `tracker checks list [--scope <run|change|ticket>] [--id <SCOPE-ID>]`
- `tracker checks view <CHECK-ID>`
- `tracker checks record --scope <run|change|ticket> --id <SCOPE-ID> --name <NAME> [--check-id <CHECK-ID>] [--source <local|provider|manual>] [--provider <local|github>] [--status <queued|running|completed>] [--conclusion <unknown|success|failure|neutral|cancelled|timed_out|skipped>] [--summary <TEXT>] [--url <URL>] [--external-id <ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker checks sync <CHANGE-ID> [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- checks update in place by stable `check_id`; the audit trail lives in the event log
- change-scoped and ticket-scoped checks feed the same readiness rollup used by `ticket view` and `inspect`
- `checks sync` is the explicit provider-read path for change-scoped checks; replay, reindex, and repair never call providers
- run-scoped checks appear in `run view` and `handoff view`

## Permission Profiles

- `tracker permission-profile list`
- `tracker permission-profile view <PROFILE-ID>`
- `tracker permission-profile create <PROFILE-ID> [--name <TEXT>] [--priority <N>] [--workspace-default] [--project <KEY>]... [--agent <ID>]... [--runbook <NAME>]... [--allow-project <KEY>]... [--allow-ticket-type <TYPE>]... [--allow-runbook <NAME>]... [--allow-capability <CAP>]... [--allow-action <ACTION>]... [--deny-action <ACTION>]... [--allow-path <GLOB>]... [--forbid-path <GLOB>]... [--require-owner-for-sensitive-ops] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker permission-profile edit <PROFILE-ID> [same flags as create]`
- `tracker permission-profile bind <PROFILE-ID> (--workspace | --project <KEY> | --agent <ID> | --runbook <NAME> | --ticket <ID>) [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker permission-profile unbind <PROFILE-ID> (--workspace | --project <KEY> | --agent <ID> | --runbook <NAME> | --ticket <ID>) [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker permissions view <TARGET> [--actor <ACTOR>] [--action <ACTION>]`

Rules:

- explicit deny beats explicit allow across every matching profile
- matching order is workspace default, project default, agent binding, runbook binding, then direct ticket overlays
- `permissions view` returns the ordered profile matches, the effective allow or deny decision, and stable reason codes for every blocked checkpoint
- protected and sensitive tickets can require `human:owner`; when the owner is the actor Atlas records an explicit override event instead of silently bypassing the profile
- path restrictions normalize to repo-root-relative slash paths and block with `unverifiable_path_scope` when Atlas cannot verify the changed-file set for the action
- enforcement currently happens at dispatch, run launch, change create, change merge, gate open, gate approve, run completion, and ticket completion

## Import / Export

- `tracker import preview <PATH> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker import apply <JOB-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker import list`
- `tracker import view <JOB-ID>`
- `tracker export create [--scope <SCOPE>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker export list`
- `tracker export view <BUNDLE-ID>`
- `tracker export verify <PATH|BUNDLE-ID> [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- preview is deterministic and side-effect free with respect to imported canonical data; it records a persistent import-job snapshot and audit event
- apply transitions the job through `validated`, `applying`, and then `applied` or `failed`
- Atlas bundle export writes three sidecars under `.tracker/exports/`: the `.tar.gz` archive, `.manifest.json`, and `.sha256`
- `export verify` works by bundle id or direct archive path and validates manifest membership plus per-file checksums
- Atlas bundle import is snapshot-first: it restores canonical markdown snapshots into the target workspace, but it does not copy the source workspace's `.tracker/events/` files into the live target workspace
- structured Jira CSV and GitHub JSON imports are create-only in v1.5; existing ticket ids are reported as conflicts during preview and block apply
- GitHub JSON import is metadata-link import only; it creates Atlas tickets and preserves the external source URL as import provenance
- Atlas bundle import rejects path traversal and staged-copy conflicts before canonical writes land

## Evidence

- `tracker evidence list <RUN-ID>`
- `tracker evidence view <EVIDENCE-ID>`
- `tracker run checkpoint <RUN-ID> [--title <TEXT>] [--body <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run evidence add <RUN-ID> --type <TYPE> [--title <TEXT>] [--body <TEXT>] [--artifact <PATH>] [--supersedes <EVIDENCE-ID>] [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- evidence is immutable in v1.4; supersession creates a new evidence item instead of rewriting history
- artifact files are copied into `.tracker/evidence/<run-id>/` with normalized filenames
- evidence survives `run cleanup`

## Handoffs

- `tracker run handoff <RUN-ID> [--open-question <TEXT>]... [--risk <TEXT>]... [--next-actor <ACTOR>] [--next-gate <KIND>] [--next-status <STATUS>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker handoff view <HANDOFF-ID>`
- `tracker handoff export <HANDOFF-ID>`

Rules:

- handoff export uses deterministic markdown derived from the stored packet
- handoffs survive `run cleanup`
- handoff creation does not auto-open gates in PR-405; that comes with PR-406

## TUI

- `tracker tui [--actor <ACTOR>]`

Panels:

- `Detail` now includes run, evidence, handoff, and runtime panels for the selected ticket
- `Inbox` includes approvals, derived human inbox items, notification deliveries, and dead letters
- `Ops` includes agents, dispatch queue, worktrees, automation explain, and bulk preview state

Palette shortcuts:

- `/ticket ...`
- `/run open <RUN-ID>`
- `/run launch <RUN-ID> [--refresh]`
- `/bulk ...`
- `/views run <NAME>`

## Project

- `tracker project create <KEY> <NAME>`
- `tracker project list`
- `tracker project view <KEY>`
- `tracker project policy get <KEY>`
- `tracker project policy set <KEY> [flags]`

## Ticket CRUD

- `tracker ticket create --project <KEY> --title <TEXT> [--type <epic|task|bug|subtask>] [--template <NAME>] [flags]`
- `tracker ticket view <ID>`
- `tracker ticket edit <ID> [flags]`
- `tracker ticket delete <ID>`
- `tracker ticket list [--project <KEY>] [--status <STATUS>] [--assignee <ACTOR>] [--type <TYPE>]`

## Ticket Mutation

- `tracker ticket move <ID> <STATUS>`
- `tracker ticket assign <ID> <ACTOR>`
- `tracker ticket priority <ID> <PRIORITY>`
- `tracker ticket label add <ID> <LABEL>`
- `tracker ticket label remove <ID> <LABEL>`
- `tracker ticket claim <ID> [--actor <ACTOR>]`
- `tracker ticket release <ID> [--actor <ACTOR>]`
- `tracker ticket heartbeat <ID> [--actor <ACTOR>]`
- `tracker ticket request-review <ID> [--actor <ACTOR>]`
- `tracker ticket approve <ID> [--actor <ACTOR>]`
- `tracker ticket reject <ID> --reason <TEXT> [--actor <ACTOR>]`
- `tracker ticket complete <ID> [--actor <ACTOR>]`
- `tracker ticket policy get <ID>`
- `tracker ticket policy set <ID> [flags]`

## Relationships

- `tracker ticket link <ID> --blocks <OTHER_ID>`
- `tracker ticket link <ID> --blocked-by <OTHER_ID>`
- `tracker ticket link <ID> --parent <PARENT_ID>`
- `tracker ticket unlink <ID> <OTHER_ID>`

## Comments and History

- `tracker ticket comment <ID> --body <TEXT>`
- `tracker ticket history <ID>`

## Views

- `tracker board [--view <NAME>]`
- `tracker backlog`
- `tracker next [--actor <ACTOR>] [--view <NAME>]`
- `tracker blocked`
- `tracker queue [--actor <ACTOR>] [--view <NAME>]`
- `tracker review-queue [--actor <ACTOR>]`
- `tracker owner-queue`
- `tracker who`
- `tracker search <QUERY>`
- `tracker search --view <NAME>`
- `tracker render <ID>`

## Saved Views

- `tracker views list`
- `tracker views view <NAME>`
- `tracker views save <NAME> --kind <board|search|queue|next> [--title <TEXT>] [--project <KEY>] [--assignee <ACTOR>] [--type <TYPE>] [--actor <ACTOR>] [--query <QUERY>] [--column <STATUS>] [--queue-category <CATEGORY>]`
- `tracker views delete <NAME>`
- `tracker views run <NAME> [--actor <ACTOR>]`

## Watchers

- `tracker watch list [--actor <ACTOR>]`
- `tracker watch ticket <ID> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch project <KEY> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch view <NAME> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker unwatch ticket <ID> [--actor <ACTOR>]`
- `tracker unwatch project <KEY> [--actor <ACTOR>]`
- `tracker unwatch view <NAME> [--actor <ACTOR>]`

Rules:

- watchers stay stored even if the target ticket, project, or saved view disappears
- `watch list` marks unresolved targets as inactive instead of dropping them
- inactive watchers are ignored during notification audience resolution

## Bulk Operations

- `tracker bulk move <STATUS> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk assign <ACTOR> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk request-review [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk complete [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk claim [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk release [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- `--dry-run` previews the batch without mutating anything
- live bulk mutations require `--yes`
- `--ticket` may be repeated
- `--view` expands any saved board/search/queue/next view into ticket IDs in the same order the view returns them
- duplicate ticket IDs are deduplicated before the batch runs
- every committed per-ticket event carries the same `metadata.batch_id`

## Maintenance

- `tracker sweep`
- `tracker doctor --repair`
- `tracker inspect <ID>`

Rules:

- `doctor` audits orchestration snapshots and derived state in addition to tickets and projection health
- `doctor --json` reports run, gate, handoff, evidence, runtime, and worktree issue counts under `issues.orchestration`
- `doctor --repair` may rebuild projection state and reconcile Git worktree metadata, but it will not recreate missing worktrees, runtime artifacts, or evidence artifacts

## Notify

- `tracker notify send --event-type <TYPE> [--ticket <ID>] [--project <KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker notify log [--limit <N>]`
- `tracker notify dead-letter [--limit <N>]`

## Git

- `tracker git status`
- `tracker git branch-name <ID>`
- `tracker git refs <ID>`
- `tracker git commit <ID> --message <TEXT>`

Rules:

- `tracker git commit` only commits already staged files
- it never auto-stages changes
- it fails in detached HEAD
- it fails when no staged files exist
- it rejects nested repo ambiguity under the workspace root

## Automation

- `tracker automation list`
- `tracker automation view <NAME>`
- `tracker automation create <NAME> --on <EVENT_TYPE> [--on <EVENT_TYPE>] --action <ACTION> [--action <ACTION>] [flags]`
- `tracker automation edit <NAME> --on <EVENT_TYPE> [--on <EVENT_TYPE>] --action <ACTION> [--action <ACTION>] [flags]`
- `tracker automation delete <NAME>`
- `tracker automation dry-run <NAME> --event-type <EVENT_TYPE> [--ticket <ID>] [--actor <ACTOR>]`
- `tracker automation explain <NAME> --event-type <EVENT_TYPE> [--ticket <ID>] [--actor <ACTOR>]`

Supported automation actions:

- `comment:<TEXT>`
- `move:<STATUS>`
- `request_review`
- `notify:<TEXT>`

## Shell Mode

- `tracker shell`

Slash command examples:

- `/project create APP "App Project"`
- `/ticket create --project APP --title "Task" --type task`
- `/ticket move APP-1 in_review --actor agent:builder-1`
- `/ticket history APP-1`
- `/board`
- `/agent eligible APP-1`
- `/dispatch queue`
- `/dispatch run APP-1 --agent builder-1 --actor human:owner`
- `/run launch <RUN-ID> --actor human:owner`
- `/worktree view <RUN-ID>`
- `/approvals`
- `/inbox view gate:<GATE-ID>`
- `/handoff export <HANDOFF-ID>`
- `/evidence view <EVIDENCE-ID>`

## TUI Shortcuts

Once `tracker tui` is running:

- `/` opens the slash command palette
- `b` previews a bulk action against the current ticket list
- `y` applies the last bulk preview
- `n` opens the create-ticket form
- `e` edits the selected ticket
- `m` opens the move prompt
- `s` opens the assign prompt
- `l` opens the link prompt
- `u` opens the unlink prompt
- `c` toggles claim/release for the selected ticket
- `o` opens the comment prompt
- `p` requests review for the selected ticket
- `v` approves the selected ticket
- `x` opens the reject prompt
- `d` completes the selected ticket
- `tab` / `shift+tab` switch tabs
- `j` / `k` or arrow keys move the list cursor
- `enter` opens detail or submits the active dialog
- `esc` cancels the active dialog

TUI tabs:

- `Board`
- `Queues`
- `Detail`
- `Search`
- `Review`
- `Owner`
- `Inbox`
- `Views`
- `Ops`

## Common Flags

Read commands:

- `--pretty`
- `--md`
- `--json`

Mutating commands:

- `--actor <ACTOR>`
- `--reason <TEXT>`

Useful config keys:

- `workflow.completion_mode`
- `actor.default`
- `notifications.terminal`
- `notifications.file_enabled`
- `notifications.file_path`
- `notifications.webhook_url`
- `notifications.webhook_timeout_seconds`
- `notifications.webhook_retries`
- `notifications.delivery_log_path`
- `notifications.dead_letter_path`
