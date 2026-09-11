package mcp

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type projectRef struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

type runRef struct {
	RunID    string `json:"run_id"`
	TicketID string `json:"ticket_id,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	Status   string `json:"status"`
}

type actorResolution struct {
	Actor      contracts.Actor
	Configured bool
	SetupError string
	Source     string
}

func deliveryEnabledLocally(opts Options) bool {
	return opts.Profile == ProfileDelivery || opts.Profile == ProfileAdmin
}

func resolveManagedActor(tc ToolContext, args map[string]any) actorResolution {
	if raw := stringArg(args, "actor"); raw != "" {
		actor := contracts.Actor(raw)
		if !actor.IsValid() {
			return actorResolution{SetupError: "invalid actor " + raw, Source: "argument"}
		}
		return actorResolution{Actor: actor, Configured: true, Source: "argument"}
	}
	if hint := strings.TrimSpace(string(tc.Server.Options.ConfiguredActor)); hint != "" {
		actor := contracts.Actor(hint)
		if actor.IsValid() {
			return actorResolution{Actor: actor, Configured: true, Source: "integration"}
		}
	}
	if resolved, err := tc.Server.Workspace.Queries.ResolveActor(tc.Context, ""); err == nil && resolved != "" {
		return actorResolution{Actor: resolved, Configured: true, Source: "workspace"}
	}
	return actorResolution{
		SetupError: "configured Atlas actor is missing: pass actor, set TRACKER_ACTOR, configure actor.default, or finish setup so this integration has an ActorHint",
		Source:     "unconfigured",
	}
}

func loadWorkspaceIdentity(tc ToolContext) (string, error) {
	id, err := service.LoadWorkspaceIdentity(tc.Server.Workspace.Root)
	if err != nil {
		return "", err
	}
	return id, nil
}

func listProjectRefs(tc ToolContext) ([]projectRef, error) {
	projects, err := tc.Server.Workspace.Queries.Projects.ListProjects(tc.Context)
	if err != nil {
		return nil, err
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Key < projects[j].Key })
	out := make([]projectRef, 0, len(projects))
	for _, project := range projects {
		out = append(out, projectRef{Key: project.Key, Name: project.Name})
	}
	return out, nil
}

func resolveNamedProject(projects []projectRef, key string) (projectRef, []string, bool) {
	key = strings.TrimSpace(key)
	keys := make([]string, 0, len(projects))
	for _, project := range projects {
		keys = append(keys, project.Key)
		if project.Key == key {
			return project, keys, true
		}
	}
	return projectRef{}, keys, false
}

func eventHeads(tc ToolContext, projects []projectRef) (map[string]int64, error) {
	heads := map[string]int64{}
	for _, project := range projects {
		events, err := tc.Server.Workspace.Queries.Events.StreamEvents(tc.Context, project.Key, 0)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if len(events) == 0 {
			continue
		}
		heads[project.Key] = events[len(events)-1].EventID
	}
	return heads, nil
}

func assignedActiveTickets(tc ToolContext, actor contracts.Actor, project string, limit int) ([]service.TicketRef, int, error) {
	if actor == "" {
		return []service.TicketRef{}, 0, nil
	}
	tickets, err := tc.Server.Workspace.Queries.Tickets.ListTickets(tc.Context, contracts.TicketListOptions{
		Project:  project,
		Assignee: actor,
	})
	if err != nil {
		return nil, 0, err
	}
	refs := make([]service.TicketRef, 0, len(tickets))
	for _, ticket := range tickets {
		if !isActiveWorkflowStatus(ticket.Status) {
			continue
		}
		refs = append(refs, compactTicket(ticket, nil))
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	total := len(refs)
	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	return refs, total, nil
}

func compactTicket(ticket contracts.TicketSnapshot, reasons []string) service.TicketRef {
	ref := service.TicketRef{
		ID:          ticket.ID,
		Project:     ticket.Project,
		Title:       strings.TrimSpace(ticket.Title),
		Status:      string(ticket.Status),
		Priority:    string(ticket.Priority),
		Type:        string(ticket.Type),
		Assignee:    string(ticket.Assignee),
		ReasonCodes: append([]string(nil), reasons...),
	}
	return ref
}

func isActiveWorkflowStatus(status contracts.Status) bool {
	switch status {
	case contracts.StatusReady, contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusBlocked:
		return true
	default:
		return false
	}
}

func compactWork(entries []service.AgentWorkEntry, project string, limit int) ([]service.TicketRef, int) {
	refs := make([]service.TicketRef, 0, len(entries))
	for _, entry := range entries {
		if project != "" && entry.Ticket.Project != project {
			continue
		}
		refs = append(refs, compactTicket(entry.Ticket, entry.ReasonCodes))
	}
	total := len(refs)
	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	return refs, total
}

func compactActiveRuns(tc ToolContext, project string, actor contracts.Actor, limit int) ([]runRef, int, error) {
	runs, err := tc.Server.Workspace.Queries.ListRuns(tc.Context, "", "", "")
	if err != nil {
		return nil, 0, err
	}
	out := make([]runRef, 0, len(runs))
	for _, run := range runs {
		if !isActiveRunStatus(run.Status) {
			continue
		}
		if actor != "" && run.AgentID != "" && "agent:"+run.AgentID != string(actor) && run.AgentID != string(actor) {
			// keep runs whose agent matches; unscoped runs still appear
			if !strings.HasSuffix(string(actor), run.AgentID) {
				continue
			}
		}
		if project != "" && run.Project != project {
			continue
		}
		out = append(out, runRef{RunID: run.RunID, TicketID: run.TicketID, AgentID: run.AgentID, Status: string(run.Status)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunID < out[j].RunID })
	total := len(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

func isActiveRunStatus(status contracts.RunStatus) bool {
	switch status {
	case contracts.RunStatusDispatched, contracts.RunStatusAttached, contracts.RunStatusActive,
		contracts.RunStatusHandoffReady, contracts.RunStatusAwaitingReview, contracts.RunStatusAwaitingOwner:
		return true
	default:
		return false
	}
}

func boardPresentation(tc ToolContext, project string, args map[string]any) (render.CompactBoard, map[string]any, error) {
	view, err := tc.Server.Workspace.Queries.Board(tc.Context, contracts.BoardQueryOptions{Project: project})
	if err != nil {
		return render.CompactBoard{}, nil, err
	}
	paged := paginateBoard(view, args, tc.Server.Options.MaxItems)
	cursors, _ := paged["next_cursor_by_status"].(map[string]string)
	board := render.NewCompactBoard(project, view.Board.Columns, tc.Server.Options.MaxItems, cursors)
	return board, paged, nil
}

func invalidActorError(res actorResolution) error {
	if res.SetupError != "" && strings.HasPrefix(res.SetupError, "invalid actor") {
		return apperr.New(apperr.CodeInvalidInput, res.SetupError)
	}
	return nil
}

func pageMeta(total int, shown int, cursor string) map[string]any {
	meta := map[string]any{"total": total, "shown": shown, "truncated": shown < total}
	if cursor != "" {
		meta["next_cursor"] = cursor
	}
	return meta
}

func generatedAt(tc ToolContext) time.Time {
	if tc.Server != nil && tc.Server.Options.Now != nil {
		return tc.Server.Options.Now().UTC()
	}
	return time.Now().UTC()
}

func formatTicketList(title string, items []service.TicketRef, total int) string {
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString(fmt.Sprintf(" (%d)\n", total))
	if len(items) == 0 {
		b.WriteString("- (none)\n")
		return b.String()
	}
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item.ID)
		if item.Status != "" {
			b.WriteString(" [")
			b.WriteString(item.Status)
			b.WriteString("]")
		}
		if item.Title != "" {
			b.WriteString(" ")
			b.WriteString(item.Title)
		}
		if len(item.ReasonCodes) > 0 {
			b.WriteString(" reasons=")
			b.WriteString(strings.Join(item.ReasonCodes, ","))
		}
		b.WriteString("\n")
	}
	return b.String()
}
