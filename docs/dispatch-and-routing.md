# Dispatch and Routing

Eligibility filters:
- ticket status
- blockers/dependencies
- allowed workers/policy
- open gates
- existing active runs
- required capabilities
- enabled agent state
- concurrency limit
- source repo cleanliness
- runbook stage requirements

Dispatch modes:
- manual assignment
- auto-suggest
- auto-route if exactly one eligible agent remains
- bulk dispatch from saved view

Stable rejection reason codes:
- `blocked_dependency`
- `missing_capability`
- `disallowed_worker`
- `active_run_exists`
- `parallel_runs_disabled`
- `dirty_repo`
- `runbook_requirement_unsatisfied`
- `open_gate_prevents_dispatch`
- `agent_disabled`
- `agent_at_capacity`
- `ticket_status_ineligible`
