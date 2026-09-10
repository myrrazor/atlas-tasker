// Package setup holds the contracts of the v1.14 unified setup engine
// (tracker setup). Sprint 114.0 freezes the operation state machine defined in
// docs/v1.14-setup-transaction-model.md; the planner, journal, and executor
// arrive in Sprint 114.1 and must use these states unchanged.
package setup

import "fmt"

// OperationState is the lifecycle state of one setup transaction: one provider
// integration, the managed-mode write, or the backup configuration group. It
// is distinct from the integration state an adapter reports (connected,
// pending_workspace_trust, ...): the operation state says where the
// transaction is, the integration state says what it produced.
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
	// StatePendingApproval means the writes are complete and correct but a
	// provider trust or approval dialog, or a restart, is outstanding.
	StatePendingApproval OperationState = "pending_approval"
	// StateFailed means apply or verify failed. If writes happened they were
	// rolled back; if rollback itself failed the journal says so and the
	// operation stays failed until an operator runs repair.
	StateFailed OperationState = "failed"
	// StateRollingBack means rollback actions are executing in reverse order.
	StateRollingBack OperationState = "rolling_back"
	// StateRolledBack means every reversible write was undone; irreversible
	// steps are listed in the journal.
	StateRolledBack OperationState = "rolled_back"
	// StateRepairRequired means a later inspection found drift between the
	// recorded state and disk.
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

// Succeeded reports whether the transaction reached a state in which its
// writes are kept.
func (s OperationState) Succeeded() bool {
	return s == StateConnected || s == StatePendingApproval
}

// MayHaveWritten reports whether the state can coexist with writes on disk
// that have not been rolled back.
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
	StateApplying:        {StateApplied, StateRollingBack, StateFailed},
	StateApplied:         {StateVerifying, StateRollingBack},
	StateVerifying:       {StateConnected, StatePendingApproval, StateRepairRequired, StateRollingBack, StateFailed},
	StateConnected:       {StateVerifying, StateRepairRequired},
	StatePendingApproval: {StateVerifying, StateRepairRequired, StateRollingBack},
	StateFailed:          {StateRollingBack, StatePlanned},
	StateRollingBack:     {StateRolledBack, StateFailed},
	StateRolledBack:      {StatePlanned},
	StateRepairRequired:  {StatePlanned},
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
	// decide connected, pending_approval, repair_required, or failed.
	RecoveryVerify Recovery = "verify"
	// RecoveryResumeRollback means rollback was interrupted: rollback actions
	// are idempotent, so run the remaining ones again.
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
