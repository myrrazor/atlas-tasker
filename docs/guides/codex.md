# Codex Guide

Atlas works best with Codex when Atlas owns workflow state and Codex owns code changes.

## Recommended Loop

```bash
tracker queue --actor agent:builder-1 --json
tracker inspect APP-1 --actor agent:builder-1 --json
tracker ticket claim APP-1 --actor agent:builder-1
tracker ticket move APP-1 in_progress --actor agent:builder-1 --reason "start implementation"
```

Use JSON reads when Codex needs exact state and Markdown reads when you want a pasteable prompt.

## Goal Prompts

```bash
tracker goal brief APP-1 --md
tracker goal brief <RUN-ID> --json
```

Paste the Markdown output into Codex `/goal` when you want a compact objective, constraints, gates, and verification checklist. Atlas only writes a manifest when you explicitly run:

```bash
tracker goal manifest APP-1 --actor human:owner --reason "prepare Codex goal" --md
```

## Evidence And Review

```bash
tracker run evidence add <RUN-ID> \
  --type test_result \
  --title "go test ./..." \
  --body "paste or summarize the passing output" \
  --actor agent:builder-1 \
  --reason "record verification"

tracker ticket request-review APP-1 --actor agent:builder-1 --reason "implementation ready"
```

Do not treat a status move as completion. Completion still follows the active Atlas governance policy.

## MCP

Start Codex with the read profile first. Add workflow or delivery profiles only for a short session where the human expects Codex to mutate Atlas state.

Read the MCP setup details in [Codex MCP setup](../mcp-codex.md) and the safety boundary in [MCP security](../mcp-security.md).
