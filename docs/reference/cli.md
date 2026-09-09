# CLI Behavior

The CLI is the source of truth for command syntax. Use `tracker --help` and command-specific help before copying examples into automation.

Output modes:

- `--json` returns machine-readable envelopes where commands support it.
- `--md` returns Markdown for agent-readable briefs and reports where commands support it.
- `--pretty` returns terminal-oriented output where commands support it.
- `--plain` disables terminal styling for the current invocation.

Pretty output uses bordered tables for repeated-row views such as boards, queues, agent work, saved views, runs, evidence, and worktrees. `--plain`, `NO_COLOR=1`, and non-interactive output keep ASCII-safe borders.

Tracked CLI mutations require an actor, resolved from explicit `--actor`, `TRACKER_ACTOR`, then
`actor.default`. A non-empty `--reason` is recommended for ordinary writes and required for
security-sensitive, protected, scheduling, and other guarded actions. Public examples include both
flags so copied agent workflows leave useful audit history.

Terminal output strips control bytes from user-controlled content before display. JSON output preserves stored values.
