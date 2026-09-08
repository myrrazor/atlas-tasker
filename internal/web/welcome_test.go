package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestWelcomeRendersRollupsRecentChangesAndNavigation(t *testing.T) {
	h := newWebHarness(t, false)
	if err := config.Set(h.root, "web.owner_name", "Ada Lovelace"); err != nil {
		t.Fatalf("set owner name: %v", err)
	}

	res := h.doAuthed(t, http.MethodGet, "/", "", nil)
	if res.code != http.StatusOK {
		t.Fatalf("welcome status = %d body=%s", res.code, res.body)
	}
	for _, want := range []string{
		"Welcome, Ada Lovelace",
		`href="/board?project=WEB"`,
		"<strong>1</strong> active",
		"<strong>0</strong> backlog",
		"<strong>0</strong> done",
		h.ticketID + " created",
		`href="/settings"`,
		`data-dialog-open="new-project-dialog"`,
		`href="/board"`,
	} {
		if !strings.Contains(res.body, want) {
			t.Fatalf("welcome missing %q:\n%s", want, res.body)
		}
	}
	if strings.Contains(res.body, `src="/static/vendor/sortable.min.js"`) {
		t.Fatalf("welcome should not load board-only drag code")
	}

	board := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if !strings.Contains(board.body, `href="/"`) || !strings.Contains(board.body, ">Overview</a>") {
		t.Fatalf("board does not link back to welcome:\n%s", board.body)
	}
}

func TestCreateProjectUsesActionServiceAndPreservesVerbatimErrors(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"actor":      {"human:owner"},
		"key":        {"OPS"},
		"name":       {"Operations"},
	}
	created := h.doAuthed(t, http.MethodPost, "/actions/projects/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if created.code != http.StatusSeeOther || created.header.Get("Location") != "/?flash=created+project+OPS" {
		t.Fatalf("unexpected project create response: code=%d location=%q body=%s", created.code, created.header.Get("Location"), created.body)
	}
	projects, err := h.actions.Projects.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	foundOPS := false
	for _, project := range projects {
		foundOPS = foundOPS || project.Key == "OPS"
	}
	if len(projects) != 2 || !foundOPS {
		t.Fatalf("project was not created through the service: %#v", projects)
	}

	bad := url.Values{
		"csrf_token": {"test-csrf"},
		"actor":      {"human:owner"},
		"key":        {"bad key"},
		"name":       {"Kept name"},
	}
	rejected := h.doAuthed(t, http.MethodPost, "/actions/projects/create", bad.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if rejected.code != http.StatusBadRequest {
		t.Fatalf("expected invalid project to return 400, got %d body=%s", rejected.code, rejected.body)
	}
	wantError := contracts.ProjectKeyValidationMessage()
	if !strings.Contains(rejected.body, wantError) {
		t.Fatalf("expected verbatim error %q in:\n%s", wantError, rejected.body)
	}
	if !strings.Contains(rejected.body, `value="bad key"`) || !strings.Contains(rejected.body, `value="Kept name"`) {
		t.Fatalf("rejected project form values were not preserved:\n%s", rejected.body)
	}
}

func TestProjectCreateHonorsCSRFAndReadOnly(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{"key": {"OPS"}, "name": {"Operations"}}
	missingCSRF := h.doAuthed(t, http.MethodPost, "/actions/projects/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if missingCSRF.code != http.StatusForbidden || !strings.Contains(missingCSRF.body, "invalid csrf token") {
		t.Fatalf("expected project CSRF error on welcome page, got %d body=%s", missingCSRF.code, missingCSRF.body)
	}

	readOnly := newWebHarness(t, true)
	blocked := readOnly.doAuthed(t, http.MethodPost, "/actions/projects/create", withCSRF(form).Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if blocked.code != http.StatusForbidden || !strings.Contains(blocked.body, "web board is read-only") {
		t.Fatalf("expected read-only project create rejection, got %d body=%s", blocked.code, blocked.body)
	}
}

func TestSettingsShowsReadOnlyWebConfigWithMappedClasses(t *testing.T) {
	h := newWebHarness(t, false)
	if err := config.Set(h.root, "web.owner_name", "Ada Lovelace"); err != nil {
		t.Fatalf("set owner name: %v", err)
	}
	if err := config.Set(h.root, "web.agent_colors.merlin", "chartreuse"); err != nil {
		t.Fatalf("set agent color: %v", err)
	}
	res := h.doAuthed(t, http.MethodGet, "/settings", "", nil)
	if res.code != http.StatusOK {
		t.Fatalf("settings status = %d body=%s", res.code, res.body)
	}
	for _, want := range []string{
		"Ada Lovelace",
		"claude",
		"chip--orange",
		"codex",
		"chip--blue",
		"merlin",
		"chip--plain",
		"Read only in the browser",
	} {
		if !strings.Contains(res.body, want) {
			t.Fatalf("settings missing %q:\n%s", want, res.body)
		}
	}
}
