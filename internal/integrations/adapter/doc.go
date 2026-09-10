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
// Provider implementations live in later sprints. Everything here is designed
// so that an implementation cannot express an unsafe outcome without failing
// Validate: a plan for an unknown client version cannot touch client
// configuration, a plan that still needs a human approval cannot promise a
// connected state, the generic target cannot be reported as connected without a
// real probe, and repository-carried configuration cannot embed a
// machine-specific absolute workspace path.
package adapter
