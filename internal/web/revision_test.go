package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestTicketRevisionStableAndSensitiveToEdits(t *testing.T) {
	base := contracts.TicketSnapshot{
		ID:        "WEB-1",
		TicketUID: "uid-1",
		Project:   "WEB",
		Title:     "Seed",
		Type:      contracts.TicketTypeTask,
		Status:    contracts.StatusReady,
		Priority:  contracts.PriorityHigh,
		Labels:    []string{"b", "a"},
		Assignee:  "human:owner",
		UpdatedAt: newWebHarness(t, false).now,
	}
	first := TicketRevision(base)
	if !validRevision(first) {
		t.Fatalf("digest %q", first)
	}
	reordered := base
	reordered.Labels = []string{"a", "b"}
	if TicketRevision(reordered) != first {
		t.Fatal("label order must not change the digest")
	}
	edited := base
	edited.Title = "Agent rewrote this"
	if TicketRevision(edited) == first {
		t.Fatal("title change must change the digest")
	}
}

func TestStaleEditAfterAgentMutationConflicts(t *testing.T) {
	h := newWebHarness(t, false)
	page := h.doAuthed(t, http.MethodGet, "/board?ticket="+h.ticketID, "", nil)
	rev := formValueFromBody(t, page.body, "expected_revision")
	if !validRevision(rev) {
		t.Fatalf("rendered revision %q", rev)
	}
	if _, err := h.actions.MutateTrackedTicket(context.Background(), h.ticketID, "human:owner", "agent edit", "agent rewrite", func(ticket *contracts.TicketSnapshot) error {
		ticket.Title = "newer agent title"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":        {"test-csrf"},
		"title":             {"stale client title"},
		"notes":             {"should not land"},
		"reason":            {"web edit ticket"},
		"expected_revision": {rev},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
		"Accept":       "application/json",
	})
	if res.code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, `"conflict"`) {
		t.Fatalf("conflict envelope:\n%s", res.body)
	}
	got, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "newer agent title" {
		t.Fatalf("canonical title=%q", got.Title)
	}
	if got.Notes == "should not land" {
		t.Fatal("stale notes were written")
	}
}

func TestMatchingRevisionEditSucceeds(t *testing.T) {
	h := newWebHarness(t, false)
	current, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":        {"test-csrf"},
		"title":             {"kept after matching revision"},
		"notes":             {"operator notes"},
		"reason":            {"web edit ticket"},
		"expected_revision": {TicketRevision(current)},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	got, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "kept after matching revision" || got.Notes != "operator notes" {
		t.Fatalf("title=%q notes=%q", got.Title, got.Notes)
	}
}

func TestStaleDragMoveConflicts(t *testing.T) {
	h := newWebHarness(t, false)
	current, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	stale := TicketRevision(current)
	if _, err := h.actions.MutateTrackedTicket(context.Background(), h.ticketID, "human:owner", "agent edit", "agent rewrite", func(ticket *contracts.TicketSnapshot) error {
		ticket.Description = "touched by agent"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":        {"test-csrf"},
		"status":            {string(contracts.StatusInProgress)},
		"reason":            {"web drag move"},
		"expected_revision": {stale},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/move", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
		"Accept":       "application/json",
	})
	if res.code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	got, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != contracts.StatusReady {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestBulkStaleRevisionWritesNothing(t *testing.T) {
	h := newWebHarness(t, false)
	second := h.createTicket(t, "Second bulk ticket")
	first, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	staleFirst := TicketRevision(first)
	freshSecond := TicketRevision(second)
	if _, err := h.actions.MutateTrackedTicket(context.Background(), h.ticketID, "human:owner", "agent edit", "agent rewrite", func(ticket *contracts.TicketSnapshot) error {
		ticket.Title = "first changed under us"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":                      {"test-csrf"},
		"kind":                            {"move"},
		"status":                          {string(contracts.StatusInProgress)},
		"ticket_ids":                      {h.ticketID + "," + second.ID},
		"confirm":                         {"1"},
		"reason":                          {"web bulk"},
		"expected_revision." + h.ticketID: {staleFirst},
		"expected_revision." + second.ID:  {freshSecond},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/bulk", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
		"Accept":       "application/json",
	})
	if res.code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	gotFirst, _ := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	gotSecond, _ := h.actions.Tickets.GetTicket(context.Background(), second.ID)
	if gotFirst.Status != contracts.StatusReady || gotSecond.Status != contracts.StatusReady {
		t.Fatalf("partial write first=%s second=%s", gotFirst.Status, gotSecond.Status)
	}
}

func TestRevisionOptionalKeepsLegacyCallers(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"status":     {string(contracts.StatusInProgress)},
		"reason":     {"web move"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/move", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	got, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != contracts.StatusInProgress {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestMalformedRevisionConflicts(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token":        {"test-csrf"},
		"title":             {"nope"},
		"reason":            {"web edit ticket"},
		"expected_revision": {"not-a-digest"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
		"Accept":       "application/json",
	})
	if res.code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	if apperr.CodeOf(apperr.New(apperr.CodeConflict, "x")) != apperr.CodeConflict {
		t.Fatal("sanity")
	}
}

func formValueFromBody(t *testing.T, body, name string) string {
	t.Helper()
	needle := `name="` + name + `" value="`
	start := strings.Index(body, needle)
	if start < 0 {
		t.Fatalf("missing %s in:\n%s", name, excerpt(body, name))
	}
	start += len(needle)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		t.Fatal("unterminated value")
	}
	return body[start : start+end]
}
