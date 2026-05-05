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

Operator flow in v1.4:
- `tracker run handoff` opens any required runbook gates for the run and can also open an explicit `--next-gate`
- `tracker gate approve|reject|waive` resolves the gate and updates the linked run state
- rejected gates send the run back to `active`
- approved or waived run-scoped gates relax the run back to `handoff_ready` once no other gates remain open for that run
- `tracker approvals` shows open gate work only
- `tracker inbox` is derived from open gates plus handoff-ready runs
