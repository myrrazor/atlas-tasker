package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestClassificationInheritanceKeepsHigherParentSensitivity(t *testing.T) {
	_, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Inherited sensitivity",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "project:APP", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "protect project"); err != nil {
		t.Fatalf("classify project: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationPublic, contracts.Actor("human:owner"), "try lower label"); err != nil {
		t.Fatalf("classify ticket: %v", err)
	}
	run := contracts.RunSnapshot{
		RunID:         "run_list_child",
		TicketID:      "APP-1",
		Project:       "APP",
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	evidence := contracts.EvidenceItem{
		EvidenceID:    "ev_list_child",
		RunID:         run.RunID,
		TicketID:      "APP-1",
		Type:          contracts.EvidenceTypeNote,
		Title:         "list child evidence",
		Actor:         contracts.Actor("human:owner"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Evidence.SaveEvidence(ctx, evidence); err != nil {
		t.Fatalf("save evidence: %v", err)
	}
	handoff := contracts.HandoffPacket{
		HandoffID:     "handoff_list_child",
		SourceRunID:   run.RunID,
		TicketID:      "APP-1",
		Actor:         contracts.Actor("human:owner"),
		StatusSummary: "handoff label",
		GeneratedAt:   now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Handoffs.SaveHandoff(ctx, handoff); err != nil {
		t.Fatalf("save handoff: %v", err)
	}
	for _, entity := range []string{"run:" + run.RunID, "evidence:" + evidence.EvidenceID, "handoff:" + handoff.HandoffID} {
		if _, err := actions.SetClassification(ctx, entity, contracts.ClassificationConfidential, contracts.Actor("human:owner"), "classify child entity"); err != nil {
			t.Fatalf("classify %s: %v", entity, err)
		}
	}

	detail, err := actions.ClassificationDetail(ctx, "ticket:APP-1")
	if err != nil {
		t.Fatalf("classification detail: %v", err)
	}
	if detail.Level != contracts.ClassificationRestricted {
		t.Fatalf("ticket should inherit restricted project level despite public explicit label: %#v", detail)
	}
	if detail.Label == nil || detail.Label.Level != contracts.ClassificationPublic {
		t.Fatalf("detail should still expose the explicit ticket label: %#v", detail.Label)
	}

	labels, err := actions.ListClassifications(ctx, "APP")
	if err != nil {
		t.Fatalf("list labels: %v", err)
	}
	for _, want := range []struct {
		kind contracts.ClassifiedEntityKind
		id   string
	}{
		{contracts.ClassifiedEntityProject, "APP"},
		{contracts.ClassifiedEntityTicket, "APP-1"},
		{contracts.ClassifiedEntityRun, run.RunID},
		{contracts.ClassifiedEntityEvidence, evidence.EvidenceID},
		{contracts.ClassifiedEntityHandoff, handoff.HandoffID},
	} {
		if !classificationListContains(labels.Items, want.kind, want.id) {
			t.Fatalf("project filter omitted %s:%s from %#v", want.kind, want.id, labels.Items)
		}
	}
}

func TestGovernanceClassificationScopeUsesEffectiveExactLevel(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if _, err := actions.SetClassification(ctx, "project:APP", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "protect project"); err != nil {
		t.Fatalf("classify project: %v", err)
	}
	publicPolicy := contracts.GovernancePolicy{
		PolicyID:         "public-only-quorum",
		Name:             "Public only quorum",
		ScopeKind:        contracts.PolicyScopeClassification,
		ScopeID:          string(contracts.ClassificationPublic),
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "public-review",
			ActionKind:    contracts.ProtectedActionTicketComplete,
			RequiredCount: 1,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, publicPolicy); err != nil {
		t.Fatalf("save public policy: %v", err)
	}
	public, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{Action: contracts.ProtectedActionTicketComplete, Target: "ticket:" + ticket.ID, Actor: contracts.Actor("human:owner")})
	if err != nil {
		t.Fatalf("explain public policy: %v", err)
	}
	if !public.Allowed || len(public.MatchedPolicies) != 0 {
		t.Fatalf("classification:public must not match effective restricted tickets: %#v", public)
	}

	restrictedPolicy := publicPolicy
	restrictedPolicy.PolicyID = "restricted-quorum"
	restrictedPolicy.Name = "Restricted quorum"
	restrictedPolicy.ScopeID = string(contracts.ClassificationRestricted)
	restrictedPolicy.QuorumRules[0].RuleID = "restricted-review"
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, restrictedPolicy); err != nil {
		t.Fatalf("save restricted policy: %v", err)
	}
	restricted, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{Action: contracts.ProtectedActionTicketComplete, Target: "ticket:" + ticket.ID, Actor: contracts.Actor("human:owner")})
	if err != nil {
		t.Fatalf("explain restricted policy: %v", err)
	}
	if restricted.Allowed || !strings.Contains(strings.Join(restricted.ReasonCodes, ","), "quorum_unsatisfied") {
		t.Fatalf("classification:restricted should match inherited restricted ticket and block without quorum: %#v", restricted)
	}
}

func TestGovernanceRunTargetUsesRunClassification(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	run := contracts.RunSnapshot{
		RunID:         "run_restricted",
		TicketID:      ticket.ID,
		Project:       "APP",
		Status:        contracts.RunStatusHandoffReady,
		Kind:          contracts.RunKindWork,
		CreatedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "run:"+run.RunID, contracts.ClassificationRestricted, contracts.Actor("human:owner"), "run has restricted evidence"); err != nil {
		t.Fatalf("classify run: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "restricted-run-quorum",
		Name:             "Restricted run quorum",
		ScopeKind:        contracts.PolicyScopeClassification,
		ScopeID:          string(contracts.ClassificationRestricted),
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionRunComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "restricted-run-review",
			ActionKind:    contracts.ProtectedActionRunComplete,
			RequiredCount: 1,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save run classification policy: %v", err)
	}
	explained, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{Action: contracts.ProtectedActionRunComplete, Target: "run:" + run.RunID, Actor: contracts.Actor("human:owner")})
	if err != nil {
		t.Fatalf("explain run governance: %v", err)
	}
	if explained.Allowed || !strings.Contains(strings.Join(explained.ReasonCodes, ","), "quorum_unsatisfied") {
		t.Fatalf("run-level restricted label should match classification-scoped run policy: %#v", explained)
	}

	change := contracts.ChangeRef{
		ChangeID:      "chg_run_restricted",
		Provider:      contracts.ChangeProviderLocal,
		TicketID:      ticket.ID,
		RunID:         run.RunID,
		Status:        contracts.ChangeStatusOpen,
		CreatedAt:     actions.now(),
		UpdatedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Changes.SaveChange(ctx, change); err != nil {
		t.Fatalf("save run-backed change: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "restricted-change-quorum",
		Name:             "Restricted change quorum",
		ScopeKind:        contracts.PolicyScopeClassification,
		ScopeID:          string(contracts.ClassificationRestricted),
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionChangeMerge},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "restricted-change-review",
			ActionKind:    contracts.ProtectedActionChangeMerge,
			RequiredCount: 1,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save change classification policy: %v", err)
	}
	changeExplanation, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{Action: contracts.ProtectedActionChangeMerge, Target: "change:" + change.ChangeID, Actor: contracts.Actor("human:owner")})
	if err != nil {
		t.Fatalf("explain change governance: %v", err)
	}
	if changeExplanation.Allowed || !strings.Contains(strings.Join(changeExplanation.ReasonCodes, ","), "quorum_unsatisfied") {
		t.Fatalf("run-backed change should inherit run-level restricted governance: %#v", changeExplanation)
	}

	gate := contracts.GateSnapshot{
		GateID:        "gate_run_restricted",
		TicketID:      ticket.ID,
		RunID:         run.RunID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Gates.SaveGate(ctx, gate); err != nil {
		t.Fatalf("save run-backed gate: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "restricted-gate-quorum",
		Name:             "Restricted gate quorum",
		ScopeKind:        contracts.PolicyScopeClassification,
		ScopeID:          string(contracts.ClassificationRestricted),
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionGateApprove},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "restricted-gate-review",
			ActionKind:    contracts.ProtectedActionGateApprove,
			RequiredCount: 1,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate classification policy: %v", err)
	}
	gateExplanation, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{Action: contracts.ProtectedActionGateApprove, Target: "gate:" + gate.GateID, Actor: contracts.Actor("human:owner")})
	if err != nil {
		t.Fatalf("explain gate governance: %v", err)
	}
	if gateExplanation.Allowed || !strings.Contains(strings.Join(gateExplanation.ReasonCodes, ","), "quorum_unsatisfied") {
		t.Fatalf("run-backed gate should inherit run-level restricted governance: %#v", gateExplanation)
	}
}

func TestRedactedExportOmitRestrictedTicketsAndConsumesPreview(t *testing.T) {
	root, actions, _, projectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Public ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		Description:   "PUBLIC-CONTENT",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "create public ticket"); err != nil {
		t.Fatalf("create public ticket: %v", err)
	}
	if _, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Restricted ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Description:   "SECRET-RED-123",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "create restricted ticket"); err != nil {
		t.Fatalf("create restricted ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-2", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "ticket has restricted content"); err != nil {
		t.Fatalf("classify restricted ticket: %v", err)
	}
	if err := actions.Gates.SaveGate(ctx, contracts.GateSnapshot{
		GateID:         "gate_secret",
		TicketID:       "APP-2",
		Kind:           contracts.GateKindReview,
		State:          contracts.GateStateRejected,
		CreatedBy:      contracts.Actor("human:owner"),
		DecidedBy:      contracts.Actor("human:owner"),
		DecisionReason: "SECRET-GATE-705",
		CreatedAt:      now,
		DecidedAt:      now,
		SchemaVersion:  contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate metadata: %v", err)
	}
	if err := actions.Changes.SaveChange(ctx, contracts.ChangeRef{
		ChangeID:      "chg_secret",
		Provider:      contracts.ChangeProviderLocal,
		TicketID:      "APP-2",
		Status:        contracts.ChangeStatusOpen,
		ReviewSummary: "SECRET-CHANGE-705",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save change metadata: %v", err)
	}
	if err := actions.Checks.SaveCheck(ctx, contracts.CheckResult{
		CheckID:       "chk_secret",
		Source:        contracts.CheckSourceLocal,
		Scope:         contracts.CheckScopeTicket,
		ScopeID:       "APP-2",
		Name:          "restricted check",
		Status:        contracts.CheckStatusCompleted,
		Conclusion:    contracts.CheckConclusionFailure,
		Summary:       "SECRET-CHECK-705",
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save check metadata: %v", err)
	}

	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create redaction preview: %v", err)
	}
	if len(preview.Preview.Items) == 0 {
		t.Fatalf("expected at least one omitted restricted item: %#v", preview.Preview)
	}
	previewInfo, err := os.Stat(filepath.Join(storage.RedactionPreviewsDir(root), preview.Preview.PreviewID+".json"))
	if err != nil {
		t.Fatalf("stat preview file: %v", err)
	}
	if previewInfo.Mode().Perm() != 0o600 {
		t.Fatalf("preview file mode = %04o, want 0600", previewInfo.Mode().Perm())
	}

	created, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "create redacted export")
	if err != nil {
		t.Fatalf("create redacted export: %v", err)
	}
	if created.Bundle.RedactionPreviewID != preview.Preview.PreviewID || created.Omitted == 0 {
		t.Fatalf("redacted export should bind and omit previewed content: %#v", created)
	}
	verified, err := actions.VerifyRedactedArtifact(ctx, created.Bundle.BundleID)
	if err != nil {
		t.Fatalf("verify redacted artifact: %v", err)
	}
	if !verified.Verified {
		t.Fatalf("expected redacted artifact to verify: %#v", verified)
	}
	copiedArtifact := filepath.Join(storage.ExportsDir(root), "copied-redacted.tar.gz")
	copyTestFile(t, created.Bundle.ArtifactPath, copiedArtifact)
	copyTestFile(t, created.Bundle.ManifestPath, filepath.Join(storage.ExportsDir(root), "copied-redacted.manifest.json"))
	copyTestFile(t, created.Bundle.ChecksumPath, filepath.Join(storage.ExportsDir(root), "copied-redacted.sha256"))
	verifiedByCopiedPath, err := actions.VerifyRedactedArtifact(ctx, copiedArtifact)
	if err != nil {
		t.Fatalf("verify copied redacted artifact: %v", err)
	}
	if !verifiedByCopiedPath.Verified || verifiedByCopiedPath.RedactionPreviewID != preview.Preview.PreviewID {
		t.Fatalf("copied redacted artifact should carry preview binding in its manifest: %#v", verifiedByCopiedPath)
	}

	entries := exportBundleEntries(t, created.Bundle.ArtifactPath)
	if _, ok := entries["projects/APP/tickets/APP-2.md"]; ok {
		t.Fatalf("restricted ticket file should be omitted from redacted export")
	}
	for _, path := range []string{
		".tracker/classification/labels/" + classificationLabelID(contracts.ClassifiedEntityTicket, "APP-2") + ".md",
		".tracker/gates/gate_secret.md",
		".tracker/changes/chg_secret.md",
		".tracker/checks/chk_secret.md",
	} {
		if _, ok := entries[path]; ok {
			t.Fatalf("restricted metadata file %s should be omitted from redacted export", path)
		}
	}
	if _, ok := entries["projects/APP/tickets/APP-1.md"]; !ok {
		t.Fatalf("public ticket file should remain in redacted export")
	}
	for path, raw := range entries {
		if strings.Contains(string(raw), "SECRET-RED-123") ||
			strings.Contains(string(raw), "SECRET-GATE-705") ||
			strings.Contains(string(raw), "SECRET-CHANGE-705") ||
			strings.Contains(string(raw), "SECRET-CHECK-705") {
			t.Fatalf("restricted content leaked through %s", path)
		}
	}

	if _, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "reuse preview"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected single-use preview rejection, got %v", err)
	}
}

func TestRedactedExportOmitsRestrictedProjectExtraFiles(t *testing.T) {
	root, actions, _, projectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	extraPath := filepath.Join(root, "projects", "APP", "notes.md")
	if err := os.WriteFile(extraPath, []byte("SECRET-PROJECT-EXTRA-705"), 0o644); err != nil {
		t.Fatalf("write project extra file: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "project:APP", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "project contains restricted files"); err != nil {
		t.Fatalf("classify project: %v", err)
	}
	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	created, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "create redacted export")
	if err != nil {
		t.Fatalf("create redacted export: %v", err)
	}
	entries := exportBundleEntries(t, created.Bundle.ArtifactPath)
	if _, ok := entries["projects/APP/notes.md"]; ok {
		t.Fatalf("restricted project extra file should be omitted")
	}
	for path, raw := range entries {
		if strings.Contains(string(raw), "SECRET-PROJECT-EXTRA-705") {
			t.Fatalf("restricted project extra file leaked through %s", path)
		}
	}
}

func TestRedactedExportEnforcesGovernanceAndUnsupportedActions(t *testing.T) {
	_, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Restricted ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Description:   "SECRET-MASK-789",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "ticket has restricted content"); err != nil {
		t.Fatalf("classify restricted ticket: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "export-quorum",
		Name:             "Export quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionExportCreate},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "export-review",
			ActionKind:    contracts.ProtectedActionExportCreate,
			RequiredCount: 1,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save export policy: %v", err)
	}
	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	if _, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "blocked redacted export"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("redacted export should enforce export_create governance, got %v", err)
	}

	if err := os.RemoveAll(storage.GovernancePoliciesDir(actions.Root)); err != nil {
		t.Fatalf("clear governance policies: %v", err)
	}
	if err := actions.RedactionRules.SaveRedactionRule(ctx, contracts.RedactionRule{
		RuleID:        "mask-restricted-export",
		Target:        contracts.RedactionTargetExport,
		FieldPath:     "*",
		MinLevel:      contracts.ClassificationRestricted,
		Action:        contracts.RedactionMask,
		Reason:        "mask restricted export",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save mask redaction rule: %v", err)
	}
	maskPreview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview mask redaction")
	if err != nil {
		t.Fatalf("create mask preview: %v", err)
	}
	if _, err := actions.CreateRedactedExport(ctx, "workspace", maskPreview.Preview.PreviewID, contracts.Actor("human:owner"), "unsupported mask export"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "only supports omit") {
		t.Fatalf("unsupported export redaction action should fail closed, got %v", err)
	}
}

func TestRedactedExportRejectsTamperedPreviewItems(t *testing.T) {
	_, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Restricted ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Description:   "SECRET-TAMPER-PREVIEW-705",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "restricted ticket"); err != nil {
		t.Fatalf("classify ticket: %v", err)
	}
	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	tampered := preview.Preview
	tampered.Items = nil
	if err := actions.RedactionPreviews.SaveRedactionPreview(ctx, tampered); err != nil {
		t.Fatalf("save tampered preview: %v", err)
	}
	if _, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "use tampered preview"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict || !strings.Contains(err.Error(), "items mismatch") {
		t.Fatalf("expected tampered preview item rejection, got %v", err)
	}
}

func TestRedactedVerifyRejectsBundleThatStillContainsOmittedFiles(t *testing.T) {
	_, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Restricted ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Description:   "SECRET-VERIFY-LEAK-705",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "restricted ticket"); err != nil {
		t.Fatalf("classify ticket: %v", err)
	}
	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	normalExport, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "create non-redacted export")
	if err != nil {
		t.Fatalf("create normal export: %v", err)
	}
	manifest, err := loadBundleManifest(normalExport.Bundle.ManifestPath)
	if err != nil {
		t.Fatalf("load normal export manifest: %v", err)
	}
	manifest.RedactionPreviewID = preview.Preview.PreviewID
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal tampered manifest: %v", err)
	}
	if err := os.WriteFile(normalExport.Bundle.ManifestPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered manifest: %v", err)
	}

	verified, err := actions.VerifyRedactedArtifact(ctx, normalExport.Bundle.ArtifactPath)
	if err != nil {
		t.Fatalf("verify tampered redacted artifact: %v", err)
	}
	if verified.Verified || !strings.Contains(strings.Join(verified.Errors, ","), "redaction_omitted_file_present:projects/APP/tickets/APP-1.md") {
		t.Fatalf("redaction verify should reject bundles containing omitted files: %#v", verified)
	}
}

func TestCustomGoalRuleKeepsDefaultExportOmission(t *testing.T) {
	root, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Restricted ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Description:   "SECRET-GOAL-RULE-705",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "restricted ticket"); err != nil {
		t.Fatalf("classify ticket: %v", err)
	}
	if err := os.MkdirAll(storage.RedactionRulesDir(root), 0o755); err != nil {
		t.Fatalf("create redaction rules dir: %v", err)
	}
	if err := actions.RedactionRules.SaveRedactionRule(ctx, contracts.RedactionRule{
		RuleID:        "goal-only-marker",
		Target:        contracts.RedactionTargetGoal,
		FieldPath:     "*",
		MinLevel:      contracts.ClassificationRestricted,
		Action:        contracts.RedactionReplaceWithMarker,
		Marker:        "[redacted]",
		Reason:        "goal output marker",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save goal-only rule: %v", err)
	}

	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create redaction preview: %v", err)
	}
	if len(preview.Preview.Items) == 0 {
		t.Fatalf("export preview should keep default restricted omission even with goal-only custom rule: %#v", preview.Preview)
	}
	created, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "create redacted export")
	if err != nil {
		t.Fatalf("create redacted export: %v", err)
	}
	entries := exportBundleEntries(t, created.Bundle.ArtifactPath)
	if _, ok := entries["projects/APP/tickets/APP-1.md"]; ok {
		t.Fatalf("default export omission was disabled by a goal-only rule")
	}
	for path, raw := range entries {
		if strings.Contains(string(raw), "SECRET-GOAL-RULE-705") {
			t.Fatalf("restricted content leaked through %s", path)
		}
	}
}

func TestRedactedExportOmitsRunBackedRestrictedMetadata(t *testing.T) {
	_, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Internal ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := contracts.RunSnapshot{
		RunID:         "run_secret_705",
		TicketID:      "APP-1",
		Project:       "APP",
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		Summary:       "SECRET-RUN-705",
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "run:"+run.RunID, contracts.ClassificationRestricted, contracts.Actor("human:owner"), "run has restricted content"); err != nil {
		t.Fatalf("classify run: %v", err)
	}
	if err := actions.Gates.SaveGate(ctx, contracts.GateSnapshot{
		GateID:         "gate_run_secret",
		TicketID:       "APP-1",
		RunID:          run.RunID,
		Kind:           contracts.GateKindReview,
		State:          contracts.GateStateRejected,
		CreatedBy:      contracts.Actor("human:owner"),
		DecidedBy:      contracts.Actor("human:owner"),
		DecisionReason: "SECRET-RUN-GATE-705",
		CreatedAt:      now,
		DecidedAt:      now,
		SchemaVersion:  contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	if err := actions.Changes.SaveChange(ctx, contracts.ChangeRef{
		ChangeID:      "chg_run_secret",
		Provider:      contracts.ChangeProviderLocal,
		TicketID:      "APP-1",
		RunID:         run.RunID,
		Status:        contracts.ChangeStatusOpen,
		ReviewSummary: "SECRET-RUN-CHANGE-705",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save change: %v", err)
	}
	if err := actions.Checks.SaveCheck(ctx, contracts.CheckResult{
		CheckID:       "chk_run_change_secret",
		Source:        contracts.CheckSourceLocal,
		Scope:         contracts.CheckScopeChange,
		ScopeID:       "chg_run_secret",
		Name:          "run change check",
		Status:        contracts.CheckStatusCompleted,
		Conclusion:    contracts.CheckConclusionFailure,
		Summary:       "SECRET-RUN-CHECK-705",
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save check: %v", err)
	}

	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	created, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "create redacted export")
	if err != nil {
		t.Fatalf("create redacted export: %v", err)
	}
	entries := exportBundleEntries(t, created.Bundle.ArtifactPath)
	for _, path := range []string{
		".tracker/runs/run_secret_705.md",
		".tracker/gates/gate_run_secret.md",
		".tracker/changes/chg_run_secret.md",
		".tracker/checks/chk_run_change_secret.md",
		".tracker/classification/labels/" + classificationLabelID(contracts.ClassifiedEntityRun, run.RunID) + ".md",
	} {
		if _, ok := entries[path]; ok {
			t.Fatalf("run-restricted metadata file %s should be omitted", path)
		}
	}
	if _, ok := entries["projects/APP/tickets/APP-1.md"]; !ok {
		t.Fatalf("internal parent ticket should remain when only the run is restricted")
	}
	for path, raw := range entries {
		text := string(raw)
		if strings.Contains(text, "SECRET-RUN-705") ||
			strings.Contains(text, "SECRET-RUN-GATE-705") ||
			strings.Contains(text, "SECRET-RUN-CHANGE-705") ||
			strings.Contains(text, "SECRET-RUN-CHECK-705") {
			t.Fatalf("run-restricted content leaked through %s", path)
		}
	}
}

func TestRedactionPreviewRejectsStaleClassificationState(t *testing.T) {
	_, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Restricted ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Description:   "SECRET-STALE-456",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "initial restricted label"); err != nil {
		t.Fatalf("classify ticket: %v", err)
	}
	preview, err := actions.CreateRedactionPreview(ctx, "workspace", contracts.RedactionTargetExport, contracts.Actor("human:owner"), "preview redacted export")
	if err != nil {
		t.Fatalf("create redaction preview: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-1", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "refresh restricted label"); err != nil {
		t.Fatalf("refresh ticket classification: %v", err)
	}

	if _, err := actions.CreateRedactedExport(ctx, "workspace", preview.Preview.PreviewID, contracts.Actor("human:owner"), "use stale preview"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict || !strings.Contains(err.Error(), "classification hash mismatch") {
		t.Fatalf("expected stale classification preview rejection, got %v", err)
	}
}

func TestClassificationLabelIDsDoNotCollideAfterSanitizing(t *testing.T) {
	root, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	for _, id := range []string{"APP_secret", "APP-secret"} {
		if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
			ID:            id,
			Project:       "APP",
			Title:         "collision candidate " + id,
			Type:          contracts.TicketTypeTask,
			Status:        contracts.StatusReady,
			Priority:      contracts.PriorityMedium,
			CreatedAt:     now,
			UpdatedAt:     now,
			SchemaVersion: contracts.CurrentSchemaVersion,
		}); err != nil {
			t.Fatalf("create ticket %s: %v", id, err)
		}
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP_secret", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "restricted underscore ticket"); err != nil {
		t.Fatalf("classify underscore ticket: %v", err)
	}
	legacyPath := legacyClassificationLabelPath(root, contracts.ClassifiedEntityTicket, "APP_secret")
	if err := os.WriteFile(legacyPath, []byte("stale legacy label"), 0o644); err != nil {
		t.Fatalf("write stale legacy label: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP_secret", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "refresh underscore ticket"); err != nil {
		t.Fatalf("refresh underscore ticket: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy slug-only label should be removed after hashed write, stat err=%v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:APP-secret", contracts.ClassificationPublic, contracts.Actor("human:owner"), "public hyphen ticket"); err != nil {
		t.Fatalf("classify hyphen ticket: %v", err)
	}

	underscore, err := actions.ClassificationDetail(ctx, "ticket:APP_secret")
	if err != nil {
		t.Fatalf("load underscore classification: %v", err)
	}
	hyphen, err := actions.ClassificationDetail(ctx, "ticket:APP-secret")
	if err != nil {
		t.Fatalf("load hyphen classification: %v", err)
	}
	if underscore.Label == nil || underscore.Label.Level != contracts.ClassificationRestricted {
		t.Fatalf("underscore label was overwritten: %#v", underscore.Label)
	}
	if hyphen.Label == nil || hyphen.Label.Level != contracts.ClassificationPublic {
		t.Fatalf("hyphen label was overwritten: %#v", hyphen.Label)
	}
	if classificationLabelID(contracts.ClassifiedEntityTicket, "APP_secret") == classificationLabelID(contracts.ClassifiedEntityTicket, "APP-secret") {
		t.Fatalf("classification label ids still collide")
	}
}

func TestClassificationEventUsesResolvedTicketProject(t *testing.T) {
	_, actions, _, projectStore, ticketStore, eventsLog := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP-OPS", Name: "App Ops", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID:            "custom-ticket",
		Project:       "APP-OPS",
		Title:         "Custom ID",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetClassification(ctx, "ticket:custom-ticket", contracts.ClassificationRestricted, contracts.Actor("human:owner"), "route to real project"); err != nil {
		t.Fatalf("classify custom ticket: %v", err)
	}
	events, err := eventsLog.StreamEvents(ctx, "APP-OPS", 0)
	if err != nil {
		t.Fatalf("stream project events: %v", err)
	}
	for _, event := range events {
		if event.Type == contracts.EventClassificationSet && event.TicketID == "custom-ticket" {
			return
		}
	}
	t.Fatalf("classification event was not written to the resolved ticket project: %#v", events)
}

func exportBundleEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open export bundle: %v", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("read export gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string][]byte{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read export tar: %v", err)
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read export entry %s: %v", header.Name, err)
		}
		entries[filepath.ToSlash(header.Name)] = raw
	}
	return entries
}

func classificationListContains(items []contracts.ClassificationLabel, kind contracts.ClassifiedEntityKind, id string) bool {
	for _, item := range items {
		if item.EntityKind == kind && item.EntityID == id {
			return true
		}
	}
	return false
}

func copyTestFile(t *testing.T, src string, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
