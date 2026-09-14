package setup

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func TestOperationStatesMatchThePlanExactly(t *testing.T) {
	t.Parallel()
	want := []OperationState{"planned", "applying", "applied", "verifying", "connected", "pending_approval", "failed", "rolling_back", "rolled_back", "repair_required"}
	got := States()
	if len(got) != len(want) {
		t.Fatalf("got %d states, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("state %d = %s, want %s", i, got[i], want[i])
		}
		if !got[i].IsValid() {
			t.Fatalf("%s must be valid", got[i])
		}
	}
	if OperationState("connected_restart_required").IsValid() {
		t.Fatalf("integration states are not operation states")
	}
	got[0] = "mutated"
	if States()[0] != StatePlanned {
		t.Fatalf("States must return a copy")
	}
}

func TestOperationClassification(t *testing.T) {
	t.Parallel()
	inFlight := map[OperationState]bool{StateApplying: true, StateApplied: true, StateVerifying: true, StateRollingBack: true}
	for _, state := range States() {
		if state.InFlight() != inFlight[state] {
			t.Fatalf("%s InFlight=%v want %v", state, state.InFlight(), inFlight[state])
		}
		if state.Resting() == state.InFlight() {
			t.Fatalf("%s must be exactly one of resting/in-flight", state)
		}
	}
	if OperationState("bogus").Resting() {
		t.Fatalf("invalid state is not resting")
	}
	for _, state := range []OperationState{StateConnected, StatePendingApproval} {
		if !state.KeepsWrites() {
			t.Fatalf("%s must keep its writes", state)
		}
	}
	for _, state := range []OperationState{StatePlanned, StateFailed, StateRolledBack, StateRepairRequired, StateApplied} {
		if state.KeepsWrites() {
			t.Fatalf("%s must not count as kept writes", state)
		}
	}
	final := map[OperationState]bool{StateRolledBack: true, StateRepairRequired: true}
	for _, state := range States() {
		if state.Final() != final[state] {
			t.Fatalf("%s Final=%v want %v", state, state.Final(), final[state])
		}
		if state.Final() && len(transitions[state]) != 0 {
			t.Fatalf("%s is final but has edges %v", state, transitions[state])
		}
		if !state.Final() && len(transitions[state]) == 0 {
			t.Fatalf("%s is not final but has no outgoing edge", state)
		}
	}
	if StatePlanned.MayHaveWritten() || StateRolledBack.MayHaveWritten() {
		t.Fatalf("planned and rolled_back guarantee no writes remain")
	}
	if !StateFailed.MayHaveWritten() || !StateApplying.MayHaveWritten() || !StateRepairRequired.MayHaveWritten() {
		t.Fatalf("failed, applying, and repair_required may leave writes")
	}
	if OperationState("bogus").MayHaveWritten() {
		t.Fatalf("invalid state must not claim writes")
	}
}

// Review round 1 (S-A): a state that may hold writes can never re-enter a
// state that guarantees none, except through the rollback itself.
func TestOperationNoEdgeErasesWriteEvidence(t *testing.T) {
	t.Parallel()
	for _, from := range States() {
		if !from.MayHaveWritten() {
			continue
		}
		for _, to := range transitions[from] {
			if to.MayHaveWritten() {
				continue
			}
			if from != StateRollingBack || to != StateRolledBack {
				t.Fatalf("%s -> %s erases the write evidence without rolling back", from, to)
			}
		}
	}
	for _, from := range []OperationState{StateFailed, StateRepairRequired, StateRolledBack} {
		if from.Allows(StatePlanned) {
			t.Fatalf("%s -> planned must be a new operation, not an edge", from)
		}
	}
}

// Review round 1 (S-B): failed is reached only when a rollback action itself
// fails; apply and verify errors always pass through rolling_back.
func TestOperationFailedOnlyThroughRollback(t *testing.T) {
	t.Parallel()
	for _, from := range States() {
		if from == StateFailed {
			continue
		}
		if from.Allows(StateFailed) != (from == StateRollingBack) {
			t.Fatalf("%s -> failed allowed=%v; only rolling_back may fail", from, from.Allows(StateFailed))
		}
	}
	if action, _ := StateFailed.RecoveryAction(); action != RecoveryNone {
		t.Fatalf("failed is resting for the operator, got recovery %s", action)
	}
	if !StateFailed.Allows(StateRollingBack) {
		t.Fatalf("an operator may retry the rollback from failed")
	}
}

// Review round 1 (S-C): every integration state has exactly one operation
// state, and the pending/connected distinction is never collapsed.
func TestOutcomeForIsTotalOverIntegrationStates(t *testing.T) {
	t.Parallel()
	want := map[adapter.State]OperationState{
		adapter.StateConnected:                StateConnected,
		adapter.StateConnectedRestartRequired: StateConnected,
		adapter.StatePendingWorkspaceTrust:    StatePendingApproval,
		adapter.StatePendingMCPApproval:       StatePendingApproval,
		adapter.StateConfiguredUnverified:     StatePendingApproval,
		adapter.StatePortableReady:            StatePendingApproval,
		adapter.StateUnsupportedClientVersion: StatePendingApproval,
		adapter.StateRepairRequired:           StateRepairRequired,
		adapter.StateFailed:                   StateRollingBack,
	}
	states := adapter.States()
	if len(states) != len(want) {
		t.Fatalf("mapping covers %d states, adapter has %d", len(want), len(states))
	}
	for _, state := range states {
		got, err := OutcomeFor(state)
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if got != want[state] {
			t.Fatalf("%s -> %s, want %s", state, got, want[state])
		}
		if !StateVerifying.Allows(got) {
			t.Fatalf("%s -> %s is not a legal verify outcome", state, got)
		}
		dropsWrites := state == adapter.StateRepairRequired || state == adapter.StateFailed
		if got.KeepsWrites() == dropsWrites {
			t.Fatalf("%s -> %s: kept writes=%v", state, got, got.KeepsWrites())
		}
		if state.Verified() != (got == StateConnected) {
			t.Fatalf("%s: verified=%v but operation %s", state, state.Verified(), got)
		}
	}
	if _, err := OutcomeFor("ready"); err == nil {
		t.Fatalf("unknown integration state must be an error")
	}
}

func TestOperationTransitionsNoWriteBeforePlanAndNoSkipOfVerification(t *testing.T) {
	t.Parallel()
	legal := [][2]OperationState{
		{StatePlanned, StateApplying},
		{StateApplying, StateApplied},
		{StateApplying, StateRollingBack},
		{StateApplied, StateVerifying},
		{StateApplied, StateRollingBack},
		{StateVerifying, StateConnected},
		{StateVerifying, StatePendingApproval},
		{StateVerifying, StateRepairRequired},
		{StateVerifying, StateRollingBack},
		{StateConnected, StateVerifying},
		{StateConnected, StateRepairRequired},
		{StatePendingApproval, StateVerifying},
		{StatePendingApproval, StateRepairRequired},
		{StatePendingApproval, StateRollingBack},
		{StateFailed, StateRollingBack},
		{StateRollingBack, StateRolledBack},
		{StateRollingBack, StateFailed},
	}
	for _, edge := range legal {
		if _, err := edge[0].Transition(edge[1]); err != nil {
			t.Fatalf("%s -> %s must be legal: %v", edge[0], edge[1], err)
		}
	}
	edges := 0
	for _, targets := range transitions {
		edges += len(targets)
	}
	if edges != len(legal) {
		t.Fatalf("state machine has %d edges, test pins %d", edges, len(legal))
	}
	forbidden := [][2]OperationState{
		{StatePlanned, StateApplied},      // cannot claim writes without applying
		{StatePlanned, StateConnected},    // cannot connect without writing and verifying
		{StateApplying, StateConnected},   // verification cannot be skipped
		{StateApplied, StateConnected},    // verification cannot be skipped
		{StateApplying, StateVerifying},   // all steps must finish first
		{StateApplying, StateFailed},      // an apply error rolls back first (review S-B)
		{StateVerifying, StateFailed},     // a verify error rolls back first (review S-B)
		{StateRolledBack, StateConnected}, // nothing left to verify
		{StateFailed, StateConnected},     // no silent recovery
		{StateFailed, StatePlanned},       // a fresh plan is a new operation (review S-A)
		{StateRolledBack, StatePlanned},   // same
		{StateRepairRequired, StatePlanned},
		{StateConnected, StateApplying}, // re-apply needs a new plan
		{StateRepairRequired, StateApplying},
		{StateRepairRequired, StateRollingBack}, // the drifted disk is not this operation's snapshot
		{StatePendingApproval, StateConnected},  // approval must be re-verified, never assumed
		{StateRolledBack, StateRollingBack},
	}
	for _, edge := range forbidden {
		if edge[0].Allows(edge[1]) {
			t.Fatalf("%s -> %s must be forbidden", edge[0], edge[1])
		}
		if _, err := edge[0].Transition(edge[1]); err == nil || !strings.Contains(err.Error(), "cannot move") {
			t.Fatalf("%s -> %s: unexpected error %v", edge[0], edge[1], err)
		}
	}
	for _, state := range States() {
		if !state.Allows(state) {
			t.Fatalf("%s must allow a journal re-mark to itself", state)
		}
		if state.Allows("bogus") || OperationState("bogus").Allows(state) {
			t.Fatalf("invalid states never transition")
		}
	}
	// Every state except planned is reachable; planned is entered only by
	// creating a new operation.
	reachable := map[OperationState]bool{StatePlanned: true}
	for _, targets := range transitions {
		for _, target := range targets {
			if target == StatePlanned {
				t.Fatalf("no edge may lead back to planned")
			}
			reachable[target] = true
		}
	}
	for _, state := range States() {
		if !reachable[state] {
			t.Fatalf("%s is unreachable", state)
		}
	}
}

func TestOperationRecoveryFromInterruption(t *testing.T) {
	t.Parallel()
	cases := map[OperationState]struct {
		action Recovery
		target OperationState
	}{
		StatePlanned:         {RecoveryNone, StatePlanned},
		StateApplying:        {RecoveryRollBack, StateRollingBack},
		StateApplied:         {RecoveryVerify, StateVerifying},
		StateVerifying:       {RecoveryVerify, StateVerifying},
		StateConnected:       {RecoveryNone, StateConnected},
		StatePendingApproval: {RecoveryNone, StatePendingApproval},
		StateFailed:          {RecoveryNone, StateFailed},
		StateRollingBack:     {RecoveryResumeRollback, StateRollingBack},
		StateRolledBack:      {RecoveryNone, StateRolledBack},
		StateRepairRequired:  {RecoveryNone, StateRepairRequired},
	}
	for state, want := range cases {
		action, err := state.RecoveryAction()
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if action != want.action {
			t.Fatalf("%s: action %s want %s", state, action, want.action)
		}
		target, err := state.RecoveryTarget()
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if target != want.target {
			t.Fatalf("%s: target %s want %s", state, target, want.target)
		}
		if !state.Allows(target) {
			t.Fatalf("%s: recovery target %s must be a legal move", state, target)
		}
		if (action != RecoveryNone) != state.InFlight() {
			t.Fatalf("%s: only in-flight states recover", state)
		}
	}
	if _, err := OperationState("bogus").RecoveryAction(); err == nil {
		t.Fatalf("unknown journal state must be an error")
	}
	if _, err := OperationState("bogus").RecoveryTarget(); err == nil {
		t.Fatalf("unknown journal state must be an error")
	}
}
