# Claude Code Guide

Use Atlas as the durable workflow layer around Claude Code sessions. Claude can generate code, but Atlas records the ticket, run, evidence, handoff, and governance state.

## Start A Session

```bash
tracker agent available builder-1 --json
tracker agent pending builder-1 --json
tracker inspect APP-1 --actor agent:builder-1 --json
tracker ticket claim APP-1 --actor agent:builder-1
tracker goal brief APP-1 --md
```

Use `available` for actionable work and `pending` for blocker explanations. Paste the goal brief into Claude Code when you want the session to stay inside Atlas constraints.

Install the Claude guidance and slash-command templates with:

```bash
tracker integrations install claude
```

## Record Progress

```bash
tracker ticket comment APP-1 \
  --body "implementation notes or risk" \
  --actor agent:builder-1 \
  --reason "record durable context"

tracker run checkpoint <RUN-ID> \
  --title "progress" \
  --body "what changed and what remains" \
  --actor agent:builder-1 \
  --reason "record progress"
```

## Finish For Review

```bash
tracker run evidence add <RUN-ID> \
  --type test_result \
  --title "verification" \
  --body "test output or artifact summary" \
  --actor agent:builder-1 \
  --reason "record verification"

tracker ticket request-review APP-1 --actor agent:builder-1 --reason "ready for review"
```

For MCP setup, use [Claude Code MCP setup](../mcp-claude-code.md). Keep high-impact tools out of normal Claude Code sessions unless a human has created a specific external operation approval.
