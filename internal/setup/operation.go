// Package setup holds the contracts of the v1.14 unified setup engine
// (tracker setup). Sprint 114.0 freezes the operation state machine defined in
// docs/v1.14-setup-transaction-model.md; the planner, journal, and executor
// arrive in Sprint 114.1 and must use these states unchanged.
package setup

import (
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

// OperationState is the lifecycle state of one setup transaction: one provider
// integration, the managed-mode write, or the backup configuration group. It
// is distinct from the integration state an adapter reports (connected,
// pending_workspace_trust, ...): the operation state says where the
// transaction is, the integration state says what it produced. OutcomeFor is
// the one mapping between the two vocabularies.
//
// An operation is one journal entry. It never returns to planned: a fresh plan
// after failed, rolled_back, or repair_required is a new operation whose
// planning step reads the previous journal entry, so "planned" always means
// "this operation has written nothing".
type OperationState string

const (
	// StatePlanned means a validated plan exists and nothing has been written.
	StatePlanned OperationState = "planned"
	// StateApplying means at least one step may have written; the journal holds
	// pre-write snapshots for every step started so far.
	StateApplying OperationState = "applying"
	// StateApplied means every step ran; nothing has been verified yet.
	StateApplied OperationState = "applied"
	// StateVerifying means verification probes are running.
	StateVerifying OperationState = "verifying"
	// StateConnected means verification passed; the integration state is
	// connected or connected_restart_required.
	StateConnected OperationState = "connected"
	// StatePendingApproval means the writes are complete, correct, and kept,
	// but the integration is not proven connected: a provider trust or
	// approval dialog, a client restart or manual check, a portable descriptor
	// the user still has to install, or an unsupported client version is
	// outstanding. The integration state in the journal and the run report
	// says which.
	StatePendingApproval OperationState = "pending_approval"
	// StateFailed means a rollback action itself failed, so writes may remain
	// and the journal lists the exact paths left for the operator. It is
	// reached only from rolling_back; an apply or verify error always goes
	// through rolling_back first.
	StateFailed OperationState = "failed"
	// StateRollingBack means rollback actions are executing in reverse order.
	StateRollingBack OperationState = "rolling_back"
	// StateRolledBack means every reversible write was undone; irreversible
	// steps are listed in the journal. It is final for this operation.
	StateRolledBack OperationState = "rolled_back"
	// StateRepairRequired means verification or a later inspection found
	// drift between the recorded state and disk. It is final for this
	// operation; repair is a new operation planned from this journal entry.
	StateRepairRequired OperationState = "repair_required"
)

var allOperationStates = []OperationState{
	StatePlanned, StateApplying, StateApplied, StateVerifying, StateConnected,
	StatePendingApproval, StateFailed, StateRollingBack, StateRolledBack, StateRepairRequired,
}

// States returns the closed set in plan order.
func States() []OperationState {
	out := make([]OperationState, len(allOperationStates))
	copy(out, allOperationStates)
	return out
}

func (s OperationState) IsValid() bool {
	for _, candidate := range allOperationStates {
		if candidate == s {
			return true
		}
	}
	return false
}

// InFlight reports whether a process is expected to be mid-operation. Finding
// an in-flight state in the journal at startup means a previous run was
// interrupted; see RecoveryAction.
func (s OperationState) InFlight() bool {
	switch s {
	case StateApplying, StateApplied, StateVerifying, StateRollingBack:
		return true
	default:
		return false
	}
}

// Resting is the complement of InFlight for valid states.
func (s OperationState) Resting() bool { return s.IsValid() && !s.InFlight() }

// KeepsWrites reports whether the transaction reached a resting state in which
// its writes are kept as correct. It does not mean the integration is
// connected: pending_approval keeps its writes while a human or client step
// is still outstanding. Run-level reporting must therefore carry the
// integration state next to the operation state, never this flag alone.
func (s OperationState) KeepsWrites() bool {
	return s == StateConnected || s == StatePendingApproval
}

// Final reports whether the operation can never move again. A new plan or a
// repair starts a new operation.
func (s OperationState) Final() bool {
	return s == StateRolledBack || s == StateRepairRequired
}

// MayHaveWritten reports whether the state can coexist with writes on disk
// that have not been rolled back. Because no edge leads back to planned or
// rolled_back except the rollback itself, the answer is a property of the
// state alone.
func (s OperationState) MayHaveWritten() bool {
	switch s {
	case StatePlanned, StateRolledBack:
		return false
	default:
		return s.IsValid()
	}
}

var transitions = map[OperationState][]OperationState{
	StatePlanned:         {StateApplying},
	StateApplying:        {StateApplied, StateRollingBack},
	StateApplied:         {StateVerifying, StateRollingBack},
	StateVerifying:       {StateConnected, StatePendingApproval, StateRepairRequired, StateRollingBack},
	StateConnected:       {StateVerifying, StateRepairRequired},
	StatePendingApproval: {StateVerifying, StateRepairRequired, StateRollingBack},
	StateFailed:          {StateRollingBack},
	StateRollingBack:     {StateRolledBack, StateFailed},
	StateRolledBack:      {},
	StateRepairRequired:  {},
}

// OutcomeFor maps the integration state an adapter reported after apply and
// verify to the operation state the engine journals next. The mapping is
// total over the nine integration states, so a run report can never show an
// integration outcome the transaction has no state for:
//
//   - connected, connected_restart_required -> connected
//   - pending_workspace_trust, pending_mcp_approval, configured_unverified,
//     portable_ready, unsupported_client_version -> pending_approval (writes
//     kept; the integration state names the outstanding step)
//   - repair_required -> repair_required
//   - failed -> rolling_back (the engine undoes the started steps; rolled_back
//     or failed is decided by the rollback, never by the adapter)
//
// A plan with no write steps (a no-op, or an unsupported client for which
// Atlas writes nothing) creates no operation; its integration state is
// reported straight from the plan.
func OutcomeFor(state adapter.State) (OperationState, error) {
	switch state {
	case adapter.StateConnected, adapter.StateConnectedRestartRequired:
		return StateConnected, nil
	case adapter.StatePendingWorkspaceTrust, adapter.StatePendingMCPApproval, adapter.StateConfiguredUnverified, adapter.StatePortableReady, adapter.StateUnsupportedClientVersion:
		return StatePendingApproval, nil
	case adapter.StateRepairRequired:
		return StateRepairRequired, nil
	case adapter.StateFailed:
		return StateRollingBack, nil
	}
	return "", fmt.Errorf("unknown integration state %q", state)
}

// Allows reports whether the edge s -> next is legal. Every state is legal to
// itself as a journal re-mark (resume), which is not a transition.
func (s OperationState) Allows(next OperationState) bool {
	if !s.IsValid() || !next.IsValid() {
		return false
	}
	if s == next {
		return true
	}
	for _, candidate := range transitions[s] {
		if candidate == next {
			return true
		}
	}
	return false
}

// Transition validates and returns the next state.
func (s OperationState) Transition(next OperationState) (OperationState, error) {
	if !s.Allows(next) {
		return "", fmt.Errorf("setup operation cannot move from %q to %q", s, next)
	}
	return next, nil
}

// Recovery is what the engine does when it finds a journal entry in an
// in-flight state at startup.
type Recovery string

const (
	// RecoveryNone means the state is resting; nothing to do.
	RecoveryNone Recovery = "none"
	// RecoveryRollBack means writes may be partial: run the rollback actions of
	// every step the journal marked as started, in reverse order.
	RecoveryRollBack Recovery = "roll_back"
	// RecoveryVerify means all writes completed: re-run verification and let it
	// decide connected, pending_approval, repair_required, or rolling_back.
	RecoveryVerify Recovery = "verify"
	// RecoveryResumeRollback means rollback was interrupted: run the remaining
	// rollback actions again. Each action kind defines how a repeat is
	// recognised as already done (docs/v1.14-setup-transaction-model.md §6),
	// so resuming never reports a failure for work the crash had finished.
	RecoveryResumeRollback Recovery = "resume_rollback"
)

// RecoveryAction maps an in-flight state to its crash-recovery behavior.
// Resting states need none. An invalid state is an error because the journal
// was written by something other than this engine.
func (s OperationState) RecoveryAction() (Recovery, error) {
	switch s {
	case StateApplying:
		return RecoveryRollBack, nil
	case StateApplied, StateVerifying:
		return RecoveryVerify, nil
	case StateRollingBack:
		return RecoveryResumeRollback, nil
	}
	if !s.IsValid() {
		return "", fmt.Errorf("unknown setup operation state %q", s)
	}
	return RecoveryNone, nil
}

// RecoveryTarget is the state the engine marks before running the recovery
// action, so an interruption during recovery is itself recoverable.
func (s OperationState) RecoveryTarget() (OperationState, error) {
	action, err := s.RecoveryAction()
	if err != nil {
		return "", err
	}
	switch action {
	case RecoveryRollBack, RecoveryResumeRollback:
		return StateRollingBack, nil
	case RecoveryVerify:
		return StateVerifying, nil
	default:
		return s, nil
	}
}
