package mcp

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type contextPayload struct {
	WorkspaceID     string                      `json:"workspace_id,omitempty"`
	WorkspaceRoot   string                      `json:"workspace_root,omitempty"`
	Projects        []projectRef                `json:"projects"`
	Actor           string                      `json:"actor,omitempty"`
	ActorSource     string                      `json:"actor_source,omitempty"`
	ActorConfigured bool                        `json:"actor_configured"`
	SetupErrors     []string                    `json:"setup_errors,omitempty"`
	ManagedMode     service.ManagedModeView     `json:"managed_mode"`
	AssignedActive  []service.TicketRef         `json:"assigned_active"`
	AvailableWork   []service.TicketRef         `json:"available_work"`
	PendingBlockers []service.TicketRef         `json:"pending_blockers"`
	ActiveRuns      []runRef                    `json:"active_runs"`
	BackupHealth    service.BackupHealthSummary `json:"backup_health"`
	StateRevision   map[string]any              `json:"state_revision"`
	Pagination      map[string]any              `json:"pagination"`
	Markdown        string                      `json:"markdown"`
}

func contextTool(tc ToolContext, args map[string]any) (any, error) {
	actor := resolveManagedActor(tc, args)
	if err := invalidActorError(actor); err != nil {
		return nil, err
	}
	project := stringArg(args, "project")
	projects, err := listProjectRefs(tc)
	if err != nil {
		return nil, err
	}
	var setupErrors []string
	if actor.SetupError != "" {
		setupErrors = append(setupErrors, actor.SetupError)
	}
	if project != "" {
		if _, keys, ok := resolveNamedProject(projects, project); !ok {
			return nil, apperr.New(apperr.CodeNotFound, fmt.Sprintf("unknown project %q; known projects: %s", project, strings.Join(keys, ", ")))
		}
	}
	workspaceID, err := loadWorkspaceIdentity(tc)
	if err != nil {
		return nil, err
	}
	mode, err := tc.Server.Workspace.Queries.ManagedModeView(deliveryEnabledLocally(tc.Server.Options))
	if err != nil {
		return nil, err
	}
	limit := tc.Server.Options.MaxItems
	assigned, assignedTotal, err := assignedActiveTickets(tc, actor.Actor, project, limit)
	if err != nil {
		return nil, err
	}
	var available, pending []service.TicketRef
	var availableTotal, pendingTotal int
	if actor.Actor != "" {
		work, workErr := tc.Server.Workspace.Queries.AgentWork(tc.Context, actor.Actor)
		if workErr != nil {
			return nil, workErr
		}
		available, availableTotal = compactWork(work.Available, project, limit)
		pending, pendingTotal = compactWork(work.Pending, project, limit)
	}
	runs, runTotal, err := compactActiveRuns(tc, project, "", limit)
	if err != nil {
		return nil, err
	}
	backup, err := tc.Server.Workspace.Queries.BackupHealth(tc.Context)
	if err != nil {
		return nil, err
	}
	heads, err := eventHeads(tc, projects)
	if err != nil {
		return nil, err
	}
	payload := contextPayload{
		WorkspaceID:     workspaceID,
		Projects:        projects,
		Actor:           string(actor.Actor),
		ActorSource:     actor.Source,
		ActorConfigured: actor.Configured,
		SetupErrors:     setupErrors,
		ManagedMode:     mode,
		AssignedActive:  assigned,
		AvailableWork:   available,
		PendingBlockers: pending,
		ActiveRuns:      runs,
		BackupHealth:    backup,
		StateRevision: map[string]any{
			"workspace_id": workspaceID,
			"event_heads":  heads,
			"generated_at": generatedAt(tc),
		},
		Pagination: map[string]any{
			"projects":         pageMeta(len(projects), len(projects), ""),
			"assigned_active":  pageMeta(assignedTotal, len(assigned), ""),
			"available_work":   pageMeta(availableTotal, len(available), ""),
			"pending_blockers": pageMeta(pendingTotal, len(pending), ""),
			"active_runs":      pageMeta(runTotal, len(runs), ""),
		},
	}
	if tc.Server.Options.IncludeLocalOnlyPaths {
		payload.WorkspaceRoot = tc.Server.Workspace.Root
	}
	payload.Markdown = contextMarkdown(payload)
	return payload, nil
}

func contextMarkdown(payload contextPayload) string {
	var b strings.Builder
	b.WriteString("# Atlas context\n")
	if payload.WorkspaceID != "" {
		b.WriteString("Workspace `")
		b.WriteString(payload.WorkspaceID)
		b.WriteString("`.\n")
	}
	if len(payload.SetupErrors) > 0 {
		b.WriteString("\nSetup errors:\n")
		for _, item := range payload.SetupErrors {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}
	if payload.Actor != "" {
		b.WriteString("Actor `")
		b.WriteString(payload.Actor)
		b.WriteString("`.\n")
	}
	b.WriteString(fmt.Sprintf("Managed mode declared `%s`, effective `%s`. Capture `%s`. Progress `%s`. Status `%s`. Completion follows workspace `%s`.\n",
		payload.ManagedMode.DeclaredMode, payload.ManagedMode.EffectiveMode,
		payload.ManagedMode.Policy.CapturePolicy, payload.ManagedMode.Policy.ProgressPolicy,
		payload.ManagedMode.Policy.StatusPolicy, payload.ManagedMode.CompletionMode))
	keys := make([]string, 0, len(payload.Projects))
	for _, project := range payload.Projects {
		keys = append(keys, project.Key)
	}
	b.WriteString("Projects: ")
	if len(keys) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(keys, ", "))
	}
	b.WriteString(".\n\n")
	b.WriteString(formatTicketList("Assigned active", payload.AssignedActive, intFromPage(payload.Pagination, "assigned_active")))
	b.WriteString("\n")
	b.WriteString(formatTicketList("Available work", payload.AvailableWork, intFromPage(payload.Pagination, "available_work")))
	b.WriteString("\n")
	b.WriteString(formatTicketList("Pending blockers", payload.PendingBlockers, intFromPage(payload.Pagination, "pending_blockers")))
	b.WriteString(fmt.Sprintf("\nActive runs: %d. Backup snapshots: %d.\n", intFromPage(payload.Pagination, "active_runs"), payload.BackupHealth.SnapshotCount))
	return b.String()
}

func intFromPage(pages map[string]any, key string) int {
	raw, ok := pages[key].(map[string]any)
	if !ok {
		return 0
	}
	total, _ := raw["total"].(int)
	return total
}
