package service

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestPermissionsViewRequiresOwnerOverrideForProtectedTicket(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Protected work",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Protected:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SaveAgentProfile(ctx, contracts.AgentProfile{
		AgentID:      "builder-1",
		DisplayName:  "Builder One",
		Provider:     contracts.AgentProviderCodex,
		Enabled:      true,
		Capabilities: []string{"go"},
	}, contracts.Actor("human:owner"), "seed agent"); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	if _, err := actions.SavePermissionProfile(ctx, contracts.PermissionProfile{
		ProfileID:                    "owner-ops",
		DisplayName:                  "Owner Ops",
		WorkspaceDefault:             true,
		RequiresOwnerForSensitiveOps: true,
		SchemaVersion:                contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed profile"); err != nil {
		t.Fatalf("save permission profile: %v", err)
	}

	agentView, err := queries.PermissionsView(ctx, "ticket:"+ticket.ID, contracts.Actor("agent:builder-1"), contracts.PermissionActionDispatch)
	if err != nil {
		t.Fatalf("permissions view for agent: %v", err)
	}
	if len(agentView.Decisions) != 1 {
		t.Fatalf("expected one decision, got %#v", agentView.Decisions)
	}
	if agentView.Decisions[0].Allowed {
		t.Fatalf("expected protected dispatch to be blocked for non-owner: %#v", agentView.Decisions[0])
	}
	if !agentView.Decisions[0].RequiresOwnerOverride || agentView.Decisions[0].OverrideApplied {
		t.Fatalf("expected owner override requirement without apply, got %#v", agentView.Decisions[0])
	}
	if !slices.Contains(agentView.Decisions[0].ReasonCodes, "owner_override_required") {
		t.Fatalf("expected owner override reason, got %#v", agentView.Decisions[0].ReasonCodes)
	}

	ownerView, err := queries.PermissionsView(ctx, "ticket:"+ticket.ID, contracts.Actor("human:owner"), contracts.PermissionActionDispatch)
	if err != nil {
		t.Fatalf("permissions view for owner: %v", err)
	}
	if len(ownerView.Decisions) != 1 {
		t.Fatalf("expected one decision, got %#v", ownerView.Decisions)
	}
	if !ownerView.Decisions[0].Allowed || !ownerView.Decisions[0].RequiresOwnerOverride || !ownerView.Decisions[0].OverrideApplied {
		t.Fatalf("expected owner override to permit dispatch, got %#v", ownerView.Decisions[0])
	}
}

func TestDispatchRunBlockedByPermissionProfile(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Permission block",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SaveAgentProfile(ctx, contracts.AgentProfile{
		AgentID:      "builder-1",
		DisplayName:  "Builder One",
		Provider:     contracts.AgentProviderCodex,
		Enabled:      true,
		Capabilities: []string{"go"},
	}, contracts.Actor("human:owner"), "seed agent"); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	if _, err := actions.SavePermissionProfile(ctx, contracts.PermissionProfile{
		ProfileID:        "deny-dispatch",
		DisplayName:      "Deny Dispatch",
		WorkspaceDefault: true,
		DenyActions:      []contracts.PermissionAction{contracts.PermissionActionDispatch},
		SchemaVersion:    contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed profile"); err != nil {
		t.Fatalf("save permission profile: %v", err)
	}

	if _, err := actions.DispatchRun(ctx, ticket.ID, "builder-1", contracts.RunKindWork, contracts.Actor("human:owner"), "dispatch"); err == nil {
		t.Fatal("expected dispatch to be blocked")
	} else if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}

	report, err := queries.AgentEligibility(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("agent eligibility: %v", err)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("expected one eligibility entry, got %#v", report.Entries)
	}
	if report.Entries[0].Eligible {
		t.Fatalf("expected entry to be ineligible, got %#v", report.Entries[0])
	}
	if !slices.Contains(report.Entries[0].ReasonCodes, "permission_action_denied") {
		t.Fatalf("expected permission deny reason, got %#v", report.Entries[0].ReasonCodes)
	}
}

func TestBindPermissionProfileUpdatesProjectAndTicketTargets(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)

	project := contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Binding target",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SavePermissionProfile(ctx, contracts.PermissionProfile{
		ProfileID:     "scoped",
		DisplayName:   "Scoped",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed profile"); err != nil {
		t.Fatalf("save permission profile: %v", err)
	}

	if _, err := actions.BindPermissionProfile(ctx, "scoped", "project", "APP", contracts.Actor("human:owner"), "bind project"); err != nil {
		t.Fatalf("bind project: %v", err)
	}
	if _, err := actions.BindPermissionProfile(ctx, "scoped", "ticket", ticket.ID, contracts.Actor("human:owner"), "bind ticket"); err != nil {
		t.Fatalf("bind ticket: %v", err)
	}

	loadedProject, err := projectStore.GetProject(ctx, "APP")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if !slices.Contains(loadedProject.Defaults.PermissionProfiles, "scoped") {
		t.Fatalf("expected project defaults to include profile, got %#v", loadedProject.Defaults.PermissionProfiles)
	}
	loadedTicket, err := ticketStore.GetTicket(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	if !slices.Contains(loadedTicket.PermissionProfiles, "scoped") {
		t.Fatalf("expected ticket to include profile, got %#v", loadedTicket.PermissionProfiles)
	}

	if _, err := actions.UnbindPermissionProfile(ctx, "scoped", "project", "APP", contracts.Actor("human:owner"), "unbind project"); err != nil {
		t.Fatalf("unbind project: %v", err)
	}
	if _, err := actions.UnbindPermissionProfile(ctx, "scoped", "ticket", ticket.ID, contracts.Actor("human:owner"), "unbind ticket"); err != nil {
		t.Fatalf("unbind ticket: %v", err)
	}

	loadedProject, err = projectStore.GetProject(ctx, "APP")
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if slices.Contains(loadedProject.Defaults.PermissionProfiles, "scoped") {
		t.Fatalf("expected project defaults to drop profile, got %#v", loadedProject.Defaults.PermissionProfiles)
	}
	loadedTicket, err = ticketStore.GetTicket(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	if slices.Contains(loadedTicket.PermissionProfiles, "scoped") {
		t.Fatalf("expected ticket to drop profile, got %#v", loadedTicket.PermissionProfiles)
	}
}

func TestEvaluatePathRestrictionsRequiresKnownFilesAndMatchesGlobs(t *testing.T) {
	if reasons := evaluatePathRestrictions([]string{"src/**"}, nil, nil, false); !slices.Equal(reasons, []string{"unverifiable_path_scope"}) {
		t.Fatalf("expected unverifiable path scope, got %#v", reasons)
	}

	reasons := evaluatePathRestrictions([]string{"src/**"}, []string{"src/secrets/**"}, []string{"src/main.go", "src/secrets/key.txt"}, true)
	if !slices.Contains(reasons, "permission_forbidden_path") {
		t.Fatalf("expected forbidden path reason, got %#v", reasons)
	}
	if slices.Contains(reasons, "permission_path_not_allowed") {
		t.Fatalf("did not expect allow-path miss for src/** files, got %#v", reasons)
	}

	reasons = evaluatePathRestrictions([]string{"src/**"}, nil, []string{"cmd/tool.go"}, true)
	if !slices.Equal(reasons, []string{"permission_path_not_allowed"}) {
		t.Fatalf("expected allow-path miss, got %#v", reasons)
	}
}

func TestPermissionsViewUsesObservedFilesForTicketCompletion(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 19, 0, 0, 0, time.UTC)

	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Observed files",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SaveAgentProfile(ctx, contracts.AgentProfile{
		AgentID:      "builder-1",
		DisplayName:  "Builder One",
		Provider:     contracts.AgentProviderCodex,
		Enabled:      true,
		Capabilities: []string{"go"},
	}, contracts.Actor("human:owner"), "seed agent"); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	dispatch, err := actions.DispatchRun(ctx, ticket.ID, "builder-1", contracts.RunKindWork, contracts.Actor("human:owner"), "dispatch")
	if err != nil {
		t.Fatalf("dispatch run: %v", err)
	}
	writeFile(t, filepath.Join(dispatch.WorktreePath, "README.md"), "# atlas\n\nupdated\n")

	if _, err := actions.SavePermissionProfile(ctx, contracts.PermissionProfile{
		ProfileID:        "readme-only",
		DisplayName:      "README Only",
		WorkspaceDefault: true,
		AllowActions:     []contracts.PermissionAction{contracts.PermissionActionTicketComplete},
		AllowedPaths:     []string{"README.md"},
		SchemaVersion:    contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed profile"); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	view, err := queries.PermissionsView(ctx, "ticket:"+ticket.ID, contracts.Actor("human:owner"), contracts.PermissionActionTicketComplete)
	if err != nil {
		t.Fatalf("permissions view: %v", err)
	}
	if len(view.Decisions) != 1 {
		t.Fatalf("expected one decision, got %#v", view.Decisions)
	}
	if !view.Decisions[0].ChangedFilesKnown {
		t.Fatalf("expected ticket completion to observe changed files, got %#v", view.Decisions[0])
	}
	if !slices.Contains(view.Decisions[0].ChangedFiles, "README.md") {
		t.Fatalf("expected README.md in changed files, got %#v", view.Decisions[0].ChangedFiles)
	}
	if !view.Decisions[0].Allowed || slices.Contains(view.Decisions[0].ReasonCodes, "unverifiable_path_scope") {
		t.Fatalf("expected path-scoped ticket completion to stay allowed, got %#v", view.Decisions[0])
	}
}
