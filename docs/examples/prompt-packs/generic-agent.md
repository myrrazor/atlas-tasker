# Generic Agent Prompt Pack

Use this for any coding agent that can run shell commands.

```text
Atlas is the source of truth for this task.

Before editing:
1. Run `tracker inspect APP-1 --actor <actor> --json`.
2. Read `tracker goal brief APP-1 --md`.
3. Confirm the ticket is claimed or claim it.

During work, record checkpoints and evidence. If blocked, write a ticket comment or handoff instead of silently stopping.
```
