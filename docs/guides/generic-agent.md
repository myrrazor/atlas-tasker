# Generic Agent Guide

Any coding agent can use Atlas if it follows a small contract: read before writing, claim before editing, record evidence, and never bypass gates.

## Minimum Contract

```bash
tracker queue --actor <actor> --json
tracker inspect <TICKET-ID> --actor <actor> --json
tracker ticket claim <TICKET-ID> --actor <actor>
tracker ticket move <TICKET-ID> in_progress --actor <actor> --reason "start work"
```

Agents should treat `tracker inspect` as the truth for policy, lease, gate, and review state.

## Prompt Contract

```bash
tracker goal brief <TICKET-ID|RUN-ID> --md
```

The brief is designed to be pasted into any agent prompt. It includes allowed actions and do-not-do constraints so the agent gets workflow limits in the same context as the objective.

## Mutation Rules

- Use a valid Atlas actor such as `agent:builder-1`.
- Include `--reason` whenever the command exposes that flag.
- Prefer JSON for reads and Markdown for human/agent handoff text.
- Attach test output or artifacts with run-scoped evidence.
- Request review instead of marking work complete directly unless the active policy allows it.

## Safe Reads

```bash
tracker board --json
tracker dashboard --json
tracker timeline <TICKET-ID> --json
tracker approvals --json
tracker goal brief <TICKET-ID> --json
```

Agents that use MCP should start with `--tool-profile read`. Workflow writes require actor, reason, permissions, and Atlas write locks. High-impact writes also require an external operation approval.
