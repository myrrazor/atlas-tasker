// Package adapter defines the v1.14 provider adapter contract shared by every
// coding-agent integration target (codex, claude, cursor, openclaw, grok,
// generic).
//
// The package holds contracts only: the AgentIntegrationAdapter interface, the
// integration state vocabulary, the structured (never shell-interpolated)
// command model, the canonical Atlas MCP registration model, the plan and
// rollback model consumed by the setup transaction engine, and the capability
// matrix that records what each client verifiably supports.
//
// Provider implementations live in later sprints. The validators here are the
// setup engine's fail-closed gate against a buggy or over-claiming adapter, for
// the outcomes the contract can see: a plan for an unknown client version
// cannot touch client configuration, a plan that still needs a human approval
// must promise exactly that pending state, the generic target cannot be
// reported as connected without a real probe, repository-carried configuration
// cannot embed a machine-specific path, file steps stay inside Atlas-owned or
// documented client-configuration locations, rollbacks undo only their own
// step, and every command runs the detected client or the registered server
// executable with a structured argv (no shell interpolation). What the
// validators cannot prove, the journal engine checks against the live
// filesystem at apply time (symlinks, ownership, current file identity).
package adapter
