# Approval Gates

Kinds:
- `review`
- `owner`
- `qa`
- `release`
- `design`

States:
- `open`
- `approved`
- `rejected`
- `waived`

Rules:
- every gate belongs to a ticket
- run-scoped gates also carry `run_id`
- rejected and waived gates remain historical
- reopening creates a new gate linked by `replaces_gate_id`
- gates may block dispatch, stage promotion, run completion, and ticket completion
- gates do not block evidence, comments, or handoff generation
