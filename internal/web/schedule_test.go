package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestSchedulePageRendersHumanAgentAndCompletionHistory(t *testing.T) {
	h := newWebHarness(t, false)
	location := mustLocation(t, "America/New_York")
	h = h.withLocation(t, location)
	ctx := context.Background()
	if err := (service.AgentStore{Root: h.root}).SaveAgent(ctx, contracts.AgentProfile{
		AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: true,
	}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	humanAt := time.Date(2026, 6, 16, 9, 30, 0, 0, location)
	if _, err := h.actions.SetTicketSchedule(ctx, h.ticketID, humanAt, contracts.Actor("human:owner"), contracts.Actor("human:owner"), "human reminder"); err != nil {
		t.Fatalf("set human schedule: %v", err)
	}
	agentTicket := h.createTicket(t, "Agent investigates flaky build")
	agentAt := time.Date(2026, 6, 16, 14, 45, 0, 0, location)
	if _, err := h.actions.SetTicketSchedule(ctx, agentTicket.ID, agentAt, contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "agent run"); err != nil {
		t.Fatalf("set agent schedule: %v", err)
	}
	completed := h.createTicket(t, "Publish the release notes")
	if _, err := h.actions.MoveTicket(ctx, completed.ID, contracts.StatusInProgress, contracts.Actor("human:owner"), "start"); err != nil {
		t.Fatalf("move completed ticket: %v", err)
	}
	if _, err := h.actions.RequestReviewWithReviewer(ctx, completed.ID, "", contracts.Actor("human:owner"), "review"); err != nil {
		t.Fatalf("request review: %v", err)
	}
	if _, err := h.actions.ApproveTicket(ctx, completed.ID, contracts.Actor("human:owner"), "approved"); err != nil {
		t.Fatalf("approve ticket: %v", err)
	}
	if _, err := h.actions.CompleteTicket(ctx, completed.ID, contracts.Actor("human:owner"), "shipped"); err != nil {
		t.Fatalf("complete ticket: %v", err)
	}

	res := h.doAuthed(t, http.MethodGet, "/schedule?date=2026-06-16&project=WEB", "", nil)
	if res.code != http.StatusOK {
		t.Fatalf("schedule status = %d body=%s", res.code, res.body)
	}
	for _, wanted := range []string{
		"Tuesday, June 16",
		"America/New_York",
		"Human reminder",
		"agent:builder-1",
		"codex · Builder",
		"Agent investigates flaky build",
		"Publish the release notes",
		`aria-current="date"`,
		"&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;",
	} {
		if !strings.Contains(res.body, wanted) {
			t.Fatalf("schedule page missing %q:\n%s", wanted, res.body)
		}
	}
	if strings.Contains(res.body, `<script>alert("x")</script>`) {
		t.Fatalf("schedule rendered unsafe ticket title:\n%s", res.body)
	}
	if strings.Contains(res.body, ">12 AM<") {
		t.Fatalf("timeline should begin at the first scheduled hour, not midnight:\n%s", res.body)
	}
}

func TestScheduleSetUsesLocalTimezoneAndPreservesRejectedForm(t *testing.T) {
	h := newWebHarness(t, false).withLocation(t, mustLocation(t, "America/New_York"))
	form := withCSRF(url.Values{
		"ticket_id": {h.ticketID},
		"at":        {"2026-06-16T09:30"},
		"runner":    {"human:owner"},
		"actor":     {"human:owner"},
		"reason":    {"morning reminder"},
	})
	res := h.doAuthed(t, http.MethodPost, "/actions/schedule/set?return=schedule&date=2026-06-16&project=WEB", form.Encode(), formHeaders())
	if res.code != http.StatusSeeOther {
		t.Fatalf("set status = %d body=%s", res.code, res.body)
	}
	redirect, err := url.Parse(res.header.Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if redirect.Path != "/schedule" || redirect.Query().Get("date") != "2026-06-16" || redirect.Query().Get("project") != "WEB" {
		t.Fatalf("unexpected redirect: %s", res.header.Get("Location"))
	}
	ticket, err := h.actions.Tickets.GetTicket(context.Background(), h.ticketID)
	if err != nil {
		t.Fatalf("get scheduled ticket: %v", err)
	}
	wantUTC := time.Date(2026, 6, 16, 13, 30, 0, 0, time.UTC)
	if ticket.Schedule == nil || !ticket.Schedule.At.Equal(wantUTC) {
		t.Fatalf("schedule = %#v, want %s", ticket.Schedule, wantUTC)
	}
	board := h.doAuthed(t, http.MethodGet, "/board?ticket="+h.ticketID+"&project=WEB", "", nil)
	for _, wanted := range []string{"Tue, Jun 16 at 9:30 AM", "Clear schedule", "America/New_York"} {
		if !strings.Contains(board.body, wanted) {
			t.Fatalf("ticket drawer missing schedule value %q:\n%s", wanted, board.body)
		}
	}
	history, err := h.projection.QueryHistory(context.Background(), h.ticketID)
	if err != nil {
		t.Fatalf("query schedule history: %v", err)
	}
	last := history[len(history)-1]
	if last.Type != contracts.EventTicketScheduleSet || last.Metadata.Surface != contracts.EventSurfaceWeb {
		t.Fatalf("unexpected schedule event: %#v", last)
	}

	rejectedForm := withCSRF(url.Values{
		"ticket_id": {h.ticketID},
		"at":        {"2026-06-17T10:15"},
		"runner":    {"agent:missing"},
		"actor":     {"human:owner"},
		"reason":    {"keep these fields"},
	})
	rejected := h.doAuthed(t, http.MethodPost, "/actions/schedule/set?return=schedule&date=2026-06-16&project=WEB", rejectedForm.Encode(), formHeaders())
	if rejected.code != http.StatusNotFound {
		t.Fatalf("rejected status = %d body=%s", rejected.code, rejected.body)
	}
	for _, wanted := range []string{"2026-06-17T10:15", "agent:missing", "keep these fields", "scheduled agent not found", `<details class="schedule-composer" open>`} {
		if !strings.Contains(rejected.body, wanted) {
			t.Fatalf("rejected form missing %q:\n%s", wanted, rejected.body)
		}
	}
}

func TestScheduleTickClearAndReadOnlyProtection(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := context.Background()
	if _, err := h.actions.SetTicketSchedule(ctx, h.ticketID, h.now.Add(-time.Minute), contracts.Actor("human:owner"), contracts.Actor("human:owner"), "due reminder"); err != nil {
		t.Fatalf("set due schedule: %v", err)
	}
	tick := h.doAuthed(t, http.MethodPost, "/actions/schedule/tick?return=schedule&date=2026-06-16&project=WEB", withCSRF(url.Values{
		"actor": {"human:owner"}, "reason": {"process due"},
	}).Encode(), formHeaders())
	if tick.code != http.StatusSeeOther || !strings.Contains(tick.header.Get("Location"), "processed+1+due+schedule") {
		t.Fatalf("tick status = %d location=%q body=%s", tick.code, tick.header.Get("Location"), tick.body)
	}
	ticket, err := h.actions.Tickets.GetTicket(ctx, h.ticketID)
	if err != nil {
		t.Fatalf("get triggered ticket: %v", err)
	}
	if ticket.Schedule == nil || ticket.Schedule.TriggeredAt.IsZero() {
		t.Fatalf("schedule was not triggered: %#v", ticket.Schedule)
	}

	clear := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/schedule/clear?return=schedule&date=2026-06-16&project=WEB", withCSRF(url.Values{
		"actor": {"human:owner"}, "reason": {"clear it"},
	}).Encode(), formHeaders())
	if clear.code != http.StatusSeeOther {
		t.Fatalf("clear status = %d body=%s", clear.code, clear.body)
	}
	ticket, err = h.actions.Tickets.GetTicket(ctx, h.ticketID)
	if err != nil {
		t.Fatalf("get cleared ticket: %v", err)
	}
	if ticket.Schedule != nil {
		t.Fatalf("schedule was not cleared: %#v", ticket.Schedule)
	}

	readOnly := newWebHarness(t, true)
	blocked := readOnly.doAuthed(t, http.MethodPost, "/actions/schedule/set?return=schedule&date=2026-06-16", withCSRF(url.Values{
		"ticket_id": {readOnly.ticketID}, "at": {"2026-06-16T12:00"}, "runner": {"human:owner"}, "actor": {"human:owner"},
	}).Encode(), formHeaders())
	if blocked.code != http.StatusForbidden || !strings.Contains(blocked.body, "web schedule is read-only") {
		t.Fatalf("read-only status = %d body=%s", blocked.code, blocked.body)
	}
}

func TestScheduleAPIAndInputErrors(t *testing.T) {
	h := newWebHarness(t, false)
	api := h.doAuthed(t, http.MethodGet, "/api/schedule?date=2026-06-16&project=WEB", "", map[string]string{"Accept": "application/json"})
	if api.code != http.StatusOK {
		t.Fatalf("schedule API status = %d body=%s", api.code, api.body)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(api.body), &envelope); err != nil {
		t.Fatalf("decode schedule API: %v", err)
	}
	if envelope["kind"] != "atlas_web_schedule" || strings.Contains(api.body, "test-csrf") {
		t.Fatalf("unexpected schedule API envelope: %s", api.body)
	}
	invalidDate := h.doAuthed(t, http.MethodGet, "/api/schedule?date=16-06-2026", "", map[string]string{"Accept": "application/json"})
	if invalidDate.code != http.StatusBadRequest || !strings.Contains(invalidDate.body, "YYYY-MM-DD") {
		t.Fatalf("invalid date status = %d body=%s", invalidDate.code, invalidDate.body)
	}
	invalidTime := h.doAuthed(t, http.MethodPost, "/actions/schedule/set?return=schedule&date=2026-06-16", withCSRF(url.Values{
		"ticket_id": {h.ticketID}, "at": {"tomorrow-ish"}, "runner": {"human:owner"}, "actor": {"human:owner"},
	}).Encode(), formHeaders())
	if invalidTime.code != http.StatusBadRequest || !strings.Contains(invalidTime.body, "local date and time") {
		t.Fatalf("invalid time status = %d body=%s", invalidTime.code, invalidTime.body)
	}
	dstGap := newWebHarness(t, false).withLocation(t, mustLocation(t, "America/New_York"))
	nonexistent := dstGap.doAuthed(t, http.MethodPost, "/actions/schedule/set?return=schedule&date=2026-03-08", withCSRF(url.Values{
		"ticket_id": {dstGap.ticketID}, "at": {"2026-03-08T02:30"}, "runner": {"human:owner"}, "actor": {"human:owner"},
	}).Encode(), formHeaders())
	if nonexistent.code != http.StatusBadRequest || !strings.Contains(nonexistent.body, "does not exist") {
		t.Fatalf("DST gap status = %d body=%s", nonexistent.code, nonexistent.body)
	}
	wrongMethod := h.doAuthed(t, http.MethodPost, "/schedule", withCSRF(url.Values{}).Encode(), formHeaders())
	if wrongMethod.code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d body=%s", wrongMethod.code, wrongMethod.body)
	}
}

func (h webHarness) withLocation(t *testing.T, location *time.Location) webHarness {
	t.Helper()
	server, err := NewServer(Services{Actions: h.actions, Queries: h.queries}, Config{
		Root: h.root, Workspace: "test-workspace", Host: "127.0.0.1", Project: "WEB",
		Actor: contracts.Actor("human:owner"), ReadOnly: h.server.cfg.ReadOnly, TokenMode: "random",
		Token: "test-token", CSRFToken: "test-csrf", Clock: func() time.Time { return h.now }, Location: location,
	})
	if err != nil {
		t.Fatalf("new server with location: %v", err)
	}
	h.server = server
	h.handler = server.Handler()
	return h
}

func (h webHarness) createTicket(t *testing.T, title string) contracts.TicketSnapshot {
	t.Helper()
	ticket, err := h.actions.CreateTrackedTicket(context.Background(), contracts.TicketSnapshot{
		Project: "WEB", Title: title, Type: contracts.TicketTypeTask, Status: contracts.StatusReady,
		Priority: contracts.PriorityMedium, CreatedAt: h.now, UpdatedAt: h.now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "schedule test fixture")
	if err != nil {
		t.Fatalf("create ticket %q: %v", title, err)
	}
	return ticket
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load timezone %s: %v", name, err)
	}
	return location
}

func formHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "http://atlas.local"}
}
