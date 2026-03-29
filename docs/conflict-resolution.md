# Conflict Resolution

v1.6 uses semantic reconciliation and explicit conflict records.

Atlas never silently picks an unsafe winner.

Conflict taxonomy:
- scalar_divergence
- terminal_state_divergence
- uid_collision
- trust_state_divergence
- membership_divergence
- gate_divergence
- run_state_divergence
- change_divergence
- check_divergence

Resolution modes:
- `use_local`
- `use_remote`

If an operator wants a merged outcome, they resolve to one side first, then make a new canonical edit and sync again.
