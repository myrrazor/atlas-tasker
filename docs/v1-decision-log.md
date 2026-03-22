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
