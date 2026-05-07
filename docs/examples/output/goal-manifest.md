$ tracker goal manifest APP-1 --actor human:owner --reason "prepare demo goal" --md
# Agent Goal

Add health check

## Goal

Add health check

## Objective

Add health check

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
- tracker run open <RUN-ID> --json
- tracker goal brief <RUN-ID> --md

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

