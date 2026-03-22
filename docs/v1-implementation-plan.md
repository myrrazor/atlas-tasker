# Atlas Tasker v1 Implementation Plan

## Summary

Atlas Tasker v1 is a local-first, terminal-first, markdown-native issue tracker for one human owner and AI agents. v1 implementation is intentionally constrained to:

- project and ticket CRUD
- comments
- issue types: epic/task/bug/subtask
- parent-child links
- blocks/blocked_by links
- board/backlog/blocked/next/search/detail views
- CLI + `tracker shell` slash aliases
- terminal rendering modes (`pretty`, `md`, `json`)
- minimal completion permission modes (`open`, `owner_gate`, `review_gate`)

Excluded from v1: remote sync, collaboration ACL systems, vector/RAG features, advanced automation, leases/heartbeats, full TUI.

## Architecture and Service Contracts

- **Markdown files** are current readable state for tickets/projects.
- **JSONL event log** is append-only and retains full mutation history.
- **SQLite projection** is a fast local index/query layer for list/board/search/history views.

Core contract rules:

1. Every mutation appends exactly one event entry with actor, timestamp, type, reason, and payload.
2. Event history remains authoritative for rebuild and recovery.
3. Reindex reconstructs SQLite projection deterministically from markdown + events.
4. Ticket delete is soft-delete in v1 (`status=canceled`, `archived=true`).
5. Transition validation and permission mode checks are enforced on `ticket move`.
6. Links must remain valid and symmetric where required.

## Decision Log

All architecture and UX decisions are recorded in:

- [v1-decision-log.md](v1-decision-log.md)

This file is mandatory for all PRs in the v1 sequence. If a decision changes, create a new decision entry and cross-reference the superseded one.

## PR Sequencing

Execution is split into nine reviewable tracks with explicit dependencies:

1. PR-001 foundation/bootstrap
2. PR-002 domain contracts and schemas
3. PR-003 markdown + event-log storage
4. PR-004 SQLite projection + reindex + queries
5. PR-005 CLI framework + tracker shell
6. PR-006 workflow, links, and permission gates
7. PR-007 CRUD/comments/history/mutation commands
8. PR-008 renderers and terminal presentation
9. PR-009 end-to-end tests, fixtures, recovery, docs

Detailed responsibilities and decision mappings are in:

- [v1-ticket-pr-breakdown.md](v1-ticket-pr-breakdown.md)

## QA and Review Rules

Before merging each PR:

1. Run `/review`.
2. Confirm any new or changed decision appears in `docs/v1-decision-log.md`.
3. Confirm superseded decisions are linked with rationale.

Final v1 hardening (PR-009) must include a **Decision Audit** proving implementation behavior aligns with the decision log and this plan.
