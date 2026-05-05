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

func TestAuditOrchestrationReportsCorruptDocsWithoutFailing(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Doctor corruption",
		Summary:       "Doctor corruption",
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
	validRun := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_valid",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := (RunStore{Root: root}).SaveRun(ctx, validRun); err != nil {
		t.Fatalf("save valid run: %v", err)
	}
	if err := os.MkdirAll(storage.EvidenceDir(root, validRun.RunID), 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	for _, item := range []struct {
		path string
		body string
	}{
		{storage.RunFile(root, "run_broken"), "---\nrun_id: run_broken\nstatus: [\n"},
		{storage.GateFile(root, "gate_broken"), "---\ngate_id: gate_broken\nkind: review\nstate: [\n"},
		{storage.HandoffFile(root, "handoff_broken"), "---\nhandoff_id: handoff_broken\nsource_run_id: run_valid\n"},
		{storage.EvidenceFile(root, validRun.RunID, "evidence_broken"), "---\nevidence_id: evidence_broken\nrun_id: run_valid\ntype: [\n"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", item.path, err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.path, err)
		}
	}

	report, err := AuditOrchestration(ctx, root, ticketStore)
	if err != nil {
		t.Fatalf("audit orchestration: %v", err)
	}
	for _, code := range []string{"run_doc_corrupt", "gate_doc_corrupt", "handoff_doc_corrupt", "evidence_doc_corrupt"} {
		if !stringSliceContains(report.IssueCodes, code) {
			t.Fatalf("expected issue code %s in %#v", code, report.IssueCodes)
		}
	}

	actions, err := RepairOrchestration(ctx, root)
	if err != nil {
		t.Fatalf("repair orchestration: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected no worktree repair actions without tracked worktrees, got %#v", actions)
	}
}

func TestAuditOrchestrationAcceptsCompactedRuntimeWithoutLaunchFiles(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Compacted runtime",
		Summary:       "Compacted runtime",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusDone,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_compacted",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		Status:        contracts.RunStatusCompleted,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		CompletedAt:   now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, item := range []struct {
		path string
		body string
	}{
		{storage.RuntimeBriefFile(root, run.RunID), "brief"},
		{storage.RuntimeContextFile(root, run.RunID), "{}"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", item.path, err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.path, err)
		}
	}

	report, err := AuditOrchestration(ctx, root, ticketStore)
	if err != nil {
		t.Fatalf("audit orchestration: %v", err)
	}
	if stringSliceContains(report.IssueCodes, "runtime_artifacts_partial") {
		t.Fatalf("expected compacted runtime launch files to be acceptable, got %#v", report.IssueCodes)
	}
}

func TestAuditOrchestrationReportsV16CollaborationAndSyncIssues(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Doctor v1.6.1",
		Summary:       "Doctor v1.6.1",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := os.Stat(storage.CollaboratorsDir(root)); os.IsNotExist(err) {
		if err := os.MkdirAll(storage.CollaboratorsDir(root), 0o755); err != nil {
			t.Fatalf("mkdir collaborators dir: %v", err)
		}
	}
	if err := os.WriteFile(storage.CollaboratorFile(root, "broken"), []byte("---\ncollaborator_id: broken\nstatus: [\n"), 0o644); err != nil {
		t.Fatalf("write broken collaborator: %v", err)
	}
	if err := (CollaboratorStore{Root: root}).SaveCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Reviewer One",
		Status:         contracts.CollaboratorStatusActive,
		TrustState:     contracts.CollaboratorTrustStateTrusted,
		CreatedAt:      now,
		UpdatedAt:      now,
		SchemaVersion:  contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save collaborator: %v", err)
	}
	if err := (MembershipStore{Root: root}).SaveMembership(ctx, contracts.MembershipBinding{
		MembershipUID:  "membership_bad",
		CollaboratorID: "ghost",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "NOPE",
		Role:           contracts.MembershipRoleReviewer,
		Status:         contracts.MembershipStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("save bad membership: %v", err)
	}
	if err := (MentionStore{Root: root}).SaveMention(ctx, contracts.Mention{
		MentionUID:        "mention_bad",
		CollaboratorID:    "ghost",
		SourceKind:        "ticket_comment",
		SourceID:          "comment_1",
		SourceEventUID:    "event_1",
		TicketID:          "APP-99",
		OriginWorkspaceID: "ws-1",
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("save bad mention: %v", err)
	}
	if err := (SyncRemoteStore{Root: root}).SaveSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      filepath.Join(root, ".tracker", "sync"),
		Enabled:       true,
		DefaultAction: contracts.SyncDefaultActionPull,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("save bad sync remote: %v", err)
	}
	if err := (SyncJobStore{Root: root}).SaveSyncJob(ctx, contracts.SyncJob{
		JobID:         "sync_pull_1",
		RemoteID:      "missing-remote",
		Mode:          contracts.SyncJobModePull,
		State:         contracts.SyncJobStateFailed,
		StartedAt:     now,
		FinishedAt:    now.Add(time.Minute),
		ConflictIDs:   []string{"conflict_missing"},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save bad sync job: %v", err)
	}
	if err := (ConflictStore{Root: root}).SaveConflict(ctx, contracts.ConflictRecord{
		ConflictID:    "conflict_1",
		EntityKind:    "ticket",
		EntityUID:     contracts.TicketUID("APP", "APP-1"),
		ConflictType:  contracts.ConflictTypeScalarDivergence,
		LocalRef:      filepath.Join(root, "missing-local.json"),
		RemoteRef:     filepath.Join(root, "missing-remote.json"),
		Status:        contracts.ConflictStatusOpen,
		OpenedByJob:   "missing-job",
		OpenedAt:      now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save bad conflict: %v", err)
	}

	report, err := AuditOrchestration(ctx, root, ticketStore)
	if err != nil {
		t.Fatalf("audit orchestration: %v", err)
	}
	for _, code := range []string{
		"collaborator_doc_corrupt",
		"membership_collaborator_missing",
		"membership_scope_missing",
		"mention_collaborator_missing",
		"mention_ticket_missing",
		"sync_remote_invalid",
		"sync_job_remote_missing",
		"sync_job_conflict_missing",
		"conflict_job_missing",
		"conflict_snapshot_missing",
	} {
		if !stringSliceContains(report.IssueCodes, code) {
			t.Fatalf("expected issue code %s in %#v", code, report.IssueCodes)
		}
	}
	if report.CollaboratorIssues == 0 || report.MembershipIssues == 0 || report.MentionIssues == 0 || report.SyncIssues == 0 || report.ConflictIssues == 0 {
		t.Fatalf("expected v1.6 issue buckets to be populated, got %#v", report)
	}
}
