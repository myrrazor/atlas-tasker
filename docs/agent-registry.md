# Agent Registry

Storage:
- `.tracker/agents/<agent-id>.toml`

Frozen schema:
- `agent_id`
- `display_name`
- `provider`
- `enabled`
- `capabilities`
- `allowed_ticket_types`
- `default_runbook`
- `max_active_runs`
- `preferred_roles`
- `routing_weight`
- `instruction_profile`
- `launch_target`
- `integration_template`
- `notes`

Authority rules:
- declared capabilities and preferences are static profile data
- active run counts are projection data
- effective eligibility is always computed
