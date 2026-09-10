package adapter

// State is the exact integration state vocabulary from the v1.14 plan
// (locked decision 5). Setup, status, repair, and removal report one of these
// values and nothing else; provider-specific nuance goes into Verification
// fields, not into new states.
type State string

const (
	// StateConnected means the MCP registration was written, the server was
	// probed successfully, and the client can use it in its next interaction.
	StateConnected State = "connected"
	// StateConnectedRestartRequired means the registration is verified but the
	// client (or its gateway) loads configuration per process and must be
	// restarted or reloaded before it sees the server.
	StateConnectedRestartRequired State = "connected_restart_required"
	// StatePendingWorkspaceTrust means the client only reads project
	// configuration for trusted projects and the user has not trusted this
	// workspace yet. Atlas never grants trust on the user's behalf.
	StatePendingWorkspaceTrust State = "pending_workspace_trust"
	// StatePendingMCPApproval means the client requires an explicit per-server
	// approval that Atlas cannot and must not perform.
	StatePendingMCPApproval State = "pending_mcp_approval"
	// StateConfiguredUnverified means configuration exists but no probe or
	// client-native check could confirm it; treat it as a claim, not a fact.
	StateConfiguredUnverified State = "configured_unverified"
	// StatePortableReady is the honest generic-target state: portable
	// descriptor and skill exist, but no universal client health check exists.
	StatePortableReady State = "portable_ready"
	// StateUnsupportedClientVersion means the client is installed but its
	// version is unknown or outside the adapter's verified range; Atlas does not
	// heuristically modify configuration it has not verified.
	StateUnsupportedClientVersion State = "unsupported_client_version"
	// StateRepairRequired means a previously verified integration no longer
	// matches its recorded fingerprint, path, binary, or workspace identity.
	StateRepairRequired State = "repair_required"
	// StateFailed means apply or verification failed and rollback completed or
	// was not needed.
	StateFailed State = "failed"
)

var allStates = []State{
	StateConnected,
	StateConnectedRestartRequired,
	StatePendingWorkspaceTrust,
	StatePendingMCPApproval,
	StateConfiguredUnverified,
	StatePortableReady,
	StateUnsupportedClientVersion,
	StateRepairRequired,
	StateFailed,
}

// States returns the closed vocabulary in documentation order.
func States() []State {
	out := make([]State, len(allStates))
	copy(out, allStates)
	return out
}

func (s State) IsValid() bool {
	for _, candidate := range allStates {
		if candidate == s {
			return true
		}
	}
	return false
}

// Verified reports whether the state may only be produced by a successful
// probe or client-native check. Plans may predict these states but a
// Verification must back them with at least one passed check.
func (s State) Verified() bool {
	return s == StateConnected || s == StateConnectedRestartRequired
}

// RequiresHumanAction reports whether setup stopped at a boundary Atlas must
// not cross on its own: a provider trust dialog, an MCP approval, an
// unsupported client, a repair decision, or a failure.
func (s State) RequiresHumanAction() bool {
	switch s {
	case StatePendingWorkspaceTrust, StatePendingMCPApproval, StateUnsupportedClientVersion, StateRepairRequired, StateFailed:
		return true
	default:
		return false
	}
}

// ConnectionKind refines a verified connection without adding states. The
// generic target uses it to distinguish a standard-config host from a
// custom-adapter host (plan AT114-208) while still reporting State
// "connected".
type ConnectionKind string

const (
	ConnectionKindClientNative   ConnectionKind = "client_native"
	ConnectionKindSelfProbe      ConnectionKind = "self_probe"
	ConnectionKindStandardConfig ConnectionKind = "standard_config"
	ConnectionKindCustomAdapter  ConnectionKind = "custom_adapter"
)

func (k ConnectionKind) IsValid() bool {
	switch k {
	case ConnectionKindClientNative, ConnectionKindSelfProbe, ConnectionKindStandardConfig, ConnectionKindCustomAdapter:
		return true
	default:
		return false
	}
}
