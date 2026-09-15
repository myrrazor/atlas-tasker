package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type HomePage struct {
	Page            string
	Workspace       string
	Host            string
	Actor           string
	CSRFToken       string
	ReadOnly        bool
	HomePath        string
	BoardPath       string
	ActionPrefix    string
	Flash           string
	Error           string
	Workspaces      []app.WorkspaceRecord
	Rows            []HomeWorkspaceRow
	Attention       []app.AttentionItem
	Hits            []app.SearchHit
	Query           string
	Sort            string
	Settings        app.MachineSettings
	Agents          []app.AgentClientReport
	Backup          *service.BackupHealthSummary
	BackupView      HomeBackupView
	Projects        []ProjectRow
	Recent          []RecentChange
	WorkspaceID     string
	DisplayName     string
	Health          app.Health
	HealthLabel     string
	HealthDetail    string
	Path            string
	Location        string
	Visibility      app.Visibility
	FindHits        []HomeGrantHit
	ShowFind        bool
	ShowInit        bool
	ShowNewProject  bool
	ShowHidden      bool
	Form            url.Values
	AuthorizedRoots []app.AuthorizedRoot
	BrowseRef       string
	BrowseRel       string
	BrowseChildren  []app.AuthorizedChild
	PendingGrant    *app.PathGrant
}

type HomeWorkspaceRow struct {
	Record      app.WorkspaceRecord
	Title       string
	Location    string
	HealthLabel string
	Attention   int
	Projects    int
	Blocked     int
	Available   bool
	Hidden      bool
}

type HomeGrantHit struct {
	GrantID     string
	Purpose     string
	Title       string
	Location    string
	WorkspaceID string
	Registered  bool
}

type HomeBackupView struct {
	Present           bool
	LocalLabel        string
	LocalDetail       string
	OffDeviceLabel    string
	OffDeviceDetail   string
	OffDeviceVerified bool
	PendingLabel      string
}

func (s *HomeServer) pageBase(page string) HomePage {
	return HomePage{
		Page:      page,
		Workspace: "Atlas Home",
		Host:      s.cfg.Host,
		Actor:     string(s.cfg.Actor),
		CSRFToken: s.csrf,
		ReadOnly:  s.cfg.ReadOnly,
		HomePath:  "/",
		BoardPath: "/",
		Sort:      "attention",
	}
}

func (s *HomeServer) renderHome(w http.ResponseWriter, r *http.Request, page HomePage, status int) {
	cloned, err := s.templates.Clone()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lang := defaultLanguage
	cloned = cloned.Funcs(template.FuncMap{
		"lang":    func() string { return lang },
		"langURL": func(next string) string { return languageURL(r.URL, next) },
		"t":       translator(lang),
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := cloned.ExecuteTemplate(w, "homeLayout", page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *HomeServer) handleHome(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/actions/workspaces/repair" && r.Method == http.MethodPost:
		s.handleRepairWorkspace(w, r)
		return
	case r.URL.Path == "/actions/settings" && r.Method == http.MethodPost:
		s.handleUpdateSettings(w, r)
		return
	case r.URL.Path != "/":
		http.NotFound(w, r)
		return
	}
	page := s.buildHomeListing(r)
	s.renderHome(w, r, page, http.StatusOK)
}

func (s *HomeServer) buildHomeListing(r *http.Request) HomePage {
	page := s.pageBase("home")
	page.Flash = r.URL.Query().Get("flash")
	page.Sort = homeSort(r.URL.Query().Get("sort"))
	page.ShowFind = r.URL.Query().Get("find") == "1"
	page.ShowInit = r.URL.Query().Get("init") == "1"
	page.ShowHidden = s.application.Settings().Home.ShowHidden || r.URL.Query().Get("hidden") == "1"
	page.Settings = s.application.Settings()

	listed, err := s.application.ListWorkspaces(r.Context(), app.ListOptions{IncludeHidden: page.ShowHidden})
	if err != nil {
		page.Error = err.Error()
	} else {
		page.Workspaces = listed
	}
	attention, err := s.application.Attention(r.Context(), app.AttentionOptions{Actor: s.cfg.Actor, Limit: 24})
	if err == nil {
		page.Attention = attention.Items
	}
	page.Rows = s.decorateWorkspaceRows(r, listed, page.Attention)
	sortHomeRows(page.Rows, page.Sort)
	s.decorateHomeCreate(r, &page)
	return page
}

func (s *HomeServer) decorateHomeCreate(r *http.Request, page *HomePage) {
	page.AuthorizedRoots = s.application.AuthorizedRoots()
	if !page.ShowFind && !page.ShowInit {
		return
	}
	page.BrowseRef = strings.TrimSpace(firstNonEmpty(r.Form.Get("root_ref"), r.URL.Query().Get("root"), app.BoardsRootRef))
	page.BrowseRel = strings.TrimSpace(firstNonEmpty(r.Form.Get("folder"), r.URL.Query().Get("dir")))
	if page.BrowseRef == "" {
		page.BrowseRef = app.BoardsRootRef
	}
	if children, err := s.application.ListAuthorizedChildren(page.BrowseRef, page.BrowseRel); err == nil {
		page.BrowseChildren = children
	}
	purpose := app.PathGrantRegister
	if page.ShowInit {
		purpose = app.PathGrantInit
	}
	page.FindHits = s.discoveryGrants(r, page.ShowInit)
	page.FindHits = append(page.FindHits, pendingGrantHits(s.application.ListPendingGrants(), purpose)...)
}

func (s *HomeServer) handleAttention(w http.ResponseWriter, r *http.Request) {
	page := s.pageBase("attention")
	page.Settings = s.application.Settings()
	report, err := s.application.Attention(r.Context(), app.AttentionOptions{Actor: s.cfg.Actor, Limit: 80})
	if err != nil {
		page.Error = err.Error()
	} else {
		page.Attention = report.Items
		page.Workspaces = report.Missing
	}
	s.renderHome(w, r, page, http.StatusOK)
}

func (s *HomeServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	page := s.pageBase("search")
	page.Settings = s.application.Settings()
	page.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	if page.Query != "" {
		report, err := s.application.Search(r.Context(), app.SearchOptions{Query: page.Query, Limit: 50})
		if err != nil {
			page.Error = err.Error()
		} else {
			page.Hits = report.Hits
		}
	}
	s.renderHome(w, r, page, http.StatusOK)
}

func (s *HomeServer) handleHomeSettings(w http.ResponseWriter, r *http.Request) {
	page := s.pageBase("settings")
	page.Settings = s.application.Settings()
	page.Flash = r.URL.Query().Get("flash")
	s.renderHome(w, r, page, http.StatusOK)
}

func (s *HomeServer) handleSettingsAgents(w http.ResponseWriter, r *http.Request) {
	page := s.pageBase("settings-agents")
	page.Settings = s.application.Settings()
	page.Flash = r.URL.Query().Get("flash")
	report := s.application.ListAgentClients(r.Context())
	page.Agents = report.Clients
	workspaces, err := s.application.ListWorkspaces(r.Context(), app.ListOptions{})
	if err != nil {
		page.Error = err.Error()
	} else {
		page.Workspaces = workspaces
	}
	s.renderHome(w, r, page, http.StatusOK)
}

func (s *HomeServer) handleSettingsWorkspaces(w http.ResponseWriter, r *http.Request) {
	page := s.pageBase("settings-workspaces")
	page.Settings = s.application.Settings()
	page.Flash = r.URL.Query().Get("flash")
	page.ShowFind = true
	listed, err := s.application.ListWorkspaces(r.Context(), app.ListOptions{IncludeHidden: true})
	if err != nil {
		page.Error = err.Error()
	} else {
		page.Workspaces = listed
		page.Rows = s.decorateWorkspaceRows(r, listed, nil)
	}
	page.FindHits = s.discoveryGrants(r, false)
	page.FindHits = append(page.FindHits, pendingGrantHits(s.application.ListPendingGrants(), app.PathGrantRegister)...)
	s.renderHome(w, r, page, http.StatusOK)
}

func (s *HomeServer) handleInitWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderWorkspaceFormError(w, r, err, "init")
		return
	}
	projectKey := strings.TrimSpace(r.Form.Get("project_key"))
	projectName := strings.TrimSpace(r.Form.Get("project_name"))
	if projectKey != "" {
		project := contracts.NormalizeProject(contracts.Project{
			Key:           projectKey,
			Name:          firstNonEmpty(projectName, projectKey),
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		if err := project.Validate(); err != nil {
			s.renderWorkspaceFormError(w, r, apperr.New(apperr.CodeInvalidInput, err.Error()), "init")
			return
		}
		projectKey = project.Key
		if projectName == "" {
			projectName = project.Name
		}
	}
	if err := requireHumanActor(s.cfg.Actor); err != nil {
		s.renderWorkspaceFormError(w, r, err, "init")
		return
	}
	grant, err := s.resolvePathGrant(r, app.PathGrantInit, true)
	if err != nil {
		s.renderWorkspaceFormError(w, r, err, "init")
		return
	}
	settings := s.application.Settings()
	result, err := s.application.Init(r.Context(), app.InitOptions{
		Root:            grant.Path,
		Register:        true,
		Agents:          settings.Agents.AutoInstall,
		Backup:          settings.LocalCheckpoints,
		DefaultProject:  settings.DefaultProject,
		ProjectKey:      projectKey,
		ProjectName:     projectName,
		WriteClientCfg:  false,
		SkipHomeService: true,
		Actor:           s.cfg.Actor,
	})
	if err != nil {
		s.renderWorkspaceFormError(w, r, err, "init")
		return
	}
	next := "/w/" + url.PathEscape(result.WorkspaceID)
	if result.DefaultProject != "" {
		next += "/projects/" + url.PathEscape(result.DefaultProject)
	}
	message := "Board created and added to Home"
	if result.DefaultProject != "" {
		message += "; project " + result.DefaultProject + " is ready"
	}
	http.Redirect(w, r, next+"?flash="+url.QueryEscape(message), http.StatusSeeOther)
}

func (s *HomeServer) handleRegisterWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderWorkspaceFormError(w, r, err, "register")
		return
	}
	if err := requireHumanActor(s.cfg.Actor); err != nil {
		s.renderWorkspaceFormError(w, r, err, "register")
		return
	}
	grant, err := s.resolvePathGrant(r, app.PathGrantRegister, false)
	if err != nil {
		s.renderWorkspaceFormError(w, r, err, "register")
		return
	}
	rec, err := s.application.Register(r.Context(), app.RegisterOptions{Root: grant.Path})
	if err != nil {
		s.renderWorkspaceFormError(w, r, err, "register")
		return
	}
	http.Redirect(w, r, "/w/"+url.PathEscape(rec.WorkspaceID)+"?flash="+url.QueryEscape("registered "+workspaceTitle(rec)), http.StatusSeeOther)
}

func (s *HomeServer) handlePreviewDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderWorkspaceFormError(w, r, err, previewIntent(r))
		return
	}
	if err := requireHumanActor(s.cfg.Actor); err != nil {
		s.renderWorkspaceFormError(w, r, err, previewIntent(r))
		return
	}
	purpose := strings.TrimSpace(r.Form.Get("purpose"))
	if purpose == "" {
		purpose = app.PathGrantInit
	}
	grant, err := s.application.PreviewDirectoryGrant(r.Context(), r.Form.Get("directory"), purpose)
	if err != nil {
		s.renderWorkspaceFormError(w, r, err, previewIntent(r))
		return
	}
	page := s.buildHomeListing(r)
	page.Form = r.Form
	page.ShowInit = purpose == app.PathGrantInit
	page.ShowFind = purpose == app.PathGrantRegister
	page.PendingGrant = &grant
	s.decorateHomeCreate(r, &page)
	s.renderHome(w, r, page, http.StatusOK)
}

func previewIntent(r *http.Request) string {
	if strings.TrimSpace(r.Form.Get("purpose")) == app.PathGrantRegister {
		return "register"
	}
	return "init"
}

func requireHumanActor(actor contracts.Actor) error {
	if !actor.IsValid() || !strings.HasPrefix(string(actor), "human:") {
		return apperr.New(apperr.CodePermissionDenied, "directory authorization requires a human session actor")
	}
	return nil
}

func (s *HomeServer) resolvePathGrant(r *http.Request, purpose string, create bool) (app.PathGrant, error) {
	grantID := strings.TrimSpace(r.Form.Get("grant_id"))
	rootRef := strings.TrimSpace(r.Form.Get("root_ref"))
	folder := strings.TrimSpace(r.Form.Get("folder"))
	if grantID == "" && rootRef == "" {
		if purpose == app.PathGrantInit {
			return app.PathGrant{}, apperr.New(apperr.CodeInvalidInput, "grant_id is required; the browser cannot invent filesystem paths")
		}
		return app.PathGrant{}, apperr.New(apperr.CodeInvalidInput, "grant_id is required")
	}
	if grantID != "" {
		pending, err := s.application.LookupPendingGrant(grantID)
		if err == nil && pending.Source == app.PathGrantSourceHome {
			confirm := strings.TrimSpace(r.Form.Get("confirm_path"))
			if confirm == "" || confirm != pending.Path {
				return app.PathGrant{}, apperr.New(apperr.CodeInvalidInput, "confirm the exact directory shown before initializing")
			}
		} else if err != nil && !strings.Contains(strings.ToLower(err.Error()), "missing") {
			return app.PathGrant{}, err
		}
		return s.application.ConsumeGrantFor(r.Context(), grantID, purpose)
	}
	target, err := s.application.ResolveAuthorizedDir(rootRef, folder, create)
	if err != nil {
		return app.PathGrant{}, err
	}
	if purpose == app.PathGrantInit {
		if err := app.RefuseNestedWorkspaceInit(target); err != nil {
			return app.PathGrant{}, err
		}
	}
	grant, err := s.application.GrantPath(r.Context(), target, purpose)
	if err != nil {
		return app.PathGrant{}, err
	}
	return s.application.ConsumeGrantFor(r.Context(), grant.ID, purpose)
}

func (s *HomeServer) renderWorkspaceFormError(w http.ResponseWriter, r *http.Request, err error, intent string) {
	page := s.buildHomeListing(r)
	page.Error = err.Error()
	page.Form = r.Form
	page.ShowInit = intent == "init" || page.ShowInit
	page.ShowFind = intent == "register" || page.ShowFind
	if id := strings.TrimSpace(r.Form.Get("grant_id")); id != "" {
		if grant, lookupErr := s.application.LookupPendingGrant(id); lookupErr == nil {
			page.PendingGrant = &grant
		}
	}
	s.decorateHomeCreate(r, &page)
	s.renderHome(w, r, page, statusForError(err))
}

func (s *HomeServer) handleRepairWorkspace(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.Form.Get("workspace_id"))
	action := app.RepairAction(strings.TrimSpace(r.Form.Get("action")))
	opts := app.RepairOptions{WorkspaceID: id, Action: action}
	switch action {
	case app.RepairUpdatePath, app.RepairForkCopy:
		grantID := strings.TrimSpace(r.Form.Get("grant_id"))
		if grantID == "" {
			http.Error(w, "grant_id is required; the browser cannot invent filesystem paths", http.StatusBadRequest)
			return
		}
		grant, err := s.application.ConsumeGrantFor(r.Context(), grantID, app.PathGrantRepair)
		if err != nil {
			http.Error(w, err.Error(), statusForError(err))
			return
		}
		opts.NewPath = grant.Path
	case app.RepairHide, app.RepairUnhide, app.RepairRemovePointer:
	default:
		http.Error(w, "unknown repair action", http.StatusBadRequest)
		return
	}
	result, err := s.application.Repair(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	flash := repairFlash(action, result)
	next := "/"
	if action != app.RepairRemovePointer && strings.TrimSpace(result.Record.WorkspaceID) != "" {
		next = "/w/" + url.PathEscape(result.Record.WorkspaceID)
	}
	if action == app.RepairHide {
		next = "/"
	}
	http.Redirect(w, r, next+"?flash="+url.QueryEscape(flash), http.StatusSeeOther)
}

func (s *HomeServer) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	patch := app.MachineSettingsPatch{}
	if raw, ok := formBool(r.Form, "auto_register"); ok {
		patch.AutoRegister = &raw
	}
	if raw, ok := formBool(r.Form, "agents_auto_install"); ok {
		patch.Agents = &app.AgentSettings{AutoInstall: raw}
	}
	if raw, ok := formBool(r.Form, "local_checkpoints"); ok {
		patch.LocalCheckpoints = &raw
	}
	if raw, ok := formBool(r.Form, "open_home"); ok {
		patch.Browser = &app.BrowserSettings{OpenHome: raw}
	}
	if raw, ok := formBool(r.Form, "default_project"); ok {
		patch.DefaultProject = &raw
	}
	if raw, ok := formBool(r.Form, "show_hidden"); ok {
		patch.Home = &app.HomeSettings{ShowHidden: raw}
	}
	if mode := app.GitMode(strings.TrimSpace(r.Form.Get("git_mode"))); mode.IsValid() && mode != "" {
		normalized := mode.Normalized()
		patch.GitMode = &normalized
	}
	if _, err := s.application.UpdateSettings(r.Context(), patch); err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	http.Redirect(w, r, "/settings?flash="+url.QueryEscape("settings saved"), http.StatusSeeOther)
}

func (s *HomeServer) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/w/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	ws, err := s.bind(r, id)
	if err != nil {
		s.renderUnavailable(w, r, id, err)
		return
	}
	if len(parts) == 1 {
		s.renderWorkspaceOverview(w, r, ws)
		return
	}
	switch parts[1] {
	case "projects":
		key := ""
		if len(parts) > 2 {
			key = parts[2]
		}
		if key == "" {
			s.renderWorkspaceOverview(w, r, ws)
			return
		}
		inner := s.innerServer(ws, key)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/board"
		q := r2.URL.Query()
		q.Set("project", key)
		r2.URL.RawQuery = q.Encode()
		inner.handleBoard(w, r2)
	case "activity":
		s.renderWorkspaceActivity(w, r, ws)
	case "backup":
		s.renderWorkspaceBackup(w, r, ws)
	case "actions":
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		project := s.resolveWorkspaceProject(r, ws, parts)
		inner := s.innerServer(ws, project)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/actions/" + strings.Join(parts[2:], "/")
		if project != "" {
			q := r2.URL.Query()
			q.Set("project", project)
			r2.URL.RawQuery = q.Encode()
		}
		if parts[2] == "projects" && len(parts) > 3 && parts[3] == "create" {
			inner.handleCreateProject(w, r2)
			return
		}
		if parts[2] == "agents" && len(parts) > 3 && parts[3] == "create" {
			inner.handleCreateAgent(w, r2)
			return
		}
		if parts[2] == "tickets" && len(parts) > 3 && parts[3] == "create" {
			inner.handleCreateTicket(w, r2)
			return
		}
		if parts[2] == "schedule" {
			inner.handleScheduleAction(w, r2)
			return
		}
		inner.handleTicketAction(w, r2)
	case "schedule":
		project := s.resolveWorkspaceProject(r, ws, parts)
		inner := s.innerServer(ws, project)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/schedule"
		if project != "" {
			q := r2.URL.Query()
			q.Set("project", project)
			r2.URL.RawQuery = q.Encode()
		}
		inner.handleSchedule(w, r2)
	default:
		http.NotFound(w, r)
	}
}

func (s *HomeServer) renderWorkspaceOverview(w http.ResponseWriter, r *http.Request, ws *app.Workspace) {
	inner := s.innerServer(ws, "")
	page, err := inner.buildWelcomePage(r.Context(), r)
	home := s.workspacePage(r, ws, "workspace")
	home.Projects = page.Projects
	home.Recent = page.Recent
	home.ShowNewProject = r.URL.Query().Get("new_project") == "1"
	if err != nil {
		home.Error = err.Error()
	}
	if health, err := ws.Queries.BackupHealth(r.Context()); err == nil {
		home.Backup = &health
		home.BackupView = backupView(health, s.cfg.Clock)
	}
	s.renderHome(w, r, home, http.StatusOK)
}

func (s *HomeServer) renderWorkspaceActivity(w http.ResponseWriter, r *http.Request, ws *app.Workspace) {
	inner := s.innerServer(ws, "")
	page, err := inner.buildWelcomePage(r.Context(), r)
	home := s.workspacePage(r, ws, "activity")
	home.Projects = page.Projects
	home.Recent = page.Recent
	if err != nil {
		home.Error = err.Error()
	}
	s.renderHome(w, r, home, http.StatusOK)
}

func (s *HomeServer) renderWorkspaceBackup(w http.ResponseWriter, r *http.Request, ws *app.Workspace) {
	home := s.workspacePage(r, ws, "backup")
	if health, err := ws.Queries.BackupHealth(r.Context()); err == nil {
		home.Backup = &health
		home.BackupView = backupView(health, s.cfg.Clock)
	} else {
		home.Error = err.Error()
	}
	s.renderHome(w, r, home, http.StatusOK)
}

func (s *HomeServer) renderUnavailable(w http.ResponseWriter, r *http.Request, id string, bindErr error) {
	page := s.pageBase("unavailable")
	page.WorkspaceID = id
	page.HomePath = "/w/" + id
	page.Error = bindErr.Error()
	page.Settings = s.application.Settings()
	listed, err := s.application.ListWorkspaces(r.Context(), app.ListOptions{IncludeHidden: true})
	if err == nil {
		for _, rec := range listed {
			if rec.WorkspaceID == id {
				page.Workspaces = []app.WorkspaceRecord{rec}
				page.DisplayName = workspaceTitle(rec)
				page.Health = rec.Health
				page.HealthLabel = healthLabel(rec.Health)
				page.HealthDetail = rec.HealthDetail
				page.Path = rec.Path
				page.Location = workspaceLocation(rec)
				page.Visibility = rec.Visibility
				break
			}
		}
	}
	if page.DisplayName == "" {
		page.DisplayName = id
		page.Health = app.HealthUnavailable
		page.HealthLabel = healthLabel(app.HealthUnavailable)
	}
	page.FindHits = s.discoveryRepairGrants(r)
	page.FindHits = append(page.FindHits, pendingGrantHits(s.application.ListPendingGrants(), app.PathGrantRepair)...)
	status := statusForError(bindErr)
	if status == http.StatusInternalServerError && apperr.CodeOf(bindErr) == apperr.CodeRepairNeeded {
		status = http.StatusOK
	}
	if status == http.StatusNotFound {
		status = http.StatusOK
	}
	s.renderHome(w, r, page, status)
}

func (s *HomeServer) workspacePage(r *http.Request, ws *app.Workspace, pageName string) HomePage {
	home := s.pageBase(pageName)
	home.Workspace = ws.ID
	home.WorkspaceID = ws.ID
	home.Path = ws.Root
	home.HomePath = "/w/" + ws.ID
	home.BoardPath = "/w/" + ws.ID
	home.ActionPrefix = "/w/" + ws.ID
	home.Flash = r.URL.Query().Get("flash")
	home.Settings = s.application.Settings()
	listed, err := s.application.ListWorkspaces(r.Context(), app.ListOptions{IncludeHidden: true})
	if err == nil {
		for _, rec := range listed {
			if rec.WorkspaceID == ws.ID {
				home.DisplayName = workspaceTitle(rec)
				home.Health = rec.Health
				home.HealthLabel = healthLabel(rec.Health)
				home.Location = workspaceLocation(rec)
				home.Visibility = rec.Visibility
				home.Workspaces = listed
				break
			}
		}
	}
	if home.DisplayName == "" {
		home.DisplayName = workspaceTitle(app.WorkspaceRecord{WorkspaceID: ws.ID, Path: ws.Root})
	}
	return home
}

func (s *HomeServer) handleWorkspaceAPI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/workspaces/")
	id, _, _ := strings.Cut(rest, "/")
	if id == "" {
		listed, err := s.application.ListWorkspaces(r.Context(), app.ListOptions{})
		if err != nil {
			http.Error(w, err.Error(), statusForError(err))
			return
		}
		writeJSONHome(w, "atlas_home_workspaces", listed)
		return
	}
	ws, err := s.bind(r, id)
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	inner := s.innerServer(ws, r.URL.Query().Get("project"))
	r2 := r.Clone(r.Context())
	if strings.Contains(rest, "/board") {
		r2.URL.Path = "/api/board"
		inner.handleBoardAPI(w, r2)
		return
	}
	writeJSONHome(w, "atlas_home_workspace", map[string]any{"id": ws.ID, "name": workspaceTitle(app.WorkspaceRecord{WorkspaceID: ws.ID, Path: ws.Root})})
}

func writeJSONHome(w http.ResponseWriter, kind string, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"kind": kind, "payload": payload})
}

func (s *HomeServer) decorateWorkspaceRows(r *http.Request, listed []app.WorkspaceRecord, attention []app.AttentionItem) []HomeWorkspaceRow {
	counts := map[string]int{}
	for _, item := range attention {
		if item.TicketID != "" {
			counts[item.WorkspaceID]++
		} else if item.Health != "" && item.Health != app.HealthAvailable {
			counts[item.WorkspaceID]++
		}
	}
	rows := make([]HomeWorkspaceRow, 0, len(listed))
	for _, rec := range listed {
		row := HomeWorkspaceRow{
			Record:      rec,
			Title:       workspaceTitle(rec),
			Location:    workspaceLocation(rec),
			HealthLabel: healthLabel(rec.Health),
			Attention:   counts[rec.WorkspaceID],
			Available:   rec.Health == app.HealthAvailable,
			Hidden:      rec.Visibility == app.VisibilityHidden,
		}
		if row.Available {
			if ws, err := s.bind(r, rec.WorkspaceID); err == nil {
				if rollups, err := ws.Queries.ProjectRollups(r.Context()); err == nil {
					row.Projects = len(rollups)
					for _, rollup := range rollups {
						row.Blocked += rollup.Blocked
					}
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *HomeServer) discoveryGrants(r *http.Request, forInit bool) []HomeGrantHit {
	settings := s.application.Settings()
	if !settings.Discovery.Enabled || len(settings.Discovery.Roots) == 0 {
		return nil
	}
	hits, err := s.application.Discover(r.Context(), app.DiscoverOptions{})
	if err != nil {
		return nil
	}
	out := make([]HomeGrantHit, 0, len(hits))
	for _, hit := range hits {
		purpose := app.PathGrantRegister
		if forInit && !hit.Registered && strings.TrimSpace(hit.WorkspaceID) == "" {
			purpose = app.PathGrantInit
		}
		if !forInit && (hit.Registered || strings.TrimSpace(hit.WorkspaceID) == "") {
			continue
		}
		if forInit && hit.Registered {
			continue
		}
		grant, err := s.application.GrantPath(r.Context(), hit.Path, purpose)
		if err != nil {
			continue
		}
		out = append(out, HomeGrantHit{
			GrantID:     grant.ID,
			Purpose:     purpose,
			Title:       firstNonEmpty(hit.DisplayName, filepath.Base(hit.Path)),
			Location:    filepath.Base(hit.Path),
			WorkspaceID: hit.WorkspaceID,
			Registered:  hit.Registered,
		})
	}
	return out
}

func (s *HomeServer) discoveryRepairGrants(r *http.Request) []HomeGrantHit {
	settings := s.application.Settings()
	if !settings.Discovery.Enabled || len(settings.Discovery.Roots) == 0 {
		return nil
	}
	hits, err := s.application.Discover(r.Context(), app.DiscoverOptions{})
	if err != nil {
		return nil
	}
	out := make([]HomeGrantHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Registered {
			continue
		}
		grant, err := s.application.GrantPath(r.Context(), hit.Path, app.PathGrantRepair)
		if err != nil {
			continue
		}
		out = append(out, HomeGrantHit{
			GrantID:     grant.ID,
			Purpose:     app.PathGrantRepair,
			Title:       firstNonEmpty(hit.DisplayName, filepath.Base(hit.Path)),
			Location:    filepath.Base(hit.Path),
			WorkspaceID: hit.WorkspaceID,
		})
	}
	return out
}

func pendingGrantHits(grants []app.PathGrant, purpose string) []HomeGrantHit {
	out := make([]HomeGrantHit, 0, len(grants))
	for _, grant := range grants {
		// Home previews must return through their exact-directory confirmation form.
		if grant.Source == app.PathGrantSourceHome {
			continue
		}
		if purpose != "" && grant.Purpose != purpose {
			continue
		}
		out = append(out, HomeGrantHit{
			GrantID:  grant.ID,
			Purpose:  grant.Purpose,
			Title:    filepath.Base(grant.Path),
			Location: filepath.Base(grant.Path),
		})
	}
	return out
}

func homeBoardPath(workspaceID, project string) string {
	base := "/w/" + strings.TrimSpace(workspaceID)
	project = strings.TrimSpace(project)
	if project == "" {
		return base
	}
	return base + "/projects/" + project
}

func (s *HomeServer) resolveWorkspaceProject(r *http.Request, ws *app.Workspace, parts []string) string {
	if ws == nil {
		return ""
	}
	allowed := map[string]struct{}{}
	if projects, err := ws.Queries.Projects.ListProjects(r.Context()); err == nil {
		for _, project := range projects {
			allowed[project.Key] = struct{}{}
		}
	}
	accept := func(raw string) string {
		key := strings.TrimSpace(raw)
		if !contracts.IsValidProjectKey(key) {
			return ""
		}
		if len(allowed) == 0 {
			return key
		}
		if _, ok := allowed[key]; ok {
			return key
		}
		return ""
	}
	if len(parts) > 3 && parts[2] == "tickets" {
		id := parts[3]
		if id != "create" && id != "bulk" && contracts.IsValidTicketID(id) {
			if ticket, err := ws.Actions.Tickets.GetTicket(r.Context(), id); err == nil {
				if key := accept(ticket.Project); key != "" {
					return key
				}
			}
		}
		if id == "bulk" && r.Form != nil {
			ids := r.Form["ticket_id"]
			if len(ids) == 0 {
				ids = splitCSV(r.Form.Get("ticket_ids"))
			}
			for _, ticketID := range ids {
				if !contracts.IsValidTicketID(ticketID) {
					continue
				}
				if ticket, err := ws.Actions.Tickets.GetTicket(r.Context(), ticketID); err == nil {
					if key := accept(ticket.Project); key != "" {
						return key
					}
				}
			}
		}
	}
	if r.Form != nil {
		if ticketID := strings.TrimSpace(r.Form.Get("ticket_id")); contracts.IsValidTicketID(ticketID) && ticketID != "create" && ticketID != "bulk" {
			if ticket, err := ws.Actions.Tickets.GetTicket(r.Context(), ticketID); err == nil {
				if key := accept(ticket.Project); key != "" {
					return key
				}
			}
		}
	}
	if key := accept(r.URL.Query().Get("project")); key != "" {
		return key
	}
	if r.Form != nil {
		if key := accept(r.Form.Get("return_project")); key != "" {
			return key
		}
		if key := accept(r.Form.Get("project")); key != "" {
			return key
		}
	}
	if len(allowed) == 1 {
		for key := range allowed {
			return key
		}
	}
	return ""
}

func workspaceTitle(rec app.WorkspaceRecord) string {
	if name := strings.TrimSpace(rec.DisplayName); name != "" {
		return name
	}
	if loc := workspaceLocation(rec); loc != "" && loc != rec.WorkspaceID {
		return loc
	}
	return rec.WorkspaceID
}

func workspaceLocation(rec app.WorkspaceRecord) string {
	clean := filepath.Clean(strings.TrimSpace(rec.Path))
	if clean == "" || clean == "." {
		return ""
	}
	base := filepath.Base(clean)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func healthLabel(h app.Health) string {
	switch h {
	case app.HealthAvailable:
		return "Available"
	case app.HealthUnavailable:
		return "Unavailable"
	case app.HealthMoved:
		return "Moved"
	case app.HealthCopiedIdentityConflict:
		return "Copy conflict"
	case app.HealthReplacedPath:
		return "Replaced"
	case app.HealthPermissionDenied:
		return "No permission"
	case app.HealthSchemaUpgradeRequired:
		return "Needs upgrade"
	case app.HealthCorruptIdentity:
		return "Corrupt identity"
	case app.HealthDisabled:
		return "Hidden"
	default:
		if h == "" {
			return "Unknown"
		}
		return strings.ReplaceAll(string(h), "_", " ")
	}
}

func homeSort(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "recent":
		return "recent"
	case "name":
		return "name"
	default:
		return "attention"
	}
}

func sortHomeRows(rows []HomeWorkspaceRow, mode string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch mode {
		case "recent":
			if !a.Record.LastSeenAt.Equal(b.Record.LastSeenAt) {
				return a.Record.LastSeenAt.After(b.Record.LastSeenAt)
			}
		case "name":
		default:
			if a.Attention != b.Attention {
				return a.Attention > b.Attention
			}
			if a.Blocked != b.Blocked {
				return a.Blocked > b.Blocked
			}
		}
		if a.Title != b.Title {
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		}
		return a.Record.WorkspaceID < b.Record.WorkspaceID
	})
}

func backupView(health service.BackupHealthSummary, now func() time.Time) HomeBackupView {
	view := HomeBackupView{Present: true}
	if health.LastLocalCheckpointAt.IsZero() && health.LastLocalCheckpointID == "" {
		view.LocalLabel = "No local checkpoint yet"
		view.LocalDetail = "Atlas keeps a machine-local snapshot of canonical workspace files. Editing still works offline."
	} else {
		view.LocalLabel = "Local checkpoint"
		when := health.LastLocalCheckpointAt
		if when.IsZero() {
			view.LocalDetail = "A local snapshot exists."
		} else {
			view.LocalDetail = "Last snapshot " + relativeTime(when, now()) + "."
		}
	}
	if health.VerifiedRemote && health.LastRemoteCheckpointID != "" {
		view.OffDeviceVerified = true
		view.OffDeviceLabel = "Off-device copy verified"
		if health.LastRemoteVerifiedAt.IsZero() {
			view.OffDeviceDetail = "An independently fetched replica matched the local snapshot."
		} else {
			view.OffDeviceDetail = "Verified " + relativeTime(health.LastRemoteVerifiedAt, now()) + ". Push is not proof; Atlas re-fetches the ref."
		}
	} else if health.LastRemoteCheckpointID != "" {
		view.OffDeviceLabel = "Off-device copy not verified"
		view.OffDeviceDetail = "A remote target is configured, but Atlas has not independently verified the replica."
	} else {
		view.OffDeviceLabel = "No off-device copy"
		view.OffDeviceDetail = "Only this machine has a checkpoint until a remote target is configured and verified."
	}
	if health.UnbackedEventCount > 0 {
		view.PendingLabel = "Unbacked events waiting for the next local snapshot"
	}
	return view
}

func relativeTime(at, now time.Time) string {
	if at.IsZero() {
		return "unknown"
	}
	delta := now.UTC().Sub(at.UTC())
	if delta < 0 {
		delta = -delta
	}
	switch {
	case delta < 90*time.Second:
		return "just now"
	case delta < 90*time.Minute:
		return strconv.Itoa(int(delta/time.Minute)) + "m ago"
	case delta < 36*time.Hour:
		return strconv.Itoa(int(delta/time.Hour)) + "h ago"
	default:
		return at.Local().Format("Jan 2, 15:04")
	}
}

func repairFlash(action app.RepairAction, result app.RepairResult) string {
	switch action {
	case app.RepairHide:
		return "Hidden from Home. The workspace files and backups are unchanged."
	case app.RepairUnhide:
		return "Visible on Home again."
	case app.RepairRemovePointer:
		return "Removed the Home pointer. Project files, tickets, and backup history stay on disk."
	case app.RepairUpdatePath:
		return "Repaired the workspace location."
	case app.RepairForkCopy:
		if result.ForkedID != "" {
			return "Registered the copy as a new workspace. The original pointer is unchanged."
		}
		return "Registered the copy as a new workspace."
	default:
		return "Workspace updated."
	}
}

func formBool(form url.Values, key string) (bool, bool) {
	if _, ok := form[key]; !ok {
		return false, false
	}
	raw := strings.ToLower(strings.TrimSpace(form.Get(key)))
	switch raw {
	case "1", "true", "on", "yes":
		return true, true
	case "0", "false", "off", "no", "":
		return false, true
	default:
		return false, false
	}
}

func (p HomePage) SortHref(mode string) string {
	q := url.Values{}
	if mode != "" && mode != "attention" {
		q.Set("sort", mode)
	}
	if p.ShowHidden {
		q.Set("hidden", "1")
	}
	enc := q.Encode()
	if enc == "" {
		return "/"
	}
	return "/?" + enc
}

func (p HomePage) WorkspaceHref() string {
	if p.WorkspaceID == "" {
		return "/"
	}
	return "/w/" + url.PathEscape(p.WorkspaceID)
}

func (p HomePage) ProjectHref(key string) string {
	if p.WorkspaceID == "" {
		return "/board?project=" + url.QueryEscape(key)
	}
	return "/w/" + url.PathEscape(p.WorkspaceID) + "/projects/" + url.PathEscape(key)
}

func (p HomePage) FormValue(name, fallback string) string {
	if p.Form != nil && p.Form.Has(name) {
		return p.Form.Get(name)
	}
	return fallback
}

func (p HomePage) BrowseHref(ref, rel string) string {
	q := url.Values{}
	if p.ShowInit {
		q.Set("init", "1")
	} else {
		q.Set("find", "1")
	}
	if ref != "" && ref != app.BoardsRootRef {
		q.Set("root", ref)
	} else if ref == app.BoardsRootRef {
		q.Set("root", ref)
	}
	if strings.TrimSpace(rel) != "" {
		q.Set("dir", rel)
	}
	return "/?" + q.Encode()
}

func (p HomePage) ParentBrowseRel() string {
	rel := strings.TrimSpace(p.BrowseRel)
	if rel == "" {
		return ""
	}
	parent := filepath.Dir(rel)
	if parent == "." {
		return ""
	}
	return parent
}
