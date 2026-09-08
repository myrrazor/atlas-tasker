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

**Status:** Superseded by DEC-038 for motion timing and feedback; the card information model remains current.

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

## DEC-051

1. **Decision ID:** DEC-051
2. **Date:** 2026-09-08
3. **Question:** How should the v1.10 interfaces enforce their existing local workspace and secret boundaries?
4. **Options Considered:** Keep surface-specific checks; share initialized-root validation and preflight every integration destination.
5. **Chosen Option:** Use a shared initialized-root check for CLI config/reads, MCP serving/approvals, and TUI. Keep init and explicit integration installation as bootstrap operations, and version/help/MCP schema/tools as workspace-independent discovery. Mask config-set JSON exactly like config-get. Normalize web bind hosts before listening and validate the actual loopback listener. Reject symlink components in every integration output path before writing any file.
6. **Why We Chose It:** The review reproduced silent workspace creation through implicit MCP/TUI startup, default config reads from the wrong directory, webhook secrets echoed by the JSON setter, a wildcard listener created by an empty host, and integration writes escaping through symlinks. These changes enforce DEC-044 and DEC-047 without adding remote serving or shared-skill installation.
7. **Confidence:** high
8. **Revisit Trigger:** A future explicitly approved remote web mode or shared integration installer requires a separate trust model.
9. **Affected PRs/Files:** PRs #121, #122, #127; internal/service/workspace.go, internal/cli, internal/mcp/workspace.go, internal/tui/app.go, internal/web/listener.go, internal/integrations/install.go, AGENTS.md

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
