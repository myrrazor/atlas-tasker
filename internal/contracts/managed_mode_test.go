package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustPolicyForMode(t *testing.T, mode ManagedMode) ManagedModePolicy {
	t.Helper()
	policy, err := ManagedModePolicyForMode(mode)
	if err != nil {
		t.Fatalf("%s: %v", mode, err)
	}
	return policy
}

func TestManagedModeDefaultsValidateForEveryMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []ManagedMode{ManagedModeGuidance, ManagedModeManaged, ManagedModeDelivery, ManagedModeDisabled} {
		policy := mustPolicyForMode(t, mode)
		if err := policy.Validate(); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if policy.MCPPreferred != mode.UsesMCP() {
			t.Fatalf("%s: mcp_preferred=%v want %v", mode, policy.MCPPreferred, mode.UsesMCP())
		}
	}
	// Review round 1 (M-B): an invalid mode is an error, not a policy that
	// fails validation later.
	if _, err := ManagedModePolicyForMode("auto"); err == nil || !strings.Contains(err.Error(), "invalid managed mode") {
		t.Fatalf("invalid mode must be refused by the constructor, got %v", err)
	}
	def := DefaultManagedModePolicy()
	want := ManagedModePolicy{
		Format: ManagedModeFormat, Mode: ManagedModeManaged, CapturePolicy: CapturePolicyMaterialWork,
		ProgressPolicy: ProgressPolicyMilestones, StatusPolicy: StatusPolicyAtlasRequired,
		CompletionPolicy: ManagedCompletionFollowWorkspace, MCPPreferred: true,
	}
	if def != want {
		t.Fatalf("default policy = %+v, want %+v", def, want)
	}
	if !ManagedModeDelivery.RequiresExplicitEnable() || ManagedModeManaged.RequiresExplicitEnable() {
		t.Fatalf("only delivery requires explicit enable")
	}
	if ManagedModeDisabled.TracksWork() || !ManagedModeGuidance.TracksWork() || ManagedMode("x").TracksWork() {
		t.Fatalf("TracksWork mismatch")
	}
	if !ManagedModeDelivery.AllowsDelivery() || ManagedModeManaged.AllowsDelivery() {
		t.Fatalf("AllowsDelivery mismatch")
	}
}

func TestManagedModeValidateRejectsBadVocabularyAndCrossFieldRules(t *testing.T) {
	t.Parallel()
	base := DefaultManagedModePolicy()
	cases := []struct {
		name   string
		mutate func(*ManagedModePolicy)
		want   string
	}{
		{"format", func(p *ManagedModePolicy) { p.Format = "atlas_managed_mode_v2" }, "format must be"},
		{"mode", func(p *ManagedModePolicy) { p.Mode = "auto" }, "invalid managed mode"},
		{"capture", func(p *ManagedModePolicy) { p.CapturePolicy = "always" }, "invalid capture policy"},
		{"progress", func(p *ManagedModePolicy) { p.ProgressPolicy = "hourly" }, "invalid progress policy"},
		{"status plan alternate spelling", func(p *ManagedModePolicy) { p.StatusPolicy = "query_atlas" }, "invalid status policy"},
		{"completion plan alternate spelling", func(p *ManagedModePolicy) { p.CompletionPolicy = "follow_workspace_policy" }, "invalid completion policy"},
		{"completion waiver", func(p *ManagedModePolicy) { p.CompletionPolicy = "self_approve" }, "invalid completion policy"},
		{"disabled captures", func(p *ManagedModePolicy) {
			p.Mode = ManagedModeDisabled
			p.ProgressPolicy = ProgressPolicyNone
		}, "disabled mode requires capture policy"},
		{"disabled records progress", func(p *ManagedModePolicy) {
			p.Mode = ManagedModeDisabled
			p.CapturePolicy = CapturePolicyNever
		}, "disabled mode requires progress policy"},
		{"guidance prefers mcp", func(p *ManagedModePolicy) { p.Mode = ManagedModeGuidance }, "cannot prefer MCP"},
		// Review round 1 (M-A): disabled mode registers no server, so a
		// document that says disabled yet prefers MCP is contradictory.
		{"disabled prefers mcp", func(p *ManagedModePolicy) {
			p.Mode = ManagedModeDisabled
			p.CapturePolicy = CapturePolicyNever
			p.ProgressPolicy = ProgressPolicyNone
		}, "cannot prefer MCP"},
	}
	for _, tc := range cases {
		policy := base
		tc.mutate(&policy)
		err := policy.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: err=%v want containing %q", tc.name, err, tc.want)
		}
	}
}

// Review round 1 (M-C): the shared document may declare delivery, but the
// mode agents follow on a machine is delivery only when that machine's private
// setup state records the separately registered delivery-profile server. A
// cloned or restored repository can therefore never switch an agent into
// advanced operations on its own.
func TestManagedModeEffectiveModeRequiresLocalDeliveryEnablement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode    ManagedMode
		enabled bool
		want    ManagedMode
	}{
		{ManagedModeDelivery, false, ManagedModeManaged},
		{ManagedModeDelivery, true, ManagedModeDelivery},
		{ManagedModeManaged, true, ManagedModeManaged},
		{ManagedModeManaged, false, ManagedModeManaged},
		{ManagedModeGuidance, true, ManagedModeGuidance},
		{ManagedModeGuidance, false, ManagedModeGuidance},
		{ManagedModeDisabled, true, ManagedModeDisabled},
		{ManagedModeDisabled, false, ManagedModeDisabled},
	}
	for _, tc := range cases {
		policy := mustPolicyForMode(t, tc.mode)
		got, err := policy.EffectiveMode(tc.enabled)
		if err != nil {
			t.Fatalf("%s/%v: %v", tc.mode, tc.enabled, err)
		}
		if got != tc.want {
			t.Fatalf("%s/enabled=%v: effective mode %s, want %s", tc.mode, tc.enabled, got, tc.want)
		}
		if got.AllowsDelivery() != (tc.mode == ManagedModeDelivery && tc.enabled) {
			t.Fatalf("%s/enabled=%v: AllowsDelivery=%v", tc.mode, tc.enabled, got.AllowsDelivery())
		}
	}
	// A document that declares delivery still parses and validates: the
	// downgrade is an evaluation rule, not a rejection of the shared file.
	declared, err := mustPolicyForMode(t, ManagedModeDelivery).Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManagedModePolicy(declared)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Mode != ManagedModeDelivery {
		t.Fatalf("shared document must keep its declared mode, got %s", parsed.Mode)
	}
	// An invalid policy is an error, never a silently downgraded mode.
	if _, err := (ManagedModePolicy{}).EffectiveMode(true); err == nil {
		t.Fatalf("invalid policy must error")
	}
}

func TestManagedModeEncodeParseRoundTripIsCanonical(t *testing.T) {
	t.Parallel()
	policy := DefaultManagedModePolicy()
	data, err := policy.Encode()
	if err != nil {
		t.Fatal(err)
	}
	const want = `{
  "format": "atlas_managed_mode_v1",
  "mode": "managed",
  "capture_policy": "material_work",
  "progress_policy": "milestones",
  "status_policy": "atlas_required",
  "completion_policy": "follow_workspace",
  "mcp_preferred": true
}
`
	if string(data) != want {
		t.Fatalf("encoded form:\n%s\nwant:\n%s", data, want)
	}
	parsed, err := ParseManagedModePolicy(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != policy {
		t.Fatalf("round trip mismatch: %+v", parsed)
	}
	if _, err := (ManagedModePolicy{}).Encode(); err == nil {
		t.Fatalf("encoding an invalid policy must fail")
	}
}

func TestManagedModeParseFailsClosed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"unknown waiver field": `{"format":"atlas_managed_mode_v1","mode":"managed","capture_policy":"material_work","progress_policy":"milestones","status_policy":"atlas_required","completion_policy":"follow_workspace","mcp_preferred":true,"skip_review":true}`,
		"missing format":       `{"mode":"managed","capture_policy":"material_work","progress_policy":"milestones","status_policy":"atlas_required","completion_policy":"follow_workspace","mcp_preferred":true}`,
		"trailing document":    `{"format":"atlas_managed_mode_v1","mode":"managed","capture_policy":"material_work","progress_policy":"milestones","status_policy":"atlas_required","completion_policy":"follow_workspace","mcp_preferred":true}{}`,
		"wrong type":           `{"format":"atlas_managed_mode_v1","mode":"managed","capture_policy":"material_work","progress_policy":"milestones","status_policy":"atlas_required","completion_policy":"follow_workspace","mcp_preferred":"yes"}`,
		"not json":             `mode = "managed"`,
	}
	for name, doc := range cases {
		if _, err := ParseManagedModePolicy([]byte(doc)); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	// The plan's example document, with the canonical status spelling, parses.
	plan := `{
  "format": "atlas_managed_mode_v1",
  "mode": "managed",
  "capture_policy": "material_work",
  "progress_policy": "milestones",
  "status_policy": "atlas_required",
  "completion_policy": "follow_workspace",
  "mcp_preferred": true
}`
	if _, err := ParseManagedModePolicy([]byte(plan)); err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal([]byte(plan), &generic); err != nil {
		t.Fatal(err)
	}
	if len(generic) != 7 {
		t.Fatalf("the v1 document has exactly seven fields, got %d", len(generic))
	}
}

func TestManagedModeCaptureDecisionNeverTracksNonMaterialWork(t *testing.T) {
	t.Parallel()
	nonMaterial := []WorkIntent{WorkIntentStatusQuery, WorkIntentReadOnlyExplanation, WorkIntentCasualDiscussion, WorkIntentSimpleQuestion, WorkIntentTrackingExcluded}
	for _, mode := range []ManagedMode{ManagedModeGuidance, ManagedModeManaged, ManagedModeDelivery, ManagedModeDisabled} {
		for _, capture := range []CapturePolicy{CapturePolicyNever, CapturePolicyAsk, CapturePolicyMaterialWork} {
			policy := mustPolicyForMode(t, mode)
			if mode != ManagedModeDisabled {
				policy.CapturePolicy = capture
			}
			for _, intent := range nonMaterial {
				got, err := policy.CaptureDecision(intent)
				if err != nil {
					t.Fatal(err)
				}
				if got != CaptureNoTicket {
					t.Fatalf("%s/%s/%s: got %s, want no_ticket", mode, capture, intent, got)
				}
			}
		}
	}
	if _, err := DefaultManagedModePolicy().CaptureDecision("vibes"); err == nil {
		t.Fatalf("invalid intent must error")
	}
	if _, err := (ManagedModePolicy{}).CaptureDecision(WorkIntentMaterialWork); err == nil {
		t.Fatalf("invalid policy must error, not decide")
	}
}

func TestManagedModeCaptureDecisionForMaterialWork(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode    ManagedMode
		capture CapturePolicy
		want    CaptureDecision
	}{
		{ManagedModeManaged, CapturePolicyMaterialWork, CaptureAttachOrCreate},
		{ManagedModeManaged, CapturePolicyAsk, CaptureAskUser},
		{ManagedModeManaged, CapturePolicyNever, CaptureNoTicket},
		{ManagedModeGuidance, CapturePolicyMaterialWork, CaptureAttachOrCreate},
		{ManagedModeDelivery, CapturePolicyAsk, CaptureAskUser},
		{ManagedModeDisabled, CapturePolicyNever, CaptureNoTicket},
	}
	for _, tc := range cases {
		policy := mustPolicyForMode(t, tc.mode)
		policy.CapturePolicy = tc.capture
		got, err := policy.CaptureDecision(WorkIntentMaterialWork)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.mode, tc.capture, err)
		}
		if got != tc.want {
			t.Fatalf("%s/%s: got %s want %s", tc.mode, tc.capture, got, tc.want)
		}
	}
}

func TestManagedModeProgressDecisions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode       ManagedMode
		progress   ProgressPolicy
		milestone  bool
		checkpoint bool
	}{
		{ManagedModeManaged, ProgressPolicyMilestones, true, false},
		{ManagedModeManaged, ProgressPolicyEveryCheckpoint, true, true},
		{ManagedModeManaged, ProgressPolicyNone, false, false},
		{ManagedModeDisabled, ProgressPolicyNone, false, false},
	}
	for _, tc := range cases {
		policy := mustPolicyForMode(t, tc.mode)
		policy.ProgressPolicy = tc.progress
		gotMilestone, err := policy.RecordsProgress(ProgressEventMilestone)
		if err != nil {
			t.Fatal(err)
		}
		gotCheckpoint, err := policy.RecordsProgress(ProgressEventCheckpoint)
		if err != nil {
			t.Fatal(err)
		}
		if gotMilestone != tc.milestone || gotCheckpoint != tc.checkpoint {
			t.Fatalf("%s/%s: milestone=%v checkpoint=%v want %v/%v", tc.mode, tc.progress, gotMilestone, gotCheckpoint, tc.milestone, tc.checkpoint)
		}
	}
	if _, err := DefaultManagedModePolicy().RecordsProgress("hourly"); err == nil {
		t.Fatalf("invalid progress event must error")
	}
}

func TestManagedModeNeverOverridesWorkspaceCompletion(t *testing.T) {
	t.Parallel()
	policy := DefaultManagedModePolicy()
	for _, mode := range []CompletionMode{CompletionModeOpen, CompletionModeOwnerGate, CompletionModeReviewGate, CompletionModeDualGate} {
		got, err := policy.EffectiveCompletionMode(mode)
		if err != nil {
			t.Fatal(err)
		}
		if got != mode {
			t.Fatalf("policy changed completion mode %s to %s", mode, got)
		}
	}
	if _, err := policy.EffectiveCompletionMode("anyone"); err == nil {
		t.Fatalf("invalid workspace completion mode must not fall back to open")
	}
}
