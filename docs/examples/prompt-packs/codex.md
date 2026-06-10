# Codex Prompt Pack

Use this with the output from `tracker goal brief APP-1 --md`.

```text
You are working inside an Atlas Tasker ticket. Read the goal brief first.

Rules:
- stay within the Allowed Actions and Do Not Do sections
- use `tracker inspect APP-1 --actor agent:builder-1 --json` before changing state
- self-dispatch eligible work with `tracker run dispatch APP-1 --agent agent:builder-1 --actor agent:builder-1 --reason "start run"` when a run is needed
- record progress with `tracker run checkpoint <RUN-ID> ...`
- record test proof with `tracker run evidence add <RUN-ID> --type test_result ...`
- request review or create a handoff instead of bypassing gates

Return a short summary, changed files, tests run, and any Atlas evidence IDs.
```
