package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestModelLoadsDataAndSwitchesTabs(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 6, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Actor:    contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Board item",
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
	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	m, err := newModel(root, "")
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	msg := m.refresh()().(loadedMsg)
	updated, _ := m.Update(msg)
	m = updated.(model)
	view := m.View()
	if !strings.Contains(view, "Board") || !strings.Contains(view, "APP-1") {
		t.Fatalf("unexpected view: %s", view)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.screen != screenQueues {
		t.Fatalf("expected queue screen, got %v", m.screen)
	}
}

func TestCursorClampsAcrossScreenSizes(t *testing.T) {
	m := model{
		screen: screenOwner,
		cursor: 7,
		owner: service.QueueView{
			Categories: map[service.QueueCategory][]service.QueueEntry{
				service.QueueAwaitingOwner: {
					{Ticket: contracts.TicketSnapshot{ID: "APP-1"}},
					{Ticket: contracts.TicketSnapshot{ID: "APP-2"}},
				},
			},
		},
	}

	m = m.moveCursor(-1)
	if m.cursor != 0 {
		t.Fatalf("expected cursor to clamp to first valid index, got %d", m.cursor)
	}

	m.cursor = 9
	m = m.moveCursor(1)
	if m.cursor != 1 {
		t.Fatalf("expected cursor to clamp to last valid index, got %d", m.cursor)
	}
}

func TestTicketsListViewUsesNarrowWidth(t *testing.T) {
	out := ticketsListView("Board", []contracts.TicketSnapshot{{
		ID:       "APP-1",
		Status:   contracts.StatusReady,
		Priority: contracts.PriorityHigh,
		Title:    "A very long title for a narrow terminal",
	}}, 0, 36)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "> ") && lipgloss.Width(line) > 36 {
			t.Fatalf("expected selected line to fit narrow width, got width=%d line=%q", lipgloss.Width(line), line)
		}
	}
	if !strings.Contains(out, "...") {
		t.Fatalf("expected narrow list view to truncate long title, got %s", out)
	}
}

func TestModelViewTruncatesFooterAtWidth(t *testing.T) {
	m := model{
		screen:             screenBoard,
		width:              32,
		actor:              contracts.Actor("human:owner"),
		collaboratorFilter: "all",
		status:             "a very long status message that should not push the footer past the terminal",
	}
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "actor: ") && lipgloss.Width(line) > m.width {
			t.Fatalf("expected footer to fit width=%d, got width=%d line=%q", m.width, lipgloss.Width(line), line)
		}
	}
}

func TestPaletteMutationMovesTicket(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	msg := m.refresh()().(loadedMsg)
	updated, _ := m.Update(msg)
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(model)
	m.dialog.Input.SetValue("/ticket move APP-1 in_progress --actor agent:builder-1")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected mutation command from palette submit")
	}
	updated, _ = m.Update(cmd().(loadedMsg))
	m = updated.(model)
	if m.detail.Ticket.Status != contracts.StatusInProgress {
		t.Fatalf("expected in_progress, got %#v", m.detail.Ticket)
	}
}

func TestCreateFormCreatesTicket(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(model)
	for _, field := range []string{"APP", "New ticket", "task", "Short desc"} {
		m.dialog.Fields[m.dialog.Focus].Input.SetValue("")
		for _, ch := range field {
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
			m = updated.(model)
		}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(model)
		if cmd != nil {
			updated, _ = m.Update(cmd().(loadedMsg))
			m = updated.(model)
		}
	}
	if m.detail.Ticket.ID != "APP-2" {
		t.Fatalf("expected APP-2 detail after create, got %#v", m.detail.Ticket)
	}
	if m.detail.Ticket.Title != "New ticket" {
		t.Fatalf("unexpected created ticket: %#v", m.detail.Ticket)
	}
}

func TestViewsTabRunsSavedView(t *testing.T) {
	root := seededTUIWorkspace(t)
	if err := (service.ViewStore{Root: root}).SaveView(contracts.SavedView{Name: "ready-search", Kind: contracts.SavedViewKindSearch, Query: "status=ready"}); err != nil {
		t.Fatalf("save view: %v", err)
	}
	m, err := newModel(root, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)
	for m.screen != screenViews {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(model)
	}
	if !strings.Contains(m.View(), "ready-search") {
		t.Fatalf("expected views panel to include saved view, got %s", m.View())
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected saved view load command")
	}
	updated, _ = m.Update(cmd().(loadedMsg))
	m = updated.(model)
	if m.screen != screenSearch {
		t.Fatalf("expected search screen after loading saved view, got %v", m.screen)
	}
	if len(m.searchHits) == 0 || m.searchHits[0].ID != "APP-1" {
		t.Fatalf("expected saved view search hits, got %#v", m.searchHits)
	}
}

func TestBulkPreviewAndApplyFromTUI(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updated.(model)
	m.dialog.Input.SetValue("move in_progress")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected bulk preview command")
	}
	updated, _ = m.Update(cmd().(bulkMsg))
	m = updated.(model)
	if m.screen != screenOps {
		t.Fatalf("expected ops screen after bulk preview, got %v", m.screen)
	}
	if m.lastBulk == nil || m.lastBulk.Summary.Skipped != 1 {
		t.Fatalf("expected dry-run summary on ops screen, got %#v", m.lastBulk)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected bulk apply command")
	}
	updated, cmd = m.Update(cmd().(bulkMsg))
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected reload after bulk apply")
	}
	updated, _ = m.Update(cmd().(loadedMsg))
	m = updated.(model)
	if m.detail.Ticket.Status != contracts.StatusInProgress {
		t.Fatalf("expected selected ticket to move in_progress, got %#v", m.detail.Ticket)
	}
}

func TestDetailViewIncludesGitContext(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)
	m.screen = screenDetail
	view := m.View()
	if !strings.Contains(view, "Git Context:") {
		t.Fatalf("expected git context in detail view, got %s", view)
	}
	if !strings.Contains(view, "Runs:") || !strings.Contains(view, "Evidence:") || !strings.Contains(view, "Handoffs:") || !strings.Contains(view, "Runtime:") {
		t.Fatalf("expected orchestration panels in detail view, got %s", view)
	}
	if !strings.Contains(view, "Timeline:") || !strings.Contains(view, "ticket.created") {
		t.Fatalf("expected timeline panel in detail view, got %s", view)
	}
}

func TestInboxViewShowsApprovalsAndHumanInboxPanels(t *testing.T) {
	root := seededTUIWorkspace(t)
	now := time.Date(2026, 3, 23, 11, 0, 0, 0, time.UTC)
	if err := (service.GateStore{Root: root}).SaveGate(context.Background(), contracts.GateSnapshot{
		GateID:          "gate_1",
		TicketID:        "APP-1",
		Kind:            contracts.GateKindReview,
		State:           contracts.GateStateOpen,
		RequiredRole:    contracts.AgentRoleReviewer,
		RequiredAgentID: "agent:reviewer-1",
		CreatedBy:       contracts.Actor("human:owner"),
		CreatedAt:       now,
		SchemaVersion:   contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)
	m.screen = screenInbox
	view := m.View()
	if !strings.Contains(view, "Approvals:") || !strings.Contains(view, "Human Inbox:") {
		t.Fatalf("expected approvals and inbox panels, got %s", view)
	}
	if !strings.Contains(view, "gate_1") {
		t.Fatalf("expected open gate to appear in inbox view, got %s", view)
	}
}

func TestOpsViewShowsAgentsDispatchAndWorktreesPanels(t *testing.T) {
	root := seededTUIWorkspace(t)
	if err := (service.AgentStore{Root: root}).SaveAgent(context.Background(), contracts.AgentProfile{
		AgentID:       "builder-1",
		DisplayName:   "Builder One",
		Provider:      contracts.AgentProviderCodex,
		Enabled:       true,
		Capabilities:  []string{"go"},
		MaxActiveRuns: 1,
	}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)
	m.screen = screenOps
	view := m.View()
	if !strings.Contains(view, "Dashboard:") || !strings.Contains(view, "Agents:") || !strings.Contains(view, "Dispatch Queue:") || !strings.Contains(view, "Worktrees:") {
		t.Fatalf("expected ops panels in view, got %s", view)
	}
	if !strings.Contains(view, "active_runs:") || !strings.Contains(view, "builder-1") || !strings.Contains(view, "APP-1") {
		t.Fatalf("expected populated agent/dispatch content, got %s", view)
	}
}

func TestOpsViewShowsCollaborationConsole(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()

	ctx := service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: contracts.EventSurfaceTUI})
	actor, err := m.queries.ResolveActor(ctx, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if _, err := m.actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Reviewer One",
		AtlasActors:    []contracts.Actor{contracts.Actor("agent:reviewer-1")},
		SchemaVersion:  contracts.CurrentSchemaVersion,
	}, actor, "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := m.actions.SetCollaboratorTrust(ctx, "rev-1", true, actor, "trust collaborator"); err != nil {
		t.Fatalf("trust collaborator: %v", err)
	}
	if _, err := m.actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "rev-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleReviewer,
	}, actor, "bind reviewer"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}
	if err := m.actions.CommentTicket(ctx, "APP-1", "ping @rev-1", contracts.Actor("agent:builder-1"), "mention reviewer"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}
	if err := (service.GateStore{Root: root}).SaveGate(context.Background(), contracts.GateSnapshot{
		GateID:        "gate_review_1",
		TicketID:      "APP-1",
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		RequiredRole:  contracts.AgentRoleReviewer,
		CreatedBy:     actor,
		CreatedAt:     time.Date(2026, 3, 23, 10, 2, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	if err := (service.SyncRemoteStore{Root: root}).SaveSyncRemote(context.Background(), contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      filepath.Join(root, "remote"),
		Enabled:       true,
		DefaultAction: contracts.SyncDefaultActionPull,
		CreatedAt:     time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save sync remote: %v", err)
	}
	if err := (service.SyncJobStore{Root: root}).SaveSyncJob(context.Background(), contracts.SyncJob{
		JobID:         "sync_pull_1",
		RemoteID:      "origin",
		Mode:          contracts.SyncJobModePull,
		State:         contracts.SyncJobStateFailed,
		StartedAt:     time.Date(2026, 3, 23, 10, 3, 0, 0, time.UTC),
		FinishedAt:    time.Date(2026, 3, 23, 10, 4, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save sync job: %v", err)
	}
	if err := (service.ConflictStore{Root: root}).SaveConflict(context.Background(), contracts.ConflictRecord{
		ConflictID:    "conflict_ticket_1",
		EntityKind:    "ticket",
		EntityUID:     contracts.TicketUID("APP", "APP-1"),
		ConflictType:  contracts.ConflictTypeScalarDivergence,
		Status:        contracts.ConflictStatusOpen,
		OpenedByJob:   "sync_pull_1",
		OpenedAt:      time.Date(2026, 3, 23, 10, 5, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save conflict: %v", err)
	}

	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(model)
	m.dialog.Input.SetValue("rev-1")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected collaborator filter reload")
	}
	updated, _ = m.Update(cmd().(loadedMsg))
	m = updated.(model)
	m.screen = screenOps
	view := m.View()
	for _, needle := range []string{"collaborator: rev-1", "Remote Health:", "Conflict Queue:", "Mention Queue:"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected %q in ops view, got %s", needle, view)
		}
	}
}

func TestPaletteRunLaunchWritesRuntimeArtifacts(t *testing.T) {
	root, runID := seededTUIRunWorkspace(t)
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(model)
	m.dialog.Input.SetValue("/run launch " + runID)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected launch command from palette submit")
	}
	updated, _ = m.Update(cmd().(loadedMsg))
	m = updated.(model)
	if _, err := os.Stat(filepath.Join(root, ".tracker", "runtime", runID, "brief.md")); err != nil {
		t.Fatalf("expected runtime brief to exist: %v", err)
	}
	if !strings.Contains(m.status, "launched runtime artifacts") {
		t.Fatalf("expected launch status, got %s", m.status)
	}
}

func seededTUIWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	seededTUIWorkspaceAt(root, func(format string, args ...any) {
		t.Fatalf(format, args...)
	})
	return root
}

func seededTUIRunWorkspace(t *testing.T) (string, string) {
	t.Helper()
	root := seededTUIWorkspace(t)
	gitMustRun(t, root, "init", "-b", "main")
	gitMustRun(t, root, "config", "user.email", "atlas@example.com")
	gitMustRun(t, root, "config", "user.name", "Atlas")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# atlas\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	gitMustRun(t, root, "add", "README.md")
	gitMustRun(t, root, "commit", "-m", "init")

	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	ctx := service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: contracts.EventSurfaceTUI})
	actor, err := m.queries.ResolveActor(ctx, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if _, err := m.actions.SaveAgentProfile(ctx, contracts.AgentProfile{
		AgentID:       "builder-1",
		DisplayName:   "Builder One",
		Provider:      contracts.AgentProviderCodex,
		Enabled:       true,
		Capabilities:  []string{"go"},
		MaxActiveRuns: 1,
	}, actor, "seed agent"); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	result, err := m.actions.DispatchRun(ctx, "APP-1", "builder-1", contracts.RunKindWork, actor, "seed run")
	if err != nil {
		t.Fatalf("dispatch run: %v", err)
	}
	return root, result.RunID
}

func seededTUIWorkspaceAt(root string, fail func(string, ...any)) {
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		fail("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}, Actor: contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")}}); err != nil {
		fail("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		fail("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Seed", Summary: "Seed", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		fail("create ticket: %v", err)
	}
	event := contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		fail("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, event); err != nil {
		fail("apply event: %v", err)
	}
}

func gitMustRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
