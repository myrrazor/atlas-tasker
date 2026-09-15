package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestHomeCreateBoardUnderAtlasBoards(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	page := homeGET(t, srv, cookie, "/?init=1")
	if !strings.Contains(page, "Create a board") || !strings.Contains(page, "Atlas boards on this computer") {
		t.Fatalf("init dialog missing create path:\n%s", excerpt(page, "Create"))
	}
	if !strings.Contains(page, "Existing directory on this computer") || !strings.Contains(page, "/actions/workspaces/preview-path") {
		t.Fatal("init dialog must offer reviewed absolute-directory selection")
	}
	if !strings.Contains(page, "To connect your coding agents afterward") {
		t.Fatal("init copy must not claim browser init configures editor clients")
	}
	if !strings.Contains(page, `name="root_ref"`) || !strings.Contains(page, `name="folder"`) {
		t.Fatal("create form must choose an authorized root and folder")
	}
	form := url.Values{
		"csrf_token":   {"home-csrf"},
		"root_ref":     {app.BoardsRootRef},
		"folder":       {"widgets"},
		"project_key":  {"OPS"},
		"project_name": {"Operations"},
	}
	rec := homePOST(t, srv, cookie, "/actions/workspaces/init", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("init status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/projects/OPS") {
		t.Fatalf("expected project navigation, got %s", loc)
	}
	listed, err := application.ListWorkspaces(context.Background(), app.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range listed {
		if strings.HasSuffix(filepath.Clean(rec.Path), "widgets") {
			found = true
			if _, err := os.Stat(filepath.Join(rec.Path, "projects", "OPS", "project.md")); err != nil {
				t.Fatalf("project markdown missing: %v", err)
			}
		}
	}
	if !found {
		t.Fatalf("created workspace not registered: %#v", listed)
	}
}

func TestHomeInitPreservesValuesOnError(t *testing.T) {
	srv, _, _ := newHomeHarness(t)
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	form := url.Values{
		"csrf_token":   {"home-csrf"},
		"root_ref":     {app.BoardsRootRef},
		"folder":       {"bad key dir"},
		"project_key":  {"not a key"},
		"project_name": {"Kept name"},
	}
	rec := homePOST(t, srv, cookie, "/actions/workspaces/init", form)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "Kept name") || !strings.Contains(body, "not a key") {
		t.Fatalf("error must keep typed values:\n%s", excerpt(body, "project_key"))
	}
}

func TestHomeInitRejectsRawPathAndWrongPurposeAndReplay(t *testing.T) {
	srv, application, root := newHomeHarness(t)
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	raw := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"path":       {"/tmp/nope"},
	})
	if raw.Code != http.StatusBadRequest || !strings.Contains(raw.Body.String(), "cannot invent filesystem paths") {
		t.Fatalf("raw path status=%d body=%s", raw.Code, raw.Body.String())
	}

	grant, err := application.GrantPath(context.Background(), root, app.PathGrantRegister)
	if err != nil {
		t.Fatal(err)
	}
	wrong := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"grant_id":   {grant.ID},
	})
	if wrong.Code != http.StatusForbidden && wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong purpose status=%d body=%s", wrong.Code, wrong.Body.String())
	}

	initGrant, err := application.GrantPath(context.Background(), root, app.PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	first := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"grant_id":   {initGrant.ID},
	})
	if first.Code != http.StatusSeeOther && first.Code != http.StatusBadRequest && first.Code != http.StatusConflict {
		t.Fatalf("first consume status=%d body=%s", first.Code, first.Body.String())
	}
	replay := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"grant_id":   {initGrant.ID},
	})
	if replay.Code == http.StatusSeeOther {
		t.Fatal("replayed grant must not initialize again")
	}
}

func TestHomeInitRejectsSymlinkGrant(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	realDir := filepath.Join(application.Home(), "real-board")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(application.Home(), "link-board")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	_, err := application.GrantPath(context.Background(), link, app.PathGrantInit)
	if err == nil {
		t.Fatal("granting a symlink must fail")
	}
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	rec := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"root_ref":   {app.BoardsRootRef},
		"folder":     {"../link-board"},
	})
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("escaped symlink folder initialized: %s", rec.Header().Get("Location"))
	}
}

func TestHomeInitCSRFAndReadOnly(t *testing.T) {
	srv, _, _ := newHomeHarness(t)
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	missing := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"root_ref": {app.BoardsRootRef},
		"folder":   {"nope"},
	})
	if missing.Code != http.StatusForbidden {
		t.Fatalf("csrf status=%d body=%s", missing.Code, missing.Body.String())
	}
	_, application, _ := newHomeHarness(t)
	ro, err := NewHomeServer(application, HomeConfig{Host: "127.0.0.1", Port: 7432, Actor: "human:owner", Token: "home-token", CSRF: "home-csrf", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	blocked := homePOST(t, ro, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"root_ref":   {app.BoardsRootRef},
		"folder":     {"nope"},
	})
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("read-only status=%d body=%s", blocked.Code, blocked.Body.String())
	}
}

func TestDuplicateProjectCreateIsConflict(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{"csrf_token": {"test-csrf"}, "key": {"WEB"}, "name": {"Again"}}
	res := h.doAuthed(t, http.MethodPost, "/actions/projects/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusConflict {
		t.Fatalf("duplicate project status=%d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, "already exists") {
		t.Fatalf("duplicate error missing: %s", res.body)
	}
}

func TestTicketClaimReleaseArchiveAndLink(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := context.Background()
	if err := (service.AgentStore{Root: h.root}).SaveAgent(ctx, contracts.AgentProfile{
		AgentID:     "builder-1",
		DisplayName: "Builder",
		Provider:    contracts.AgentProviderCodex,
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	board := h.doAuthed(t, http.MethodGet, "/board?ticket="+h.ticketID, "", nil)
	if !strings.Contains(board.body, `value="agent:builder-1"`) {
		t.Fatalf("registered agent missing from picker:\n%s", excerpt(board.body, "actor-options"))
	}
	if !strings.Contains(board.body, ">Claim<") || !strings.Contains(board.body, "Delete ticket") {
		t.Fatalf("claim/delete missing:\n%s", excerpt(board.body, "Claim"))
	}
	if !strings.Contains(board.body, "/actions/agents/create") {
		t.Fatal("board must offer workspace agent registration")
	}

	rev := func() string {
		ticket, err := h.actions.Tickets.GetTicket(ctx, h.ticketID)
		if err != nil {
			t.Fatal(err)
		}
		return TicketRevision(ticket)
	}
	claim := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/claim", url.Values{
		"csrf_token":        {"test-csrf"},
		"reason":            {"web claim ticket"},
		"expected_revision": {rev()},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if claim.code != http.StatusSeeOther {
		t.Fatalf("claim status=%d body=%s", claim.code, claim.body)
	}
	claimed, err := h.actions.Tickets.GetTicket(ctx, h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Lease.Actor != "human:owner" {
		t.Fatalf("lease=%#v", claimed.Lease)
	}

	release := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/release", url.Values{
		"csrf_token":        {"test-csrf"},
		"reason":            {"web release ticket"},
		"expected_revision": {rev()},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if release.code != http.StatusSeeOther {
		t.Fatalf("release status=%d body=%s", release.code, release.body)
	}

	other, err := h.actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "WEB",
		Title:         "Downstream",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusBacklog,
		Priority:      contracts.PriorityLow,
		CreatedAt:     h.now,
		UpdatedAt:     h.now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "seed other")
	if err != nil {
		t.Fatal(err)
	}
	link := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/link", url.Values{
		"csrf_token":        {"test-csrf"},
		"other_id":          {other.ID},
		"kind":              {"blocks"},
		"reason":            {"web link ticket"},
		"expected_revision": {rev()},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if link.code != http.StatusSeeOther {
		t.Fatalf("link status=%d body=%s", link.code, link.body)
	}
	linked, err := h.actions.Tickets.GetTicket(ctx, h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range linked.Blocks {
		if id == other.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("blocks=%v", linked.Blocks)
	}

	human := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/assign", url.Values{
		"csrf_token":        {"test-csrf"},
		"assignee":          {"human:ada"},
		"reason":            {"web assign"},
		"expected_revision": {rev()},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if human.code != http.StatusSeeOther {
		t.Fatalf("human assign status=%d body=%s", human.code, human.body)
	}

	badActor := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/assign", url.Values{
		"csrf_token":        {"test-csrf"},
		"assignee":          {"not-an-actor"},
		"reason":            {"web assign"},
		"expected_revision": {rev()},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if badActor.code != http.StatusBadRequest {
		t.Fatalf("invalid actor status=%d body=%s", badActor.code, badActor.body)
	}

	stale := rev()
	if _, err := h.actions.MutateTrackedTicket(ctx, h.ticketID, "human:owner", "race", "concurrent edit", func(ticket *contracts.TicketSnapshot) error {
		ticket.Notes = "changed under the lock"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	staleEdit := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/delete", url.Values{
		"csrf_token":        {"test-csrf"},
		"reason":            {"stale archive"},
		"expected_revision": {stale},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if staleEdit.code != http.StatusConflict {
		t.Fatalf("stale delete status=%d body=%s", staleEdit.code, staleEdit.body)
	}

	archived := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/delete", url.Values{
		"csrf_token":        {"test-csrf"},
		"reason":            {"web delete ticket"},
		"expected_revision": {rev()},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if archived.code != http.StatusSeeOther {
		t.Fatalf("delete status=%d body=%s", archived.code, archived.body)
	}
	got, err := h.actions.Tickets.GetTicket(ctx, h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Archived || got.Status != contracts.StatusCanceled {
		t.Fatalf("archived ticket %#v", got)
	}
	if !strings.Contains(got.Notes, "Archived by human:owner") {
		t.Fatalf("audit notes=%q", got.Notes)
	}
	md := filepath.Join(storage.TicketsDir(h.root, "WEB"), got.ID+".md")
	if _, err := os.Stat(md); err != nil {
		t.Fatalf("markdown must remain: %v", err)
	}
	hidden := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if strings.Contains(hidden.body, `data-ticket-id="`+h.ticketID+`"`) {
		t.Fatal("archived ticket still on default board")
	}
	shown := h.doAuthed(t, http.MethodGet, "/board?archived=1", "", nil)
	if !strings.Contains(shown.body, h.ticketID) {
		t.Fatalf("archived listing missing ticket:\n%s", excerpt(shown.body, "Archived"))
	}
	if strings.Contains(shown.body, "/actions/tickets/"+h.ticketID+"/restore") {
		t.Fatal("must not invent restore-from-delete")
	}
	if !strings.Contains(shown.body, "files and history intact") || !strings.Contains(shown.body, "cannot be restored") {
		t.Fatalf("deleted-ticket copy must distinguish retention restore:\n%s", excerpt(shown.body, "Deleted"))
	}

	getWrite := h.doAuthed(t, http.MethodGet, "/actions/tickets/"+h.ticketID+"/delete", "", nil)
	if getWrite.code != http.StatusMethodNotAllowed {
		t.Fatalf("GET delete status=%d", getWrite.code)
	}
}

func TestArchiveCSRFReadOnlyAndPermission(t *testing.T) {
	h := newWebHarness(t, false)
	missing := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/delete", url.Values{
		"reason": {"no csrf"},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if missing.code != http.StatusForbidden {
		t.Fatalf("csrf status=%d body=%s", missing.code, missing.body)
	}
	ro := newWebHarness(t, true)
	blocked := ro.doAuthed(t, http.MethodPost, "/actions/tickets/"+ro.ticketID+"/delete", url.Values{
		"csrf_token": {"test-csrf"},
		"reason":     {"readonly"},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if blocked.code != http.StatusForbidden {
		t.Fatalf("read-only status=%d body=%s", blocked.code, blocked.body)
	}
}

func TestSavedViewsAreLinkedOnBoard(t *testing.T) {
	h := newWebHarness(t, false)
	if err := (service.ViewStore{Root: h.root}).SaveView(contracts.SavedView{
		Name:  "web-ready",
		Title: "Ready work",
		Kind:  contracts.SavedViewKindBoard,
	}); err != nil {
		t.Fatal(err)
	}
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if !strings.Contains(res.body, "Ready work") || !strings.Contains(res.body, "view=web-ready") {
		t.Fatalf("saved view missing:\n%s", excerpt(res.body, "Saved views"))
	}
}

func TestHomeEmptyStateOffersCreate(t *testing.T) {
	home := t.TempDir()
	application, err := app.Open(app.Options{
		Home:            home,
		StateDir:        filepath.Join(home, "state"),
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   app.SilentRunner{},
		SkipHostInstall: true,
		Process:         app.NoopSpawner{},
		Now:             func() time.Time { return time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	srv, err := NewHomeServer(application, HomeConfig{Host: "127.0.0.1", Port: 7432, Actor: "human:owner", Token: "home-token", CSRF: "home-csrf"})
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	body := homeGET(t, srv, cookie, "/")
	if !strings.Contains(body, "Initialize board") || !strings.Contains(body, "/?init=1") {
		t.Fatalf("fresh home missing create path:\n%s", excerpt(body, "Initialize"))
	}
}

func TestHomePreviewDirectoryGrantThenConfirm(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	dir := filepath.Join(application.Home(), "from-ui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical, err := app.CanonicalExistingDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	unauth := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/preview-path", strings.NewReader(url.Values{
		"csrf_token": {"home-csrf"},
		"purpose":    {app.PathGrantInit},
		"directory":  {dir},
	}.Encode()))
	unauth.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unauth.Header.Set("Origin", "http://127.0.0.1")
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauth)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth preview status=%d body=%s", unauthRec.Code, unauthRec.Body.String())
	}

	invalid := homePOST(t, srv, cookie, "/actions/workspaces/preview-path", url.Values{
		"csrf_token": {"home-csrf"},
		"purpose":    {app.PathGrantInit},
		"directory":  {"relative-folder"},
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid preview status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	preview := homePOST(t, srv, cookie, "/actions/workspaces/preview-path", url.Values{
		"csrf_token":   {"home-csrf"},
		"purpose":      {app.PathGrantInit},
		"directory":    {dir},
		"project_key":  {"APP"},
		"project_name": {"App"},
	})
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	body := preview.Body.String()
	if !strings.Contains(body, "Confirm this directory") || !strings.Contains(body, canonical) {
		t.Fatalf("preview must show canonical path:\n%s", excerpt(body, "Confirm"))
	}
	if _, err := os.Stat(filepath.Join(dir, ".tracker")); !os.IsNotExist(err) {
		t.Fatal("preview must not initialize")
	}
	grantID := htmlInputValue(t, body, "grant_id")
	confirmPath := htmlInputValue(t, body, "confirm_path")
	if confirmPath != canonical {
		t.Fatalf("confirm_path=%q canonical=%q", confirmPath, canonical)
	}

	noConfirm := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token": {"home-csrf"},
		"grant_id":   {grantID},
	})
	if noConfirm.Code != http.StatusBadRequest || !strings.Contains(noConfirm.Body.String(), "confirm the exact directory") {
		t.Fatalf("missing confirm status=%d body=%s", noConfirm.Code, noConfirm.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".tracker")); !os.IsNotExist(err) {
		t.Fatal("denied confirm must not initialize")
	}

	wrongPurpose := homePOST(t, srv, cookie, "/actions/workspaces/register", url.Values{
		"csrf_token":   {"home-csrf"},
		"grant_id":     {grantID},
		"confirm_path": {canonical},
	})
	if wrongPurpose.Code != http.StatusForbidden && wrongPurpose.Code != http.StatusBadRequest {
		t.Fatalf("wrong purpose status=%d body=%s", wrongPurpose.Code, wrongPurpose.Body.String())
	}

	confirmed := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token":   {"home-csrf"},
		"grant_id":     {grantID},
		"confirm_path": {canonical},
		"project_key":  {"APP"},
		"project_name": {"App"},
	})
	if confirmed.Code != http.StatusSeeOther {
		t.Fatalf("confirm status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	if _, err := os.Stat(filepath.Join(canonical, ".tracker")); err != nil {
		t.Fatalf("confirmed init missing tracker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(canonical, "projects", "APP", "project.md")); err != nil {
		t.Fatalf("confirmed project missing: %v", err)
	}

	replay := homePOST(t, srv, cookie, "/actions/workspaces/init", url.Values{
		"csrf_token":   {"home-csrf"},
		"grant_id":     {grantID},
		"confirm_path": {canonical},
	})
	if replay.Code == http.StatusSeeOther {
		t.Fatal("replayed home grant must not initialize again")
	}
}

func TestHomePreviewCSRFAndReadOnly(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	cookie := &http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"}
	dir := filepath.Join(application.Home(), "locked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := homePOST(t, srv, cookie, "/actions/workspaces/preview-path", url.Values{
		"purpose":   {app.PathGrantInit},
		"directory": {dir},
	})
	if missing.Code != http.StatusForbidden {
		t.Fatalf("csrf status=%d body=%s", missing.Code, missing.Body.String())
	}
	ro, err := NewHomeServer(application, HomeConfig{Host: "127.0.0.1", Port: 7432, Actor: "human:owner", Token: "home-token", CSRF: "home-csrf", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	blocked := homePOST(t, ro, cookie, "/actions/workspaces/preview-path", url.Values{
		"csrf_token": {"home-csrf"},
		"purpose":    {app.PathGrantInit},
		"directory":  {dir},
	})
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("read-only status=%d body=%s", blocked.Code, blocked.Body.String())
	}
}

func TestBoardRegistersWorkspaceAgent(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodPost, "/actions/agents/create", url.Values{
		"csrf_token": {"test-csrf"},
		"agent_id":   {"reviewer-1"},
		"name":       {"Reviewer"},
		"provider":   {"claude"},
		"reason":     {"web register agent"},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if res.code != http.StatusSeeOther {
		t.Fatalf("create agent status=%d body=%s", res.code, res.body)
	}
	board := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if !strings.Contains(board.body, `value="agent:reviewer-1"`) {
		t.Fatalf("created agent missing from picker:\n%s", excerpt(board.body, "actor-options"))
	}
}

func TestBoardAgentRegistrationPreservesExistingProfile(t *testing.T) {
	h := newWebHarness(t, false)
	before, err := h.actions.SaveAgentProfile(context.Background(), contracts.AgentProfile{
		AgentID: "build-er", DisplayName: "Original builder", Provider: contracts.AgentProviderCustom,
		Enabled: true, Capabilities: []string{"backend"}, MaxActiveRuns: 7,
	}, "human:owner", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/agents/create", url.Values{
		"csrf_token": {"test-csrf"}, "agent_id": {"BUILD_ER"}, "name": {"Replacement"}, "provider": {"claude"},
	}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
	if res.code != http.StatusConflict {
		t.Fatalf("duplicate agent registration status=%d body=%s", res.code, res.body)
	}
	after, err := h.actions.Agents.LoadAgent(context.Background(), before.AgentID)
	if err != nil || after.DisplayName != before.DisplayName || after.Provider != before.Provider || after.MaxActiveRuns != 7 || len(after.Capabilities) != 1 || after.Capabilities[0] != "backend" {
		t.Fatalf("duplicate registration changed profile: %#v err=%v", after, err)
	}
}

func TestBoardAgentRegistrationRejectsInvalidInput(t *testing.T) {
	h := newWebHarness(t, false)
	for _, form := range []url.Values{
		{"agent_id": {""}, "name": {"Builder"}, "provider": {"custom"}},
		{"agent_id": {"new-builder"}, "name": {""}, "provider": {"custom"}},
		{"agent_id": {"new-builder"}, "name": {"Builder"}, "provider": {"unknown"}},
	} {
		form.Set("csrf_token", "test-csrf")
		res := h.doAuthed(t, http.MethodPost, "/actions/agents/create", form.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"})
		if res.code != http.StatusBadRequest {
			t.Fatalf("invalid agent registration status=%d body=%s", res.code, res.body)
		}
	}
	if _, err := h.actions.Agents.LoadAgent(context.Background(), "new-builder"); err == nil {
		t.Fatal("invalid registration persisted an agent")
	}
}

func TestPendingGrantListingsRequireHomePreviewConfirmation(t *testing.T) {
	grants := []app.PathGrant{
		{ID: "home-preview", Source: app.PathGrantSourceHome, Purpose: app.PathGrantInit, Path: "/workspace/preview"},
		{ID: "cli-approved", Source: "cli", Purpose: app.PathGrantInit, Path: "/workspace/cli"},
		{ID: "other-purpose", Source: "cli", Purpose: app.PathGrantRegister, Path: "/workspace/attach"},
	}
	hits := pendingGrantHits(grants, app.PathGrantInit)
	if len(hits) != 1 || hits[0].GrantID != "cli-approved" {
		t.Fatalf("listings must not offer Home previews without exact-path confirmation: %+v", hits)
	}
}
