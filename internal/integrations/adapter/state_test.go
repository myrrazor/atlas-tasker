package adapter

import (
	"reflect"
	"testing"
)

func TestStatesAreTheLockedVocabulary(t *testing.T) {
	want := []State{
		"connected",
		"connected_restart_required",
		"pending_workspace_trust",
		"pending_mcp_approval",
		"configured_unverified",
		"portable_ready",
		"unsupported_client_version",
		"repair_required",
		"failed",
	}
	if got := States(); !reflect.DeepEqual(got, want) {
		t.Fatalf("States() = %v, want %v", got, want)
	}
	for _, state := range want {
		if !state.IsValid() {
			t.Fatalf("%s should be valid", state)
		}
	}
	for _, bad := range []State{"", "ok", "Connected", "connected_by_standard_config"} {
		if bad.IsValid() {
			t.Fatalf("%q should not be a state", bad)
		}
	}
}

func TestStateClassification(t *testing.T) {
	verified := map[State]bool{StateConnected: true, StateConnectedRestartRequired: true}
	human := map[State]bool{StatePendingWorkspaceTrust: true, StatePendingMCPApproval: true, StateUnsupportedClientVersion: true, StateRepairRequired: true, StateFailed: true}
	for _, state := range States() {
		if state.Verified() != verified[state] {
			t.Fatalf("%s Verified() = %v", state, state.Verified())
		}
		if state.RequiresHumanAction() != human[state] {
			t.Fatalf("%s RequiresHumanAction() = %v", state, state.RequiresHumanAction())
		}
	}
}

func TestConnectionKinds(t *testing.T) {
	for _, kind := range []ConnectionKind{ConnectionKindClientNative, ConnectionKindSelfProbe, ConnectionKindStandardConfig, ConnectionKindCustomAdapter} {
		if !kind.IsValid() {
			t.Fatalf("%s should be valid", kind)
		}
	}
	if ConnectionKind("").IsValid() || ConnectionKind("magic").IsValid() {
		t.Fatal("unknown connection kinds must be invalid")
	}
}
