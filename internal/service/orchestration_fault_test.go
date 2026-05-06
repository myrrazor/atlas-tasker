package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestDispatchRunConcurrentSingleWinner(t *testing.T) {
	root, ctx, _, ticketStore, actions := setupRunActionTest(t)
	seedTicketAndAgent(t, ctx, root, ticketStore, actions)

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := actions.DispatchRun(ctx, "APP-1", "builder-1", contracts.RunKindWork, contracts.Actor("human:owner"), "dispatch")
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if strings.Contains(err.Error(), "already has active run") || strings.Contains(err.Error(), "parallel_runs_disabled") || strings.Contains(err.Error(), "agent_at_capacity") {
			conflicts++
			continue
		}
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one dispatch success and one conflict, got successes=%d conflicts=%d", successes, conflicts)
	}

	runs, err := (RunStore{Root: root}).ListRuns(ctx, "APP-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one persisted run, got %#v", runs)
	}
}

func TestDispatchRunRollsBackWhenInterruptedBeforeWorktreeCreate(t *testing.T) {
	root, ctx, _, ticketStore, actions := setupRunActionTest(t)
	seedTicketAndAgent(t, ctx, root, ticketStore, actions)

	testBeforeRunWorktreeCreateHook = func(contracts.RunSnapshot) error {
		return errors.New("dispatch interrupted")
	}
	defer func() { testBeforeRunWorktreeCreateHook = nil }()

	if _, err := actions.DispatchRun(ctx, "APP-1", "builder-1", contracts.RunKindWork, contracts.Actor("human:owner"), "dispatch"); err == nil || !strings.Contains(err.Error(), "dispatch interrupted") {
		t.Fatalf("expected interrupted dispatch error, got %v", err)
	}

	runs, err := (RunStore{Root: root}).ListRuns(ctx, "APP-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected rollback to remove persisted run, got %#v", runs)
	}
	ticket, err := ticketStore.GetTicket(ctx, "APP-1")
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	if ticket.LatestRunID != "" || !ticket.LastDispatchAt.IsZero() {
		t.Fatalf("expected ticket dispatch metadata rollback, got %#v", ticket)
	}
}

func TestWriteRuntimeArtifactsRollsBackOnInterruptedWrite(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, ".tracker", "runtime", "run_1")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	files := map[string]string{
		filepath.Join(runtimeDir, "brief.md"):          "brief",
		filepath.Join(runtimeDir, "context.json"):      "{}",
		filepath.Join(runtimeDir, "launch.codex.txt"):  "codex",
		filepath.Join(runtimeDir, "launch.claude.txt"): "claude",
	}

	calls := 0
	testRuntimeArtifactWriteHook = func(path string) error {
		calls++
		if calls == 2 {
			return fmt.Errorf("write interrupted for %s", filepath.Base(path))
		}
		return nil
	}
	defer func() { testRuntimeArtifactWriteHook = nil }()

	if _, _, err := writeRuntimeArtifacts(false, files); err == nil || !strings.Contains(err.Error(), "write interrupted") {
		t.Fatalf("expected interrupted runtime write, got %v", err)
	}
	for path := range files {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected runtime rollback to leave %s absent, err=%v", path, err)
		}
	}
}

func TestAddEvidenceCleansUpPartialArtifactOnCopyFailure(t *testing.T) {
	root, ctx, _, ticketStore, actions := setupRunActionTest(t)
	seedTicketAndAgent(t, ctx, root, ticketStore, actions)

	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_1",
		TicketID:      "APP-1",
		Project:       "APP",
		AgentID:       "builder-1",
		Provider:      contracts.AgentProviderCodex,
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     time.Date(2026, 3, 25, 15, 0, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	source := filepath.Join(root, "proof.log")
	if err := os.WriteFile(source, []byte(strings.Repeat("x", 1024)), 0o644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	testEvidenceArtifactCopyHook = func(_ string, copied int64) error {
		if copied > 0 {
			return errors.New("copy interrupted")
		}
		return nil
	}
	defer func() { testEvidenceArtifactCopyHook = nil }()

	if _, err := actions.AddEvidence(ctx, run.RunID, contracts.EvidenceTypeTestResult, "proof", "copied", source, "", contracts.Actor("human:owner"), "attach", contracts.EventRunEvidenceAdded); err == nil || !strings.Contains(err.Error(), "copy interrupted") {
		t.Fatalf("expected interrupted artifact copy, got %v", err)
	}

	items, err := (EvidenceStore{Root: root}).ListEvidence(ctx, run.RunID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected failed add to avoid evidence records, got %#v", items)
	}
	entries, err := os.ReadDir(storage.EvidenceDir(root, run.RunID))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read evidence dir: %v", err)
	}
	for _, entry := range entries {
		t.Fatalf("expected failed add to leave no copied artifacts, found %s", entry.Name())
	}
}

func TestCompleteRunConcurrentAttemptsFailWhileGateIsOpen(t *testing.T) {
	root, ctx, _, ticketStore, actions := setupRunActionTest(t)
	seedTicketAndAgent(t, ctx, root, ticketStore, actions)

	ticket, err := ticketStore.GetTicket(ctx, "APP-1")
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	ticket.OpenGateIDs = []string{"gate_1"}
	if err := ticketStore.UpdateTicket(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_1",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		AgentID:       "builder-1",
		Provider:      contracts.AgentProviderCodex,
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     time.Date(2026, 3, 25, 16, 0, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	gate := contracts.GateSnapshot{
		GateID:        "gate_1",
		TicketID:      ticket.ID,
		RunID:         run.RunID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		RequiredRole:  contracts.AgentRoleReviewer,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     run.CreatedAt,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (GateStore{Root: root}).SaveGate(ctx, gate); err != nil {
		t.Fatalf("save gate: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := actions.CompleteRun(ctx, run.RunID, contracts.Actor("human:owner"), "done", "ship it")
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	for err := range results {
		if err == nil || !strings.Contains(err.Error(), "cannot complete while gates are open") {
			t.Fatalf("expected gate conflict, got %v", err)
		}
	}
	persisted, err := (RunStore{Root: root}).LoadRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if persisted.Status != contracts.RunStatusActive {
		t.Fatalf("expected run to remain active, got %#v", persisted)
	}
}

func setupRunActionTest(t *testing.T) (string, context.Context, mdstore.ProjectStore, mdstore.TicketStore, *ActionService) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	return root, ctx, projectStore, ticketStore, actions
}

func seedTicketAndAgent(t *testing.T, ctx context.Context, root string, ticketStore mdstore.TicketStore, actions *ActionService) {
	t.Helper()
	now := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)
	if err := actions.Projects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Dispatch me",
		Summary:       "Dispatch me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := (AgentStore{Root: root}).SaveAgent(ctx, contracts.AgentProfile{
		AgentID:       "builder-1",
		DisplayName:   "Builder One",
		Provider:      contracts.AgentProviderCodex,
		Enabled:       true,
		Capabilities:  []string{"go"},
		MaxActiveRuns: 1,
	}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	if err := actions.AppendAndProject(ctx, contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("append ticket create event: %v", err)
	}
}
