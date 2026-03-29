package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func BenchmarkQueryQueue(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = root
	_ = projection
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.Queue(ctx, contracts.Actor("agent:builder-1")); err != nil {
			b.Fatalf("queue: %v", err)
		}
	}
}

func BenchmarkTicketDetail(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = root
	_ = projection
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.TicketDetail(ctx, "APP-12"); err != nil {
			b.Fatalf("detail: %v", err)
		}
	}
}

func BenchmarkMoveTicketMutation(b *testing.B) {
	_, _, actions, projection := benchmarkQueryService(b)
	_ = projection
	ctx := context.Background()
	status := contracts.StatusInProgress
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := actions.MoveTicket(ctx, "APP-1", status, contracts.Actor("human:owner"), "bench"); err != nil {
			b.Fatalf("move ticket: %v", err)
		}
		if status == contracts.StatusInProgress {
			status = contracts.StatusReady
		} else {
			status = contracts.StatusInProgress
		}
	}
}

func BenchmarkProjectionRebuild(b *testing.B) {
	_, _, _, projection := benchmarkQueryService(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := projection.Rebuild(ctx, ""); err != nil {
			b.Fatalf("rebuild projection: %v", err)
		}
	}
}

func BenchmarkAutomationExplain(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = projection
	store := AutomationStore{Root: root}
	rule := contracts.AutomationRule{
		Name:       "ready-review",
		Enabled:    true,
		Trigger:    contracts.AutomationTrigger{EventTypes: []contracts.EventType{contracts.EventTicketUpdated}},
		Conditions: contracts.AutomationCondition{Status: contracts.StatusReady},
		Actions:    []contracts.AutomationAction{{Kind: contracts.AutomationActionNotify, Message: "ready"}},
	}
	if err := store.SaveRule(rule); err != nil {
		b.Fatalf("save rule: %v", err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.ExplainAutomationRules(ctx, "APP-1"); err != nil {
			b.Fatalf("explain rules: %v", err)
		}
	}
}

func BenchmarkDispatchSuggest(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = root
	_ = projection
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.DispatchSuggest(ctx, "APP-1"); err != nil {
			b.Fatalf("dispatch suggest: %v", err)
		}
	}
}

func BenchmarkRunDetail(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = root
	_ = projection
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.RunDetail(ctx, "run_bench"); err != nil {
			b.Fatalf("run detail: %v", err)
		}
	}
}

func BenchmarkApprovals(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = root
	_ = projection
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.Approvals(ctx, ""); err != nil {
			b.Fatalf("approvals: %v", err)
		}
	}
}

func BenchmarkWorktreeList(b *testing.B) {
	root, queries, _, projection := benchmarkQueryService(b)
	_ = root
	_ = projection
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := queries.WorktreeList(ctx); err != nil {
			b.Fatalf("worktree list: %v", err)
		}
	}
}

func benchmarkQueryService(b *testing.B) (string, *QueryService, *ActionService, *sqlitestore.Store) {
	b.Helper()
	root := b.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 14, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	b.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}, Actor: contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")}}); err != nil {
		b.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		b.Fatalf("create project: %v", err)
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	for i := 1; i <= 24; i++ {
		status := contracts.StatusReady
		if i%3 == 0 {
			status = contracts.StatusInProgress
		}
		if i%5 == 0 {
			status = contracts.StatusInReview
		}
		ticket := contracts.TicketSnapshot{
			Project:       "APP",
			Title:         "Bench ticket",
			Type:          contracts.TicketTypeTask,
			Status:        status,
			Priority:      contracts.PriorityHigh,
			Reviewer:      contracts.Actor("agent:reviewer-1"),
			CreatedAt:     now,
			UpdatedAt:     now.Add(time.Duration(i) * time.Minute),
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if _, err := actions.CreateTrackedTicket(ctx, ticket, contracts.Actor("human:owner"), "seed"); err != nil {
			b.Fatalf("create tracked ticket %d: %v", i, err)
		}
	}
	if err := (AgentStore{Root: root}).SaveAgent(ctx, contracts.AgentProfile{
		AgentID:       "builder-1",
		DisplayName:   "Builder One",
		Provider:      contracts.AgentProviderCodex,
		Enabled:       true,
		Capabilities:  []string{"go"},
		MaxActiveRuns: 2,
	}); err != nil {
		b.Fatalf("save agent: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:          "run_bench",
		TicketID:       "APP-1",
		Project:        "APP",
		AgentID:        "builder-1",
		Provider:       contracts.AgentProviderCodex,
		Status:         contracts.RunStatusAwaitingReview,
		Kind:           contracts.RunKindWork,
		BlueprintStage: "implement",
		WorktreePath:   filepath.Join(root, "bench-worktree"),
		CreatedAt:      now,
	})
	if err := os.MkdirAll(run.WorktreePath, 0o755); err != nil {
		b.Fatalf("mkdir worktree: %v", err)
	}
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		b.Fatalf("save run: %v", err)
	}
	if err := (GateStore{Root: root}).SaveGate(ctx, contracts.GateSnapshot{
		GateID:          "gate_bench",
		TicketID:        "APP-1",
		RunID:           run.RunID,
		Kind:            contracts.GateKindReview,
		State:           contracts.GateStateOpen,
		RequiredRole:    contracts.AgentRoleReviewer,
		RequiredAgentID: "agent:reviewer-1",
		CreatedBy:       contracts.Actor("human:owner"),
		CreatedAt:       now,
		SchemaVersion:   contracts.CurrentSchemaVersion,
	}); err != nil {
		b.Fatalf("save gate: %v", err)
	}
	return root, NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }), actions, projection
}
