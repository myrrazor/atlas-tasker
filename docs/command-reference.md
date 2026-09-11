# Atlas Tasker Command Reference

## Top-Level

- `tracker init [--integrations|--skip-integrations]`
- `tracker setup [--plan|--yes] [--agents <list|all>] [--mode guidance|managed|disabled] [--team <POLICY>] [--backup] [--backup-target <ID>]`
- `tracker setup status`
- `tracker setup repair [--yes]`
- `tracker help`
- `tracker doctor [--repair]`
- `tracker reindex`
- `tracker inspect <ID> [--actor <ACTOR>]`
- `tracker schedule set <ID> --at <RFC3339> --runner <ACTOR> --actor <ACTOR> --reason <TEXT>`
- `tracker schedule clear <ID> --actor <ACTOR> --reason <TEXT>`
- `tracker schedule list [--project <KEY>] [--from <RFC3339>] [--to <RFC3339>]`
- `tracker schedule history [--project <KEY>] [--from <RFC3339>] [--to <RFC3339>]`
- `tracker schedule tick [--now <RFC3339>] --actor <ACTOR> --reason <TEXT>`
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
- `tracker bulk move <STATUS> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--override-deps] [--actor <ACTOR>]`
- `tracker bulk assign <ACTOR> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk request-review [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--override-deps] [--actor <ACTOR>]`
- `tracker bulk complete [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--override-deps] [--actor <ACTOR>]`
- `tracker bulk claim [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk release [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker templates list`
- `tracker templates view <NAME>`
- `tracker integrations detect [--json]`
- `tracker integrations status [--json]`
- `tracker integrations repair <target> [--yes]`
- `tracker integrations disconnect <target> [--yes]`
- `tracker integrations install [codex|claude|openclaw|cursor|grok|generic] [--force] [--targets <list>] [--global]`
- `tracker integrations install` with no target opens an interactive multi-select when stdin/stdout are a TTY
- `tracker web serve [--host 127.0.0.1] [--port 0] [--project <KEY>] [--actor <ACTOR>] [--open|--no-browser] [--read-only]`
- `tracker web open`
- `tracker web status [--pretty|--md|--json]`
- `tracker update [--check|--dry-run|--yes] [--force] [--version <TAG>] [--skip-attestations] [--json]`
- `tracker version [--json]`
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
- `tracker key list`
- `tracker key view <KEY-ID>`
- `tracker key generate [--scope <workspace|collaborator|admin|release>] [--owner-id <ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key export-public <KEY-ID>`
- `tracker key import-public <PATH> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key rotate <KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key revoke <KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key verify <KEY-ID>`
- `tracker trust status`
- `tracker trust list`
- `tracker trust collaborator <COLLABORATOR-ID>`
- `tracker trust bind-key <COLLABORATOR-ID> <PUBLIC-KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker trust revoke-key <PUBLIC-KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker trust explain <TARGET>`
- `tracker governance pack list`
- `tracker governance pack view <PACK-ID>`
- `tracker governance pack create <NAME> [--scope <SCOPE>] [--protected-action <ACTION>]... [--required-signatures <N>] [--quorum-count <N>] [--quorum-role <ROLE>]... [--separation-event <EVENT>]... [--allow-owner-override] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker governance pack apply <PACK-ID> [--scope <SCOPE>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker governance validate`
- `tracker governance explain <TARGET> [--action <ACTION>] [--actor <ACTOR>] [--reason <TEXT>] [--approval-actor <ACTOR>]... [--trusted-signatures <N>]`
- `tracker governance simulate <ACTION> [--ticket <ID>] [--run <ID>] [--change <ID>] [--gate <ID>] [--actor <ACTOR>] [--reason <TEXT>] [--approval-actor <ACTOR>]... [--trusted-signatures <N>]`
- `tracker classify list [--project <KEY>]`
- `tracker classify get <ENTITY>`
- `tracker classify set <ENTITY> <public|internal|confidential|restricted> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker classify explain <ENTITY>`
- `tracker redact preview [--scope <SCOPE>] [--target <export|sync|audit|backup|goal>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker redact export [--scope <SCOPE>] --preview-id <PREVIEW-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker redact verify <BUNDLE-ID|PATH>`
- `tracker backup create [--scope <workspace|project:KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup list`
- `tracker backup view <BACKUP-ID>`
- `tracker backup verify <BACKUP-ID|PATH>`
- `tracker backup restore-plan <BACKUP-ID|PATH> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup restore-apply <BACKUP-ID|PATH> --yes [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup drill`
- `tracker admin security-status`
- `tracker admin trust-store`
- `tracker admin recovery-status`
- `tracker goal brief <TICKET-ID|RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker goal manifest <TICKET-ID|RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker goal verify <MANIFEST-ID|PATH>`

Setup and update behavior:

- `tracker setup --plan` inspects the workspace and detected agents and writes nothing; `--json` prints only JSON
- `tracker setup --yes` applies the plan; it is not consent for backup or for unnamed machine-wide scopes such as OpenClaw
- OpenClaw and other machine-wide targets must be named with `--agents`; generic is never auto-selected
- `--backup` / `--backup-target` are a separate consent group; Sprint 114.1 records the request and does not write backup state (AT114-501/505)
- `--mode delivery` is refused; enable delivery as a separate power-user action
- `--team` records the requested policy and does not overwrite existing agent roles (AT114-209)
- Re-running setup is a no-op when the workspace is already current; interactive cancel and EOF write nothing
- `tracker setup status` and `tracker integrations status` report skill/block versions, workspace binding, and repair reasons
- `tracker integrations repair <target>` refreshes drifted Atlas-owned files; `disconnect` removes only matching Atlas-owned entries and requires confirmation after manual edits
- The shell installer may offer `tracker setup` after an explicit TTY yes; unattended install never initializes the current directory
- plain `tracker init` can offer the six-target integration picker only when stdin and stdout are TTYs; `--skip-integrations` suppresses it
- `tracker integrations install` accepts `claude`, `codex`, `cursor`, `openclaw`, `grok`, and `generic`; scripts should pass one target or `--targets <list>`
- `none` and `q` leave integration installation skipped; JSON and non-TTY invocations never prompt
- integration installation writes project guidance and skills but does not register an MCP client
- `tracker update --check` and `--dry-run` do not replace the binary; applying an update requires `--yes`
- update apply verifies `checksums.txt` and, unless explicitly skipped, a GitHub build attestation before replacing the current executable
- `tracker update` replaces only the binary; rerun the desired integration install to refresh generated skill files

See [coding-agent integrations](guides/agent-integrations.md) and [updating](guides/updating.md).

## Agents

- `tracker agent list`
- `tracker agent view <AGENT-ID>`
- `tracker agent create <AGENT-ID> --name <NAME> --provider <PROVIDER> [flags]`
- `tracker agent edit <AGENT-ID> [flags]`
- `tracker agent enable <AGENT-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker agent disable <AGENT-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker agent eligible <TICKET-ID>`
- `tracker agent available [AGENT-ID] [--actor <ACTOR>]`
- `tracker agent pending [AGENT-ID] [--actor <ACTOR>]`
- `tracker agent wakeups list [AGENT-ID]`
- `tracker agent wakeups view <WAKEUP-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker agent wakeups ack <WAKEUP-ID> --actor <ACTOR> --reason <TEXT>`
- `tracker agent auto status <AGENT-ID>`
- `tracker agent auto set <AGENT-ID> --mode notify|command [--argv <ARG> ...] --actor human:owner --reason <TEXT>`
- `tracker agent auto off <AGENT-ID> --actor human:owner --reason <TEXT>`

Behavior:
- agent profiles live under `.tracker/agents/`
- eligibility is deterministic and returns the same ranking order used later by dispatch
- disabled agents and capability mismatches are reported explicitly in JSON mode
- `available` lists tickets an agent can start, continue, review, promote, or complete now; `promote` is a backlog ticket whose blockers are all `done`, and its first suggested command is the `ticket move <ID> ready`
- `pending` lists relevant tickets that are blocked by dependencies, review, owner gates, claims, capacity, or policy; `not_ready_status` means backlog that never had blockers or someone else's `in_progress` work; a backlog ticket whose blockers are all `done` is listed under `available` as `promote` instead
- wake-ups are event-driven records created under `.tracker/runtime/agent-wakeups/` when a `done` ticket unblocks assigned agent work; a `backlog` dependent assigned to an agent is promoted to `ready` first (a `ticket.moved` by `agent:atlas`), a hand-set `blocked` one is only woken, and a failed promotion still leaves the wake-up with `promoted=false` and `promotion_error` in its metadata (a move that died after its canonical write leaves the wake-up `failed` and pointing at `tracker doctor --repair`, which replays the move)
- scheduled wake-ups use the same store and command path; `{scheduled_at}` expands to the UTC due instant
- `agent.work_available` events use reserved actor `agent:atlas`
- auto mode defaults to `notify`; command mode stores argv items and refuses shell interpreters

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
- `--agent` accepts `builder-1` or `agent:builder-1`; the run stores the bare agent id
- an agent can self-dispatch its own eligible work without project membership, but cross-agent dispatch still uses membership and permission policies
- one active run per ticket is the default; parallel dispatch requires `allow_parallel_runs=true`
- `run attach` is idempotent for the same provider/session pair
- `run open` is read-only and reports the canonical runtime, evidence, and worktree paths; if `needs_launch=true`, run `tracker run launch <RUN-ID> --actor <ACTOR> --reason "prepare launch files"` before handing the files to an agent
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

## Security Keys And Trust

- `tracker key list`
- `tracker key view <KEY-ID>`
- `tracker key generate [--scope <workspace|collaborator|admin|release>] [--owner-id <ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key export-public <KEY-ID>`
- `tracker key import-public <PATH> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key rotate <KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key revoke <KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker key verify <KEY-ID>`
- `tracker trust status`
- `tracker trust list`
- `tracker trust collaborator <COLLABORATOR-ID>`
- `tracker trust bind-key <COLLABORATOR-ID> <PUBLIC-KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker trust revoke-key <PUBLIC-KEY-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker trust explain <TARGET>`

Rules:

- local signing keys use Ed25519 private material under `.tracker/security/keys/private/` with Unix mode `0600`; unsupported permission semantics are reported as unverified instead of silently trusted
- public key records and revocations are syncable, but trust bindings are local-only
- imported public keys stay untrusted until `trust bind-key` records a local trust decision
- `key export-public` never exports private key bytes; private-key export is intentionally absent in v1.7
- rotated and revoked keys cannot sign new artifacts, but old signatures still return deterministic verification states

## Sign And Verify

- `tracker sign bundle <BUNDLE-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign sync-publication <BUNDLE-ID|PATH> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign approval <GATE-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign handoff <HANDOFF-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign evidence <EVIDENCE-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign audit <AUDIT-REPORT-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign audit-packet <PACKET-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign backup <BACKUP-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker sign goal <MANIFEST-ID> [--signing-key <KEY-ID>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker verify bundle <BUNDLE-ID|PATH>`
- `tracker verify sync-publication <BUNDLE-ID|PATH>`
- `tracker verify approval <GATE-ID>`
- `tracker verify handoff <HANDOFF-ID>`
- `tracker verify evidence <EVIDENCE-ID>`
- `tracker verify audit <REPORT-ID|PATH>`
- `tracker verify audit-packet <PACKET-ID|PATH>`
- `tracker verify backup <BACKUP-ID|PATH>`
- `tracker verify goal <MANIFEST-ID|PATH>`

Rules:

- signing first verifies artifact integrity, then signs an artifact-bound canonical payload
- signature envelopes are stored under `.tracker/security/signatures/`; export bundles also get an adjacent `<bundle>.signatures.json` sidecar so copied artifacts can verify by path
- sync publications store signatures in the matching publication metadata; directory-level `publication.json` is only used when it names the requested archive
- approval, handoff, and evidence signatures are stored as standalone signature envelopes and do not rewrite the source artifact
- backup signatures are embedded in the local backup snapshot record after integrity verification; copied archive verification reports archive integrity and `missing_signature` unless the local snapshot record is present
- goal signatures are embedded in the local goal manifest record and verify the stored manifest snapshot, not current live policy meaning
- verification is pure by default and returns `missing_signature` for unsigned artifacts

## Classification And Redaction

- `tracker classify list [--project <KEY>]`
- `tracker classify get <ENTITY>`
- `tracker classify set <ENTITY> <public|internal|confidential|restricted> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker classify explain <ENTITY>`
- `tracker redact preview [--scope <SCOPE>] [--target <export|sync|audit|backup|goal>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker redact export [--scope <SCOPE>] --preview-id <PREVIEW-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker redact verify <BUNDLE-ID|PATH>`

Rules:

- classification entities are `workspace` or `kind:id`, for example `project:APP`, `ticket:APP-1`, `run:run_1`, `evidence:evidence_1`, or `handoff:handoff_1`
- explicit labels are stored under `.tracker/classification/labels/` using collision-resistant `class-<slug>-<hash>.md` filenames; workspace default is `internal`
- project and ticket labels inherit downward, and higher sensitivity wins over lower child labels
- legacy `protected` or `sensitive` ticket flags still contribute `restricted`
- redaction previews are local actor-bound records under `.tracker/redaction/previews/` with Unix mode `0600`
- previews are single-use and bound to target, actor, source hash, policy hash, classification hash, command target, recomputed items, and a 10-minute TTL
- redacted workspace exports omit restricted files, restricted ticket- or run-owned gate/change/check/classification metadata, and extra files under restricted project directories, always omit `.tracker/events/` history, and write `redaction_preview_id` into both the bundle record and artifact manifest
- built-in defaults stay active per redaction target unless a custom stored rule exists for that same target
- redacted exports enforce the same `export_create` governance policies as normal exports
- export redaction only supports `omit`; stored `mask`, `hash`, or marker export rules fail closed
- redacted artifact verification checks bundle integrity, confirms the preview binding, and fails if omitted preview paths are present

## Audit Reports

- `tracker audit report [--scope <SCOPE>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker audit list`
- `tracker audit view <REPORT-ID>`
- `tracker audit export <REPORT-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker audit verify <REPORT-ID|PATH>`
- `tracker audit explain-policy <EVENT-UID>`

Rules:

- scopes are `workspace`, `project:<KEY>`, `ticket:<ID>`, `run:<ID>`, `change:<ID>`, `release:<ID>`, or `incident:<ID>`
- report creation records `audit.report.created`; packet export records `audit.report.exported`
- scoped packet exports are recorded in the scoped project event stream when Atlas can resolve one
- report verification and packet verification are read-only and check the snapshot artifact, not current workspace meaning
- packet verification recomputes `packet_hash` from the canonical report payload and reports `packet_hash_mismatch` on tampering
- policy explanation requires `event_uid`; numeric `event_id` values are project-scoped and intentionally rejected
- policy explanation loads the target event and current local policy context; historical reports bind the exact `policy_snapshot_hash`

## Backups And Recovery

- `tracker backup create [--scope <workspace|project:KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup list`
- `tracker backup view <BACKUP-ID>`
- `tracker backup verify <BACKUP-ID|PATH>`
- `tracker backup restore-plan <BACKUP-ID|PATH> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup restore-apply <BACKUP-ID|PATH> --yes [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker backup drill`
- `tracker admin security-status`
- `tracker admin trust-store`
- `tracker admin recovery-status`

Rules:

- backup snapshots include canonical Atlas-owned data only; private keys, local trust decisions, redaction previews, backup snapshots, generated goal files, runtime/worktree/provider state, remotes, notifiers, and MCP approvals are excluded
- backup records and manifests live under `.tracker/backups/manifests/`; archives live under `.tracker/backups/snapshots/`
- `backup restore-plan` is side-effect free and does not persist a plan or append an event
- `backup restore-apply` recomputes the plan under the write lock, requires `--yes`, and writes only paths on the restore allowlist
- `backup drill` is read-only and reports recovery warnings without mutating the workspace
- admin diagnostics are read-only and never print private key material

## Goal Manifests

- `tracker goal brief <TICKET-ID|RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker goal manifest <TICKET-ID|RUN-ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker goal verify <MANIFEST-ID|PATH>`

Rules:

- goal briefs are pure derived output for agent handoff; optional actor/reason flags are accepted for copy-paste parity and do not create an event
- goal manifests write local derived artifacts under `.tracker/goal/manifests/` and require actor/reason because Atlas records the manifest creation event
- goal markdown uses the ticket title as the H1, puts ticket/run context in `Ticket / Run`, and uses the stable section order from `docs/goal-manifests.md`
- manifests bind `policy_snapshot_hash`, `trust_snapshot_hash`, and `source_hash`
- verification checks the stored manifest snapshot and signatures, not current live policy meaning

## Governance

- `tracker governance pack list`
- `tracker governance pack view <PACK-ID>`
- `tracker governance pack create <NAME> [--scope <SCOPE>] [--protected-action <ACTION>]... [--required-signatures <N>] [--quorum-count <N>] [--quorum-role <ROLE>]... [--separation-event <EVENT>]... [--allow-owner-override] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker governance pack apply <PACK-ID> [--scope <SCOPE>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker governance validate`
- `tracker governance explain <TARGET> [--action <ACTION>] [--actor <ACTOR>] [--reason <TEXT>] [--approval-actor <ACTOR>]... [--trusted-signatures <N>]`
- `tracker governance simulate <ACTION> [--ticket <ID>] [--run <ID>] [--change <ID>] [--gate <ID>] [--actor <ACTOR>] [--reason <TEXT>] [--approval-actor <ACTOR>]... [--trusted-signatures <N>]`

Rules:

- governance packs and applied policies are TOML files under `.tracker/governance/`
- governance TOML uses the same snake_case field names as JSON output, and `tracker governance validate` emits a structured report plus non-zero exit when any pack or policy is invalid
- all protected write paths use one evaluator after legacy permission checks and before live side effects
- gate rejection is not governed by `gate_approve`; `gate_approve` protects gate approval, while `gate_waive` protects waiver
- `ticket_approve` is a protected action, so opt-in governance separation-of-duties and override policies can guard reviewer approval before completion.
- ticket-level `ticket approve` is a convenience path for the effective reviewer or `human:owner`; when no reviewer is configured, the assignee or active worker may approve their own work by default. Reviewer quorum workflows should bind collaborators to a project reviewer membership and resolve review gates with `tracker gate approve <GATE-ID> --actor <ACTOR> --reason <TEXT>` so approvals are auditable.
- trusted-signature requirements are not bypassed by owner override
- structured CSV/GitHub import apply uses `import_apply`; signed sync/bundle imports use `sync_import_apply` and `bundle_import_apply`
- sync export/import governance runs before migration scaffolding writes, so denied operations do not stamp migration state
- `explain` and `simulate` accept `--reason` so reason-required owner overrides can be modeled before a mutation
- trusted-signature requirements are accepted for artifact import actions with real signature evidence: `bundle_import_apply` and `sync_import_apply`
- duplicate envelopes from the same trusted signer count once toward trusted-signature requirements
- quorum rules with `require_trusted_signatures` count distinct trusted signer identities instead of gate approval actors
- classification-scoped governance uses the exact effective inherited classification level; redaction rules use the ordered hierarchy
- remote sync pulls enforce `sync_import_apply` once and do not also require manual `bundle_import_apply` policy
- denied remote sync pulls do not promote fetched publications or Git fetch caches into the durable sync mirror
- project-filtered archive apply/restore evaluates project-scoped governance policies
- applying a pack to multiple scopes creates scope-bound applied policy ids instead of overwriting the earlier scope
- quorum counts root collaborator identities at action time; suspended or removed collaborators' old approvals stay historical but do not satisfy active quorum
- owner overrides require an explicit policy rule on every failed matching policy, must also satisfy matching `owner_override` policies, and record `governance.override.recorded` only after the protected mutation succeeds

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
- Atlas bundle export includes active governance packs and applied policies under `.tracker/governance/`
- `export verify` works by bundle id or direct archive path and validates manifest membership plus per-file checksums
- direct-path verification reports missing sidecars with structured reason strings such as `sidecar_manifest_missing:<path>` or `sidecar_checksum_missing:<path>` instead of raw filesystem errors
- Atlas bundle import is snapshot-first: it restores canonical markdown snapshots into the target workspace, but it does not copy the source workspace's `.tracker/events/` files into the live target workspace
- structured Jira CSV and GitHub JSON imports are create-only in v1.5; existing ticket ids are reported as conflicts during preview and block apply
- GitHub JSON import is metadata-link import only; it creates Atlas tickets and preserves the external source URL as import provenance
- Atlas bundle import rejects path traversal and staged-copy conflicts before canonical writes land

## Evidence

- `tracker evidence list <RUN-ID>`
- `tracker evidence view <EVIDENCE-ID>`
- `tracker run checkpoint <RUN-ID> [--title <TEXT>] [--body <TEXT>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker run evidence list <RUN-ID>`
- `tracker run evidence add <RUN-ID> --type <note|test_result|file_diff_summary|log_excerpt|screenshot|artifact_ref|commit_ref|manual_assertion|unresolved_question|review_checklist> [--title <TEXT>] [--body <TEXT>] [--artifact <PATH>] [--actor <ACTOR>] [--reason <TEXT>]`
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
- handoff creation records a handoff packet; explicit review gates remain visible through `tracker gate list` and `tracker inbox`

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

## Local Web Board

- `tracker web serve [--host 127.0.0.1] [--port 0] [--project <KEY>] [--actor <ACTOR>] [--open|--no-browser] [--read-only]`
- `tracker web open`
- `tracker web status [--pretty|--md|--json]`

Rules:

- `serve` binds to loopback by default and chooses a random free port when `--port 0` is used
- non-loopback hosts are rejected; use a separately authenticated and TLS-protected product if remote access is required
- runtime status is written without secrets under `.tracker/runtime/web/server.json`
- browser mutations use the same `ActionService` paths as CLI mutations and record `surface: "web"`
- descriptions and comments are escaped text in v1.10; raw Markdown HTML is not rendered

## Project

- `tracker project create <KEY> <NAME>`
- `tracker project list`
- `tracker project view <KEY>`
- `tracker project policy get <KEY>`
- `tracker project policy set <KEY> [flags]`

Project keys are path-derived identifiers and must match `^[A-Z][A-Z0-9_-]{0,31}$`. Atlas rejects slashes, dots, whitespace, shell home markers, control characters, and lowercase project keys instead of normalizing them into paths.

Template names are path-derived identifiers under `.tracker/templates/` and must match `^[A-Za-z][A-Za-z0-9_-]{0,63}$`.

## Ticket CRUD

- `tracker ticket create --project <KEY> --title <TEXT> --type <epic|task|bug|subtask> [--template <NAME>] [flags]`
- `tracker ticket view <ID>` (alias: `show`)
- `tracker ticket edit <ID> [flags] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket archive <ID>` (`ticket delete` is kept as a compatibility alias)
- `tracker ticket list [--project <KEY>] [--status <STATUS>] [--assignee <ACTOR>] [--type <TYPE>]`

Ticket IDs are path-derived and must match `^[A-Za-z][A-Za-z0-9_-]{0,63}$`. Ticket titles are normalized for terminal display: layout controls, C0/C1 controls, and bidirectional override codepoints are removed or flattened before they can affect board, queue, TUI, or Markdown rendering.

## Ticket Mutation

- `tracker ticket move <ID> <STATUS> [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket assign <ID> <ACTOR> [--actor <ACTOR>] [--reason <TEXT>]` sets the assignee only
- `tracker ticket priority <ID> <PRIORITY> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket label add <ID> <LABEL> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket label remove <ID> <LABEL> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket claim <ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket release <ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket heartbeat <ID> [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket request-review <ID> [--reviewer <ACTOR>] [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `ticket request-review` now opens or reuses a review gate for the ticket so `gate list`, `approvals`, and `inbox` show the review work explicitly. `--reviewer` sets the ticket reviewer in the same mutation; when omitted, Atlas uses the ticket reviewer or effective workspace/project/epic/ticket `required_reviewer`.
- `tracker ticket approve <ID> [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket reject <ID> --reason <TEXT> [--actor <ACTOR>]`
- `tracker ticket complete <ID> [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker ticket policy get <ID>`
- `tracker ticket policy set <ID> [flags]`

`ticket approve` records approval and, in `review_gate` mode, also moves the ticket to `done` in the
same mutation. In `open`, `owner_gate`, and `dual_gate` modes, the approved ticket remains
`in_review` until an actor allowed by the active completion policy runs `ticket complete`.

## Scheduled Work

- `tracker schedule set <ID> --at <RFC3339> --runner <ACTOR> --actor <ACTOR> --reason <TEXT>`
- `tracker schedule clear <ID> --actor <ACTOR> --reason <TEXT>`
- `tracker schedule list [--project <KEY>] [--from <RFC3339>] [--to <RFC3339>]`
- `tracker schedule history [--project <KEY>] [--from <RFC3339>] [--to <RFC3339>]`
- `tracker schedule tick [--now <RFC3339>] --actor <ACTOR> --reason <TEXT>`

`set` creates or replaces a one-time schedule and makes `--runner` the ticket assignee. Human runners receive the normal `ticket.schedule_triggered` notification when the schedule is ticked. Agent runners must reference an enabled agent profile; Atlas creates an agent wakeup and launches the configured argv only when that profile uses `agent auto` command mode. The default notify mode leaves a pending wakeup for explicit pickup.

The new `--at` instant must be strictly in the future. A past or current instant fails with exit 2 before the schedule or assignee changes. Existing schedules may become overdue and can then be processed by `tick`.

`tick` is a one-shot, idempotent command. Run it from cron, launchd, or another scheduler; Atlas does not start a background daemon. A failed agent launch is recorded once as `ticket.schedule_failed` and does not retry until the ticket is rescheduled. `history` derives completion time and actor from existing immutable done events rather than maintaining a second completion record.

## Relationships

- `tracker ticket link <ID> --blocks <OTHER_ID>`
- `tracker ticket link <ID> --blocked-by <OTHER_ID>`
- `tracker ticket link <ID> --parent <PARENT_ID>`
- `tracker ticket unlink <ID> <OTHER_ID>`

Dependency rules:

- `blocked_by` is enforced for unsafe progress: `in_progress`, `in_review`, approval, and completion are rejected while any blocker is unresolved.
- Only `done` counts as terminal-success for dependency unblocking. `canceled` does not unblock dependents.
- Canceled work appears in a separate board column, retaining `canceled` in JSON and web card status data. Board and ticket views show assignment directly. An ordinary assigned backlog ticket remains visible but is pending agent work until promoted to `ready`.
- A non-review claim on another assignee's ticket fails with conflict (exit 4), including owner claims. Reassign explicitly first. Review leases still belong to the authorized reviewer while the worker remains the ticket assignee.
- `human:owner` can override unresolved dependencies with `--override-deps --reason <TEXT>`; the mutation event includes a `dependency_override` payload with the unresolved blockers.
- Board, blocked list, ticket view, inspect, and reindex derive blocked buckets from current blocker status, not only the historical link.

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

Queue categories, in the order `next` walks them: `ready_for_me`, `unblocked_for_me` (backlog tickets whose blockers are all `done` but that nobody moved to `ready` yet), `claimed_by_me`, `needs_review`, `awaiting_owner`, `blocked_for_me`, `stale_claims`, `policy_violations`. Backlog that never had blockers is not queued.

Search query terms:

- `status=<STATUS>`
- `type=<TYPE>`
- `project=<KEY>`
- `assignee=<ACTOR>`
- `label=<LABEL>`
- `text~<TEXT>`; multi-word values can be written as `text~logout flow` or `text~"logout flow"`

Examples:

- `tracker search 'status=in_progress'`
- `tracker search 'project=AUTH text~logout flow'`
- `tracker search 'text~"scenario 1000"'`

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

- `tracker bulk move <STATUS> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk assign <ACTOR> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk request-review [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk complete [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--override-deps] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk claim [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk release [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- `--dry-run` previews the batch without mutating anything
- live bulk mutations require `--yes`
- dry-run result text uses "would ..." language; live apply output uses applied-state verbs such as "moved", "updated", "completed", or "failed"
- `--ticket` may be repeated
- `--view` expands any saved board/search/queue/next view into ticket IDs in the same order the view returns them
- duplicate ticket IDs are deduplicated before the batch runs
- `--override-deps` is owner-only and only applies to unsafe `move`, `request-review`, and `complete` batches
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

## MCP Adapter

- `tracker mcp serve [--workspace <ABSOLUTE-PATH>] [--init-if-missing] [--workspace-from-cwd --expected-workspace-id <ID>] [--tool-profile read|workflow|delivery|admin] [--read-only] [--dangerously-allow-high-impact-tools]`
- `tracker mcp schema --json [--tool-profile <PROFILE>]`
- `tracker mcp tools --json [--tool-profile <PROFILE>]`
- `tracker mcp approve-operation --operation <TOOL> --target <ID> --actor <ACTOR> --reason <TEXT> [--ttl 10m]`
- `tracker mcp approvals list --json`
- `tracker mcp approvals revoke <APPROVAL-ID>`

Default MCP setup uses `--tool-profile read`. The exact visible counts are read 41, workflow 73,
delivery 77 (79 with `--dangerously-allow-high-impact-tools`), and admin 77 (the full 88 with the
flag). High-impact tools require both a profile that includes the tool and the danger flag; execution
still requires a one-time approval created outside MCP.

`--workspace-from-cwd` resolves the nearest Atlas root from the current directory and requires
`--expected-workspace-id`. It never initializes, never follows a registry path as a fallback, and
refuses nested workspaces, symlink substitution, a wrong ID, a replaced directory, and a copied
workspace that still has a stale registration. Use it only when a provider cannot safely carry an
absolute machine path. `--workspace` remains the binding for providers that can. The two flags are
mutually exclusive with each other and with `--init-if-missing`. If `--workspace` is also given
`--expected-workspace-id`, Atlas verifies the ID and still does not fall back to another workspace.

`--init-if-missing` is an opt-in server-startup bootstrap. It requires `--workspace` to be an
explicit absolute path to an existing directory and requires a write-capable workflow, delivery, or
admin profile. It is noninteractive, refuses nested Atlas workspaces, `--read-only`, and redirected
initialization outputs, and does not register MCP or install agent integrations. Without the flag,
an uninitialized workspace still fails at startup.

The six ordinary workflow additions are `atlas.project.create`, `atlas.ticket.heartbeat`,
`atlas.ticket.priority`, `atlas.ticket.label.add`, `atlas.ticket.label.remove`, and
`atlas.ticket.edit`. Project create accepts only `key` and `name`; as an untracked container action,
it takes no actor/reason and records no event. The five ticket tools require actor and reason. Edit
accepts optional `title`, `description`, `acceptance`, `priority`, `labels`, `assignee`, and
`reviewer`; omitted fields are preserved, explicit empty values clear supported fields, and status
or policy cannot be changed. Description input passes unchanged from the adapter to existing
Markdown storage, whose normal boundary-whitespace formatting still applies. All MCP schemas reject
unknown fields (`additionalProperties: false`), and tool/argument names have no silent aliases.
`atlas.ticket.create` always requires `type`; MCP stores a supplied template name without applying
template defaults. The CLI `tracker ticket create` command may derive type from its selected template.

See [MCP adapter](mcp.md), [MCP security](mcp-security.md), and [MCP tools](mcp-tools.md).

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

`--json` is also on every write command, so agents can script mutations without parsing text. The only leaves without it are `shell`, `tui`, `mcp serve`, `web serve`, and `web open`, which own their output for other reasons.

Mutating commands:

- `--actor <ACTOR>`
- `--reason <TEXT>`

For tracked CLI mutations, an explicit `--actor` wins, followed by `TRACKER_ACTOR`, then `actor.default`.
There is no implicit `human:owner` fallback. Supply `--reason` for an auditable explanation; Atlas
requires it for security-sensitive, protected, scheduling, and other explicitly guarded actions.

Useful config keys:

- `workflow.completion_mode`
- `workflow.required_reviewer`
- `actor.default`
- `web.owner_name`
- `web.lang` (`en`, `es`, `id`, `zh`, `ja`, or `ko`; blank uses the browser language)
- `web.agent_colors.<agent>` (`claude=orange` and `codex=blue` by default; unknown color names render uncolored)
- `notifications.terminal`
- `notifications.file_enabled`
- `notifications.file_path`
- `notifications.webhook_url`
- `notifications.webhook_timeout_seconds`
- `notifications.webhook_retries`
- `notifications.delivery_log_path`
- `notifications.dead_letter_path`

New projects inherit the workspace completion mode and required reviewer unless project policy
overrides them. `team apply pair` and `team apply crossfire` set the workspace reviewer to
`agent:reviewer-1`; `team apply swarm` sets it to `agent:qa-1`. Applying one of these review presets
clears an older project's explicit `open` completion override so it inherits the review gate, while
preserving stricter completion modes and custom reviewer overrides.

## Version Metadata

`tracker version` prints release metadata in text form:

```text
tracker v1.13.0
commit: abc123
build date: 2026-09-09T12:00:00Z
go: go1.26.6
platform: darwin/arm64
```

`tracker version --json` has a stable release-proof shape:

```json
{
  "format_version": "v1",
  "kind": "tracker_version",
  "version": "v1.13.0",
  "commit": "abc123",
  "build_date": "2026-09-09T12:00:00Z",
  "go_version": "go1.26.6",
  "platform": "darwin/arm64"
}
```

Source builds that are not stamped by release scripts return `version: "dev"`, `commit: "unknown"`, and `build_date: "unknown"`.
