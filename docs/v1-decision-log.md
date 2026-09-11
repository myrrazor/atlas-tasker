# Atlas Tasker v1 Decision Log

This file captures planning and implementation decisions for Atlas Tasker v1 so future changes can be made with full context.

## DEC-001

1. **Decision ID:** DEC-001
2. **Date:** 2026-03-21
3. **Question:** What is the canonical implementation target for this v1 effort?
4. **Options Considered:**
   - Implement in the current local scaffold repository.
   - Implement in `myrrazor/atlas-tasker`.
5. **Chosen Option:** Implement in `myrrazor/atlas-tasker`.
6. **Why We Chose It:** The owner explicitly directed implementation to this repository.
7. **Confidence:** high
8. **Revisit Trigger:** Repository ownership or source-of-truth project changes.
9. **Affected PRs/Files:** PR-001..PR-009; all repository paths.

## DEC-002

Superseded for ongoing release work by DEC-059. The original main-targeted v1 sequence is retained here as history; the owner's current dev/testing/main policy governs new release branches.

1. **Decision ID:** DEC-002
2. **Date:** 2026-03-21
3. **Question:** What branch/PR flow should this v1 execution use now?
4. **Options Considered:**
   - Main-only integration flow.
   - Bootstrap `dev/testing/main` flow first.
   - Hybrid transitional flow.
5. **Chosen Option:** Use `main` as the current target branch.
6. **Why We Chose It:** The current repository structure and requested execution flow are main-targeted for this phase.
7. **Confidence:** medium
8. **Revisit Trigger:** Branch policy hardens to `dev -> testing -> main`.
9. **Affected PRs/Files:** PR planning and merge process; `.github/*`, `docs/v1-ticket-pr-breakdown.md`.

## DEC-003

1. **Decision ID:** DEC-003
2. **Date:** 2026-03-21
3. **Question:** What is `ticket delete` behavior in v1?
4. **Options Considered:**
   - Soft delete (`status=canceled`, `archived=true`).
   - Hard delete with safety guards.
   - Both soft and force delete.
5. **Chosen Option:** Soft delete only.
6. **Why We Chose It:** Preserves history and minimizes accidental data loss in v1.
7. **Confidence:** high
8. **Revisit Trigger:** Explicit requirement for irreversible deletion.
9. **Affected PRs/Files:** PR-006, PR-007, PR-009; domain workflow + delete command + tests.

## DEC-004

1. **Decision ID:** DEC-004
2. **Date:** 2026-03-21
3. **Question:** Should v1 scope be reduced for faster delivery?
4. **Options Considered:**
   - Keep full v1 scope.
   - Reduce to core CRUD only.
   - Hybrid deferment of selected features.
5. **Chosen Option:** Keep full v1 scope with rigorous PR-by-PR testing.
6. **Why We Chose It:** Requirement is explicit full v1 parity with the handoff spec.
7. **Confidence:** high
8. **Revisit Trigger:** New top-down direction to cut scope.
9. **Affected PRs/Files:** PR-001..PR-009; full v1 implementation surface.

## DEC-005

1. **Decision ID:** DEC-005
2. **Date:** 2026-03-21
3. **Question:** When markdown snapshots and event history disagree during rebuild, which source is authoritative?
4. **Options Considered:**
   - Event log authoritative.
   - Markdown authoritative.
   - Fail-fast and stop rebuild.
5. **Chosen Option:** Event log authoritative.
6. **Why We Chose It:** JSONL is the append-only history contract and must drive deterministic recovery.
7. **Confidence:** high
8. **Revisit Trigger:** Event format changes or explicit data authority redesign.
9. **Affected PRs/Files:** PR-004, PR-009; `internal/storage/sqlite/reindex*`, doctor/recovery tests.

## DEC-006

1. **Decision ID:** DEC-006
2. **Date:** 2026-03-21
3. **Question:** How should v1 produce monotonic event IDs?
4. **Options Considered:**
   - Per-project strict numeric sequence.
   - ULID/time-sort IDs.
   - Implicit ordering by JSONL line number only.
5. **Chosen Option:** Per-project strict numeric sequence.
6. **Why We Chose It:** Clear monotonicity and simple deterministic ordering in history and rebuild paths.
7. **Confidence:** medium
8. **Revisit Trigger:** Multi-writer or distributed event ingestion requirements.
9. **Affected PRs/Files:** PR-003, PR-004, PR-009; event writer, projector, history tests.

## DEC-007

1. **Decision ID:** DEC-007
2. **Date:** 2026-03-21
3. **Question:** How should common mutation command behavior be implemented in CLI?
4. **Options Considered:**
   - Shared helper for repeated flags/validation.
   - Fully duplicated command definitions.
   - Heavy declarative command meta-framework.
5. **Chosen Option:** Shared helper with explicit handlers.
6. **Why We Chose It:** Reduces repetition while avoiding over-abstraction.
7. **Confidence:** high
8. **Revisit Trigger:** Helper becomes too coupled or difficult to maintain.
9. **Affected PRs/Files:** PR-005, PR-007; `internal/cli/*`.

## DEC-008

1. **Decision ID:** DEC-008
2. **Date:** 2026-03-21
3. **Question:** Which SQLite durability/performance profile is default for v1?
4. **Options Considered:**
   - WAL + normal sync.
   - DELETE journal + full sync.
   - In-memory projection.
5. **Chosen Option:** WAL + normal sync.
6. **Why We Chose It:** Best fit for local CLI read/write responsiveness with acceptable durability.
7. **Confidence:** medium
8. **Revisit Trigger:** Corruption/performance findings from field usage.
9. **Affected PRs/Files:** PR-004; sqlite init/config logic and durability tests.

## DEC-009

1. **Decision ID:** DEC-009
2. **Date:** 2026-03-21
3. **Question:** What is the default output mode for read commands?
4. **Options Considered:**
   - Pretty output default.
   - Markdown output default.
   - JSON output default.
5. **Chosen Option:** Pretty output default.
6. **Why We Chose It:** Tool is terminal-first and should be human-readable by default.
7. **Confidence:** high
8. **Revisit Trigger:** Primary usage shifts to machine-driven automation.
9. **Affected PRs/Files:** PR-008; read-command output routing.

## DEC-010

1. **Decision ID:** DEC-010
2. **Date:** 2026-03-21
3. **Question:** How should terminal color and accessibility work?
4. **Options Considered:**
   - Semantic color with non-color fallback and `NO_COLOR`.
   - Monochrome-only.
   - Color-required output.
5. **Chosen Option:** Semantic color plus text/icon fallback, `NO_COLOR`, non-TTY safe behavior.
6. **Why We Chose It:** Keeps visual clarity while preserving accessibility and compatibility.
7. **Confidence:** high
8. **Revisit Trigger:** Accessibility audit identifies gaps.
9. **Affected PRs/Files:** PR-008; renderer styling and terminal capability checks.

## DEC-011

1. **Decision ID:** DEC-011
2. **Date:** 2026-03-21
3. **Question:** How should empty states in board/list/search views behave?
4. **Options Considered:**
   - Actionable empty states.
   - Minimal one-line message.
   - Verbose troubleshooting block.
5. **Chosen Option:** Actionable empty states.
6. **Why We Chose It:** Improves usability without adding heavy UI complexity.
7. **Confidence:** high
8. **Revisit Trigger:** UX testing shows noise/confusion from guidance text.
9. **Affected PRs/Files:** PR-008; empty-state rendering branches.

## DEC-012

1. **Decision ID:** DEC-012
2. **Date:** 2026-03-21
3. **Question:** Should v1 include Windows compatibility now?
4. **Options Considered:**
   - Keep macOS/Linux/Windows in v1.
   - Defer Windows to v1.1.
5. **Chosen Option:** Defer Windows to v1.1; deliver v1 on macOS + Linux.
6. **Why We Chose It:** Current delivery priority favors faster v1 with two primary platforms.
7. **Confidence:** low
8. **Revisit Trigger:** Release criteria require Windows parity before v1 cutoff.
9. **Affected PRs/Files:** PR-001, PR-009; CI matrix and release docs.

## DEC-013

1. **Decision ID:** DEC-013
2. **Date:** 2026-03-21
3. **Question:** Should gstack upgrade run immediately before planning review?
4. **Options Considered:**
   - Upgrade now.
   - Enable auto-upgrade and upgrade now.
   - Defer upgrade ("Not now").
5. **Chosen Option:** Defer upgrade for this planning cycle.
6. **Why We Chose It:** Keep momentum on planning work and avoid tooling drift mid-session.
7. **Confidence:** medium
8. **Revisit Trigger:** Next planning/review session start.
9. **Affected PRs/Files:** Process decision only; no repo code impact.

## DEC-014

1. **Decision ID:** DEC-014
2. **Date:** 2026-03-21
3. **Question:** Should the completeness principle intro be acknowledged in this workflow?
4. **Options Considered:**
   - Acknowledge and proceed without opening link.
   - Acknowledge and open the reference link.
   - Skip acknowledgement.
5. **Chosen Option:** Acknowledge and open the reference link.
6. **Why We Chose It:** Completed the required one-time workflow gate for gstack planning skills.
7. **Confidence:** high
8. **Revisit Trigger:** None (one-time procedural acknowledgement).
9. **Affected PRs/Files:** Process decision only; no repo code impact.

## DEC-015

1. **Decision ID:** DEC-015
2. **Date:** 2026-03-22
3. **Question:** Where should `workflow.completion_mode` be modeled for v1 permission checks?
4. **Options Considered:**
   - Ticket-level field.
   - Project-level field.
   - Tracker config field (`workflow.completion_mode`).
5. **Chosen Option:** Tracker config field.
6. **Why We Chose It:** This matches the v1 spec and avoids policy drift across tickets when gate mode changes.
7. **Confidence:** high
8. **Revisit Trigger:** Requirement emerges for per-project or per-ticket completion policy.
9. **Affected PRs/Files:** PR-002+; `internal/contracts/domain.go`, later config loader/store code.

## DEC-016

1. **Decision ID:** DEC-016
2. **Date:** 2026-03-22
3. **Question:** How should v1 parse and emit markdown frontmatter for ticket/project files?
4. **Options Considered:**
   - Custom ad-hoc parser.
   - `gopkg.in/yaml.v3` for frontmatter encode/decode.
5. **Chosen Option:** Use `gopkg.in/yaml.v3`.
6. **Why We Chose It:** Reduces parsing bugs and keeps markdown snapshot contracts explicit and deterministic.
7. **Confidence:** high
8. **Revisit Trigger:** Dependency policy requires zero third-party packages.
9. **Affected PRs/Files:** PR-003+; `internal/storage/markdown/*`, `go.mod`, `go.sum`.

## DEC-017

1. **Decision ID:** DEC-017
2. **Date:** 2026-03-22
3. **Question:** Which SQLite driver should v1 use for local projection/index?
4. **Options Considered:**
   - `modernc.org/sqlite` (pure Go).
   - `github.com/mattn/go-sqlite3` (CGO).
5. **Chosen Option:** `modernc.org/sqlite`.
6. **Why We Chose It:** Keeps local setup straightforward without CGO dependencies while supporting macOS/Linux v1 targets.
7. **Confidence:** medium
8. **Revisit Trigger:** Performance or compatibility issues in projection workloads.
9. **Affected PRs/Files:** PR-004+; `internal/storage/sqlite/*`, `go.mod`, `go.sum`.

## DEC-018

1. **Decision ID:** DEC-018
2. **Date:** 2026-03-22
3. **Question:** Which framework should define v1 CLI command tree and shell parity scaffolding?
4. **Options Considered:**
   - `github.com/spf13/cobra`.
   - Custom command parser/dispatcher.
5. **Chosen Option:** `github.com/spf13/cobra`.
6. **Why We Chose It:** Matches the v1 stack recommendation and simplifies exact command-surface scaffolding with subcommands and flags.
7. **Confidence:** high
8. **Revisit Trigger:** CLI ergonomics or dependency policy changes.
9. **Affected PRs/Files:** PR-005+; `cmd/tracker/*`, `internal/cli/*`, `go.mod`, `go.sum`.
## DEC-019

1. **Decision ID:** DEC-019
2. **Date:** 2026-03-22
3. **Question:** How should v1 load and persist `config.toml` for completion-mode gates?
4. **Options Considered:**
   - Hand-rolled parser.
   - `github.com/pelletier/go-toml/v2`.
5. **Chosen Option:** `github.com/pelletier/go-toml/v2`.
6. **Why We Chose It:** Keeps config parsing/writing predictable while supporting the spec's `config.toml` contract.
7. **Confidence:** high
8. **Revisit Trigger:** Dependency policy requires parser removal.
9. **Affected PRs/Files:** PR-006+; `internal/config/*`, `internal/cli/root.go`, `go.mod`, `go.sum`.

## DEC-020

1. **Decision ID:** DEC-020
2. **Date:** 2026-03-22
3. **Question:** How should v1 enforce relationship-link integrity before persistence commands are wired?
4. **Options Considered:**
   - Only validate at command layer later.
   - Add domain-level link apply/remove + cycle checks now.
5. **Chosen Option:** Domain-level link helpers now.
6. **Why We Chose It:** Prevents scattered link logic and guarantees symmetric blocks/blocked_by updates, self-link rejection, and parent-cycle enforcement before command handlers are implemented.
7. **Confidence:** high
8. **Revisit Trigger:** Relationship model expands beyond current fields.
9. **Affected PRs/Files:** PR-006+; `internal/domain/links.go`, `internal/domain/links_test.go`.
## DEC-021

1. **Decision ID:** DEC-021
2. **Date:** 2026-03-22
3. **Question:** How should v1 allocate ticket IDs and event IDs before a dedicated allocator exists?
4. **Options Considered:**
   - Maintain separate allocator tables now.
   - Derive next IDs from existing markdown snapshots and event stream.
5. **Chosen Option:** Derive next IDs from current markdown/events for v1.
6. **Why We Chose It:** Keeps PR-007 focused and compatible with existing storage contracts without introducing new allocation infrastructure.
7. **Confidence:** medium
8. **Revisit Trigger:** Performance issues from repeated scans or multi-writer requirements.
9. **Affected PRs/Files:** PR-007+; `internal/cli/actions.go`.

## DEC-022

1. **Decision ID:** DEC-022
2. **Date:** 2026-03-22
3. **Question:** How should `ticket comment` work before interactive editor support is added?
4. **Options Considered:**
   - Require `--body` in non-interactive mode now.
   - Add editor invocation in PR-007.
5. **Chosen Option:** Require `--body` now.
6. **Why We Chose It:** Maintains deterministic CLI behavior while deferring editor integration to hardening scope.
7. **Confidence:** high
8. **Revisit Trigger:** PR-009 usability pass adds editor-mode comment entry.
9. **Affected PRs/Files:** PR-007, PR-009; `internal/cli/root.go`, command UX tests/docs.
## DEC-023

1. **Decision ID:** DEC-023
2. **Date:** 2026-03-22
3. **Question:** Which libraries should power v1 terminal rendering for pretty/markdown output?
4. **Options Considered:**
   - Plain text only.
   - `lipgloss` + `glamour` + terminal width detection.
5. **Chosen Option:** `lipgloss` + `glamour` + `golang.org/x/term`.
6. **Why We Chose It:** Aligns with the v1 stack direction and enables readable styled output with markdown rendering and width-aware wrapping.
7. **Confidence:** high
8. **Revisit Trigger:** Terminal compatibility or performance regressions.
9. **Affected PRs/Files:** PR-008+; `internal/render/*`, read command render paths, `go.mod`, `go.sum`.

## DEC-024

1. **Decision ID:** DEC-024
2. **Date:** 2026-03-22
3. **Question:** Which source should drive board/backlog/next/blocked/search views in v1?
4. **Options Considered:**
   - Mix markdown snapshots for some views and SQLite projection for others.
   - Route all those views through SQLite projection.
5. **Chosen Option:** Route all these views through SQLite projection.
6. **Why We Chose It:** Keeps list/board/search outputs consistent and aligned with the projection/index purpose of v1.
7. **Confidence:** medium
8. **Revisit Trigger:** Projection freshness/consistency issues require fallback strategy.
9. **Affected PRs/Files:** PR-008+; `internal/cli/root.go`, `internal/storage/sqlite/*`.
## DEC-025

1. **Decision ID:** DEC-025
2. **Date:** 2026-03-22
3. **Question:** Which platforms should CI enforce for v1 completion?
4. **Options Considered:**
   - macOS + Linux + Windows.
   - macOS + Linux only (Windows deferred).
5. **Chosen Option:** macOS + Linux only for v1; Windows deferred to v1.1.
6. **Why We Chose It:** Matches the latest scope lock while still validating core cross-platform behavior.
7. **Confidence:** medium
8. **Revisit Trigger:** v1.1 planning begins or release policy requires Windows gating.
9. **Affected PRs/Files:** PR-009; `.github/workflows/ci.yml`, release docs.
## DEC-026

**Status:** Superseded by DEC-061 for canceled tickets. Dependency-aware blocked projection remains current.

1. **Decision ID:** DEC-026
2. **Date:** 2026-03-22
3. **Question:** How should board-style views bucket blocked and canceled tickets in v1?
4. **Options Considered:**
   - Use raw workflow status only for board columns.
   - Derive board columns so any ticket with `blocked_by` links appears in `blocked`, and fold `canceled` into the `done` column.
5. **Chosen Option:** Derive board columns from ticket relationships + terminal status.
6. **Why We Chose It:** Matches the handoff acceptance flow: a ticket linked as blocked must show up in the blocked column without an extra status move, and canceled work should share the done/closed terminal bucket in board output.
7. **Confidence:** high
8. **Revisit Trigger:** v1 introduces a dedicated closed/canceled view or separate board column configuration.
9. **Affected PRs/Files:** PR-004, PR-008, PR-009; `internal/contracts/domain.go`, `internal/storage/sqlite/store.go`, `internal/cli/root.go`, `internal/render/render.go`, board-related tests/docs.

## DEC-027

1. **Decision ID:** DEC-027
2. **Date:** 2026-06-10
3. **Question:** How should TUI help be exposed for users learning the keyboard and command-palette workflow?
4. **Options Considered:**
   - Keep only the footer keybinding summary.
   - Add a modal in-app help guide opened by `?` and `/help`.
5. **Chosen Option:** Add a modal in-app help guide opened by `?` and `/help`.
6. **Why We Chose It:** The footer is good for reminders but too terse for learning Atlas Tasker. A modal guide keeps help discoverable inside the TUI and can document tabs, keyboard actions, bulk flow, and slash-command examples without sending users back to external docs.
7. **Confidence:** high
8. **Revisit Trigger:** The TUI command surface grows enough that generated help or per-tab help becomes easier to keep accurate than a curated guide.
9. **Affected PRs/Files:** `internal/tui/app.go`, `internal/tui/mutations.go`, `internal/tui/app_test.go`, `docs/reference/tui.md`, `docs/tutorials/05-use-the-tui.md`.

## DEC-028

1. **Decision ID:** DEC-028
2. **Date:** 2026-06-10
3. **Question:** How should repeated terminal items be presented in CLI pretty output and the TUI?
4. **Options Considered:**
   - Keep bullet-only list output.
   - Add bordered tables to repeated-row views while preserving JSON and Markdown contracts.
   - Add a new output flag for table output.
5. **Chosen Option:** Use bordered tables for repeated-row pretty/TUI views by default.
6. **Why We Chose It:** Atlas is terminal-first, and board, queue, agent work, saved-view, run, evidence, and operations lists are easier to scan when columns line up with clear borders. Keeping this in pretty/TUI presentation avoids changing machine-readable JSON or agent-readable Markdown.
7. **Confidence:** high
8. **Revisit Trigger:** Terminal compatibility or accessibility reports show box rendering is unreliable, or users need a persistent table style setting beyond `--plain`/`NO_COLOR`.
9. **Affected PRs/Files:** `internal/render/render.go`, `internal/cli/root.go`, `internal/cli/run.go`, `internal/tui/app.go`, renderer/CLI/TUI tests, terminal output docs.

## DEC-029

1. **Decision ID:** DEC-029
2. **Date:** 2026-06-16
3. **Question:** How should Atlas expose a browser Kanban board without weakening the local-first storage and security model?
4. **Options Considered:**
   - Add a server-rendered local web board over existing services.
   - Add a SPA with a broad local JSON API.
   - Add hosted/server mode with login.
5. **Chosen Option:** Add a server-rendered local web board over existing services.
6. **Why We Chose It:** The browser board should make Atlas easier to inspect and demo while preserving Markdown snapshots, JSONL events, SQLite projection, `ActionService` writes, `QueryService` reads, and local-only security defaults.
7. **Confidence:** high
8. **Revisit Trigger:** Future product direction requires remote collaboration or a public API surface.
9. **Affected PRs/Files:** `internal/web/*`, `internal/cli/*`, `internal/contracts/events.go`, web board docs, tests.

## DEC-030

1. **Decision ID:** DEC-030
2. **Date:** 2026-07-03
3. **Question:** Which referrer policy should the web board send, given that its origin check rejects mutations whose `Origin` does not match the host?
4. **Options Considered:**
   - Keep `Referrer-Policy: no-referrer`.
   - Switch to `Referrer-Policy: same-origin`.
   - Drop the origin check and rely on the CSRF token alone.
5. **Chosen Option:** Switch to `Referrer-Policy: same-origin` and keep the origin check.
6. **Why We Chose It:** Under `no-referrer` the Fetch spec serializes `Origin` as `null` on same-origin form POSTs, so the board rejected its own create/edit/comment/approve/complete/move forms in real browsers (reproduced in Chromium; httptest could not catch it). `same-origin` keeps referrers private cross-origin while restoring `Origin`/`Referer` on the board's own requests, and `Origin: null` remains rejected as cross-origin. Session cookies are additionally scoped per port so concurrent workspace boards on 127.0.0.1 keep separate sessions.
7. **Confidence:** high
8. **Revisit Trigger:** A browser changes `Origin` serialization semantics, or the board ever runs behind TLS/proxy setups that alter origin handling.
9. **Affected PRs/Files:** `internal/web/server.go`, `internal/web/server_test.go`, `internal/web/fixes_test.go`, `docs/web-board-security.md`.

## DEC-031

1. **Decision ID:** DEC-031
2. **Date:** 2026-07-03
3. **Question:** How strictly should web mutations validate input, and how should workflow violations surface over HTTP and in exit codes?
4. **Options Considered:**
   - Keep lenient parsing (invalid enums coerced to defaults) and generic 500s.
   - Mirror CLI validation on the web surface and map workflow violations to conflict semantics everywhere.
5. **Chosen Option:** Mirror CLI validation and map `forbidden transition` errors to the conflict code.
6. **Why We Chose It:** The web surface accepted what the CLI refuses: tickets born `done`/`canceled`, invalid enum values silently coerced (an invalid drag status became a `backlog` move attempt), and edits persisting blank titles or malformed actors that later crashed rendering. Web create/edit/move now enforce the same invariants, same-status drops are no-ops, and `apperr.CodeOf` classifies `forbidden transition` as `conflict` — HTTP 409 on the web, exit code 4 (the documented conflict exit) in the CLI instead of the unmapped default 1. Plain form posts redirect back to the board with the error rendered instead of dead-ending on a text/plain page.
7. **Confidence:** high
8. **Revisit Trigger:** Scripts are found depending on exit code 1 for forbidden transitions, or a surface needs to create terminal-status tickets legitimately (import/export already bypasses this via its own path).
9. **Affected PRs/Files:** `internal/web/handlers.go`, `internal/web/server.go`, `internal/web/viewmodels.go`, `internal/apperr/errors.go`, `internal/service/query.go`, `internal/service/types.go`, `internal/storage/sqlite/store.go`, `internal/cli/web.go`, web templates/static, docs.

## DEC-032

1. **Decision ID:** DEC-032
2. **Date:** 2026-07-03
3. **Question:** How should the web board respond to a rejected non-JS form submission?
4. **Options Considered:**
   - Raw `http.Error` text page (original).
   - Post/Redirect/Get back to `/board` with the error in the query string.
   - Re-render the board in place with the error status and the submitted values echoed into the originating form.
5. **Chosen Option:** Re-render in place with scoped echo.
6. **Why We Chose It:** The raw text page dead-ends the user; PRG destroys typed content (Cache-Control: no-store disables bfcache) and reads as success to redirect-following clients. Render-in-place keeps the true 4xx status for every client and preserves everything typed. Echoed values are scoped via a FormTarget derived from the action path so a rejected mutation can only prefill the form that produced it — never another ticket's edit form. Known residual: refreshing the error page re-posts the form (inherent to render-in-place), and the browser address bar sits on the action URL until the next navigation; app.js resolves reloads against /board to compensate.
7. **Confidence:** medium
8. **Revisit Trigger:** Server-side flash/session state is introduced (enabling PRG without data loss), or users report confusion from the POST URL/refresh-repost behavior.
9. **Affected PRs/Files:** `internal/web/server.go`, `internal/web/viewmodels.go`, `internal/web/assets.go`, `internal/web/templates/*`, `internal/web/static/app.js`, web tests.

## DEC-033

**Status:** DEC-061 supersedes the canceled-as-Done board rationale. Canonical Done counting and the welcome read model remain current.

1. **Decision ID:** DEC-033
2. **Date:** 2026-07-23
3. **Question:** How should the local welcome page compute cross-project status and recent activity?
4. **Options Considered:**
   - Add a new persisted dashboard projection.
   - Derive project counts from per-project board queries and merge the existing per-project event streams at request time.
   - Read Markdown and JSONL files directly from the web handlers.
5. **Chosen Option:** Derive counts through `QueryService` board queries and merge filtered event streams in `QueryService`, capped at 20.
6. **Why We Chose It:** The welcome page stays a read model over the same contracts as CLI/TUI instead of adding a second source of truth. Canonical snapshots supply the Done count because DEC-026 intentionally folds canceled tickets into the board's Done column while this overview excludes canceled work. The event scan is cached per project for one request and its full-scan tradeoff is explicit.
7. **Confidence:** high
8. **Revisit Trigger:** Root-page latency becomes noticeable in workspaces with large event logs, or a shared indexed activity query is introduced.
9. **Affected PRs/Files:** `internal/service/rollups.go`, `internal/service/rollups_test.go`, `internal/web/viewmodels.go`, welcome web tests.

## DEC-034

1. **Decision ID:** DEC-034
2. **Date:** 2026-07-23
3. **Question:** What should the browser root route and welcome-page interaction model be?
4. **Options Considered:**
   - Keep redirecting `/` to the board.
   - Add a stat-card dashboard.
   - Render a borderless project ledger with a recent-change rail, native project-creation dialog, and a read-only settings page.
5. **Chosen Option:** Render the project ledger/activity rail at `/`; keep `/board` canonical and link both directions.
6. **Why We Chose It:** Owners need cross-project orientation before card-level manipulation. A plain ledger compares real counts without card/grid noise, while the activity rail answers what changed. Project creation reuses `ActionService` plus the existing origin/CSRF/read-only gates; rejected forms keep the exact error and submitted values. Web identity falls back from `web.owner_name` to `actor.default` to the OS username, and agent color names are mapped to a small server-side CSS class allowlist so CSP stays strict.
7. **Confidence:** high
8. **Revisit Trigger:** Usage shows owners always bypass the overview, settings become editable in-browser, or the web surface adds a shared indexed activity API.
9. **Affected PRs/Files:** `internal/contracts/domain.go`, `internal/config/config.go`, `internal/web/*`, `PRODUCT.md`, `DESIGN.md`, `docs/web-welcome-screen-brief.md`, web/config tests.

## DEC-035

**Status:** Superseded by DEC-038 for motion timing and feedback, and by DEC-061 for visible assignee text. The remaining card information model stays current.

1. **Decision ID:** DEC-035
2. **Date:** 2026-07-23
3. **Question:** How much information should Kanban cards expose, and how should secondary detail and movement feel?
4. **Options Considered:**
   - Keep assignee avatars, priority and label pills, and counters on every card.
   - Reduce the face to ID/title plus an optional configured agent color mark, with delayed local preview and full drawer detail.
   - Fetch a richer server preview on every hover.
5. **Chosen Option:** Use the minimal face, one two-second `data-*` preview, a 200ms drawer transform, and SortableJS's 150ms position animation.
6. **Why We Chose It:** The board is a high-frequency scan surface, so repeated badges and icon rows made each ticket harder to compare and forced wider columns. Escaped metadata already rendered with the card can power one viewport-clamped preview without network work or a new API. The drawer remains the authoritative detail/edit surface, agent color stays a scarce ownership hint rather than a card fill, and reduced-motion users get immediate state changes.
7. **Confidence:** high
8. **Revisit Trigger:** Owners consistently miss urgent work without face-level priority, the hover delay creates excess drawer opens, or keyboard users need an equivalent non-navigation summary.
9. **Affected PRs/Files:** `internal/web/viewmodels.go`, `internal/web/templates/board.html`, `internal/web/static/app.css`, `internal/web/static/app.js`, `internal/web/card_interactions_test.go`, `PRODUCT.md`, `DESIGN.md`, `docs/web-board-screen-brief.md`.

## DEC-036

The initial three-language scope is superseded by DEC-058. Request precedence and the stored-content boundary remain unchanged.

1. **Decision ID:** DEC-036
2. **Date:** 2026-07-23
3. **Question:** How should Atlas pilot multilingual browser chrome without adding a localization dependency or changing stored ticket content?
4. **Options Considered:**
   - Add `golang.org/x/text` and locale-aware routing.
   - Keep strings in templates and duplicate localized pages.
   - Bind a small in-process message catalog to cloned templates per request.
5. **Chosen Option:** Use flat English, Spanish, and Indonesian catalogs with a request-bound `t` template function; resolve language from `?lang=`, then `web.lang`, then `Accept-Language`, then English.
6. **Why We Chose It:** The browser remains server-rendered, dependency-free, and easy to extend. Cloning the parsed template before binding request functions keeps concurrent requests isolated. Ticket text, comments, labels, actors, event payloads, and audit reasons remain canonical data rather than translation input. A catalog key-set test makes missing translations fail in CI.
7. **Confidence:** high
8. **Revisit Trigger:** Atlas adds locale-aware dates/numbers, plural rules beyond the pilot, RTL support, or enough languages that maintaining literal maps becomes error-prone.
9. **Affected PRs/Files:** `internal/contracts/domain.go`, `internal/config/config.go`, `internal/web/i18n.go`, `internal/web/templates/*`, `internal/web/static/*`, web/config tests, `docs/i18n-notes.md`, web/config docs.

## DEC-037

1. **Decision ID:** DEC-037
2. **Date:** 2026-07-25
3. **Question:** How should a live board sync communicate card and count movement without weakening strict CSP or pulling the grid out from under an active drag?
4. **Options Considered:**
   - Keep replacing the board grid instantly.
   - Vendor Motion Mini and use its animation helper for FLIP.
   - Record ticket rectangles and column counts locally, then use the browser's Web Animations API for FLIP plus CSS classes for drop/count feedback.
5. **Chosen Option:** Use a small native FLIP implementation and CSS feedback classes.
6. **Why We Chose It:** Ticket IDs already provide stable keys across the server-rendered grid swap. Capturing rectangles only after `waitForDragEnd`, checking the drag counter again before playback, and animating transforms for 180ms preserves spatial continuity without changing the mutation or refresh contracts. Native animation needs no module loader, package metadata, inline style, external request, or additional vendored code; the same path skips all effects when reduced motion is requested.
7. **Confidence:** high
8. **Revisit Trigger:** Supported browsers no longer provide the Web Animations API, sync expands beyond simple card movement, or a shared animation runtime becomes justified by several independent interactions.
9. **Affected PRs/Files:** `internal/web/static/app.js`, `internal/web/static/app.css`, `internal/web/card_interactions_test.go`, `DESIGN.md`, `docs/web-board-screen-brief.md`.

## DEC-038

1. **Decision ID:** DEC-038
2. **Date:** 2026-07-25
3. **Question:** How should the board's drawer, hover, and press feedback change now that DEC-035's uniform 200ms drawer motion feels too linear?
4. **Options Considered:**
   - Keep the existing 200ms standard ease for every drawer direction and card lift.
   - Use CSS cubic-bezier springs for entry/lift, with shorter standard ease-out timing for close/press.
   - Add JavaScript spring physics for every interaction.
5. **Chosen Option:** Use transform-only CSS curves: 240ms restrained overshoot on drawer open, 160ms standard ease-out on close, 220ms spring lift on card hover/focus, and 90ms compression on press.
6. **Why We Chose It:** Entry benefits from a small amount of continuity while exit and direct press feedback should get out of the way. CSS keeps these frequent interactions compositor-friendly and interruptible without adding a runtime. The existing surface colors, layout, and `--ease` curve remain unchanged; one spring easing token handles the causal entry/lift cases, and the reduced-motion block removes every transform and animation.
7. **Confidence:** high
8. **Revisit Trigger:** Runtime inspection shows visible overshoot at large drawer widths, interaction latency rises on lower-performance hardware, or users report that frequent card feedback feels busy.
9. **Affected PRs/Files:** Supersedes the motion timing/feedback portion of DEC-035; `internal/web/static/app.css`, `internal/web/card_interactions_test.go`, `DESIGN.md`, `docs/web-board-screen-brief.md`.

## DEC-039

1. **Decision ID:** DEC-039
2. **Date:** 2026-08-28
3. **Question:** How should the atlas.pen visual direction reach the browser board — as a full restructure, or as a restyle of the layout people already use?
4. **Options Considered:**
   - Keep the soft charcoal skin and treat the pen design as non-binding inspiration.
   - Ship the Survey Ledger restructure (`feat/pen-design-frontend`): arrival-question headers, a workflow route line, no default drawer, selection-driven detail.
   - Keep the existing layout — topbar shell, filter row, six columns, right drawer — and restyle it to the pen tokens and type.
5. **Chosen Option:** Restyle the existing layout to the pen visual system: the #0B0F14/#111821/#18222D surface scale, #68A9FF accent, vendored Geist and Geist Mono variable fonts (Inter removed), semantic color reserved for compact indicators. The Survey Ledger implementation stays on its own branch as a complete alternative and does not ship in v1.10.
6. **Why We Chose It:** The owner asked for the layout to stay put. The board's interaction contracts (drag, filters, drawer, saved views) are pinned by tests and by muscle memory; the restructure changed where selection lives and how detail opens, which is a product change dressed as a restyle. Matching the pen tokens and type on the existing shell gets the look without renegotiating the loop, in a diff a reviewer can read in one sitting. Keeping Survey Ledger on a branch preserves the work for a deliberate product decision later instead of losing it in a merge conflict.
7. **Confidence:** high
8. **Revisit Trigger:** Owners cannot identify selected work without opening detail, or horizontal scanning of six columns is measurably slower on common screens — the two problems the Survey Ledger structure was built to solve.
9. **Affected PRs/Files:** `internal/web/static/app.css`, `internal/web/static/vendor/*`, `internal/web/templates/board.html`, `internal/web/templates/schedule.html`, `internal/web/viewmodels.go`, `DESIGN.md`, `docs/web-board.md`.

## DEC-040

1. **Decision ID:** DEC-040
2. **Date:** 2026-08-10
3. **Question:** How should Atlas handle symlinks found while collecting workspace files for export-derived artifacts?
4. **Options Considered:**
   - Follow symlinks and include their targets.
   - Silently omit symlinked inputs.
   - Reject the operation before writing an artifact.
5. **Chosen Option:** Reject the operation before writing an artifact.
6. **Why We Chose It:** The export collector is shared by normal and redacted exports, backups, audit artifacts, and goal artifacts. Following a link can copy data outside the workspace into a shareable artifact, while silently omitting it would produce an incomplete artifact without telling the operator. A fail-closed error preserves the documented boundary that private material must never enter export-derived artifacts.
7. **Confidence:** high
8. **Revisit Trigger:** Atlas adopts a separately reviewed, explicit link-materialization policy with target containment and clear artifact provenance.
9. **Affected PRs/Files:** `internal/service/import_export.go`, `internal/service/security_boundary_test.go`, `docs/v1-decision-log.md`.

## DEC-041

1. **Decision ID:** DEC-041
2. **Date:** 2026-08-12
3. **Question:** How should Atlas keep its CI and release automation from silently changing underneath a reviewed commit?
4. **Options Considered:**
   - Keep mutable major-version action tags and repository-default token permissions.
   - Pin every third-party action to a reviewed commit and declare least-privilege workflow permissions.
   - Vendor every action into this repository.
5. **Chosen Option:** Pin actions to immutable commits, declare `contents: read` at workflow scope, and let only the publish job elevate the three permissions it needs.
6. **Why We Chose It:** Immutable action references make the reviewed automation the automation that runs. An executable policy check prevents a later mutable tag or mismatched Go bootstrap from quietly reopening the same supply-chain gap without taking on the maintenance and audit burden of vendoring action code.
7. **Confidence:** high
8. **Revisit Trigger:** GitHub changes action pinning or token-permission semantics, or Atlas moves release execution to a different CI provider.
9. **Affected PRs/Files:** `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `scripts/check-workflow-security.sh`, `scripts/preflight-release-proof.sh`.

## DEC-042

1. **Decision ID:** DEC-042
2. **Date:** 2026-08-12
3. **Question:** Should the web drawer's close control optimize for a compact desktop silhouette or a reliable phone touch target?
4. **Options Considered:**
   - Keep the 28-by-28-pixel control because it clears the WCAG 2.5.8 minimum.
   - Give the existing control a 44-by-44-pixel hit area while keeping the same icon and visual treatment.
5. **Chosen Option:** Use a 44-by-44-pixel close control at every viewport.
6. **Why We Chose It:** The drawer is a primary phone interaction and its close action should not demand precise tapping. A consistent target across viewports avoids a second responsive rule while preserving the existing icon, color, and hover language.
7. **Confidence:** high
8. **Revisit Trigger:** Rendered QA finds that the larger target collides with long drawer titles at the narrowest supported width.
9. **Affected PRs/Files:** `internal/web/static/app.css`, `internal/web/fixes_test.go`.

## DEC-043

1. **Decision ID:** DEC-043
2. **Date:** 2026-08-12
3. **Question:** How should Atlas respond when its required Go runtime or a reachable transitive module has a published vulnerability?
4. **Options Considered:**
   - Record the advisories and wait for the next feature release.
   - Patch only the modules and keep the vulnerable Go runtime.
   - Move the runtime and every reachable vulnerable module to the first fixed versions, then require CI and release jobs to use the same runtime as `go.mod`.
5. **Chosen Option:** Move the runtime and all reachable vulnerable modules to fixed versions and keep the workflow toolchain synchronized with `go.mod`.
6. **Why We Chose It:** `govulncheck` traced the affected standard-library TLS code, Markdown renderer, and text renderer into Atlas. Updating all three boundaries closes the reachable paths without carrying a partial exception, while the workflow policy prevents a future runtime mismatch.
7. **Confidence:** high
8. **Revisit Trigger:** A fixed version causes a reproducible compatibility regression or a future Go release changes how the module directive maps to CI toolchains.
9. **Affected PRs/Files:** `go.mod`, `go.sum`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `scripts/check-workflow-security.sh`.

## DEC-044

1. **Decision ID:** DEC-044
2. **Date:** 2026-08-12
3. **Question:** Should the local web board retain an explicit escape hatch for plaintext non-loopback serving?
4. **Options Considered:**
   - Keep `--unsafe-host` with a warning.
   - Add authentication and TLS to turn the board into a network product.
   - Reject non-loopback hosts and keep the board local-only.
5. **Chosen Option:** Reject non-loopback hosts and remove `--unsafe-host`.
6. **Why We Chose It:** The current board uses a bearer session URL but has no remote-user identity, authorization, or TLS lifecycle. A warning does not contain exposure on an untrusted network. Loopback-only serving matches the product boundary without inventing a partial hosted security model.
7. **Confidence:** high
8. **Revisit Trigger:** Atlas intentionally designs and tests a remote web product with TLS, authenticated identities, authorization, session revocation, and deployment guidance.
9. **Affected PRs/Files:** `internal/cli/web.go`, `internal/cli/root_test.go`, `internal/web/server.go`, `internal/web/server_test.go`, `docs/command-reference.md`, `docs/web-board-security.md`.

## DEC-045

1. **Decision ID:** DEC-045
2. **Date:** 2026-07-30
3. **Question:** How do we make the README's "JSON output on every command" claim true, and keep it true?
4. **Options Considered:**
   - Register `--json` on the thirteen ticket write commands the agent skill names and stop there.
   - Register it everywhere it makes sense and add a tree-walking test with an allowlist.
   - Move output-flag registration into a shared command constructor so new commands inherit it.
5. **Chosen Option:** Register the flags on every leaf that produces a result, then walk the cobra tree in a test and fail on any leaf without `--json` outside a documented allowlist.
6. **Why We Chose It:** The commands were already printing through `writeCommandOutput`; the flags were the only thing missing, so the fix was registration rather than new output paths. A shared constructor would have meant rewriting every command declaration in the package for the same guarantee a twenty-line test gives, and the test also catches the reverse mistake — an allowlist entry that quietly gains the flag or stops being a leaf. The allowlist holds five surfaces that own stdout for something else (`mcp serve` speaks JSON-RPC on it), are interactive (`shell`, `tui`), or are long-running/human-facing (`web serve`, `web open`). `tracker init` gained a result payload at the same time, because bootstrap you cannot verify is not scriptable, and `board --json` lost its lone PascalCase `Columns` key.
7. **Confidence:** high
8. **Revisit Trigger:** The allowlist grows past a handful of entries, or a command needs machine output in a shape the versioned envelope cannot carry.
9. **Affected PRs/Files:** `internal/cli/root.go`, `internal/cli/actions.go`, `internal/cli/json_coverage_test.go`, `internal/config/config.go`, `internal/contracts/interfaces.go`.

## DEC-046

**Status:** DEC-065 adds explicit opt-in initialization. The workspace pin, default refusal of uninitialized roots, and workspace-independent discovery remain current.

1. **Decision ID:** DEC-046
2. **Date:** 2026-07-30
3. **Question:** How should an MCP client that does not control its working directory reach a specific Atlas workspace?
4. **Options Considered:**
   - Leave it at the working directory and tell people to register the server per project.
   - Add `--workspace` to `mcp serve` and validate the path before the server starts.
   - Read a workspace path from an environment variable or a config file in the user's home.
5. **Chosen Option:** `tracker mcp serve --workspace <path>`, checked at startup, with the working directory as the fallback.
6. **Why We Chose It:** User-scoped registrations (`claude mcp add --scope user`, a global Codex `mcp_servers` entry) are the normal way people install a local MCP server, and they start it wherever the client happens to be — which showed up as `not_found` for tickets the human could see in the terminal. An explicit flag keeps the workspace visible in the registration itself rather than hidden in a home-directory file. The path is validated before serving because an MCP client has no terminal to show a failure in: a missing directory is `not_found`, a directory without `.tracker/` says to run `tracker init` there. `mcp schema` and `mcp tools` describe the adapter and never open a workspace, so they do not take the flag.
7. **Confidence:** high
8. **Revisit Trigger:** MCP clients gain a standard way to pass a working directory, or Atlas needs one server to answer for several workspaces at once.
9. **Affected PRs/Files:** `internal/cli/mcp.go`, `internal/cli/mcp_test.go`, `docs/mcp.md`, `docs/mcp-claude-code.md`, `docs/mcp-codex.md`, `docs/guides/mcp-for-agents.md`.

## DEC-047

**Status:** DEC-066 records the existing explicit `openclaw --global` exception and portable generated references. Repository-local installation remains the default.

1. **Decision ID:** DEC-047
2. **Date:** 2026-07-30
3. **Question:** Where should `tracker integrations install openclaw` write, given that OpenClaw reads `AGENTS.md` like Codex does?
4. **Options Considered:**
   - Share the Codex block in `AGENTS.md` and install the skill to `~/.openclaw/skills`.
   - Write a second `AGENTS.md` block under its own markers and install the skill to the repo-local `.agents/skills` root.
   - Give OpenClaw its own instruction file so the two never meet.
5. **Chosen Option:** A second managed block in `AGENTS.md` with `atlas-tasker:openclaw` markers, plus the skill at `.agents/skills/atlas-worker/`.
6. **Why We Chose It:** OpenClaw's own precedence table puts repo-local project-agent skills at `<workspace>/.agents/skills`, which matches what the codex and claude targets already do with `.codex/skills` and `.claude/skills` — the skill travels with the repo and only applies where Atlas is. Sharing the Codex block would have made whichever target ran last silently win, and a separate instruction file would be a file OpenClaw does not read. `~/.openclaw/skills` is the shared per-machine root and stays the user's to install; a repo-scoped command reaching into a home directory is a surprise, so the guide prints the `openclaw skills install ... --global` one-liner instead. The generated skill carries `metadata.openclaw.requires.bins`, which gates it on the `tracker` binary and which other agents ignore. The same pass fixed a bare `": "` in the skill description across all four targets — that is not a legal plain YAML scalar, so no agent had been able to parse the frontmatter.
7. **Confidence:** high
8. **Revisit Trigger:** OpenClaw changes its skill roots or stops injecting `AGENTS.md`, or a fourth AGENTS.md-reading target makes per-target markers unwieldy.
9. **Affected PRs/Files:** `internal/integrations/install.go`, `internal/integrations/agent_skill.go`, `internal/integrations/install_test.go`, `internal/cli/root.go`, `docs/command-reference.md`, `docs/guides/team-presets.md`.

## DEC-048

**Status:** DEC-062 and DEC-066 clarify actor resolution and reason enforcement in the operating guidance. The operator-first documentation structure remains current.

1. **Decision ID:** DEC-048
2. **Date:** 2026-07-30
3. **Question:** Who is the root `AGENTS.md` for, now that agents arrive at this repo to use Atlas rather than to build it?
4. **Options Considered:**
   - Keep the v1 contributor guide and add a section for agents using the tracker.
   - Archive the contributor guide and write a new root `AGENTS.md` for agents operating Atlas.
   - Point `AGENTS.md` at the existing per-provider guides under `docs/guides/`.
5. **Chosen Option:** Archive the v1 guide as `docs/v1-agents-archive.md` and write a new root `AGENTS.md` for agents driving the tracker, with `CLAUDE.md` importing it via `@AGENTS.md`.
6. **Why We Chose It:** The old file described the PR-001..PR-009 delivery train and a locked v1 scope — accurate history, useless to an agent asked to work a ticket, and actively misleading as the first thing a coding agent reads. The new file leads with the failure modes rather than a feature tour: every write needs `--actor` and `--reason`, `project create` takes neither, a forbidden transition is a deliberate exit 4 rather than a bug to retry around. The web board is named as human-only in the same list, because its session token is random per process and never persisted — there is no headless path, and an agent that tries to scrape it is working against the design when the CLI and MCP are right there. Claude Code reads `CLAUDE.md` rather than `AGENTS.md`, so the bridge is an import rather than a second copy to drift.
7. **Confidence:** high
8. **Revisit Trigger:** The don'ts list stops matching real agent failures, or the web board grows a non-interactive auth path.
9. **Affected PRs/Files:** `AGENTS.md`, `CLAUDE.md`, `docs/v1-agents-archive.md`, `README.md`, `docs/README.md`.

## DEC-049

1. **Decision ID:** DEC-049
2. **Date:** 2026-09-01
3. **Question:** When the last blocker of a dependent ticket reaches `done`, should Atlas promote the dependent to `ready` itself, or keep promotion as the woken agent's first move?
4. **Options Considered:**
   - Keep the wakeup-only behavior and rely on the agent to promote the ticket.
   - Promote the dependent automatically when its last blocker completes.
   - Leave status alone but make the queues show unblocked backlog tickets.
5. **Chosen Option:** Both. Atlas promotes an agent-assigned `backlog` dependent to `ready` under the system actor `agent:atlas`, with an audited reason naming the completed blocker and who completed it, as a best-effort step after the completion commits — and `queue`/`next` gain an `unblocked_for_me` category while `agent available` reports a `promote` action, so human-assigned and unassigned dependents surface without being moved.
6. **Why We Chose It:** The wakeup pointed at a ticket no query would show. `agent.work_available` fired, the notifier printed it, and then `tracker next`, `agent available`, and `queue` all came back empty for the woken agent, because every one of them keyed on persisted status `ready` and the dependent was still `backlog`. Telling the agent "promoting is your first move" in a code comment does nothing when its own tooling never hands it the ticket. Promotion alone would have fixed the agent case and left humans in the same hole; the read-path change alone would have left agents doing a status dance on every wakeup. Doing both keeps the audit trail honest (`agent:atlas`, not the human who completed the blocker, moves the dependent) and keeps human backlog grooming manual. Plain backlog that never had blockers is deliberately excluded from the new category so the whole backlog does not pour into every agent's `next`, and a hand-set `blocked` status is never overridden. Nesting the promotion inside the completion's post-commit hook exposed a latent journal weakness: a write that dies after its canonical file keeps its event id, and the very next write (here, the wakeup) used to overwrite its journal entry, so `journal.Begin` now refuses a pending entry and points at `doctor --repair` instead of letting the half-applied move vanish.
7. **Confidence:** high
8. **Revisit Trigger:** A workspace that needs backlog grooming to stay manual even for agent-assigned tickets; that would need a config switch rather than a code comment.
9. **Affected PRs/Files:** `internal/service/agent_wakeup.go`, `internal/service/journal.go`, `internal/service/agent_work.go`, `internal/service/query.go`, `internal/service/types.go`, `internal/cli/root.go`, `internal/tui/app.go`, `internal/integrations/agent_skill.go`, `AGENTS.md`, `README.md`, `CHANGELOG.md`, `docs/v1.9-agent-workflow.md`, `docs/command-reference.md`, `docs/KNOWN_LIMITATIONS.md`, `site/mcp.html`, `site/changelog.html`, `site/docs/agents-and-dispatch.html`, `site/docs/json-and-exit-codes.html`, `site/docs/views-and-search.html`.

## DEC-050

Rebuild, watermark advancement, and recovery locking are superseded by DEC-052. The count-based on-open policy remains; DEC-052 records why live readers require a different commit strategy.

1. **Decision ID:** DEC-050
2. **Date:** 2026-09-01
3. **Question:** When the derived SQLite index is missing or stale relative to the markdown and event log, should a command error, warn, or rebuild it on its own?
4. **Options Considered:**
   - Loud error everywhere: any fingerprint mismatch is `repair_needed` (exit 7) until the operator runs `doctor --repair`.
   - Leave it as it was: `CREATE TABLE IF NOT EXISTS` on open, an empty or behind index answers as if it were the truth, and only a byte-corrupt file is detected.
   - Self-heal on open, with `doctor` as the honest reporter: rebuild under the write lock when the stamped fingerprint disagrees with the sources, print one notice, and have read-only `doctor` report drift as exit 7 instead of `ok`.
5. **Chosen Option:** Self-heal on open plus a single stderr notice; `doctor` reports drift with both fingerprints; a byte-corrupt file stays loud everywhere except `reindex`, which removes and rebuilds it.
6. **Why We Chose It:** A derived artifact that lies is worse than one that rebuilds. A deleted `index.sqlite` printed an empty board with exit 0, `ticket view` silently fell back to markdown so two surfaces disagreed, and `doctor` printed `doctor ok` from markdown counts it never compared to the projection. The fingerprint is deliberately just counts — event-log newline bytes plus ticket files — because a rebuild replays every event and then inserts only the tickets the projection is missing, so count and ID-set drift is exactly the drift a rebuild is guaranteed to clear, and every append grows one file by one line. A missing stamp is treated as the zero fingerprint, which makes a workspace fresh from `tracker init` read as fresh and an index from before the stamp existed rebuild once. The rebuild swaps files under `index.sqlite`, and reads take no lock today, so the check takes the workspace write lock and re-checks inside it. Corruption stays loud because a long-running server should never have a damaged file thrown away underneath it; `reindex` is the explicit way out and could not previously open the very file it exists to replace.
7. **Confidence:** high
8. **Revisit Trigger:** Workspaces large enough that a rebuild stops being a fraction of a second, or the per-command fingerprint stat over `.tracker/events/*.jsonl` and `projects/*/tickets/*.md` becomes visible in command latency.
9. **Affected PRs/Files:** `internal/storage/fingerprint.go`, `internal/storage/sqlite/store.go`, `internal/service/projection_freshness.go`, `internal/cli/actions.go`, `internal/cli/execute.go`, `internal/cli/root.go`, `internal/mcp/workspace.go`, `internal/tui/app.go`, `docs/invariants.md`, `docs/guides/doctor-and-repair.md`, `docs/troubleshooting.md`, `docs/operator-manual.md`, `docs/json-contracts.md`, `docs/errors.md`, `README.md`, `AGENTS.md`, `site/cli.html`, `site/docs/json-and-exit-codes.html`, `site/docs/faq.html`, `CHANGELOG.md`.

## DEC-051

**Status:** DEC-065 adds explicit MCP startup initialization. All default initialized-root and secret/path boundaries remain current.

1. **Decision ID:** DEC-051
2. **Date:** 2026-09-08
3. **Question:** How should the v1.10 interfaces enforce their existing local workspace and secret boundaries?
4. **Options Considered:** Keep surface-specific checks; share initialized-root validation and preflight every integration destination.
5. **Chosen Option:** Use a shared initialized-root check for CLI config/reads, MCP serving/approvals, and TUI. Keep init and explicit integration installation as bootstrap operations, and version/help/MCP schema/tools as workspace-independent discovery. Mask config-set JSON exactly like config-get. Normalize web bind hosts before listening and validate the actual loopback listener. Reject symlink components in every integration output path before writing any file.
6. **Why We Chose It:** The review reproduced silent workspace creation through implicit MCP/TUI startup, default config reads from the wrong directory, webhook secrets echoed by the JSON setter, a wildcard listener created by an empty host, and integration writes escaping through symlinks. These changes enforce DEC-044 and DEC-047 without adding remote serving or shared-skill installation.
7. **Confidence:** high
8. **Revisit Trigger:** A future explicitly approved remote web mode or shared integration installer requires a separate trust model.
9. **Affected PRs/Files:** PRs #121, #122, #127; internal/service/workspace.go, internal/cli, internal/mcp/workspace.go, internal/tui/app.go, internal/web/listener.go, internal/integrations/install.go, AGENTS.md

## DEC-052

1. **Decision ID:** DEC-052
2. **Date:** 2026-09-08
3. **Question:** How can projection rebuilds and recovery preserve live readers and truthful freshness?
4. **Options Considered:** Continue replacing the index file; reopen every live consumer on file changes; commit rebuilds in the existing SQLite database.
5. **Chosen Option:** Commit full and project rebuilds, event application, and schema initialization in SQLite transactions. Use immediate write transactions and configure the driver busy timeout on every pooled connection. WAL setup uses the existing workspace lock wait and polling interval (five seconds and 50ms) because SQLite can bypass its busy handler during simultaneous conversion. Hold the canonical workspace lock before corrupt-index reset and through rebuilding. Read-only doctor reports pending journals as repair_needed. Advance an incremental source watermark only when it accounts for at most the next appended event; a complete replay can stamp the complete source count.
6. **Why We Chose It:** This supersedes the file-swap portion of DEC-050 and V13-005: inode replacement stranded already-open MCP/web/TUI pools, and failed project rebuilds could erase rows. A later successful apply could also hide an earlier skipped event. Transactions preserve rollback and reader continuity. Counting projected rows is insufficient because imported duplicate source events can share projection IDs. The retained source-count policy reads complete event files and still does not detect same-count manual content edits; authoritative events remain necessary to recover edited state.
7. **Confidence:** high
8. **Revisit Trigger:** Measured event-log scan or replay latency requires a new watermark contract, or support is added for replacing a healthy database underneath a live process.
9. **Affected PRs/Files:** PR #123; internal/storage/sqlite/store.go, internal/service/projection_freshness.go, internal/cli/root.go, internal/cli/actions.go, docs/storage-transaction-model.md, docs/v1.3-decision-log.md

## DEC-053

1. **Decision ID:** DEC-053
2. **Date:** 2026-09-08
3. **Question:** Which validation must run for the reconciled v1.10 release train?
4. **Options Considered:** Rely on historical PR checks; rerun required checks and extend the gaps in the existing workflows.
5. **Chosen Option:** Use Go 1.26.6 and x/net v0.56.0 (with its required x/term v0.44.0 and x/sys v0.46.0), retaining the audit goldmark v1.7.17 and x/text v0.39.0 fixes. Run tests/vet, workflow policy, browser/site contracts, stabilization, packaged RC and local install rehearsal, vulnerability scanning, full-history secret scanning, and SBOM generation. Include testing pushes in CI and include every YAML workflow in action-pin and least-privilege checks. Exercise an actual stdio MCP initialize/tools call from an unrelated directory using an explicit workspace. Use v1.10.0-rc1 as the local rehearsal version; creation/publication of any tag remains a separate owner action.
6. **Why We Chose It:** Historical green checks did not prove the current combined tree, testing pushes had no CI, the scheduled vulnerability workflow escaped the new policy, and the packaged validator checked only MCP inventory. Hosted downloads, attestations, owner merges, and stable sign-off cannot be proven by a local rehearsal and remain explicit release gates.
7. **Confidence:** high
8. **Revisit Trigger:** The supported platforms, release version, public output contracts, or required CI policy change.
9. **Affected PRs/Files:** PRs #117, #121, #126; .github/workflows, scripts/check-workflow-security.sh, scripts/validate_rc.py, scripts/*release*.sh, scripts/validate-rc.sh, docs/release/public-release-gates.md

## DEC-054

1. **Decision ID:** DEC-054
2. **Date:** 2026-09-08
3. **Question:** May a reviewer promote another assignee's unblocked backlog ticket?
4. **Options Considered:** Offer promotion to every relevant actor including reviewers; offer it only to the assigned worker, unassigned claimable work, or the owner.
5. **Chosen Option:** Limit the new promote action to the ticket assignee, unassigned claimable work, and human:owner. Keep reviewer visibility and the existing in_review action. Preserve existing action authorization; this correction narrows the new work recommendation.
6. **Why We Chose It:** Review showed that relevance through reviewer assignment could recommend a successful claim/start sequence on human-owned backlog work. DEC-049 keeps human backlog grooming manual. Denied policy, lease, disabled-agent, and missing-capability cases also need regression coverage proving no promotion event or wakeup occurs.
7. **Confidence:** high
8. **Revisit Trigger:** The owner introduces explicit reviewer authority to take over assigned backlog work.
9. **Affected PRs/Files:** PR #123; internal/service/agent_work.go, internal/service/agent_work_test.go, internal/service/agent_wakeup_policy_test.go

## DEC-055

1. **Decision ID:** DEC-055
2. **Date:** 2026-09-08
3. **Question:** Which artifact should local release preflight use to generate its SBOM?
4. **Options Considered:** Discover the application from source and Git metadata; inspect the release binary already built and validated by preflight.
5. **Chosen Option:** Use the pinned CycloneDX generator's binary mode with the explicit release version and the preflight binary.
6. **Why We Chose It:** Source-mode version discovery failed in a linked Git worktree. Binary mode records the dependencies of the artifact actually rehearsed and avoids relying on the layout of Git's worktree references. Hosted CI and release source-mode generation remain valid in their ordinary checkouts.
7. **Confidence:** high
8. **Revisit Trigger:** Release artifacts stop carrying Go build information or the required SBOM scope expands beyond binary dependencies.
9. **Affected PRs/Files:** PR #126; scripts/preflight-release.sh

## DEC-056

1. **Decision ID:** DEC-056
2. **Date:** 2026-09-08
3. **Question:** How should the owner-merged v1.10 release train become the current stable release?
4. **Options Considered:** Stop after a published RC; publish stable immediately from local proof; publish and verify the RC before stable.
5. **Chosen Option:** Follow the owner's explicit “RC then stable” direction: promote the reviewed testing tree through a PR to main, publish the immutable v1.10.0-rc1 tag, verify hosted checksums/provenance/install/packaged smoke, record the evidence and owner acceptance, then publish v1.10.0 from main. Verify the unpinned latest installer and record its results on the stable release page. Keep testing and dev current with the released tree without rewriting unrelated work.
6. **Why We Chose It:** Hosted assets and their install behavior require direct proof. This executes the owner publication action required by DEC-053 while preserving its checks, the branch promotion flow, and immutable artifact provenance. Keeping post-tag verification on the release page avoids moving a published tag merely to include its own results.
7. **Confidence:** high
8. **Revisit Trigger:** A hosted artifact fails verification, the owner changes the release scope, or the release/branch policy changes.
9. **Affected PRs/Files:** PR #128 and subsequent v1.10 release-documentation/promotion PRs; CHANGELOG.md, README.md, docs/README.md, docs/release.md, docs/release/launch-checklist.md, docs/release/v1.10.0-release-evidence.md, site/changelog.html.

## DEC-057

1. **Decision ID:** DEC-057
2. **Date:** 2026-09-09
3. **Question:** How should a fresh install offer coding-agent setup without creating an unintended workspace or treating cancellation as consent?
4. **Options Considered:** Leave setup entirely manual; initialize automatically after installation; offer an explicit current-directory prompt through the controlling terminal.
5. **Chosen Option:** After the binary passes the existing checksum and attestation checks, offer setup only with a terminal, defaulting to No and showing the current directory. `SKIP_INTEGRATIONS=1` suppresses the offer. Explicit consent runs `tracker init --integrations`; older pinned binaries without that option retain manual setup. Keep the existing interactive init offer, preserve buffered answers for its picker, and treat EOF, `none`, and cancellation as a successful skip. Validate a non-empty integration selection and global-target compatibility before integration installation bootstraps a workspace.
6. **Why We Chose It:** The v1.11 binary offered integrations during init, but the shell installer did not offer it. Testing also reproduced buffered answers being lost, empty input accepting detected defaults, and a canceled or invalid integration install writing workspace files. The new offer makes the user's intended first-install journey real while retaining unattended installation and explicit file-write consent. Repository guidance remains separate from MCP client registration.
7. **Confidence:** high
8. **Revisit Trigger:** A new installation platform lacks a controlling terminal, or integration installation gains a distinct configuration destination or authorization model.
9. **Affected PRs/Files:** v1.12; scripts/install.sh, scripts/test-install.py, internal/cli/integrations_wizard.go, internal/integrations/select.go, CLI/integration regression tests, docs/installation.md, docs/guides/agent-integrations.md, README.md, site/mcp.html.

## DEC-058

1. **Decision ID:** DEC-058
2. **Date:** 2026-09-09
3. **Question:** Which web languages and public identity assets should the v1.12 documentation support?
4. **Options Considered:** Keep the three-language config allowlist and existing captures; accept every shipped catalog and replace public captures with synthetic data.
5. **Chosen Option:** Accept `en`, `es`, `id`, `zh`, `ja`, and `ko` through the existing config path and validate rendering for every actual catalog. This supersedes DEC-036's initial three-language scope while retaining its precedence and canonical-content rules. Capture the current browser UI in an isolated synthetic workspace with owner name `User`; remove personal contact and author data from current public files. Use byte-identical copies of the existing side-by-side UI wordmark in README and site, with a reproducible social-card render.
6. **Why We Chose It:** Chinese, Japanese, and Korean already rendered correctly through the language switcher but were rejected as persistent preferences. The owner explicitly requested all six and neutral screenshots. Reusing actual rendered UI and its existing wordmark preserves product identity without fabricating interface content. Rewriting published Git history would invalidate immutable release hashes and provenance, so historical commits and authorship remain intact.
7. **Confidence:** high
8. **Revisit Trigger:** A catalog is added or removed, the web UI gains translated schedule content, the product wordmark changes, or the owner separately authorizes a coordinated history rewrite.
9. **Affected PRs/Files:** v1.12; internal/contracts/domain.go, internal/config/web_languages_test.go, internal/web/configured_languages_test.go, examples/create-web-demo.sh, docs/assets, assets/brand, site/assets/web-board.webp, site/og.png, site public pages, docs/i18n-notes.md, docs/web-board.md, ROADMAP.md.

## DEC-059

1. **Decision ID:** DEC-059
2. **Date:** 2026-09-09
3. **Question:** What proves the v1.12 agent setup and documentation release is ready for owner approval?
4. **Options Considered:** Reuse v1.11's release proof; validate only static documentation; validate the integrated candidate and record remaining hosted gates explicitly.
5. **Chosen Option:** Run the required fresh Go tests/vet, workflow policy, site/browser contracts, local RC/rehearsal/stability and security checks. Add a Python-standard-library installer PTY harness and a real stdio MCP workflow harness to CI on Linux and macOS. Check both MCP framings, exact profile inventories, read-profile write denial, and the actor-separated ticket loop from an unrelated client directory. Review current docs against source and render Docs/Connect Your Agent at desktop and phone sizes. Prepare a PR to testing from the current dev-based release branch; this supersedes DEC-002's main-targeted v1 implementation sequence for ongoing release work. Require owner approval before promotion and release publication, then verify hosted RC and stable artifacts using the existing immutable-tag release process.
6. **Why We Chose It:** Static command examples cannot prove the installer reads the correct terminal or that an MCP-only client completes a governed ticket. The owner requested a finished, reviewable result before the approval request. Existing published v1.11 commits are included because main advanced while the dev/testing pipeline remained on v1.10; the new release must preserve those features and reconcile the pipeline.
7. **Confidence:** high
8. **Revisit Trigger:** Required CI or branch policy changes, a supported platform fails the terminal harness, or hosted artifact verification fails.
9. **Affected PRs/Files:** v1.12; .github/workflows/ci.yml, scripts/verify-mcp-workflow.py, scripts/test-install.py, examples/generate-demo-assets.sh, docs/examples, site/_tools, docs/release/public-release-gates.md, docs/release/v1.12.0-release-evidence.md, TEST_STDOUT.log.

## DEC-060

1. **Decision ID:** DEC-060
2. **Date:** 2026-09-09
3. **Question:** Which public origin and security contact should the v1.12 website preserve during publication?
4. **Options Considered:** Keep the stale staging origin and legacy personal contact from main; copy the older deployed site wholesale; reconcile the current approved site with verified production settings and the repository security policy.
5. **Chosen Option:** Preserve the existing Vercel project and verified `https://atlastasker.com` domain. Apply that origin to canonical/discovery metadata and the social card. Use the GitHub private vulnerability-reporting URL from SECURITY.md in the hidden security.txt file. Keep current product design, neutral screenshots, and repository routing. Link active release-status prose to the latest published release instead of hardcoding the previous stable version.
6. **Why We Chose It:** The production-domain change existed in a separate deployment worktree and had not reached the release source. Reconciling only its verified settings avoids regressing the approved UI or reintroducing personal metadata. A hidden contact file must receive the same privacy review as visible pages.
7. **Confidence:** high
8. **Revisit Trigger:** The owner changes the product domain, hosting project, security-reporting channel, or release policy.
9. **Affected PRs/Files:** v1.12 publication; site/_tools, site/.well-known/security.txt, site HTML and discovery files, assets/brand/social-card.html, site/og.png, README.md, SECURITY.md, ROADMAP.md, CHANGELOG.md, docs/installation.md, docs/getting-started.md, docs/guides/updating.md, docs/release.md, docs/release.

## DEC-061

1. **Decision ID:** DEC-061
2. **Date:** 2026-09-09
3. **Question:** How should board surfaces distinguish canceled work and show assignment?
4. **Options Considered:** Retain the combined terminal bucket; add a canonical canceled column and visible assignee text.
5. **Chosen Option:** Preserve `canceled` in board snapshots and a separate column across CLI, JSON, saved views, TUI, and web. Include canceled in the web move control; use raw status tokens in `data-status` and a separate translated label. Show assignee text on cards and terminal views. Keep explicitly configured saved-view columns and archived-ticket filtering.
6. **Why We Chose It:** The combined bucket made canceled work look completed and disagreed with the welcome Done count. This supersedes DEC-026's canceled bucketing, DEC-033's explanation of that mismatch, and DEC-035's color-only assignment hint. Dependency-aware blocked projection, canonical Done rollups, and the existing card design remain. Assigned plain backlog already appears on boards; its exclusion from executable agent work remains the intentional DEC-049 readiness contract.
7. **Confidence:** high
8. **Revisit Trigger:** The owner requests a configurable closed-work grouping or changes backlog readiness semantics.
9. **Affected PRs/Files:** internal/contracts/domain.go, internal/storage/sqlite/store.go, internal/service/workflow_readiness.go, internal/render, internal/cli/root.go, internal/tui, internal/web, docs/web-board.md, docs/command-reference.md.

## DEC-062

1. **Decision ID:** DEC-062
2. **Date:** 2026-09-09
3. **Question:** How should CLI mutation commands resolve identity consistently without repeated flags?
4. **Options Considered:** Keep legacy owner defaults; require an explicit flag on every write; use the existing explicit/environment/configured identity chain for every mutation-flag command.
5. **Chosen Option:** Resolve `--actor`, then `TRACKER_ACTOR`, then `actor.default` in the common mutation preflight. Missing or invalid identity fails with exit 2 before opening a writable workspace. Remove the `human:owner` fallback. Keep explicit-or-template ticket type and existing reason enforcement: MCP tracked writes and protected/security operations require reasons; ordinary CLI reasons are recommended.
6. **Why We Chose It:** Bulk, assignment, team, and scheduling commands silently attributed work to the owner while create/move/claim did not. Reusing configured identity removes repetition without inventing authority. Type remains a deliberate storage choice, and tightening every ordinary CLI reason would break existing callers without fixing attribution. This clarifies DEC-048's overly broad statement that every write needs literal actor and reason flags.
7. **Confidence:** high
8. **Revisit Trigger:** The owner chooses a breaking CLI reason policy or a new identity source.
9. **Affected PRs/Files:** internal/cli/root.go, internal/cli/actions.go, internal/cli/workflow_consistency_test.go, AGENTS.md, docs/command-reference.md.

## DEC-063

1. **Decision ID:** DEC-063
2. **Date:** 2026-09-09
3. **Question:** May a claim take another assignee's work, or a newly set schedule begin in the past?
4. **Options Considered:** Allow implicit takeover and immediate overdue execution; reject both before mutation.
5. **Chosen Option:** Non-review claims conflict with any different non-empty assignee, including owner claims; require explicit reassignment first. Review claims retain reviewer authorization while preserving the implementation assignee. Apply the same rule to bulk preview/execution. New schedules must be strictly after the service clock, with exit 2 otherwise; existing schedules can naturally become overdue and tick normally.
6. **Why We Chose It:** Assignment must mean ownership, and a date-entry mistake must not immediately dispatch work. Both checks precede state changes, so refused claims do not expire or steal a lease and refused schedules do not alter assignment. No backdated-schedule override is introduced.
7. **Confidence:** high
8. **Revisit Trigger:** A concrete import or recovery workflow needs explicitly authorized historical schedules or work takeover.
9. **Affected PRs/Files:** internal/service/action.go, internal/service/bulk.go, internal/service/schedule.go, internal/service/workflow_consistency_test.go, CLI/web schedule tests and docs.

## DEC-064

1. **Decision ID:** DEC-064
2. **Date:** 2026-09-09
3. **Question:** How should review-team presets enforce their advertised lifecycle for existing and new projects?
4. **Options Considered:** Set only workspace completion mode; force every ticket policy; supply a workspace reviewer and remove project open overrides when explicitly applying a review preset.
5. **Chosen Option:** Add optional `workflow.required_reviewer`, inherited before project/epic/ticket overrides. New projects inherit workspace completion mode. Pair/crossfire choose `agent:reviewer-1`, swarm chooses `agent:qa-1`, and solo clears the workspace reviewer. Applying a review preset clears existing project `open` overrides to inheritance and reports each change; explicit other project modes/reviewer overrides remain. Hold the workspace write lock across preset reads and writes so concurrent policy changes cannot be lost. Dry-run performs no writes. Ticket approval enforces `gate_approve` permission and, when approval completes review-gated work, `ticket_complete` permission.
6. **Why We Chose It:** The former project normalization wrote `open` over the workspace preset, and no default reviewer connected the installed roster to the effective policy. Owner/reviewer completion could therefore bypass review. The permission omission also allowed denied builders to approve. Explicit preset application now establishes the promised lifecycle without rewriting ticket policy or existing agent/runbook/profile customization. Existing explicit open projects also change on review-preset application; the output and team guide make that consequence visible.
7. **Confidence:** high
8. **Revisit Trigger:** Teams require per-project preset application or an explicit way to retain open projects during a workspace-wide review-team change.
9. **Affected PRs/Files:** internal/contracts/domain.go, internal/config/config.go, internal/service/policy.go, internal/service/team_presets.go, internal/service/action.go, workflow regression tests, docs/guides/team-presets.md, docs/command-reference.md.

## DEC-065

1. **Decision ID:** DEC-065
2. **Date:** 2026-09-09
3. **Question:** How can MCP complete ordinary workspace setup and ticket maintenance without widening default authority?
4. **Options Considered:** Enable all writes and implicit startup init; leave all gaps to the CLI; add typed workflow tools and an explicit startup bootstrap option.
5. **Chosen Option:** Add `atlas.project.create`, `atlas.ticket.heartbeat`, `atlas.ticket.priority`, `atlas.ticket.label.add`, `atlas.ticket.label.remove`, and ordinary-field `atlas.ticket.edit`. Ticket mutations require actor/reason; project creation retains the existing key/name-only, unaudited container service contract. `mcp serve --init-if-missing` requires an explicit absolute existing workspace and a write-capable profile, refuses nested roots and redirected init outputs, and never rewrites existing config or registers clients. Keep the default read profile, strict schemas, separate registration, and external CLI issuance of high-impact approval tokens.
6. **Why We Chose It:** The missing tools interrupted an otherwise complete agent loop. Explicit bootstrap extends DEC-046/DEC-051's default initialized-root rule only when the operator has named and opted into the destination. Strict field names, least-authority profiles, and external approval issuance prevent ambiguity and self-approval; they are not workflow defects. The complete inventory is 88 tools, with read 41, workflow 73, delivery 77/79, and admin 77/88 according to delivery/admin opt-ins.
7. **Confidence:** high
8. **Revisit Trigger:** Project creation gains an audited actor-bearing service contract, MCP client registration gets an explicit supported installer, or the owner changes authority boundaries.
9. **Affected PRs/Files:** internal/mcp/tools.go, internal/mcp/ticket_workflow_mutations_test.go, internal/cli/mcp_bootstrap.go, internal/cli/mcp_bootstrap_test.go, scripts/verify-mcp-workflow.py, docs/mcp*.md, docs/guides/mcp-for-agents.md, site/mcp.html.

## DEC-066

1. **Decision ID:** DEC-066
2. **Date:** 2026-09-09
3. **Question:** How should generated guidance remain executable and portable after installation?
4. **Options Considered:** Preserve literal actor placeholders and installation-root paths; derive known identities, explicitly name missing configuration, and emit relative references.
5. **Chosen Option:** Goal/runtime briefs use the run worker or ticket assignee and effective reviewer with shell quoting. Unknown identities use quoted `TRACKER_ACTOR`/`TRACKER_REVIEWER` variables plus setup instructions. Review-gate approval is the final transition, so its brief does not suggest a second completion. Generated writes teach reasons and same-status no-ops. Use workspace-relative guide/skill paths and separate Generic/Grok skill directories; leave the legacy shared path untouched. Document the existing explicit OpenClaw global installer and accurately distinguish init's default-Yes offer from the shell installer's default-No offer.
6. **Why We Chose It:** Literal placeholders, `/tmp` installation paths, and last-writer-wins shared skills made generated instructions unreliable. This clarifies DEC-047's historical global-install wording and DEC-048's write guidance without silently changing machine-wide configuration or legacy customized files.
7. **Confidence:** high
8. **Revisit Trigger:** Another provider shares an output destination or generated workflows gain a new role or completion mode.
9. **Affected PRs/Files:** internal/integrations, internal/service/backup_goal_actions.go, related regression tests, README.md, docs/installation.md, docs/guides/agent-integrations.md, docs/guides/generic-agent.md.

## DEC-067

1. **Decision ID:** DEC-067
2. **Date:** 2026-09-09
3. **Question:** Should an unrelated directory under `projects` invalidate workspace discovery?
4. **Options Considered:** Treat every directory name as an Atlas project key; discover only directories containing a project marker.
5. **Chosen Option:** Require `project.md` before loading a project directory. Ignore unmanaged folders while retaining errors for malformed managed projects and include their marker path in the error. Preserve the existing nested-init refusal and all user files.
6. **Why We Chose It:** An empty `projects/foo` left by an attempted nested initialization is not an Atlas project, but the old enumeration validated its lowercase name before checking its contents and made doctor fail. Marker-based discovery avoids hiding malformed actual projects or deleting unrelated data.
7. **Confidence:** high
8. **Revisit Trigger:** The project storage marker changes or doctor gains a dedicated orphan-content report.
9. **Affected PRs/Files:** internal/storage/markdown/project_store.go, internal/cli/workflow_consistency_test.go.

## DEC-068

The preparation-only authorization boundary is superseded by DEC-069. The minor
version, compatibility decisions, and hosted proof requirements remain unchanged.

1. **Decision ID:** DEC-068
2. **Date:** 2026-09-09
3. **Question:** How should the workflow-consistency fixes enter the next stable release while keeping public documentation truthful?
4. **Options Considered:** Call the local fixes stable immediately; publish a patch without distinguishing new MCP interfaces; prepare a minor candidate through the existing owner-controlled promotion and RC gates.
5. **Chosen Option:** Prepare v1.13.0 and its first RC, publish the feature PR to `testing`, and keep v1.12.0 as the published stable until approved promotion and hosted RC/stable proof pass. Reconcile current docs, site copy, release helpers, and board images with the candidate, labeling upcoming behavior unreleased. Preserve historical snapshots and link them to published evidence. Add the final permission regression for review approval that implicitly completes a ticket.
6. **Why We Chose It:** Six new public MCP tools and explicit bootstrap warrant a minor candidate. Local tests and screenshots prove the candidate, while hosted assets and the live site require independent verification after publication. This follows DEC-059's promotion boundary and completes the owner's requested fixes without implying authorization to merge or release during preparation.
7. **Confidence:** high
8. **Revisit Trigger:** The owner changes the release version/scope, protected-branch requirements change, or hosted RC verification exposes a release defect.
9. **Affected PRs/Files:** CHANGELOG.md, docs/release.md, docs/release/*, scripts/preflight-release.sh, scripts/validate-rc.sh, scripts/release-rehearsal.sh, current guides/references, site/*, docs/assets/*, examples/create-web-demo.sh, internal/service/workflow_consistency_test.go.

## DEC-069

1. **Decision ID:** DEC-069
2. **Date:** 2026-09-10
3. **Question:** How should the approved v1.13 candidate proceed after the owner merged PR #141 and requested the full release?
4. **Options Considered:** Keep the earlier preparation-only hold; execute the explicitly authorized promotion and publication while retaining all verification gates.
5. **Chosen Option:** Complete the remaining PRs and normal protected-branch merges using the owner's authorized accounts for required reviews. Publish an RC from the approved main commit, verify downloaded artifacts, then publish stable from the same commit and verify it independently. Deploy the final site only after stable proof passes. Record post-publication evidence on the release pages without moving tags.
6. **Why We Chose It:** The owner's explicit full-release request supersedes DEC-068's preparation-only boundary. It authorizes the remaining actions without weakening code-owner review, checksums, provenance, installation, workflow, or production verification.
7. **Confidence:** high
8. **Revisit Trigger:** A required check fails, the source or required approvals change, or hosted proof exposes a release defect.
9. **Affected PRs/Files:** docs/release/v1.13.0-release-evidence.md, docs/release/launch-checklist.md, CHANGELOG.md, site/changelog.html, site/cli.html, site/docs/getting-started.html, site/docs/json-and-exit-codes.html, site/_tools/site-contract.test.mjs.

## DEC-070

Revisited 2026-09-10 by DEC-079 after independent review round 1 (findings S-A..S-D, T-A..T-C):
the ten states stand; the edge set was tightened (no edge back to `planned`, `failed` only from
`rolling_back`), the integration-to-operation mapping was made total and tested, and rollback
idempotency is now defined per action kind instead of assumed. Items 5 and 8 below read with those
amendments.

1. **Decision ID:** DEC-070
2. **Date:** 2026-09-10
3. **Question:** How does v1.14 add seamless agent setup without turning `tracker init` or the integration installer into an unreviewable multi-write operation?
4. **Options Considered:** Extend `tracker init --integrations` with MCP registration and backup; write provider configuration directly from each installer with best-effort cleanup; add a separate `tracker setup` orchestrator that plans read-only, applies one journaled transaction per provider, and treats backup as a separately consented group.
5. **Chosen Option:** The orchestrator. `tracker init` stays low-level. Every run produces complete validated plans before any write; each provider is its own transaction with snapshots and rollback; backup is a separate transaction group; repair and removal reuse the same adapter plans; the operation state machine is the closed set `planned, applying, applied, verifying, connected, pending_approval, failed, rolling_back, rolled_back, repair_required` with `verifying` mandatory before any success state and crash recovery defined per in-flight state (`internal/setup/operation.go`). Provider trust dialogs are reported as approval steps and never bypassed; no global provider scope is written without an explicit scope choice; Atlas installs no agent software.
6. **Why We Chose It:** Client configuration files hold other servers' credentials and the user's trust decisions, so partial writes and heuristic cleanup are unacceptable. A plan-first journaled model makes AT114-103's crash-injection matrix testable and lets partial success be reported per provider instead of as one failure. It extends DEC-047/DEC-048/DEC-066 (managed blocks, no machine-wide writes) rather than replacing them.
7. **Confidence:** high
8. **Revisit Trigger:** A provider offers a transactional registration API, or the crash-injection tests in AT114-103 show a state the machine cannot represent.
9. **Affected PRs/Files:** docs/v1.14-setup-transaction-model.md, internal/setup/operation.go, internal/setup/operation_test.go, docs/v1.14-implementation-plan.md (AT114-002, AT114-101..106).

## DEC-071

1. **Decision ID:** DEC-071
2. **Date:** 2026-09-10
3. **Question:** What single contract do the six provider adapters implement, and how are AT114-208's generic states reconciled with the nine states of locked decision 5?
4. **Options Considered:** Per-provider ad hoc installers extended with MCP writes; the plan's six-method interface as written; the six methods plus `Target()`/`Capabilities()` with a registry that checks each adapter against a frozen capability matrix, and the nine-state vocabulary plus a separate connection kind.
5. **Chosen Option:** `internal/integrations/adapter.AgentIntegrationAdapter` with eight methods; `Registry.Register` rejects adapters whose capabilities disagree with `Matrix()`. `State` is exactly the nine plan states; AT114-208's `connected_by_standard_config` / `connected_by_custom_adapter` become `Verification.ConnectionKind = standard_config | custom_adapter` on a `connected` state, and a generic verified result must carry one of those two kinds. Only `Verify` may produce a verified state; `Apply` may not. Plans are deterministic, fingerprinted, validated for containment, reversibility, approval, unsupported-version, and removal-ownership rules before anything is written. Client binaries run only through the structured `Command` model, which refuses shell interpreters, credential URLs, secret-looking environment names, and relative executables.
6. **Why We Chose It:** One interface with machine-checked capabilities keeps the six adapters honest about what they can verify (generic caps at `portable_ready`; OpenClaw at `connected_restart_required`). Keeping nine states honors the locked decision while preserving the plan's generic distinction as a field, which avoids two state vocabularies in status output. Structured commands satisfy the plan's "no adapter uses shell interpolation" acceptance by construction rather than by review.
7. **Confidence:** high
8. **Revisit Trigger:** Design review prefers eleven states, a client requires a non-stdio transport for local use, or a Sprint 114.2 real-client run contradicts a matrix row (then the row and its sources change, not the contract).
9. **Affected PRs/Files:** internal/integrations/adapter/*.go, docs/v1.14-provider-adapter-contract.md, docs/v1.14-implementation-plan.md (AT114-003, AT114-201, AT114-208, Verification notes).

## DEC-072

1. **Decision ID:** DEC-072
2. **Date:** 2026-09-10
3. **Question:** How is one Atlas MCP registration bound to exactly one workspace across clients whose configuration may travel with the repository?
4. **Options Considered:** Absolute `--workspace` everywhere; client-expanded variables everywhere; per-scope bindings (`absolute_path` for machine-local scopes, `client_variable` or `verified_cwd` for repository-carried scopes) with `--expected-workspace-id` only on the cwd form as the plan minimally requires; the same per-scope bindings with `--expected-workspace-id` on every form.
5. **Chosen Option:** Per-scope bindings with `--expected-workspace-id <UUID>` always present. Server name `atlas-` plus the first twelve hex characters of SHA-256 of the workspace UUID (`^atlas-[0-9a-f]{12}$`), never a project name or path. Fixed argv `mcp serve <binding> --expected-workspace-id <id> --tool-profile workflow --max-items 30 --max-result-bytes 65536`, derived by one function and re-derived on validation; `--dangerously-allow-high-impact-tools`, `--init-if-missing`, `--read-only`, and non-workflow profiles are rejected. Repository-carried scopes refuse absolute workspace paths and home-relative executables; portable descriptors name the bare `tracker` executable and use `verified_cwd`. JSON forms always render `"type": "stdio"`.
6. **Why We Chose It:** A still-valid absolute path can point at a moved, copied, or replaced workspace; pinning the expected ID makes every registration fail closed (AT114-102's rules) at negligible cost. The hashed name is safe as a TOML key, JSON key, and CLI argument and leaks neither path nor raw identity. Single-function derivation prevents hand-assembled argv drifting from the specification. `--workspace-from-cwd`/`--expected-workspace-id` do not exist in v1.13.0, which makes AT114-102 a hard dependency of Sprint 114.2 and is recorded as such.
7. **Confidence:** high
8. **Revisit Trigger:** AT114-102 cannot implement `--workspace-from-cwd` safely for a provider's project scope, or a client forbids the `type` field.
9. **Affected PRs/Files:** internal/integrations/adapter/registration.go, internal/integrations/adapter/registration_test.go, docs/v1.14-provider-adapter-contract.md §4, docs/v1.14-implementation-plan.md (AT114-102, AT114-201).

## DEC-073

Revisited 2026-09-10 by DEC-080 after independent review round 1 (findings M-A..M-C): `disabled`
additionally forbids `mcp_preferred`, the per-mode constructor returns an error for an invalid mode,
and `delivery` in the shared document takes effect only with machine-local enablement
(`EffectiveMode`). The seven-field document and its vocabulary are unchanged.

1. **Decision ID:** DEC-073
2. **Date:** 2026-09-10
3. **Question:** How is managed project mode expressed so that agents track material work without any path to waive existing workflow authority?
4. **Options Considered:** Free-form instructions in the skill only; a policy document with fields for reviewer/approval overrides; a closed seven-field `atlas_managed_mode_v1` document whose parser rejects unknown fields and whose only completion value defers to the workspace completion mode.
5. **Chosen Option:** `contracts.ManagedModePolicy` with `mode` (`guidance | managed | delivery | disabled`), `capture_policy` (`never | ask | material_work`), `progress_policy` (`none | milestones | every_checkpoint`), `status_policy` (`atlas_required` only), `completion_policy` (`follow_workspace` only), `mcp_preferred`; stored workspace-shared at `.tracker/managed-mode.json`; `DisallowUnknownFields`; `disabled` forces `never`/`none`; `guidance` forbids `mcp_preferred`; `delivery` is never a setup default. `WorkIntent` classification makes every non-material intent and explicit tracking exclusion yield `no_ticket` regardless of policy. The plan's alternate spellings (`query_atlas`, `follow_workspace_policy`) are rejected in favor of one vocabulary (`atlas_required`, `follow_workspace`); the plan text stays as approved.
6. **Why We Chose It:** The AT114-004 acceptance criteria (existing policies authoritative, no waivers, explicit actor, no silent self-approval) are guaranteed structurally when the document cannot carry a waiver field and completion always returns the workspace mode. One spelling per field keeps stored documents and status output unambiguous.
7. **Confidence:** high
8. **Revisit Trigger:** A second status or completion strategy is needed (then `atlas_managed_mode_v2`), or the review prefers the plan's `query_atlas` spelling.
9. **Affected PRs/Files:** internal/contracts/managed_mode.go, internal/contracts/managed_mode_test.go, docs/v1.14-managed-mode-contract.md, docs/v1.14-implementation-plan.md (AT114-004, AT114-301).

## DEC-074

Revisited 2026-09-10 after independent review round 1 (findings B-A..B-F, G-A; the model stands,
these are amendments recorded in `docs/v1.14-automatic-backup-adr.md` §2.1, §2.2, §3, §3.2, §3.3,
§4.1, §4.4, §6):

- **B-A (allowlist/candidate parity).** AT114-401's shared builder adds `collaborators`,
  `memberships`, `mentions`, and archive records with their Markdown payloads to the candidate list
  (all already restore-safe) and adds a parity test between `isCanonicalRestorePlanPath` and the
  collector with named exclusion lists. The exported-but-not-restore-safe direction (item 5 of the
  chosen option) stays inherited until the AT114-507 drill shows what setup re-creates.
- **G-A (restore governance).** AT114-506 evaluates `requireGovernance{backup_restore, workspace}`
  in `ApplyRestorePlan` before `commitMutation`; a denying policy is exit 5 with zero writes; `--yes`
  is a guard, not authorization. No protected action is added for checkpoint creation.
- **B-B.** There is no read lock; the checkpoint copies under the exclusive write lock with a
  bounded hold and yields (`busy` → retry) to writers; hashing, commit, push, and verify run outside
  the lock.
- **B-C.** Threat 2 is exclusion by path with no content redaction; redaction rules are canonical
  data and are included.
- **B-D.** `manifest_sha256` = SHA-256 of the canonical encoding with the field blanked; pinned by
  golden and round-trip tests. Product integrity hashing is outside the owner's SHA-ceremony waiver.
- **B-E.** The user's global/system Git configuration is not masked (credential helpers,
  `insteadOf`, `core.sshCommand` live there); only `commit.gpgsign`, `tag.gpgsign`, `core.hooksPath`,
  `protocol.*.allow`, and `credential.interactive` are pinned per invocation. `--force-with-lease` is
  rejected because locked decision 6 says backup never force-pushes; the `ls-remote`/push window is
  benign because the ref is per replica and single-writer, and a server-side non-fast-forward
  rejection reports `diverged`.
- **B-F.** Two threats added: SSH host-key trust on first connection (`host_key_unverified`, Atlas
  never writes `known_hosts` or relaxes host-key checking) and backup target equal to a workspace
  remote (warning at configuration and in `backup status`; vanished `refs/atlas/*` detected and
  re-published). Threat 18 records the restore-governance check.
- Confidence for the allowlist question rises to high for the never-collected direction (decided)
  and stays medium for the exported-only direction (drill-dependent).

1. **Decision ID:** DEC-074
2. **Date:** 2026-09-10
3. **Question:** How does automatic off-device backup work without touching the user's Git repository, pulling, merging, force-pushing, or leaking non-Atlas data?
4. **Options Considered:** Commit Atlas files on the user's branch and push to origin; a second backup format; an Atlas-owned isolated bare repository whose checkpoints contain exactly the existing restore-safe collector output plus a manifest, pushed fast-forward to a replica-specific ref and verified remotely.
5. **Chosen Option:** The isolated repository. Checkpoint tree = `backupRestoreSafeFiles(collectExportFiles(root))` plus `.atlas-checkpoint.json` (`atlas_git_checkpoint_v1` with per-stream watermarks keyed `workspace` and project key, canonical tree hash, manifest hash, per-file hashes). Git runs only with `GIT_DIR` set to the Atlas bare repository, a temp index, plumbing commands, empty hooks path, and a temp snapshot directory as cwd; ref `refs/atlas/backups/<workspace-id>/<replica-id>`; fast-forward only with an expected-tip check; verification by fetching the exact commit and comparing commit, tree, and manifest. Trigger via a machine-local outbox marked from a post-commit hook in `commitMutation` (skipped on replay, never failing the mutation), 30 s quiet / 5 min max / 100 events, explicit `backup tick`/`watch`, consented user-level scheduling. Automatic checkpoints append no canonical events. Restore fetches into temp storage, verifies manifest, workspace ID, allowlisted `100644` blobs and hashes, then goes through the existing restore-plan/apply boundary with explicit confirmation, reindex, and doctor. Targets are explicit, machine-local, credential-free URLs with a typed private-visibility attestation; origin is never offered. The existing allowlist gap (config.toml, agents, views, automations, subscriptions, runbooks, imports exported but not restore-safe) is inherited unchanged and raised for review rather than widened silently.
6. **Why We Chose It:** Reusing the existing collector, allowlist, restore planner, and apply boundary keeps one backup format and one restore path (locked decisions 7–10). Plumbing-only Git in a separate `GIT_DIR` is the only way to guarantee the user's HEAD, index, and work tree are untouched under crash. Appending backup events would make each checkpoint dirty the tree it just captured. Per-replica refs remove multi-machine contention without merges.
7. **Confidence:** high for the model; medium for the allowlist question, which review must answer.
8. **Revisit Trigger:** Review approves widening the restore-safe allowlist; the private-Git provider proves insufficient and an encrypted object-store provider is scheduled; or the outbox hook shows measurable mutation latency in AT114-407.
9. **Affected PRs/Files:** docs/v1.14-automatic-backup-adr.md, docs/v1.14-implementation-plan.md (AT114-005, AT114-401..407, AT114-501..507), later internal/service/import_export.go, internal/service/backup_goal_actions.go, internal/service/action.go, internal/contracts/backup.go.

## DEC-075

1. **Decision ID:** DEC-075
2. **Date:** 2026-09-10
3. **Question:** From which baseline and under which branch discipline is v1.14 developed and reviewed?
4. **Options Considered:** Continue on a floating branch head from an earlier session; start from `origin/dev` at the v1.13.0 promotion commit in an isolated worktree with sprint-gated independent review and no pull request before implementation approval.
5. **Chosen Option:** Base commit `b95fd640c5b6bd956aadf5ac08d4b5b460e33de2` (tree `21d08ea3c2f40840b1730325a515b2d8b533cf64`, equal to `origin/dev`, `origin/main`, `origin/testing`, `v1.13.0`), one isolated worktree on `feat/v1.14-seamless-agent-backup`, unrelated local work audited and left untouched, no changes to `dev`/`testing`/`main`. Each sprint ends with the full gate suite captured to `TEST_STDOUT.log`, evidence summarized in `docs/release/v1.14-baseline-evidence.md`, and an independent design review before the next sprint starts; pull requests target `testing` only after the completed implementation is approved. Procedural release SHA/checksum ceremony is waived by owner override; backup-product integrity hashing is not.
6. **Why We Chose It:** The plan requires an exact recorded baseline and forbids mixing unrelated work; the owner's directives fix the review and publication order. Recording the waiver here prevents later sprints from re-adding the ceremony as a completion gate.
7. **Confidence:** high
8. **Revisit Trigger:** The owner changes the release train, base branch, or publication order.
9. **Affected PRs/Files:** docs/release/v1.14-baseline-evidence.md, docs/v1.14-acceptance.md, docs/v1.14-implementation-plan.md (Plan status section).

## DEC-076

1. **Decision ID:** DEC-076
2. **Date:** 2026-09-10
3. **Question:** How does the Claude Code shared `.mcp.json` registration bind to the workspace, and which client placeholders may a repository-carried registration carry?
4. **Options Considered:** Keep `client_variable` with `${CLAUDE_PROJECT_DIR:-.}` as the earlier contract draft did; bind with `verified_cwd` (`--workspace-from-cwd --expected-workspace-id`) and restrict placeholders to one bare `${name}` that the client documents as interpolated; refuse repository-carried Claude registration altogether.
5. **Chosen Option:** `verified_cwd` for Claude's `.mcp.json` row; `client_variable` accepts only `^\$\{[A-Za-z][A-Za-z0-9_]*\}$` and only for Cursor's `.cursor/mcp.json`, the one project file whose client documents variable interpolation. Shell-style defaults (`${VAR:-default}`) are refused everywhere. Grok's compatibility import of Cursor/Claude files is recorded as passing placeholders literally, so an imported entry fails closed at `--expected-workspace-id` and AT114-207/209 must report the duplicate.
6. **Why We Chose It:** The official Claude Code documentation (re-read for review round 1) states that `CLAUDE_PROJECT_DIR` is set in the spawned server's environment and is not expanded by Claude Code inside a project `.mcp.json`; the earlier row would have passed the literal string `.` as `--workspace`, binding the server to whatever directory it started in. `verified_cwd` is exactly the plan's AT114-102 rule for providers without reliable variable expansion, and `--expected-workspace-id` (DEC-072) still fails closed if the start directory is wrong. Refusing defaults removes the failure class rather than special-casing one client.
7. **Confidence:** high for the binding and the placeholder rule; medium for the working directory Claude Code gives a project-scoped stdio server, which AT114-204's real-client run confirms.
8. **Revisit Trigger:** Claude Code documents variable interpolation inside `.mcp.json`, or AT114-204 shows the server is not started in the project directory (then AT114-102 must use the server-environment `CLAUDE_PROJECT_DIR` as corroborating evidence and the row changes).
9. **Affected PRs/Files:** internal/integrations/adapter/registration.go, internal/integrations/adapter/capabilities.go, internal/integrations/adapter/registration_test.go, internal/integrations/adapter/review_round1_test.go, docs/v1.14-provider-adapter-contract.md §4.1, §7.1, §8, docs/release/v1.14-s0-review-round1.md.

## DEC-077

1. **Decision ID:** DEC-077
2. **Date:** 2026-09-10
3. **Question:** How far does plan validation confine what an adapter may write and run, given that the journal engine of AT114-103 does not exist yet and Sprint 114.2 adapters will code against the contract?
4. **Options Considered:** Keep containment at "inside the workspace or a consented root" and rely on adapter review; add fail-closed structural rules to `IntegrationPlan.Validate` for path classes, rollback binding, executable identity, approval promises, and required inputs; move all such checks into the journal engine.
5. **Chosen Option:** Structural rules in the contract, live-filesystem checks in the engine. Managed-file steps may touch only Atlas-owned roots (instruction file, skill directory, `.tracker/integrations`, the client's command directory) or a consented root and never a client configuration file; config-entry steps carry the plan scope and may touch only the file the target documents for that scope with `atlas_file_edit`, or a user-selected consented destination (the generic `user` scope added for AT114-208). `.git` is never writable anywhere; `.tracker` admits only `integrations/**`. A rollback is bound to its step (same path; removals only by `restore_snapshot`; command steps only by `run_command` with the same executable and a `remove`/`reload` purpose). Every plan, rollback, detection, and verification command runs the detected client executable (or, for `probe`, the registered server executable); the shell/interpreter denylist is widened (`env`, `busybox`, Python, Perl, Ruby, Node, Deno, Bun) as defence in depth only. Approval steps and pending states imply each other exactly. `PlanInput.Home` is required and a repository-carried non-portable plan must carry `Home`; when home is unknown the conventional per-user roots are refused heuristically. Writes use `0644`/`0600`; `remove_local_state` exists; `FileIdentity` records `Owner{UID, GID}`. `Registry.Register` compares the whole capability struct. A standalone `Command.Validate` does **not** reject arbitrary binaries such as `/usr/bin/curl`: without a reference executable that check would be a denylist guessing game, so the tie lives at every level where a reference exists.
6. **Why We Chose It:** Review round 1 reproduced eight ways a syntactically valid plan could reach client configuration, Git metadata, tracker state, or a foreign binary while still validating. Encoding the rules in the contract makes Sprint 114.2 adapters fail at unit-test time instead of at review time, and keeps the engine's remaining checks (symlinks, ownership, current identity) to what needs the live filesystem. Confining managed files to Atlas-owned roots is stronger than the reviewer's minimum (excluding client config files) and follows DEC-047/DEC-066's managed-block ownership model.
7. **Confidence:** high
8. **Revisit Trigger:** A Sprint 114.2 adapter needs a legitimate write outside the enumerated roots (then the matrix gains a documented root, not an exemption), or AT114-103's engine finds a containment case the contract cannot express.
9. **Affected PRs/Files:** internal/integrations/adapter/plan.go, internal/integrations/adapter/capabilities.go, internal/integrations/adapter/command.go, internal/integrations/adapter/contract.go, internal/integrations/adapter/registration.go, internal/integrations/adapter/doc.go, internal/integrations/adapter/*_test.go, docs/v1.14-provider-adapter-contract.md §2, §5, §6, §7, docs/release/v1.14-s0-review-round1.md.

## DEC-078

1. **Decision ID:** DEC-078
2. **Date:** 2026-09-10
3. **Question:** Does a target's `MaxPlannedState` bound what `Verify` may report, and what evidence must a verified result carry?
4. **Options Considered:** Clamp `Verification.State` to the target's cap (the reviewer's suggestion); leave verification unbounded and unstructured; keep verification independent of the cap but require its connection kind to match the target class and to be backed by a passed check of one of that kind's own methods, with every probe tied to the detected client or registered server executable.
5. **Chosen Option:** The third. `MaxPlannedState` bounds what a plan may *promise* before anything runs (strict ranks: `connected` 5, `connected_restart_required` 4, `configured_unverified`/`portable_ready` 2, others 0). `Verification` reports what a post-apply probe *proved*: `client_native` must be backed by `client_cli_list`/`get`/`doctor`, `self_probe` by `self_probe`, `standard_config`/`custom_adapter` by `conformance_host`; generic targets use only the last two kinds and named clients never do; probes run `ClientExecutable` or, for `probe`, `ServerExecutable`, and are read-only.
6. **Why We Chose It:** A promised-state cap and post-probe verification answer different questions. OpenClaw's cap is `connected_restart_required` because the saved definition and the live gateway differ at plan time, yet `openclaw mcp doctor --probe` opens a live session and can prove `connected` afterwards; clamping would force the contract to report less than it observed. Requiring kind-specific evidence closes the actual gap the reviewer found (a `connected` result whose only passed check was a manual one or a method that cannot prove that kind).
7. **Confidence:** high
8. **Revisit Trigger:** A client's native listing proves less than a live session (then that method leaves the `client_native` evidence set), or design review still prefers the clamp after reading the OpenClaw case.
9. **Affected PRs/Files:** internal/integrations/adapter/plan.go (`stateRank`, `connectionKindEvidence`, `Verification.Validate`), internal/integrations/adapter/review_round1_test.go, docs/v1.14-provider-adapter-contract.md §3, docs/release/v1.14-s0-review-round1.md.

## DEC-079

1. **Decision ID:** DEC-079
2. **Date:** 2026-09-10
3. **Question:** How do the setup operation states preserve "planned means nothing was written" and "failed means a rollback failed", and how is an adapter's integration outcome mapped to an operation state so partial success is reported honestly?
4. **Options Considered:** Keep DEC-070's edges (`failed -> planned`, `rolled_back -> planned`, `repair_required -> planned`, `applying -> failed`, `verifying -> failed`) and document the caveats; remove those edges and add an eleventh operation state `unverified` for outcomes no probe can verify; remove those edges, keep the ten states, add a total tested `OutcomeFor` mapping, broaden `pending_approval` to "writes kept, not proven connected", rename `Succeeded()` to `KeepsWrites()`, and carry the integration state plus a run-level status in every report.
5. **Chosen Option:** The third. Exactly seventeen edges remain; no edge leads back to `planned` (a fresh plan is a new operation reading the old journal entry); `failed` is reachable only from `rolling_back` and leads only to `rolling_back`; `rolled_back` and `repair_required` are final. `OutcomeFor` maps `connected`/`connected_restart_required` to `connected`; the two pending states, `configured_unverified`, `portable_ready`, and `unsupported_client_version` to `pending_approval`; `repair_required` to `repair_required`; `failed` to `rolling_back`. A plan with no write steps creates no operation. Run-level status is `connected | pending | unverified` (exit 0, distinct strings) or `partial | failed` (non-zero, codes fixed in AT114-104), with per-provider operation and integration states in `--json`. Rollback idempotency is defined per action kind (`restore_snapshot` compares identities, `delete_created` treats ENOENT as success, `run_command` removals are preceded by the provider's read-only listing probe) and implemented in AT114-103 with a resume-twice table test. Lock order is setup lock then workspace write lock, never reversed; a `busy` workspace lock fails the step and rolls back. Machine-wide scopes (OpenClaw gateway, any `user` scope) are never pre-selected and must be named in noninteractive mode. Rollback material is deleted at commit or `rolled_back` and retained only while `failed`.
6. **Why We Chose It:** Review round 1 showed that two documented guarantees were false under the old edges and that the integration-to-operation mapping existed only in prose. An eleventh state would contradict DEC-070's closed set for a distinction that the integration state already carries; a total mapping plus a run-level vocabulary gives agents the branch they need (`status == "connected"`) without a second state machine. Client CLIs are not idempotent (`openclaw mcp unset` fails on an absent name), so idempotency has to be constructed by the engine, not assumed.
7. **Confidence:** high
8. **Revisit Trigger:** AT114-103's crash-injection matrix finds an outcome the seventeen edges cannot express, or AT114-104 cannot give `partial`/`failed` distinct codes inside the existing exit-code table.
9. **Affected PRs/Files:** internal/setup/operation.go, internal/setup/operation_test.go, docs/v1.14-setup-transaction-model.md §1, §2, §3, §4, §5, §6, §7, §8, docs/v1.14-acceptance.md (AT114-103/104 requirements), docs/release/v1.14-s0-review-round1.md.

## DEC-080

1. **Decision ID:** DEC-080
2. **Date:** 2026-09-10
3. **Question:** How can `delivery` mode "require an explicit enable" when the managed-mode document is committed, cloned, imported, and restored with the repository?
4. **Options Considered:** Reject `mode: delivery` in the shared file and keep delivery entirely machine-local; accept it in the shared file and let it take effect on every machine; accept it in the shared file as a declaration and make the effective mode depend on this machine's private setup state recording a delivery-profile server registration.
5. **Chosen Option:** The third. `ManagedModePolicy.EffectiveMode(deliveryEnabledLocally)` returns `managed` for a declared `delivery` without local enablement and never upgrades a non-delivery declaration; `AllowsDelivery()` is read only from the effective mode; `atlas.context`/`atlas.status` report declared and effective mode. Additionally `disabled` forbids `mcp_preferred` (no server is registered in that mode) and `ManagedModePolicyForMode` returns an error for an invalid mode so no caller holds a policy that fails validation.
6. **Why We Chose It:** Rejecting the value in the shared file would make a team's intent unexpressible; letting it take effect would make the first committer's choice binding for every clone and every restored workspace, which is the property review finding M-C showed to be missing. Layering the machine-local enablement follows locked decision 3 (delivery is a separately registered profile server) and DEC-070's rule that no machine-wide capability is enabled without an explicit local action.
7. **Confidence:** high
8. **Revisit Trigger:** A deployment needs repository-wide delivery enablement (then an explicit team-policy record with its own governance, not the managed-mode file, would carry it).
9. **Affected PRs/Files:** internal/contracts/managed_mode.go, internal/contracts/managed_mode_test.go, docs/v1.14-managed-mode-contract.md §1, §2, §2.1, §5, docs/release/v1.14-s0-review-round1.md.

## DEC-081

1. **Decision ID:** DEC-081
2. **Date:** 2026-09-11
3. **Question:** What can Sprint 114.1 honestly apply for each provider, and what happens when backup is requested before Sprint 114.5 exists?
4. **Options Considered:** Invent MCP adapters and a backup writer so setup looks complete; refuse `--agents` and `--backup` until later sprints; apply the existing skill/instruction installer as a skill-only transaction and record backup as a deferred consent group that never rolls back successful agent work.
5. **Chosen Option:** Skill-only apply. Each selected provider writes Atlas-owned instruction, guide, and skill files plus a private local state record. MCP registration is planned as a remaining dependency on AT114-201..208. Backup is a separate consent group: `--yes` is not backup consent; `--backup`/`--backup-target` appear in the plan with `writes=false` and do not mutate backup state. A backup-apply failure (injected for tests, or later real apply) returns partial success and leaves connected agents untouched.
6. **Why We Chose It:** The honest-ledger rule forbids claiming adapters or off-device backup that do not exist. The existing installer already knows how to preview and preserve custom instruction content; reusing it keeps one ownership model. Treating backup as a later group matches locked decision 7 and AT114-104.
7. **Confidence:** high
8. **Revisit Trigger:** Sprint 114.2 registers adapters; then setup apply must use adapter plans instead of skill-only plans. Sprint 114.5 implements backup apply.
9. **Affected PRs/Files:** internal/setup/{planner,skillplan,engine,executor}.go, internal/cli/setup.go, docs/v1.14-acceptance.md (AT114-101, AT114-104).

## DEC-082

1. **Decision ID:** DEC-082
2. **Date:** 2026-09-11
3. **Question:** Which process exit codes carry the run-level setup statuses from DEC-079 §7?
4. **Options Considered:** Map every non-connected status to exit 1; invent new exit codes; reuse the existing table: connected/pending/unverified → 0, partial → 4 (`conflict`), failed → 1 (`internal`).
5. **Chosen Option:** The third. `connected`, `pending`, and `unverified` are successful resting outcomes (writes kept or a truthful no-op) and exit 0. `partial` is `CodeConflict` (exit 4): some consented providers were kept and at least one was not. `failed` is `CodeInternal` (exit 1): nothing consented was kept. Per-provider operation and integration states remain in `--json`.
6. **Why We Chose It:** DEC-079 already froze the five status strings and required distinct non-zero codes for partial versus failed inside the existing exit-code table. Agents can branch on `status` without a second state machine, and exit 4 already means "the workflow worked but this run did not finish cleanly."
7. **Confidence:** high
8. **Revisit Trigger:** A caller needs to distinguish unverified from pending in the exit code (then the JSON status remains the API; do not add an exit code).
9. **Affected PRs/Files:** internal/setup/report.go, internal/cli/setup.go, docs/v1.14-setup-transaction-model.md §7, docs/v1.14-acceptance.md (AT114-104).

## DEC-083

1. **Decision ID:** DEC-083
2. **Date:** 2026-09-11
3. **Question:** How does `--workspace-from-cwd` interact with `--workspace`, `--init-if-missing`, and the machine-local workspace registry?
4. **Options Considered:** Let the registry supply a fallback path when cwd is wrong; allow combining `--workspace` with `--workspace-from-cwd`; keep `--workspace` as today and add a verified-cwd mode that never initializes, never uses a registry path as the workspace, and treats the registry only as a move/copy/replace detector.
5. **Chosen Option:** The third. `--workspace-from-cwd` requires `--expected-workspace-id`, is mutually exclusive with `--workspace` and `--init-if-missing`, canonicalizes cwd, walks to the single nearest real Atlas root, verifies the ID, and refuses nested workspaces, symlink substitution, a replaced inode, and a copied workspace whose original registration still exists. The registry is never opened as a workspace. Bare `--workspace` stays for existing callers; if `--expected-workspace-id` is also set, the ID is verified on that root.
6. **Why We Chose It:** DEC-072 and AT114-102 already forbade fallback and unverified registry paths. Combining the flags would let a client select a different workspace than the one it started in. Copy-versus-move has to fail closed: a second checkout with the same ID is not the registered directory.
7. **Confidence:** high
8. **Revisit Trigger:** A provider documents a safe absolute-path placeholder that makes `--workspace-from-cwd` unnecessary for that target (the flag remains for the others).
9. **Affected PRs/Files:** internal/setup/{resolve,registry}.go, internal/cli/mcp.go, internal/cli/mcp_bootstrap.go, docs/mcp.md, docs/command-reference.md.

## DEC-084

1. **Decision ID:** DEC-084
2. **Date:** 2026-09-11
3. **Question:** When a named client is installed, what version evidence is enough for Sprint 114.2 to write MCP configuration rather than report `unsupported_client_version`?
4. **Options Considered:** Treat every installed client as supported; refuse MCP writes unless a real-client run has pinned a version range; treat a parsed `X.Y.Z` from the client's documented version probe as provisionally `supported` and keep unparsable or failed probes as `unsupported_client_version`.
5. **Chosen Option:** The third. `DetectClient` runs the matrix `VersionArgs` and, when stdout/stderr yields a parsed `X.Y.Z`, sets `VersionSupport=supported`. An installed client with no successful parse plans `unsupported_client_version` and gets skill/instruction refresh only — no client-config or CLI registration steps. Generic is exempt because it has no client.
6. **Why We Chose It:** The matrix VersionPolicy requires a parseable version before adapters mutate client configuration they have not verified. Pinning an exact supported range still waits for real-client smoke (AT114-203..207 remaining dependencies). A missing or unparsable version must not heuristically edit Codex/Claude/Cursor/OpenClaw/Grok files.
7. **Confidence:** high
8. **Revisit Trigger:** A Sprint 114.2 or 114.6 real-client run records a minimum/maximum version; then the adapter reports `unsupported_client_version` outside that range even when the version parses.
9. **Affected PRs/Files:** internal/integrations/adapter/host/detect.go, internal/integrations/adapter/host/plan.go, docs/v1.14-acceptance.md (AT114-203..207).

## DEC-085

1. **Decision ID:** DEC-085
2. **Date:** 2026-09-11
3. **Question:** What does `tracker setup --team` actually write now that Sprint 114.2 owns AT114-209?
4. **Options Considered:** Keep `--team` as a plan-only note (Sprint 114.1); invent new agent records that overwrite existing roles; apply the named `solo`/`pair`/`swarm`/`crossfire` preset through `ActionService.ApplyTeamPreset` without overwriting existing assignments, and map each selected provider to a distinct actor hint.
5. **Chosen Option:** The third. One selected provider suggests `solo`, two suggest `pair`, three or four suggest `swarm`, and more suggest `crossfire`. Existing agents, runbooks, and permission profiles are skipped when present. An existing `review_gate` / `owner_gate` / `dual_gate` completion mode or a non-empty required reviewer is left in place (so `--team solo` cannot silently undo review separation). A default `open` workspace may still be strengthened to a review-gate preset. Provider and Atlas actor stay distinct: the hint is `agent:<preset-agent-id>` when the preset has a matching slot, otherwise `agent:<target>`. MCP writes still require an explicit actor and reason.
6. **Why We Chose It:** AT114-209 forbids silently overwriting roles and requires reuse of compatible existing agents when they are already present. Applying the preset makes `--team` a real write so a re-plan that includes `--team` does not stale (DEC-081 revisit).
7. **Confidence:** high
8. **Revisit Trigger:** Setup needs an interactive picker to reuse a named existing agent instead of the positional preset slot.
9. **Affected PRs/Files:** internal/setup/team.go, internal/setup/planner.go, docs/command-reference.md, docs/guides/agent-integrations.md.

## DEC-086

1. **Decision ID:** DEC-086
2. **Date:** 2026-09-11
3. **Question:** When does OpenClaw Verify report `connected` versus `connected_restart_required`?
4. **Options Considered:** Always `connected` after a self-probe; always `connected_restart_required` until the user confirms a restart; self-probe proves Atlas and reports `connected_restart_required`, while a passing `openclaw mcp doctor <name> --probe` proves the Gateway and reports `connected`.
5. **Chosen Option:** The third. Apply still never returns a verified state. A successful Atlas self-probe with no live doctor check is `connected_restart_required` / `self_probe`. A passing client-native doctor check is `connected` / `client_native`. Pending trust/approval states are never upgraded.
6. **Why We Chose It:** OpenClaw's matrix restart requirement is `gateway_reload`, and official docs distinguish saved configuration from a live probe. Claiming `connected` from a self-probe alone would hide the Gateway reload the user still has to perform.
7. **Confidence:** high
8. **Revisit Trigger:** Official OpenClaw docs show that `mcp add` makes the server live in already-running Gateway processes without a reload.
9. **Affected PRs/Files:** internal/integrations/adapter/host/apply.go, internal/integrations/adapter/openclaw/adapter.go.

## DEC-087

1. **Decision ID:** DEC-087
2. **Date:** 2026-09-11
3. **Question:** What evidence may Verify treat as a live connection, and what must Detect/Remove prove before touching another client's MCP server?
4. **Options Considered:** Treat any client CLI exit 0 as connected and delete Atlas-prefixed names on sight; require the exact `ServerName` (and command/argv when present) before claiming `client_native`, treat `mcp list` and OpenClaw `mcp show` as inventory only, and remove or overwrite a same-name entry only when it matches the canonical registration.
5. **Chosen Option:** The second. `nativeOK` requires the inspect output to mention this workspace's server name. List never upgrades a connection. OpenClaw `show` is configuration evidence; only `doctor --probe` or a passed self-probe may verify, and a doctor sentence that reports probe failure is not a pass. Detect parses JSON `mcpServers`, TOML `[mcp_servers.<name>]`, nested Claude project maps, and read-only CLI list/get/show. Get/show that exit 0 without naming this server do not invent an existing entry. `AtlasOwned` is true only from that server's own command/args (`mcp serve` and `--expected-workspace-id` for this workspace), never from tokens elsewhere in the same stdout. CLI-registered targets plan `claude mcp remove`, `openclaw mcp unset`, or `grok mcp remove --scope project` on the detected binary.
6. **Why We Chose It:** Independent review of Sprint 114.2 found that exit 0 on an unrelated `mcp list` reported `connected`, that disconnect never unset CLI servers, and that Codex TOML / Claude / OpenClaw collisions were invisible. Saved configuration is not a live probe (DEC-086).
7. **Confidence:** high
8. **Revisit Trigger:** A real-client run shows a provider's list/get JSON schema that this parser misses, or `grok mcp remove --scope project` is rejected by the shipped CLI.
9. **Affected PRs/Files:** internal/integrations/adapter/host/{apply,existing,cli,plan}.go, docs/v1.14-acceptance.md.
