package mcp

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type statusPayload struct {
	Scope             string                      `json:"scope"`
	Project           string                      `json:"project,omitempty"`
	TicketID          string                      `json:"ticket_id,omitempty"`
	AgentID           string                      `json:"agent_id,omitempty"`
	RunID             string                      `json:"run_id,omitempty"`
	Actor             string                      `json:"actor,omitempty"`
	SetupErrors       []string                    `json:"setup_errors,omitempty"`
	Disambiguation    []string                    `json:"disambiguation,omitempty"`
	UnknownProject    bool                        `json:"unknown_project,omitempty"`
	ManagedMode       service.ManagedModeView     `json:"managed_mode"`
	CountsByStatus    map[string]int              `json:"counts_by_status,omitempty"`
	Ready             []service.TicketRef         `json:"ready,omitempty"`
	InProgress        []service.TicketRef         `json:"in_progress,omitempty"`
	Blocked           []service.TicketRef         `json:"blocked,omitempty"`
	InReview          []service.TicketRef         `json:"in_review,omitempty"`
	RecentlyCompleted []service.TicketRef         `json:"recently_completed,omitempty"`
	AssignedAgents    []string                    `json:"assigned_agents,omitempty"`
	RecentChanges     []statusChange              `json:"recent_changes,omitempty"`
	RecommendedNext   []string                    `json:"recommended_next,omitempty"`
	BackupHealth      service.BackupHealthSummary `json:"backup_health"`
	Board             render.CompactBoard         `json:"board,omitempty"`
	BoardURL          string                      `json:"board_url,omitempty"`
	MCPApp            *mcpAppDocument             `json:"mcp_app,omitempty"`
	Pagination        map[string]any              `json:"pagination,omitempty"`
	Markdown          string                      `json:"markdown"`
}

type statusChange struct {
	TicketID  string    `json:"ticket_id,omitempty"`
	Project   string    `json:"project,omitempty"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

func statusTool(tc ToolContext, args map[string]any) (any, error) {
	scope := strings.ToLower(stringArg(args, "scope"))
	project := stringArg(args, "project")
	ticketID := stringArg(args, "ticket_id")
	agentID := stringArg(args, "agent_id")
	runID := stringArg(args, "run_id")
	if scope == "" {
		switch {
		case runID != "":
			scope = "run"
		case ticketID != "":
			scope = "ticket"
		case agentID != "":
			scope = "agent"
		case project != "":
			scope = "project"
		default:
			scope = "workspace"
		}
	}
	switch scope {
	case "workspace", "project", "ticket", "agent", "run":
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, "scope must be workspace, project, ticket, agent, or run")
	}
	actor := resolveManagedActor(tc, args)
	if err := invalidActorError(actor); err != nil {
		return nil, err
	}
	projects, err := listProjectRefs(tc)
	if err != nil {
		return nil, err
	}
	mode, err := tc.Server.Workspace.Queries.ManagedModeView(deliveryEnabledLocally(tc.Server.Options))
	if err != nil {
		return nil, err
	}
	backup, err := tc.Server.Workspace.Queries.BackupHealth(tc.Context)
	if err != nil {
		return nil, err
	}
	payload := statusPayload{
		Scope:        scope,
		Project:      project,
		TicketID:     ticketID,
		AgentID:      agentID,
		RunID:        runID,
		Actor:        string(actor.Actor),
		ManagedMode:  mode,
		BackupHealth: backup,
	}
	if actor.SetupError != "" {
		payload.SetupErrors = append(payload.SetupErrors, actor.SetupError)
	}
	keys := make([]string, 0, len(projects))
	for _, item := range projects {
		keys = append(keys, item.Key)
	}
	if project != "" {
		if _, known, ok := resolveNamedProject(projects, project); !ok {
			payload.UnknownProject = true
			payload.Disambiguation = known
			payload.Markdown = statusDisambiguationMarkdown(project, known)
			return payload, nil
		}
	} else if scope == "project" {
		payload.UnknownProject = true
		payload.Disambiguation = keys
		payload.Markdown = statusDisambiguationMarkdown("", keys)
		return payload, nil
	} else if scope == "workspace" && len(projects) != 1 {
		// Multiple projects, or none: workspace overview. Do not pick a project.
	} else if scope == "workspace" && len(projects) == 1 && project == "" {
		// Still a workspace overview; include the only project's counts below
		// without claiming a different project.
	}

	switch scope {
	case "ticket":
		if ticketID == "" {
			return nil, apperr.New(apperr.CodeInvalidInput, "ticket scope requires ticket_id")
		}
		if err := fillTicketStatus(tc, &payload, ticketID); err != nil {
			return nil, err
		}
	case "agent":
		if err := fillAgentStatus(tc, &payload, actor, agentID); err != nil {
			return nil, err
		}
	case "run":
		if runID == "" {
			return nil, apperr.New(apperr.CodeInvalidInput, "run scope requires run_id")
		}
		if err := fillRunStatus(tc, &payload, runID); err != nil {
			return nil, err
		}
	default:
		if err := fillBoardStatus(tc, &payload, args, project, actor); err != nil {
			return nil, err
		}
	}
	payload.Markdown = statusMarkdown(payload)
	return payload, nil
}

func fillBoardStatus(tc ToolContext, payload *statusPayload, args map[string]any, project string, actor actorResolution) error {
	board, paged, err := boardPresentation(tc, project, args)
	if err != nil {
		return err
	}
	payload.Board = board
	payload.BoardURL = board.BoardURL
	payload.MCPApp = newBoardApp(board)
	payload.Pagination = map[string]any{"board": paged}
	counts := map[string]int{}
	agents := map[string]struct{}{}
	for _, col := range board.Columns {
		counts[col.Status] = col.Total
		var refs []service.TicketRef
		for _, card := range col.Cards {
			ref := service.TicketRef{ID: card.ID, Title: card.Title, Status: card.Status, Priority: card.Priority, Type: card.Type, Assignee: card.Assignee, Project: project}
			refs = append(refs, ref)
			if card.Assignee != "" {
				agents[card.Assignee] = struct{}{}
			}
		}
		switch contracts.Status(col.Status) {
		case contracts.StatusReady:
			payload.Ready = refs
		case contracts.StatusInProgress:
			payload.InProgress = refs
		case contracts.StatusBlocked:
			payload.Blocked = refs
		case contracts.StatusInReview:
			payload.InReview = refs
		case contracts.StatusDone:
			payload.RecentlyCompleted = refs
		}
	}
	payload.CountsByStatus = counts
	for agent := range agents {
		payload.AssignedAgents = append(payload.AssignedAgents, agent)
	}
	sort.Strings(payload.AssignedAgents)
	changes, err := recentChanges(tc, project, tc.Server.Options.MaxItems)
	if err != nil {
		return err
	}
	payload.RecentChanges = changes
	payload.RecommendedNext = recommendNext(*payload, actor)
	return nil
}

func fillTicketStatus(tc ToolContext, payload *statusPayload, ticketID string) error {
	detail, err := tc.Server.Workspace.Queries.TicketDetail(tc.Context, ticketID)
	if err != nil {
		return err
	}
	ticket := detail.Ticket
	payload.Project = ticket.Project
	payload.CountsByStatus = map[string]int{string(ticket.Status): 1}
	ref := compactTicket(ticket, nil)
	switch ticket.Status {
	case contracts.StatusReady:
		payload.Ready = []service.TicketRef{ref}
	case contracts.StatusInProgress:
		payload.InProgress = []service.TicketRef{ref}
	case contracts.StatusBlocked:
		payload.Blocked = []service.TicketRef{ref}
	case contracts.StatusInReview:
		payload.InReview = []service.TicketRef{ref}
	case contracts.StatusDone:
		payload.RecentlyCompleted = []service.TicketRef{ref}
	}
	if ticket.Assignee != "" {
		payload.AssignedAgents = []string{string(ticket.Assignee)}
	}
	board, _, err := boardPresentation(tc, ticket.Project, map[string]any{})
	if err == nil {
		payload.Board = board
		payload.BoardURL = board.BoardURL
		payload.MCPApp = newBoardApp(board)
	}
	payload.RecommendedNext = recommendNext(*payload, actorResolution{Actor: contracts.Actor(payload.Actor), Configured: payload.Actor != ""})
	return nil
}

func fillAgentStatus(tc ToolContext, payload *statusPayload, actor actorResolution, agentID string) error {
	resolved := actor.Actor
	if agentID != "" {
		if !strings.HasPrefix(agentID, "agent:") {
			resolved = contracts.Actor("agent:" + agentID)
		} else {
			resolved = contracts.Actor(agentID)
		}
		if !resolved.IsValid() {
			return apperr.New(apperr.CodeInvalidInput, "invalid agent_id")
		}
		payload.Actor = string(resolved)
	}
	if resolved == "" {
		payload.RecommendedNext = []string{"configure an Atlas actor before asking for agent status"}
		return nil
	}
	work, err := tc.Server.Workspace.Queries.AgentWork(tc.Context, resolved)
	if err != nil {
		return err
	}
	payload.Ready, _ = compactWork(work.Available, payload.Project, tc.Server.Options.MaxItems)
	payload.Blocked, _ = compactWork(work.Pending, payload.Project, tc.Server.Options.MaxItems)
	payload.RecommendedNext = recommendNext(*payload, actorResolution{Actor: resolved, Configured: true})
	return nil
}

func fillRunStatus(tc ToolContext, payload *statusPayload, runID string) error {
	detail, err := tc.Server.Workspace.Queries.RunDetail(tc.Context, runID)
	if err != nil {
		return err
	}
	payload.Project = detail.Run.Project
	payload.TicketID = detail.Run.TicketID
	payload.AgentID = detail.Run.AgentID
	payload.RecommendedNext = []string{fmt.Sprintf("run %s is %s", detail.Run.RunID, detail.Run.Status)}
	return nil
}

func recentChanges(tc ToolContext, project string, limit int) ([]statusChange, error) {
	if limit <= 0 {
		limit = 8
	}
	events, err := tc.Server.Workspace.Queries.RecentEvents(tc.Context, limit)
	if err != nil {
		return nil, err
	}
	out := make([]statusChange, 0, len(events))
	for _, event := range events {
		if project != "" && event.Project != project {
			continue
		}
		out = append(out, statusChange{
			TicketID:  event.TicketID,
			Project:   event.Project,
			Type:      string(event.Type),
			Actor:     string(event.Actor),
			Timestamp: event.Timestamp.UTC(),
		})
	}
	return out, nil
}

func recommendNext(payload statusPayload, actor actorResolution) []string {
	var out []string
	if len(payload.SetupErrors) > 0 {
		out = append(out, "fix Atlas actor setup before reporting status from memory")
	}
	if len(payload.Ready) > 0 {
		out = append(out, "claim "+payload.Ready[0].ID+" and move it to in_progress through a legal edge")
	}
	if len(payload.Blocked) > 0 {
		reasons := strings.Join(payload.Blocked[0].ReasonCodes, ",")
		if reasons == "" {
			reasons = "blocked"
		}
		out = append(out, "do not start "+payload.Blocked[0].ID+" until "+reasons+" clears")
	}
	if len(payload.InReview) > 0 {
		out = append(out, "follow workspace review policy for "+payload.InReview[0].ID)
	}
	if len(payload.InProgress) > 0 && actor.Actor != "" {
		out = append(out, "continue "+payload.InProgress[0].ID+" and record a milestone when tests or review land")
	}
	if len(out) == 0 {
		out = append(out, "no actionable Atlas work in this scope")
	}
	return out
}

func statusDisambiguationMarkdown(requested string, known []string) string {
	var b strings.Builder
	b.WriteString("# Atlas status\n\n")
	if requested != "" {
		b.WriteString("Unknown project `")
		b.WriteString(requested)
		b.WriteString("`.\n")
	} else {
		b.WriteString("Name a project for project-scoped status.\n")
	}
	b.WriteString("Known projects: ")
	if len(known) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(known, ", "))
	}
	b.WriteString(".\n")
	return b.String()
}

func statusMarkdown(payload statusPayload) string {
	if payload.UnknownProject {
		return payload.Markdown
	}
	var b strings.Builder
	b.WriteString("# Atlas status\n")
	b.WriteString("Scope `")
	b.WriteString(payload.Scope)
	b.WriteString("`")
	if payload.Project != "" {
		b.WriteString(" project `")
		b.WriteString(payload.Project)
		b.WriteString("`")
	}
	b.WriteString(".\n")
	if payload.Actor != "" {
		b.WriteString("Actor `")
		b.WriteString(payload.Actor)
		b.WriteString("`.\n")
	}
	if len(payload.SetupErrors) > 0 {
		b.WriteString("Setup errors:\n")
		for _, item := range payload.SetupErrors {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("Managed mode declared `%s`, effective `%s`. Completion `%s`.\n",
		payload.ManagedMode.DeclaredMode, payload.ManagedMode.EffectiveMode, payload.ManagedMode.CompletionMode))
	if len(payload.CountsByStatus) > 0 {
		b.WriteString("Counts:")
		keys := make([]string, 0, len(payload.CountsByStatus))
		for key := range payload.CountsByStatus {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString(fmt.Sprintf(" %s=%d", key, payload.CountsByStatus[key]))
		}
		b.WriteString(".\n")
	}
	b.WriteString(formatTicketList("Ready", payload.Ready, len(payload.Ready)))
	b.WriteString(formatTicketList("In progress", payload.InProgress, len(payload.InProgress)))
	b.WriteString(formatTicketList("Blocked", payload.Blocked, len(payload.Blocked)))
	b.WriteString(formatTicketList("In review", payload.InReview, len(payload.InReview)))
	b.WriteString(formatTicketList("Recently completed", payload.RecentlyCompleted, len(payload.RecentlyCompleted)))
	if len(payload.AssignedAgents) > 0 {
		b.WriteString("Assigned agents: ")
		b.WriteString(strings.Join(payload.AssignedAgents, ", "))
		b.WriteString(".\n")
	}
	if len(payload.RecommendedNext) > 0 {
		b.WriteString("Next:\n")
		for _, item := range payload.RecommendedNext {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}
	if payload.BoardURL != "" {
		b.WriteString("Web board: `")
		b.WriteString(payload.BoardURL)
		b.WriteString("`\n")
	}
	if payload.Board.Title != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(render.CompactBoardMarkdown(payload.Board)))
		b.WriteString("\n")
	}
	md := b.String()
	if strings.Contains(md, "% complete") || strings.Contains(md, "progress %") {
		md = strings.ReplaceAll(md, "% complete", "complete")
	}
	return md
}
