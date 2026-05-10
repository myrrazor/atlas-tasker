# Codex `/goal` Guide

Codex `/goal` is a good fit for Atlas goal briefs because both are explicit about objective, constraints, evidence, and done criteria.

## Create A Pasteable Brief

```bash
tracker goal brief APP-1 --md
```

The Markdown sections are fixed:

1. Goal
2. Objective
3. Current State
4. Ticket / Run
5. Acceptance Criteria
6. Constraints
7. Required Evidence
8. Required Gates
9. Allowed Actions
10. Do Not Do
11. Context
12. Suggested Commands
13. Done When
14. Verification

Use a run target when the work already has a run:

```bash
tracker goal brief <RUN-ID> --md
```

## Persist A Manifest

Use a manifest when the prompt itself should become an evented local artifact:

```bash
tracker goal manifest APP-1 \
  --actor human:owner \
  --reason "prepare Codex goal" \
  --md
```

Manifests live under `.tracker/goal/manifests/`, bind policy/trust/source hashes, and can be signed:

```bash
tracker sign goal <MANIFEST-ID> \
  --signing-key <KEY-ID> \
  --actor human:owner \
  --reason "sign agent goal"

tracker verify goal <MANIFEST-ID> --json
```

Atlas does not control Codex goals, call Codex app-server APIs, or resume Codex sessions. The safe integration point is this file and command output.
