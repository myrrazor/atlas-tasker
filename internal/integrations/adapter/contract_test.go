package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

// fakeAdapter is the smallest conforming implementation: it proves the
// interface is implementable from outside provider code and exercises the
// registry rules.
type fakeAdapter struct {
	target integrations.Target
	caps   Capabilities
}

var _ AgentIntegrationAdapter = fakeAdapter{}

func newFakeAdapter(t *testing.T, target integrations.Target) fakeAdapter {
	t.Helper()
	caps, err := CapabilitiesFor(target)
	if err != nil {
		t.Fatal(err)
	}
	return fakeAdapter{target: target, caps: caps}
}

func (f fakeAdapter) Target() integrations.Target { return f.target }
func (f fakeAdapter) Capabilities() Capabilities  { return f.caps }
func (f fakeAdapter) Detect(context.Context, DetectInput) Detection {
	return Detection{Target: f.target, VersionSupport: VersionUnknown}
}
func (f fakeAdapter) Plan(_ context.Context, input PlanInput) (IntegrationPlan, error) {
	if err := input.Validate(); err != nil {
		return IntegrationPlan{}, err
	}
	return IntegrationPlan{
		PlanID: "fake", ContractVersion: ContractVersion, Operation: PlanOperationSetup, Target: f.target,
		WorkspaceID: input.WorkspaceID, WorkspaceRoot: input.WorkspaceRoot, Scope: f.caps.PreferredScope,
		Detection: input.Detection, ResultingState: StateConfiguredUnverified, NoOp: true, GeneratedAt: time.Now(),
	}, nil
}
func (f fakeAdapter) Apply(context.Context, IntegrationPlan) (ApplyResult, error) {
	return ApplyResult{}, errors.New("not implemented")
}
func (f fakeAdapter) Verify(context.Context, IntegrationState) (Verification, error) {
	return Verification{}, errors.New("not implemented")
}
func (f fakeAdapter) Repair(context.Context, IntegrationState) (RepairPlan, error) {
	return RepairPlan{}, errors.New("not implemented")
}
func (f fakeAdapter) Remove(context.Context, IntegrationState) (RemovalPlan, error) {
	return RemovalPlan{}, errors.New("not implemented")
}

func TestRegistryAcceptsOneAdapterPerTarget(t *testing.T) {
	registry := NewRegistry()
	for _, target := range integrations.DetectableTargets() {
		if err := registry.Register(newFakeAdapter(t, target)); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
	}
	if err := registry.Register(newFakeAdapter(t, integrations.TargetCodex)); err == nil {
		t.Fatal("duplicate registration must fail")
	}
	if got := registry.Targets(); len(got) != 6 || got[0] != integrations.TargetClaude || got[1] != integrations.TargetCodex || got[5] != integrations.TargetGeneric {
		t.Fatalf("unexpected registry order: %v", got)
	}
	if _, ok := registry.Lookup(integrations.TargetGrok); !ok {
		t.Fatal("grok adapter missing")
	}
	if _, ok := registry.Lookup("emacs"); ok {
		t.Fatal("unknown target must not resolve")
	}
}

func TestRegistryRejectsDisagreementWithMatrix(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(nil); err == nil {
		t.Fatal("nil adapter must fail")
	}
	wrongTarget := newFakeAdapter(t, integrations.TargetCodex)
	wrongTarget.target = "emacs"
	if err := registry.Register(wrongTarget); err == nil {
		t.Fatal("unknown target must fail")
	}
	mismatch := newFakeAdapter(t, integrations.TargetCodex)
	mismatch.caps.Target = integrations.TargetClaude
	if err := registry.Register(mismatch); err == nil {
		t.Fatal("capabilities for another target must fail")
	}
	generous := newFakeAdapter(t, integrations.TargetGeneric)
	generous.caps.MaxPlannedState = StateConnected
	if err := registry.Register(generous); err == nil {
		t.Fatal("an adapter claiming more than the matrix must fail")
	}
}

func TestFakeAdapterPlanValidates(t *testing.T) {
	adapter := newFakeAdapter(t, integrations.TargetCursor)
	plan, err := adapter.Plan(context.Background(), PlanInput{WorkspaceRoot: testRoot, WorkspaceID: testWorkspaceID, Home: testHome, TrackerPath: testTracker, Detection: Detection{Target: integrations.TargetCursor, VersionSupport: VersionUnknown}})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("fake plan invalid: %v", err)
	}
}
