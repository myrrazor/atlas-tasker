# Atlas Tasker Agent Guide

This repository is building **Atlas Tasker v1**, a local-first, terminal-first, markdown-native issue tracker for AI coding agents.

## Current Delivery Mode

- Workstream is organized into nine PR tracks (`PR-001` through `PR-009`).
- This repo is currently using `main` as the integration target for the v1 implementation sequence.
- Scope is locked to v1 in `docs/v1-implementation-plan.md`.

## Mandatory Decision Logging

Any planning or implementation decision that affects architecture, behavior, storage contracts, UX behavior, testing strategy, or release scope must be recorded in:

- `docs/v1-decision-log.md`

Each decision entry must include:

1. Decision ID
2. Date
3. Question
4. Options Considered
5. Chosen Option
6. Why We Chose It
7. Confidence (`high`, `medium`, `low`)
8. Revisit Trigger
9. Affected PRs/Files

If a prior decision changes:

- Add a new decision entry (do not overwrite history).
- Mark the old decision as superseded.
- Reference both IDs and explain why the change was made.

## PR and Review Rules

- Before merge, run `/review` and confirm:
  - new/changed decisions are reflected in `docs/v1-decision-log.md`
  - superseded decisions are linked with rationale
- `PR-009` must include a final **Decision Audit** proving implementation behavior matches the decision log.

## Required Planning Docs

- `docs/v1-implementation-plan.md`
- `docs/v1-ticket-pr-breakdown.md`
- `docs/v1-decision-log.md`

These three files are the source of truth for v1 execution, sequencing, and rationale.
