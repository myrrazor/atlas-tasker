package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestBoardTemplateQuotesTicketAndActionURLs(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	page := BoardPage{
		Page:          "board",
		Workspace:     "demo",
		Host:          "127.0.0.1",
		Actor:         "human:owner",
		CSRFToken:     "csrf",
		HomePath:      "/w/ws1",
		BoardPath:     "/w/ws1/projects/DEMO",
		ActionPrefix:  "/w/ws1",
		SchedulePath:  "/w/ws1/schedule",
		NewTicketPath: "/w/ws1/projects/DEMO?new=1",
		Project:       "DEMO",
		Columns: []BoardColumn{{
			Status: contracts.StatusReady,
			Count:  1,
			Tickets: []TicketCard{{
				Ticket:      contracts.TicketSnapshot{ID: "DEMO-1", Title: "Ship home", Priority: contracts.PriorityHigh, Assignee: "human:owner"},
				BoardStatus: contracts.StatusReady,
			}},
		}, {
			Status: contracts.StatusCanceled,
			Count:  0,
		}},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", page); err != nil {
		t.Fatalf("render board: %v", err)
	}
	body := buf.String()
	for _, broken := range []string{
		`"/board"?ticket=`,
		`"/board"?column=`,
		`"/board"?new=`,
		`projects/DEMO"?ticket=`,
		`projects/DEMO"?column=`,
		`/actions/"tickets`,
		`href="{{.BoardPath}}"`,
	} {
		if strings.Contains(body, broken) {
			t.Fatalf("board still has broken URL substitution %q", broken)
		}
	}
	if !strings.Contains(body, `href="?ticket=DEMO-1"`) && !strings.Contains(body, `href="/w/ws1/projects/DEMO?ticket=DEMO-1"`) {
		t.Fatalf("card href missing ticket:\n%s", excerpt(body, "ticket-card"))
	}
	if !strings.Contains(body, `action="/w/ws1/actions/tickets/create"`) && !strings.Contains(body, `NewTicketPath`) {
		// create form only renders when ShowNew; close and filters must still be quoted
	}
	if !strings.Contains(body, `class="filters-shell"`) {
		t.Fatal("filters shell missing")
	}
	if strings.Contains(body, `<details class="filters-shell" open>`) {
		t.Fatal("advanced filters must be collapsed by default")
	}
	if !strings.Contains(body, `class="canceled-lane`) {
		t.Fatal("canceled lane should be behind a disclosure, not a default column")
	}
	if !strings.Contains(body, `class="backup-health"`) && page.BackupHealth != nil {
		t.Fatal("backup health missing")
	}
	if !strings.Contains(body, `class="ticket-card"`) || !strings.Contains(body, "Ship home") {
		t.Fatal("title-primary card missing")
	}
	if strings.Contains(body, `class="detail-drawer"`) || strings.Contains(body, "No ticket selected") {
		t.Fatal("unselected board must not render an empty detail column")
	}
	if !strings.Contains(body, `class="board-toolbar"`) {
		t.Fatal("filters and backup should share one toolbar")
	}
	if !strings.Contains(body, `href="/w/ws1/projects/DEMO?ticket=" class="close-button"`) && strings.Contains(body, `class="close-button"`) {
		t.Fatalf("close button href is not prefix-aware:\n%s", excerpt(body, "close-button"))
	}
}

func TestLegacyBoardTemplateKeepsQuotedBoardPaths(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	page := BoardPage{
		Page:          "board",
		Workspace:     "workspace",
		Host:          "127.0.0.1",
		Actor:         "human:owner",
		CSRFToken:     "csrf",
		HomePath:      "/",
		BoardPath:     "/board",
		ActionPrefix:  "",
		SchedulePath:  "/schedule",
		NewTicketPath: "/board?new=1",
		ShowNew:       true,
		Project:       "WEB",
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", page); err != nil {
		t.Fatalf("render: %v", err)
	}
	body := buf.String()
	if strings.Contains(body, `/actions/"tickets`) {
		t.Fatal("legacy create action still splits the path")
	}
	if !strings.Contains(body, `action="/actions/tickets/create"`) {
		t.Fatalf("legacy create form action: %s", excerpt(body, "tickets/create"))
	}
	if !strings.Contains(body, `href="/board?ticket=" class="close-button"`) {
		t.Fatalf("legacy close button: %s", excerpt(body, "close-button"))
	}
}

func TestHomePrefersDisplayNameOverRawPath(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	listed, err := application.ListWorkspaces(context.Background(), app.ListOptions{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %v %#v", err, listed)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, listed[0].WorkspaceID) {
		t.Fatalf("home must still carry workspace id for links:\n%s", body)
	}
	if strings.Count(body, listed[0].Path) > 2 {
		t.Fatalf("raw machine path leaked as primary home content:\n%s", body)
	}
	if !strings.Contains(body, "Needs attention") || !strings.Contains(body, "Atlas Home") {
		t.Fatal("home missing attention/overview copy")
	}
	if strings.Contains(body, "Attention, not inventory") {
		t.Fatal("home still uses the slogan copy")
	}
	if !strings.Contains(body, `href="/?sort=name"`) && !strings.Contains(body, `href="/"`) {
		t.Fatal("sort navigation missing")
	}
	if strings.Contains(body, `<input name="path"`) {
		t.Fatal("home must not take an arbitrary filesystem path")
	}
}

func TestHomeBoardUsesPrefixedActionsAndCollapsedFilters(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	listed, _ := application.ListWorkspaces(context.Background(), app.ListOptions{})
	id := listed[0].WorkspaceID
	ws, err := application.Hub().Bind(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := ws.Queries.Projects.ListProjects(context.Background())
	if err != nil || len(projects) == 0 {
		t.Fatalf("projects: %v %#v", err, projects)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/w/"+id+"/projects/"+projects[0].Key, nil)
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"/board"?ticket=`) || strings.Contains(body, `/actions/"tickets`) {
		t.Fatalf("home board has broken URL quoting:\n%s", excerpt(body, "ticket"))
	}
	if strings.Contains(body, `<details class="filters-shell" open>`) {
		t.Fatal("home board opened advanced filters by default")
	}
	if !strings.Contains(body, `class="canceled-lane`) {
		t.Fatal("canceled column should not occupy a default lane")
	}
	if !strings.Contains(body, `data-board-path="/w/`+id+`/projects/`+projects[0].Key+`"`) {
		t.Fatalf("board path prefix missing:\n%s", excerpt(body, "data-board-path"))
	}
}

func TestHomeRepairRemovePointerPreservesCopy(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	listed, _ := application.ListWorkspaces(context.Background(), app.ListOptions{})
	id := listed[0].WorkspaceID
	form := url.Values{
		"csrf_token":   {"home-csrf"},
		"workspace_id": {id},
		"action":       {"remove_pointer"},
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/repair", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "Removed+the+Home+pointer") && !strings.Contains(loc, "stay+on+disk") {
		t.Fatalf("preservation copy missing from redirect %q", loc)
	}
	left, err := application.ListWorkspaces(context.Background(), app.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("pointer still registered: %#v", left)
	}
}

func TestHomeSettingsToggleDoesNotAcceptDiscoveryRoots(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	form := url.Values{
		"csrf_token":          {"home-csrf"},
		"auto_register":       {"off"},
		"agents_auto_install": {"off"},
		"local_checkpoints":   {"on"},
		"open_home":           {"on"},
		"default_project":     {"on"},
		"show_hidden":         {"on"},
		"git_mode":            {"private"},
		"discovery_roots":     {"/tmp/nope"},
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	got := application.Settings()
	if got.AutoRegister || got.Agents.AutoInstall || got.GitMode != app.GitModePrivate || !got.Home.ShowHidden {
		t.Fatalf("settings not applied: %#v", got)
	}
	if len(got.Discovery.Roots) != 0 {
		t.Fatalf("browser must not widen discovery roots: %#v", got.Discovery.Roots)
	}
}

func TestWelcomeTemplateUsesPrefixAwareBoardLinks(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	page := WelcomePage{
		Page:         "welcome",
		Workspace:    "demo",
		Host:         "127.0.0.1",
		HomePath:     "/",
		BoardPath:    "/board",
		ActionPrefix: "",
		SchedulePath: "/schedule",
		Projects: []ProjectRow{{
			Project: contracts.Project{Key: "WEB", Name: "Web"},
			Active:  1,
		}},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", page); err != nil {
		t.Fatalf("welcome: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, `href="/board?project=WEB"`) {
		t.Fatalf("welcome lost board project links:\n%s", body)
	}
	if !strings.Contains(body, `action="/actions/projects/create"`) {
		t.Fatalf("welcome create action: %s", excerpt(body, "projects/create"))
	}
}

func TestNotesTextareaRoundTripsThroughEdit(t *testing.T) {
	h := newWebHarness(t, false)
	open := h.doAuthed(t, http.MethodGet, "/board?ticket="+h.ticketID, "", nil)
	if !strings.Contains(open.body, `name="notes"`) {
		t.Fatalf("edit form missing notes textarea:\n%s", excerpt(open.body, "notes"))
	}
	current, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":        {"test-csrf"},
		"title":             {current.Title},
		"notes":             {"durable operator notes"},
		"reason":            {"web edit ticket"},
		"expected_revision": {TicketRevision(current)},
	}
	saved := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if saved.code != http.StatusSeeOther {
		t.Fatalf("save status=%d body=%s", saved.code, saved.body)
	}
	got, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes != "durable operator notes" {
		t.Fatalf("notes=%q", got.Notes)
	}
	again := h.doAuthed(t, http.MethodGet, "/board?ticket="+h.ticketID, "", nil)
	if !strings.Contains(again.body, "durable operator notes") {
		t.Fatalf("saved notes missing from drawer:\n%s", excerpt(again.body, "notes"))
	}
}

func TestBackupCompactAvoidsEnabledCheckpointNonsense(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if strings.Contains(res.body, "Last local checkpoint Enabled") {
		t.Fatal("backup summary still treats a checkpoint as an on/off flag")
	}
	if !strings.Contains(res.body, "No local checkpoint") && !strings.Contains(res.body, "Local checkpoint") {
		t.Fatalf("backup summary missing truthful local/off-device line:\n%s", excerpt(res.body, "backup-compact"))
	}
	if !strings.Contains(res.body, "Unbacked events") {
		t.Fatal("backup details must keep unbacked events")
	}
}

func TestBoardCardsCarryRevisionForDrag(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if !strings.Contains(res.body, `data-revision="`) {
		t.Fatalf("cards missing revision token:\n%s", excerpt(res.body, "ticket-card"))
	}
}

func TestHomeInitRejectsRegisterGrant(t *testing.T) {
	srv, application, root := newHomeHarness(t)
	grant, err := application.GrantPath(context.Background(), root, app.PathGrantRegister)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"csrf_token": {"home-csrf"}, "grant_id": {grant.ID}}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/init", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRejectedEditKeepsNotesAndShowsError(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"title":      {""},
		"notes":      {"keep this typed note"},
		"reason":     {"web edit ticket"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, "keep this typed note") || !strings.Contains(res.body, `role="alert"`) {
		t.Fatalf("rejected edit must keep notes and show an error:\n%s", excerpt(res.body, "notes"))
	}
}

func TestHomeEditRedirectKeepsProjectAB(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	listed, err := application.ListWorkspaces(context.Background(), app.ListOptions{})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v %#v", err, listed)
	}
	id := listed[0].WorkspaceID
	ws, err := application.Hub().Bind(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := ws.Queries.Projects.ListProjects(context.Background())
	if err != nil || len(projects) == 0 {
		t.Fatalf("projects: %v %#v", err, projects)
	}
	projectA := projects[0].Key
	projectB := "BETA"
	if err := ws.Actions.CreateProject(context.Background(), contracts.Project{
		Key:           projectB,
		Name:          "Beta",
		CreatedAt:     time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	ticketA := homeSeedTicket(t, ws, projectA, "Alpha work")
	ticketB := homeSeedTicket(t, ws, projectB, "Beta work")
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}

	postEdit := func(project, ticketID, notes string) string {
		t.Helper()
		page := homeGET(t, srv, cookie, "/w/"+id+"/projects/"+project+"?ticket="+ticketID)
		action := htmlFormActionContaining(t, page, "/actions/tickets/"+ticketID+"/edit")
		if strings.Contains(action, "/projects/") {
			t.Fatalf("edit must post to workspace actions, got %s", action)
		}
		form := url.Values{
			"csrf_token":        {"home-csrf"},
			"title":             {htmlInputValue(t, page, "title")},
			"notes":             {notes},
			"reason":            {"web edit ticket"},
			"expected_revision": {htmlInputValue(t, page, "expected_revision")},
		}
		if ret := optionalInputValue(page, "return_project"); ret != "" {
			form.Set("return_project", ret)
		}
		rec := homePOST(t, srv, cookie, action, form)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("edit %s status=%d body=%s", ticketID, rec.Code, rec.Body.String())
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "/w/"+id+"/projects/"+project+"?") {
			t.Fatalf("redirect lost project %s: %s", project, loc)
		}
		if strings.Contains(loc, "/projects/?") || strings.Contains(loc, "/projects?&") {
			t.Fatalf("empty project path: %s", loc)
		}
		if !strings.Contains(loc, "ticket="+ticketID) {
			t.Fatalf("redirect dropped ticket: %s", loc)
		}
		other := projectA
		if project == projectA {
			other = projectB
		}
		if strings.Contains(loc, "/projects/"+other) {
			t.Fatalf("redirect switched projects: %s", loc)
		}
		followed := homeGET(t, srv, cookie, loc)
		if !strings.Contains(followed, notes) {
			t.Fatalf("followed board missing notes %q:\n%s", notes, excerpt(followed, "notes"))
		}
		return loc
	}

	postEdit(projectB, ticketB.ID, "beta notes kept")
	postEdit(projectA, ticketA.ID, "alpha notes kept")

	createPage := homeGET(t, srv, cookie, "/w/"+id+"/projects/"+projectB+"?new=1")
	createAction := htmlFormActionContaining(t, createPage, "/actions/tickets/create")
	createForm := url.Values{
		"csrf_token": {"home-csrf"},
		"project":    {projectB},
		"title":      {"Created on beta board"},
		"type":       {"task"},
		"priority":   {"medium"},
		"reason":     {"web ticket create"},
	}
	if ret := optionalInputValue(createPage, "return_project"); ret != "" {
		createForm.Set("return_project", ret)
	}
	created := homePOST(t, srv, cookie, createAction, createForm)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	createLoc := created.Header().Get("Location")
	if !strings.Contains(createLoc, "/w/"+id+"/projects/"+projectB+"?") {
		t.Fatalf("create redirect lost project: %s", createLoc)
	}

	bulkPage := homeGET(t, srv, cookie, "/w/"+id+"/projects/"+projectB)
	bulkAction := htmlFormActionContaining(t, bulkPage, "/actions/tickets/bulk")
	bulkForm := url.Values{
		"csrf_token":                      {"home-csrf"},
		"kind":                            {"move"},
		"status":                          {string(contracts.StatusInProgress)},
		"ticket_ids":                      {ticketB.ID},
		"confirm":                         {"1"},
		"reason":                          {"web bulk"},
		"expected_revision." + ticketB.ID: {TicketRevision(mustGetTicket(t, ws, ticketB.ID))},
	}
	if ret := optionalInputValue(bulkPage, "return_project"); ret != "" {
		bulkForm.Set("return_project", ret)
	}
	bulked := homePOST(t, srv, cookie, bulkAction, bulkForm)
	if bulked.Code != http.StatusSeeOther {
		t.Fatalf("bulk status=%d body=%s", bulked.Code, bulked.Body.String())
	}
	bulkLoc := bulked.Header().Get("Location")
	if !strings.Contains(bulkLoc, "/w/"+id+"/projects/"+projectB) {
		t.Fatalf("bulk redirect lost project: %s", bulkLoc)
	}
	if strings.Contains(bulkLoc, "ticket=") {
		t.Fatalf("bulk should not invent a ticket: %s", bulkLoc)
	}
}

func TestHomeStaleEditRendersFormWithTypedNotes(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	listed, err := application.ListWorkspaces(context.Background(), app.ListOptions{})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v %#v", err, listed)
	}
	id := listed[0].WorkspaceID
	ws, err := application.Hub().Bind(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := ws.Queries.Projects.ListProjects(context.Background())
	if err != nil || len(projects) == 0 {
		t.Fatal(err)
	}
	ticket := homeSeedTicket(t, ws, projects[0].Key, "Conflict subject")
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	page := homeGET(t, srv, cookie, "/w/"+id+"/projects/"+projects[0].Key+"?ticket="+ticket.ID)
	action := htmlFormActionContaining(t, page, "/actions/tickets/"+ticket.ID+"/edit")
	stale := htmlInputValue(t, page, "expected_revision")
	if _, err := ws.Actions.MutateTrackedTicket(context.Background(), ticket.ID, "human:owner", "agent edit", "agent rewrite", func(current *contracts.TicketSnapshot) error {
		current.Title = "agent won"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":        {"home-csrf"},
		"title":             {"stale client title"},
		"notes":             {"typed notes must survive 409"},
		"reason":            {"web edit ticket"},
		"expected_revision": {stale},
	}
	rec := homePOST(t, srv, cookie, action, form)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "typed notes must survive 409") {
		t.Fatalf("stale home edit dropped notes:\n%s", excerpt(body, "notes"))
	}
	if !strings.Contains(body, ticket.ID) {
		t.Fatalf("stale home edit lost ticket context:\n%s", excerpt(body, ticket.ID))
	}
	if strings.Contains(body, "/projects/?") {
		t.Fatalf("stale error rendered empty project path:\n%s", excerpt(body, "projects"))
	}
}

func homeSeedTicket(t *testing.T, ws *app.Workspace, project, title string) contracts.TicketSnapshot {
	t.Helper()
	now := time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)
	ticket, err := ws.Actions.CreateTrackedTicket(context.Background(), contracts.TicketSnapshot{
		Project:       project,
		Title:         title,
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "home ui fixture")
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func mustGetTicket(t *testing.T, ws *app.Workspace, id string) contracts.TicketSnapshot {
	t.Helper()
	ticket, err := ws.Actions.Tickets.GetTicket(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func homeGET(t *testing.T, srv *HomeServer, cookie *http.Cookie, path string) string {
	t.Helper()
	if strings.HasPrefix(path, "http") {
		u, err := url.Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		path = u.RequestURI()
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func homePOST(t *testing.T, srv *HomeServer, cookie *http.Cookie, action string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	if !strings.HasPrefix(action, "/") {
		t.Fatalf("refusing non-path action %q", action)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+action, strings.NewReader(form.Encode()))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func htmlFormActionContaining(t *testing.T, body, needle string) string {
	t.Helper()
	start := 0
	for {
		formStart := strings.Index(body[start:], "<form")
		if formStart < 0 {
			t.Fatalf("no form action containing %q", needle)
		}
		formStart += start
		formEnd := strings.Index(body[formStart:], "</form>")
		if formEnd < 0 {
			t.Fatal("unterminated form")
		}
		form := body[formStart : formStart+formEnd]
		key := `action="`
		pos := strings.Index(form, key)
		if pos >= 0 {
			pos += len(key)
			end := strings.Index(form[pos:], `"`)
			if end >= 0 {
				action := form[pos : pos+end]
				if strings.Contains(action, needle) {
					return action
				}
			}
		}
		start = formStart + 5
	}
}

func htmlInputValue(t *testing.T, body, name string) string {
	t.Helper()
	if value, ok := inputValue(body, name); ok {
		return value
	}
	t.Fatalf("missing input %s", name)
	return ""
}

func optionalInputValue(body, name string) string {
	value, _ := inputValue(body, name)
	return value
}

func inputValue(body, name string) (string, bool) {
	for _, needle := range []string{
		`name="` + name + `" value="`,
		`name="` + name + `" value='`,
	} {
		start := strings.Index(body, needle)
		if start < 0 {
			continue
		}
		start += len(needle)
		end := strings.IndexAny(body[start:], `"'>`)
		if end < 0 {
			return "", false
		}
		return body[start : start+end], true
	}
	return "", false
}

func TestHomeUIJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required for the home UI javascript regression tests")
	}
	out, err := exec.Command(node, "--test", "testdata/home_ui.test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("home ui js tests: %v\n%s", err, out)
	}
}
