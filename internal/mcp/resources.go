package mcp

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type ResourceChange struct {
	URI         string
	WorkspaceID string
	Project     string
	Entity      string
	Revision    string
	Event       string
}

type resourceHub struct {
	mu       sync.Mutex
	pending  map[string]ResourceChange
	timer    *time.Timer
	debounce time.Duration
	sdk      *mcpsdk.Server
	subs     map[string]int
}

func newResourceHub() *resourceHub {
	return &resourceHub{pending: map[string]ResourceChange{}, debounce: 75 * time.Millisecond, subs: map[string]int{}}
}

func (h *resourceHub) subscribe(_ context.Context, req *mcpsdk.SubscribeRequest) error {
	if req == nil || req.Params == nil || strings.TrimSpace(req.Params.URI) == "" {
		return apperr.New(apperr.CodeInvalidInput, "resource uri is required")
	}
	h.mu.Lock()
	h.subs[req.Params.URI]++
	h.mu.Unlock()
	return nil
}

func (h *resourceHub) unsubscribe(_ context.Context, req *mcpsdk.UnsubscribeRequest) error {
	if req == nil || req.Params == nil {
		return nil
	}
	h.mu.Lock()
	if h.subs[req.Params.URI] > 0 {
		h.subs[req.Params.URI]--
		if h.subs[req.Params.URI] == 0 {
			delete(h.subs, req.Params.URI)
		}
	}
	h.mu.Unlock()
	return nil
}

func (h *resourceHub) note(change ResourceChange) {
	if strings.TrimSpace(change.URI) == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	existing, ok := h.pending[change.URI]
	if ok {
		if change.Event != "" && existing.Event != "" && change.Event != existing.Event {
			change.Event = existing.Event + "," + change.Event
		}
		if existing.WorkspaceID != "" && change.WorkspaceID == "" {
			change.WorkspaceID = existing.WorkspaceID
		}
	}
	h.pending[change.URI] = change
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(h.debounce, h.flush)
}

func (h *resourceHub) flush() {
	h.mu.Lock()
	pending := h.pending
	h.pending = map[string]ResourceChange{}
	sdk := h.sdk
	h.mu.Unlock()
	if sdk == nil {
		return
	}
	ctx := context.Background()
	for _, change := range pending {
		_ = sdk.ResourceUpdated(ctx, &mcpsdk.ResourceUpdatedNotificationParams{
			URI: change.URI,
			Meta: mcpsdk.Meta{
				"workspace": change.WorkspaceID,
				"project":   change.Project,
				"entity":    change.Entity,
				"revision":  change.Revision,
				"event":     change.Event,
			},
		})
	}
}

func (h *resourceHub) FlushForTest() { h.flush() }

type resourceRef struct {
	Kind        string
	WorkspaceID string
	Project     string
}

func parseResourceURI(uri string) (resourceRef, error) {
	if uri == BoardAppResourceURI {
		return resourceRef{Kind: "ui-board"}, nil
	}
	if !strings.HasPrefix(uri, "atlas://") {
		return resourceRef{}, apperr.New(apperr.CodeNotFound, "unknown resource")
	}
	rest := strings.Trim(strings.TrimPrefix(uri, "atlas://"), "/")
	parts := strings.Split(rest, "/")
	switch {
	case len(parts) == 1 && parts[0] == "workspaces":
		return resourceRef{Kind: "workspaces"}, nil
	case len(parts) == 1 && parts[0] == "attention":
		return resourceRef{Kind: "attention"}, nil
	case len(parts) >= 2 && parts[0] == "workspace":
		ref := resourceRef{Kind: "workspace", WorkspaceID: parts[1]}
		if len(parts) == 2 {
			return ref, nil
		}
		if len(parts) == 3 && parts[2] == "projects" {
			ref.Kind = "projects"
			return ref, nil
		}
		if len(parts) == 3 && parts[2] == "backup" {
			ref.Kind = "backup"
			return ref, nil
		}
		if len(parts) == 3 && parts[2] == "activity" {
			ref.Kind = "activity"
			return ref, nil
		}
		if len(parts) == 5 && parts[2] == "project" && parts[4] == "board" {
			ref.Kind = "board"
			ref.Project = parts[3]
			return ref, nil
		}
	}
	return resourceRef{}, apperr.New(apperr.CodeNotFound, "unknown resource")
}

func (s *Server) resourceTemplates() []*mcpsdk.ResourceTemplate {
	jsonMIME := "application/json"
	return []*mcpsdk.ResourceTemplate{
		{URITemplate: "atlas://workspace/{workspace_id}", Name: "workspace", MIMEType: jsonMIME, Description: "One registered workspace after revalidation."},
		{URITemplate: "atlas://workspace/{workspace_id}/projects", Name: "workspace-projects", MIMEType: jsonMIME, Description: "Projects in a workspace."},
		{URITemplate: "atlas://workspace/{workspace_id}/backup", Name: "workspace-backup", MIMEType: jsonMIME, Description: "Automatic backup health for a workspace."},
		{URITemplate: "atlas://workspace/{workspace_id}/activity", Name: "workspace-activity", MIMEType: jsonMIME, Description: "Recent workspace events."},
		{URITemplate: "atlas://workspace/{workspace_id}/project/{project}/board", Name: "workspace-project-board", MIMEType: jsonMIME, Description: "Compact board for one project."},
	}
}

func (s *Server) resourceDescriptors() []*mcpsdk.Resource {
	items := []*mcpsdk.Resource{
		{URI: "atlas://workspaces", Name: "workspaces", MIMEType: "application/json", Description: "Registered workspaces and health."},
		{URI: "atlas://attention", Name: "attention", MIMEType: "application/json", Description: "Cross-workspace attention."},
		{
			URI:         BoardAppResourceURI,
			Name:        "atlas-board-app",
			MIMEType:    BoardAppMIME,
			Description: "Sandboxed MCP Apps view for atlas.board.",
			Meta:        boardAppUICSPMeta(),
		},
	}
	ids := s.knownWorkspaceIDs()
	for _, id := range ids {
		base := "atlas://workspace/" + id
		items = append(items,
			&mcpsdk.Resource{URI: base, Name: "workspace " + id, MIMEType: "application/json"},
			&mcpsdk.Resource{URI: base + "/projects", Name: "projects", MIMEType: "application/json"},
			&mcpsdk.Resource{URI: base + "/backup", Name: "backup", MIMEType: "application/json"},
			&mcpsdk.Resource{URI: base + "/activity", Name: "activity", MIMEType: "application/json"},
		)
	}
	if s.Options.MaxItems > 0 && len(items) > s.Options.MaxItems+8 {
		items = items[:s.Options.MaxItems+8]
	}
	return items
}

func (s *Server) knownWorkspaceIDs() []string {
	if s.Options.Machine != nil {
		list, err := s.Options.Machine.ListWorkspaces(context.Background(), app.ListOptions{})
		if err == nil {
			ids := make([]string, 0, len(list))
			for _, rec := range list {
				if rec.Health == app.HealthAvailable {
					ids = append(ids, rec.WorkspaceID)
				}
			}
			return ids
		}
	}
	if s.Workspace != nil {
		id := s.Workspace.ID
		if id == "" {
			id, _ = service.LoadWorkspaceIdentity(s.Workspace.Root)
		}
		if id != "" {
			return []string{id}
		}
	}
	return nil
}

func (s *Server) readResource(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "resource uri is required")
	}
	uri := req.Params.URI
	ref, err := parseResourceURI(uri)
	if err != nil {
		return nil, mcpsdk.ResourceNotFoundError(uri)
	}
	if ref.Kind == "ui-board" {
		html := boardAppHTML()
		return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{
			URI:      uri,
			MIMEType: BoardAppMIME,
			Text:     html,
			Meta:     boardAppUICSPMeta(),
		}}}, nil
	}
	payload, err := s.resourcePayload(ctx, ref)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(redactForOutput(payload, s.Options.IncludeLocalOnlyPaths))
	if err != nil {
		return nil, err
	}
	if s.Options.MaxResultBytes > 0 && len(raw) > s.Options.MaxResultBytes {
		raw, _ = json.Marshal(map[string]any{"truncated": true, "hint": "narrow the resource or raise max-result-bytes"})
	}
	return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(raw),
	}}}, nil
}

func (s *Server) resourcePayload(ctx context.Context, ref resourceRef) (any, error) {
	switch ref.Kind {
	case "workspaces":
		if s.Options.Machine == nil {
			if s.Workspace == nil {
				return map[string]any{"items": []any{}}, nil
			}
			id, _ := service.LoadWorkspaceIdentity(s.Workspace.Root)
			return map[string]any{"items": []map[string]any{{"workspace_id": id, "health": string(app.HealthAvailable)}}}, nil
		}
		items, err := s.Options.Machine.ListWorkspaces(ctx, app.ListOptions{})
		if err != nil {
			return nil, err
		}
		public := make([]map[string]any, 0, len(items))
		for _, item := range items {
			public = append(public, publicWorkspace(item, s.Options.IncludeLocalOnlyPaths))
		}
		return map[string]any{"items": public}, nil
	case "attention":
		if s.Options.Machine != nil {
			report, err := s.Options.Machine.Attention(ctx, app.AttentionOptions{Limit: s.Options.MaxItems})
			if err != nil {
				return nil, err
			}
			return publicAttention(report, s.Options.IncludeLocalOnlyPaths), nil
		}
		if s.Workspace == nil {
			return map[string]any{"kind": "attention", "items": []any{}}, nil
		}
		return attentionFromBound(ctx, s.Workspace, s.Options.MaxItems)
	case "workspace", "projects", "board", "backup", "activity":
		if ref.Kind == "workspace" && s.Options.Machine != nil {
			rec, err := s.Options.Machine.GetWorkspace(ctx, ref.WorkspaceID)
			out := publicWorkspace(rec, s.Options.IncludeLocalOnlyPaths)
			if rec.WorkspaceID == "" && err != nil {
				return nil, err
			}
			return out, nil
		}
		ws, err := s.workspaceForResource(ctx, ref.WorkspaceID)
		if err != nil {
			return nil, err
		}
		id, _ := service.LoadWorkspaceIdentity(ws.Root)
		switch ref.Kind {
		case "workspace":
			return map[string]any{"workspace_id": id, "health": string(app.HealthAvailable)}, nil
		case "projects":
			projects, err := ws.Queries.Projects.ListProjects(ctx)
			if err != nil {
				return nil, err
			}
			keys := make([]projectRef, 0, len(projects))
			for _, project := range projects {
				keys = append(keys, projectRef{Key: project.Key, Name: project.Name})
			}
			return map[string]any{"workspace_id": id, "projects": keys}, nil
		case "board":
			view, err := ws.Queries.Board(ctx, contracts.BoardQueryOptions{Project: ref.Project})
			if err != nil {
				return nil, err
			}
			return map[string]any{"workspace_id": id, "project": ref.Project, "board": view}, nil
		case "backup":
			auto, err := ws.Queries.AutoBackupStatus(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"workspace_id": id, "automatic": auto, "push_is_not_verification": true}, nil
		case "activity":
			events, err := ws.Queries.RecentEvents(ctx, s.Options.MaxItems)
			if err != nil {
				return nil, err
			}
			return map[string]any{"workspace_id": id, "events": events}, nil
		}
	}
	return nil, apperr.New(apperr.CodeNotFound, "unknown resource")
}

func attentionFromBound(ctx context.Context, ws *Workspace, limit int) (map[string]any, error) {
	id, _ := service.LoadWorkspaceIdentity(ws.Root)
	items := []map[string]any{}
	inbox, err := ws.Queries.Inbox(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, item := range inbox {
		items = append(items, map[string]any{"kind": "inbox", "workspace_id": id, "ticket_id": item.TicketID, "summary": item.Summary})
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return map[string]any{"kind": "attention", "items": items, "total": len(items)}, nil
}

func (s *Server) workspaceForResource(ctx context.Context, id string) (*Workspace, error) {
	if s.Options.Machine != nil {
		return s.Options.Machine.Bind(ctx, id)
	}
	if s.Workspace == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "workspace is not bound")
	}
	got := s.Workspace.ID
	if got == "" {
		got, _ = service.LoadWorkspaceIdentity(s.Workspace.Root)
	}
	if got != id {
		return nil, apperr.New(apperr.CodePermissionDenied, "resource workspace does not match the pinned server")
	}
	return s.Workspace, nil
}

func (s *Server) noteMutation(spec ToolSpec, args map[string]any) {
	if spec.Class == ClassRead || s.resources == nil {
		return
	}
	id := ""
	if s.Workspace != nil {
		id = s.Workspace.ID
		if id == "" {
			id, _ = service.LoadWorkspaceIdentity(s.Workspace.Root)
		}
	}
	if id == "" {
		id = stringArg(args, "workspace_id")
	}
	project := stringArg(args, "project")
	if project == "" {
		project = stringArg(args, "key")
	}
	entity := spec.Name
	if ticket := stringArg(args, "ticket_id"); ticket != "" {
		entity = ticket
	}
	rev := s.eventWatermark(project)
	if id != "" {
		s.resources.note(ResourceChange{URI: "atlas://workspace/" + id, WorkspaceID: id, Project: project, Entity: entity, Event: spec.Name, Revision: rev})
		s.resources.note(ResourceChange{URI: "atlas://attention", WorkspaceID: id, Entity: entity, Event: spec.Name, Revision: rev})
		if project != "" {
			s.resources.note(ResourceChange{URI: "atlas://workspace/" + id + "/project/" + project + "/board", WorkspaceID: id, Project: project, Entity: entity, Event: spec.Name, Revision: rev})
		}
	}
	s.resources.note(ResourceChange{URI: "atlas://workspaces", WorkspaceID: id, Event: spec.Name, Revision: rev})
	if strings.HasPrefix(spec.Name, "atlas.workspace.") || strings.HasPrefix(spec.Name, "atlas.project.") {
		s.refreshResourceCatalog()
	}
}

func (s *Server) eventWatermark(project string) string {
	if s.Workspace == nil || s.Workspace.Queries == nil {
		return ""
	}
	ctx := context.Background()
	keys := []string{}
	if strings.TrimSpace(project) != "" {
		keys = []string{project}
	} else {
		projects, err := s.Workspace.Queries.Projects.ListProjects(ctx)
		if err != nil {
			return ""
		}
		for _, p := range projects {
			keys = append(keys, p.Key)
		}
	}
	var max int64
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		events, err := s.Workspace.Queries.Events.StreamEvents(ctx, key, 0)
		if err != nil || len(events) == 0 {
			continue
		}
		if id := events[len(events)-1].EventID; id > max {
			max = id
		}
	}
	if max == 0 {
		return ""
	}
	return strconv.FormatInt(max, 10)
}

func (s *Server) refreshResourceCatalog() {
	if s.resources == nil {
		return
	}
	s.resources.mu.Lock()
	sdk := s.resources.sdk
	s.resources.mu.Unlock()
	if sdk == nil {
		return
	}
	for _, resource := range s.resourceDescriptors() {
		sdk.AddResource(resource, s.readResource)
	}
}
