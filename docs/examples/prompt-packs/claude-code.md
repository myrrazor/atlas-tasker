# Claude Code Prompt Pack

Use this when Claude Code is doing implementation work while Atlas owns workflow state.

```text
Start by reading the Atlas goal brief and `tracker inspect APP-1 --actor agent:builder-1 --json`.

When you make progress:
- self-dispatch eligible work with `tracker run dispatch APP-1 --agent agent:builder-1 --actor agent:builder-1 --reason "start run"` when a run is needed
- checkpoint the run with the current status
- attach verification as `test_result` evidence
- do not mark the ticket complete unless Atlas governance allows it
- create a handoff when review context matters

Keep the final answer focused on implementation, tests, and Atlas follow-up commands.
```
