package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestAuditOrchestrationReportsDriftAndBrokenArtifacts(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Doctor drift",
		Summary:       "Doctor drift",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_1",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		WorktreePath:  filepath.Join(root, "missing-worktree"),
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := os.MkdirAll(storage.RuntimeDir(root, run.RunID), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.WriteFile(storage.RuntimeBriefFile(root, run.RunID), []byte("brief"), 0o644); err != nil {
		t.Fatalf("write runtime brief: %v", err)
	}

	evidenceMissing := contracts.EvidenceItem{
		EvidenceID:    "evidence_missing",
		RunID:         run.RunID,
		TicketID:      ticket.ID,
		Type:          contracts.EvidenceTypeLogExcerpt,
		Title:         "missing artifact",
		Body:          "artifact drift",
		ArtifactPath:  filepath.Join(storage.EvidenceDir(root, run.RunID), "missing.log"),
		Actor:         contracts.Actor("human:owner"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (EvidenceStore{Root: root}).SaveEvidence(ctx, evidenceMissing); err != nil {
		t.Fatalf("save missing evidence: %v", err)
	}
	evidenceInvalid := contracts.EvidenceItem{
		EvidenceID:    "evidence_invalid",
		RunID:         run.RunID,
		TicketID:      ticket.ID,
		Type:          contracts.EvidenceTypeArtifactRef,
		Title:         "invalid artifact",
		Body:          "bad path",
		ArtifactPath:  filepath.Join(root, "outside.log"),
		Actor:         contracts.Actor("human:owner"),
		CreatedAt:     now.Add(time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (EvidenceStore{Root: root}).SaveEvidence(ctx, evidenceInvalid); err != nil {
		t.Fatalf("save invalid evidence: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storage.TrackerDir(root), "evidence", "run_orphan"), 0o755); err != nil {
		t.Fatalf("mkdir orphan evidence dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storage.TrackerDir(root), "runtime", "run_orphan"), 0o755); err != nil {
		t.Fatalf("mkdir orphan runtime dir: %v", err)
	}

	gate := contracts.GateSnapshot{
		GateID:        "gate_1",
		TicketID:      ticket.ID,
		RunID:         "run_missing",
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		RequiredRole:  contracts.AgentRoleReviewer,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (GateStore{Root: root}).SaveGate(ctx, gate); err != nil {
		t.Fatalf("save gate: %v", err)
	}

	handoff := contracts.HandoffPacket{
		HandoffID:         "handoff_1",
		SourceRunID:       run.RunID,
		TicketID:          ticket.ID,
		Actor:             contracts.Actor("human:owner"),
		StatusSummary:     "ready",
		EvidenceLinks:     []string{"evidence_missing", "evidence_absent"},
		SuggestedNextGate: contracts.GateKindReview,
		GeneratedAt:       now,
		SchemaVersion:     contracts.CurrentSchemaVersion,
	}
	if err := (HandoffStore{Root: root}).SaveHandoff(ctx, handoff); err != nil {
		t.Fatalf("save handoff: %v", err)
	}

	report, err := AuditOrchestration(ctx, root, ticketStore)
	if err != nil {
		t.Fatalf("audit orchestration: %v", err)
	}
	for _, code := range []string{
		"runtime_artifacts_partial",
		"worktree_missing",
		"evidence_artifact_missing",
		"evidence_artifact_invalid",
		"evidence_dir_orphaned",
		"runtime_dir_orphaned",
		"gate_run_missing",
		"gate_ticket_mismatch",
		"handoff_evidence_missing",
	} {
		if !stringSliceContains(report.IssueCodes, code) {
			t.Fatalf("expected issue code %s in %#v", code, report.IssueCodes)
		}
	}
	if report.TotalIssues() == 0 {
		t.Fatal("expected orchestration issues to be reported")
	}
}
