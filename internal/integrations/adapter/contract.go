package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

// AgentIntegrationAdapter is the single contract every one of the six targets
// implements. The setup engine drives adapters through this interface only;
// it never reaches into provider-specific code.
//
// Ordering and side-effect rules:
//
//   - Detect is read-only. It may run read-only client commands (version,
//     list) only when DetectInput.Runner is non-nil, and every command it ran
//     appears in Detection.Probes.
//   - Plan is read-only and deterministic: identical inputs yield plans with
//     identical Fingerprint values. Plan never writes.
//   - Apply executes exactly the steps of a validated plan under the setup
//     journal and returns configured_unverified, a pending_* state,
//     portable_ready, unsupported_client_version, or failed. It never returns
//     a verified state.
//   - Verify runs probes and client-native checks and is the only method that
//     may produce connected or connected_restart_required.
//   - Repair and Remove return plans; they do not write. The engine applies
//     them through Apply.
type AgentIntegrationAdapter interface {
	Target() integrations.Target
	Capabilities() Capabilities
	Detect(ctx context.Context, input DetectInput) Detection
	Plan(ctx context.Context, input PlanInput) (IntegrationPlan, error)
	Apply(ctx context.Context, plan IntegrationPlan) (ApplyResult, error)
	Verify(ctx context.Context, state IntegrationState) (Verification, error)
	Repair(ctx context.Context, state IntegrationState) (RepairPlan, error)
	Remove(ctx context.Context, state IntegrationState) (RemovalPlan, error)
}

// Registry holds one adapter per target and refuses duplicates or adapters
// whose declared target or capabilities disagree with the matrix.
type Registry struct {
	adapters map[integrations.Target]AgentIntegrationAdapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: map[integrations.Target]AgentIntegrationAdapter{}}
}

func (r *Registry) Register(adapter AgentIntegrationAdapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}
	target := adapter.Target()
	if !isKnownTarget(target) {
		return fmt.Errorf("adapter declares unknown target %q", target)
	}
	if _, dup := r.adapters[target]; dup {
		return fmt.Errorf("adapter for %s already registered", target)
	}
	caps := adapter.Capabilities()
	if caps.Target != target {
		return fmt.Errorf("adapter %s reports capabilities for %s", target, caps.Target)
	}
	expected, err := CapabilitiesFor(target)
	if err != nil {
		return err
	}
	if !capabilitiesEqual(caps, expected) {
		return fmt.Errorf("adapter %s disagrees with the capability matrix", target)
	}
	if err := caps.Validate(); err != nil {
		return err
	}
	r.adapters[target] = adapter
	return nil
}

// capabilitiesEqual compares two rows field by field through their canonical
// JSON form, so an adapter cannot report a verification method, approval,
// restart requirement, source, or support claim the matrix does not make.
func capabilitiesEqual(a, b Capabilities) bool {
	rawA, errA := json.Marshal(a)
	rawB, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(rawA, rawB)
}

func (r *Registry) Lookup(target integrations.Target) (AgentIntegrationAdapter, bool) {
	adapter, ok := r.adapters[target]
	return adapter, ok
}

// Targets lists registered targets in matrix order.
func (r *Registry) Targets() []integrations.Target {
	out := make([]integrations.Target, 0, len(r.adapters))
	for _, row := range Matrix() {
		if _, ok := r.adapters[row.Target]; ok {
			out = append(out, row.Target)
		}
	}
	return out
}
