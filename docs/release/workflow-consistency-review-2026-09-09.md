# Workflow consistency follow-up — 2026-09-09

This follow-up investigates the owner's 22 reports against released source
`433138300c49eaa3731822664f0d5c0fded0d397` and fixes the agreed defects on
`codex/fix-workflow-consistency`. It is a local candidate, not a published release.
The original checkout and its existing untracked files are preserved.

## Findings and disposition

| # | Report | Disposition |
|---|---|---|
| 1 | Canceled painted as Done | Fixed. Canonical `canceled` has its own CLI/JSON/TUI/web column and web move option. Card status data uses raw tokens. Canceled still does not satisfy dependencies or count as Done. |
| 2 | Goal brief actor placeholders | Fixed in the shared CLI/MCP service. Briefs derive worker/reviewer identities and shell-quote them; missing identities use named environment variables with setup instructions. |
| 3 | Past schedules accepted | Fixed. Set/replace requires a time strictly after the service clock and rejects before schedule or assignment changes. Existing schedules may become overdue and tick normally. |
| 4 | Silent `human:owner` defaults | Fixed centrally for mutation-flag commands. Explicit actor, `TRACKER_ACTOR`, and `actor.default` use the same precedence; missing identity fails before writes. |
| 5 | Setup and generated-doc inaccuracies | Fixed. README distinguishes init's default-Yes offer from installer default-No; install examples pin v1.12.0; canceled behavior and generated identity guidance are accurate. |
| 6 | Hidden `show` alias | Fixed. Parent ticket help names the accepted alias beside `view`. |
| 7 | Stray `projects/foo` breaks doctor | Fixed discovery. Directories without `project.md` are unmanaged and ignored. Malformed managed projects still fail with their path. Nested init remains refused; no user directory was deleted. |
| 8 | MCP cannot bootstrap | Added explicit startup `--init-if-missing` and `atlas.project.create`. Initialization requires a named absolute existing directory and a write profile, with nested-root and output-symlink preflight. No implicit client-directory initialization. |
| 9 | MCP defaults to read | Retained intentionally. Registration must opt into the authority required by the agent. |
| 10 | Separate MCP registration | Retained intentionally and documented. Repository guidance installation does not silently change client configuration. |
| 11 | CLI-issued high-impact approvals | Retained intentionally. A connected agent cannot mint its own approval for delivery/admin actions. |
| 12 | Missing ticket-maintenance MCP tools | Added heartbeat, priority, label add/remove, and ordinary-field ticket edit through existing audited services. |
| 13 | Strict MCP schemas | Retained intentionally. Exact field names and `additionalProperties: false` reject ambiguous or misspelled writes. MCP ticket creation always requires `type`; CLI alone can derive it from a template. |
| 14 | OpenClaw global-write claim | Fixed. Generated guidance distinguishes repository-local default installation from explicit `openclaw --global` writes. |
| 15 | Absolute temporary paths and skill collision | Fixed. Generated references are workspace-relative. Generic and Grok use separate skill directories; the legacy shared directory is preserved. |
| 16 | Missing no-op/reason teaching | Fixed. Generated target blocks and worker guidance explain same-status no-ops and include reasons in write examples. |
| 17 | Day-one actor/type/reason ceremony | Partly fixed through consistent configured identity and accurate reason guidance. Deliberate typing remains: CLI needs explicit or template-derived type; MCP requires explicit type. Ordinary CLI reasons are recommended, while protected/security operations and tracked MCP writes require them. |
| 18 | Claim takes another assignee's work | Fixed for single and bulk claims with conflict/exit 4, including owner takeover. Review leases retain the reviewer exception; reassignment is explicit. |
| 19 | Assigned backlog invisible | Board invisibility was not reproduced: backlog already appeared in board data. Visible assignment is now added. Plain backlog remains pending agent work until `ready`; newly unblocked dependents retain their existing promotion behavior. |
| 20 | Hidden assignee | Fixed in CLI board, saved-board view, ticket view, terminal rendering, and web card faces. |
| 21 | Index recovery with web open | Not reproduced on the released build. A real web process kept its SQLite handle while a missing source ticket was indexed automatically and while explicit reindex ran; the same process saw both repairs. Existing transactional recovery is retained. |
| 22 | Pair/crossfire bypass review | Fixed. New projects inherit workspace policy; review presets install an effective reviewer and clear project open overrides. Approval checks permission profiles. Owner/reviewer direct completion from `in_progress` fails under the inherited review policy. |

## Review corrections

Round 1 found and corrected two issues in the candidate: the preset migration now
holds the workspace write lock across policy reads and writes, and MCP docs no
longer promise CLI-only template expansion. The concurrency regression verifies
that a stronger concurrent policy update survives, including reviewer and worker
restrictions. Existing customized agent/runbook/profile definitions and explicit
non-open project policy overrides remain unchanged by preset application.
Round 2 closed both findings and found no further actionable issue in its bounded
recheck. The site contract suite separately exposed a stale MCP inventory page;
the page and its expected workflow inventory were updated and the suite passed.

DEC-061 through DEC-067 record the behavior and compatibility decisions, with
links from superseded historical decisions. No new dependencies are introduced.

## Verification

The full required output is captured in [TEST_STDOUT.log](../../TEST_STDOUT.log).
The local candidate was built with Go 1.26.6 on macOS arm64. Results:

| Check | Result |
|---|---|
| Full Go suite, vet, workflow policy, shell parsing, diff whitespace | Passed |
| Web and public-site contracts | 39 passed |
| Installer terminal/unattended harness | 6 passed, including checksum refusal and cancellation |
| Actual MCP stdio harness | Both framings, all profile inventories, read-profile write denial, explicit workspace bootstrap, project create, tracked maintenance writes, and actor-separated workflow passed |
| Stabilization | Race suite for service/CLI/TUI and all five repository fuzz targets passed |
| Vulnerability scan | No reachable or imported-package vulnerabilities; two findings in required modules whose affected code Atlas does not call |
| Candidate secret scans | Complete candidate ancestry and exported tracked/unignored candidate contents passed |
| SBOM and generated examples | Candidate-binary SBOM generated; normalized examples regenerated and freshness check passed |
| Live browser | Web move form moved APP-7 to `canceled`; CLI confirmed APP-7/APP-8 in Canceled and APP-6 in Done. Visible assignment, canceled move option, desktop 1440×900 and phone 390×844 rendering passed; phone page had no horizontal overflow |
| Live index recovery | Released and fixed binaries both passed automatic repair and explicit reindex while a real web process retained its SQLite handle |

The broader scan of every local branch found one `fileToken` entry in
`docs/design/pen/atlas-tasker.pen` at commit `4282c81`. That file and commit are
outside this candidate's ancestry and occur only on unrelated unmerged local
branches. No allowlist, unrelated-branch edit, or history rewrite was added.
Shared browser-console output was not tab-scoped, so it is not used to claim a
clean application console; the page's six own static resources returned HTTP 200.
Hosted Linux CI and release publication were not performed for this local fix.

Key regression coverage includes
`TestMutationActorResolutionIsConsistent`,
`TestScheduleRejectsNonfutureTimeWithoutChangingTicket`,
`TestClaimWrongAssigneeReturnsExitFour`,
`TestReviewTeamPresetRequiresTheActualReviewLifecycle`,
`TestTeamPresetMigrationDoesNotLoseConcurrentPolicyWrite`,
`TestMCPBootstrapPreflightsAllOutputsBeforeWriting`,
`TestProjectAndTicketWorkflowMutationTools`, and
`TestBoardAndSavedViewKeepAssignedBacklogAndCanceledVisible`.

[Structured live evidence](workflow-consistency/verification.json),
[desktop capture](workflow-consistency/board-desktop.png), and
[phone capture](workflow-consistency/board-mobile.png) contain only synthetic data.

The live-index probe covers count-detectable source drift and explicit reindex
with a healthy database open in another process. It does not establish support
for externally deleting/replacing an open SQLite file, same-count manual source
edits, or every possible filesystem/lock failure. Those limits remain in DEC-052.

## Handoff and cleanup

The fix checkout, runnable local candidate, screenshots, and review/test evidence
are retained. The synthetic web server stopped and only this task's browser tab
closed. Cleanup removed 11 verified disposable artifacts totaling 73,125,843
logical bytes; this is not a claim about physical SSD space recovered.
Automatic approval review rejected deletion of `.gstack` because exclusive
ownership was uncertain, so that directory was retained. The original checkout
remains at `9c82db39` with its pre-existing `.playwright-mcp/` and `tracker` files.
No push, PR, tag, deployment, or installed-binary replacement was performed.
