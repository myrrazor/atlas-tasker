package setup

import (
	"strings"
	"testing"
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
		if !state.Succeeded() {
			t.Fatalf("%s must count as success", state)
		}
	}
	for _, state := range []OperationState{StatePlanned, StateFailed, StateRolledBack, StateRepairRequired, StateApplied} {
		if state.Succeeded() {
			t.Fatalf("%s must not count as success", state)
		}
	}
	if StatePlanned.MayHaveWritten() || StateRolledBack.MayHaveWritten() {
		t.Fatalf("planned and rolled_back guarantee no writes remain")
	}
	if !StateFailed.MayHaveWritten() || !StateApplying.MayHaveWritten() {
		t.Fatalf("failed and applying may leave writes")
	}
	if OperationState("bogus").MayHaveWritten() {
		t.Fatalf("invalid state must not claim writes")
	}
}

func TestOperationTransitionsNoWriteBeforePlanAndNoSkipOfVerification(t *testing.T) {
	t.Parallel()
	legal := [][2]OperationState{
		{StatePlanned, StateApplying},
		{StateApplying, StateApplied},
		{StateApplying, StateRollingBack},
		{StateApplying, StateFailed},
		{StateApplied, StateVerifying},
		{StateApplied, StateRollingBack},
		{StateVerifying, StateConnected},
		{StateVerifying, StatePendingApproval},
		{StateVerifying, StateRepairRequired},
		{StateVerifying, StateRollingBack},
		{StateVerifying, StateFailed},
		{StateConnected, StateVerifying},
		{StateConnected, StateRepairRequired},
		{StatePendingApproval, StateVerifying},
		{StatePendingApproval, StateRepairRequired},
		{StatePendingApproval, StateRollingBack},
		{StateFailed, StateRollingBack},
		{StateFailed, StatePlanned},
		{StateRollingBack, StateRolledBack},
		{StateRollingBack, StateFailed},
		{StateRolledBack, StatePlanned},
		{StateRepairRequired, StatePlanned},
	}
	for _, edge := range legal {
		if _, err := edge[0].Transition(edge[1]); err != nil {
			t.Fatalf("%s -> %s must be legal: %v", edge[0], edge[1], err)
		}
	}
	forbidden := [][2]OperationState{
		{StatePlanned, StateApplied},      // cannot claim writes without applying
		{StatePlanned, StateConnected},    // cannot connect without writing and verifying
		{StateApplying, StateConnected},   // verification cannot be skipped
		{StateApplied, StateConnected},    // verification cannot be skipped
		{StateApplying, StateVerifying},   // all steps must finish first
		{StateRolledBack, StateConnected}, // nothing left to verify
		{StateFailed, StateConnected},     // no silent recovery
		{StateConnected, StateApplying},   // re-apply needs a new plan
		{StateRepairRequired, StateApplying},
		{StatePendingApproval, StateConnected}, // approval must be re-verified, never assumed
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
	// Every state except planned is reachable and every non-terminal state has a way forward.
	reachable := map[OperationState]bool{StatePlanned: true}
	for _, targets := range transitions {
		for _, target := range targets {
			reachable[target] = true
		}
	}
	for _, state := range States() {
		if !reachable[state] {
			t.Fatalf("%s is unreachable", state)
		}
		if len(transitions[state]) == 0 {
			t.Fatalf("%s has no outgoing edge", state)
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
