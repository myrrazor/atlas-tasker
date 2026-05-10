$ tracker goal manifest APP-1 --actor human:owner --reason "prepare demo goal" --md
# Add health check

## Objective

Expose a local health check endpoint and record smoke-test evidence before review.

## Current State

- ticket status: in_progress
- priority: medium
- open gate: <GATE-ID>

## Ticket / Run

- APP-1 Add health check
- latest run: <RUN-ID>

## Acceptance Criteria

- Health check behavior is documented.
- Local smoke command is recorded as evidence.

## Constraints

- preserve existing user work
- do not bypass governance gates

## Required Evidence

- Health check behavior is documented.
- Local smoke command is recorded as evidence.

## Required Gates

- <GATE-ID> review open

## Allowed Actions

- read and update Atlas-owned files for this ticket
- run local tests and attach evidence
- request review when done

## Do Not Do

- do not alter private keys or trust decisions
- do not recreate provider state from local manifests
- do not skip required approvals

## Context

- ticket:APP-1
- run:<RUN-ID> awaiting_review
- handoff:<HANDOFF-ID>
- evidence:<EVIDENCE-ID> First pass
- evidence:<EVIDENCE-ID> Smoke test

## Suggested Commands

- tracker inspect APP-1 --actor <actor> --json
- tracker ticket claim APP-1 --actor <actor>
- tracker ticket move APP-1 in_progress --actor <actor> --reason "start work"
- tracker run launch <RUN-ID> --actor <actor> --reason "prepare launch files"
- tracker run open <RUN-ID> --json
- tracker run checkpoint <RUN-ID> --title "progress" --body "what changed"
  --actor <actor> --reason "record progress"
- tracker run evidence add <RUN-ID> --type test_result --title "verification"
  --body "test output" --actor <actor> --reason "record verification"
- tracker run handoff <RUN-ID> --next-actor <reviewer> --next-gate review
  --actor <actor> --reason "ready for review"
- tracker gate list --run <RUN-ID> --json
- tracker goal brief <RUN-ID> --md
- valid evidence types: note, test_result, file_diff_summary, log_excerpt,
  screenshot, artifact_ref, commit_ref, manual_assertion, unresolved_question,
  review_checklist
- when review passes: tracker ticket complete APP-1 --actor <actor>
  --reason "done"

## Done When

- Health check behavior is documented.
- Local smoke command is recorded as evidence.
- gate <GATE-ID> is approved or resolved
- tests and handoff evidence are recorded

## Verification

- run the relevant local tests before requesting review
- attach command output or artifact evidence to Atlas
- confirm required gates are approved before completion
- review latest run <RUN-ID> before completion
