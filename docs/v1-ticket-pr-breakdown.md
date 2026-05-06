# Atlas Tasker v1 PR Breakdown

This document defines PR-sized tracks for parallel execution with minimal file overlap and explicit integration boundaries.

## Global Rules

- PRs are cumulative and dependency-aware; they are not mutually exclusive.
- Every PR must update [v1-decision-log.md](v1-decision-log.md) if decisions are introduced or changed.
- `/review` must verify decision logging before merge.

## PR-001 foundation/bootstrap

- **Scope:** repo bootstrap, scaffolding docs, AGENTS instructions, initial CI layout.
- **Required files:**
  - `AGENTS.md`
  - `docs/v1-implementation-plan.md`
  - `docs/v1-ticket-pr-breakdown.md`
  - `docs/v1-decision-log.md`
- **Decision refs:** DEC-001, DEC-002, DEC-004, DEC-012, DEC-013, DEC-014
- **Subagent owner:** architecture/contracts
- **Depends on:** none
- **Integration notes:** establishes process contracts consumed by all subsequent PRs.

## PR-002 domain contracts and schemas

- **Scope:** domain types, status model, transition matrix, validation schema, relationship contracts.
- **Decision refs:** DEC-004, DEC-006
- **Subagent owner:** architecture/contracts
- **Depends on:** PR-001
- **File overlap constraints:** no storage backend implementation yet.

## PR-003 markdown + event-log storage

- **Scope:** project/ticket markdown persistence, template seeds, JSONL append-only event writer/reader.
- **Decision refs:** DEC-003, DEC-006
- **Subagent owner:** storage
- **Depends on:** PR-002
- **File overlap constraints:** no query projection logic beyond write hooks.

## PR-004 SQLite projection + reindex + queries

- **Scope:** sqlite schema, projector, reindex, board/list/search/history query layer.
- **Decision refs:** DEC-005, DEC-006, DEC-008, DEC-026
- **Subagent owner:** index/query
- **Depends on:** PR-003
- **File overlap constraints:** avoid CLI command surface expansion except `reindex` plumbing.

## PR-005 CLI framework + tracker shell

- **Scope:** Cobra command tree, command wiring, `tracker shell`, slash parser parity scaffold.
- **Decision refs:** DEC-007
- **Subagent owner:** CLI/shell
- **Depends on:** PR-002
- **File overlap constraints:** no deep business logic; delegate to domain/service contracts.

## PR-006 workflow, links, and permission gates

- **Scope:** transition enforcement, link integrity, `completion_mode` gate behavior.
- **Decision refs:** DEC-003, DEC-004
- **Subagent owner:** workflow/permissions
- **Depends on:** PR-002, PR-004, PR-005
- **File overlap constraints:** avoid renderer changes.

## PR-007 CRUD/comments/history/mutation commands

- **Scope:** ticket/project mutation command implementations and history/comment flows.
- **Decision refs:** DEC-003, DEC-007
- **Subagent owner:** CLI/shell + workflow/permissions
- **Depends on:** PR-003, PR-004, PR-005, PR-006
- **File overlap constraints:** avoid presentation-specific formatting details.

## PR-008 renderers and terminal presentation

- **Scope:** pretty renderer, markdown mode, json mode, width handling, accessible output conventions.
- **Decision refs:** DEC-009, DEC-010, DEC-011, DEC-026
- **Subagent owner:** rendering/UX
- **Depends on:** PR-005, PR-007
- **File overlap constraints:** no mutation path behavior changes.

## PR-009 end-to-end tests, fixtures, recovery, docs

- **Scope:** full acceptance test flow, fixtures, doctor/recovery hardening, final docs alignment.
- **Decision refs:** DEC-003, DEC-005, DEC-012, DEC-026
- **Subagent owner:** QA/review
- **Depends on:** PR-001..PR-008
- **Required checks:**
  - Decision Audit against `docs/v1-decision-log.md`
  - `/qa-only` run and report
  - `/review` final pass

## Subagent Handoff Format

Each subagent deliverable must include:

1. Summary
2. Files touched
3. Tests added
4. Risks
5. Integration notes
