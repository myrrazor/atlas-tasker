# Generic Agent Prompt Pack

Use this for any coding agent that can run shell commands.

```text
Atlas is the source of truth for this task.

Before editing:
1. Read `tracker goal brief APP-1 --md` and set TRACKER_ACTOR to the valid agent identity doing the work.
2. Run `tracker inspect APP-1 --actor "$TRACKER_ACTOR" --json`.
3. Confirm the ticket is claimed or claim it.
4. Self-dispatch eligible work with `tracker run dispatch APP-1 --agent "$TRACKER_ACTOR" --actor "$TRACKER_ACTOR" --reason "start run"` when a run is needed.

During work, record checkpoints and evidence. If blocked, write a ticket comment or handoff instead of silently stopping.
```
