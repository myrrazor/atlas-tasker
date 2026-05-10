# CLI Behavior

The CLI is the source of truth for command syntax. Use `tracker --help` and command-specific help before copying examples into automation.

Output modes:

- `--json` returns machine-readable envelopes where commands support it.
- `--md` returns Markdown for agent-readable briefs and reports where commands support it.
- `--pretty` returns terminal-oriented output where commands support it.
- `--plain` disables terminal styling for the current invocation.

Mutation commands that affect workspace state generally require `--actor` and a non-empty `--reason`. Examples in public docs include those flags unless the command is read-only.

Terminal output strips control bytes from user-controlled content before display. JSON output preserves stored values.
