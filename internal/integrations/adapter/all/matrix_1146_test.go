package all

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func TestSixProviderPlanMatrixNeverClaimsVerified(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, target := range integrations.DetectableTargets() {
		t.Run(string(target), func(t *testing.T) {
			item, ok := reg.Lookup(target)
			if !ok {
				t.Fatal("missing adapter")
			}
			input, _ := testPlanInput(t, target, adapter.Detection{Installed: false, VersionSupport: adapter.VersionUnknown})
			plan, err := item.Plan(ctx, input)
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			if plan.ResultingState.Verified() {
				t.Fatalf("plan must not claim verified, got %s", plan.ResultingState)
			}
		})
	}
}

func TestGenericApplyVerifyRepairRemoveLifecycle(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir(), Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := reg.Lookup(integrations.TargetGeneric)
	if !ok {
		t.Fatal("missing generic adapter")
	}
	input, _ := testPlanInput(t, integrations.TargetGeneric, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	ctx := context.Background()
	plan, err := item.Plan(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := item.Apply(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Record == nil {
		t.Fatal("missing apply record")
	}
	if _, err := item.Verify(ctx, *applied.Record); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := item.Repair(ctx, *applied.Record); err != nil {
		t.Fatalf("repair: %v", err)
	}
	removal, err := item.Remove(ctx, *applied.Record)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := item.Apply(ctx, removal.Plan); err != nil {
		t.Fatalf("remove apply: %v", err)
	}
}

func TestCursorSymlinkConfigIsRefused(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := reg.Lookup(integrations.TargetCursor)
	if !ok {
		t.Fatal("missing cursor adapter")
	}
	input, root := testPlanInput(t, integrations.TargetCursor, adapter.Detection{Installed: true, VersionSupport: adapter.VersionSupported})
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(outside, []byte(`{"mcpServers":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".cursor", "mcp.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Plan(context.Background(), input); err == nil {
		t.Fatal("cursor plan must refuse a symlinked config")
	}
}
