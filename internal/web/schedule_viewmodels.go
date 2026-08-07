package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type SchedulePage struct {
	Page            string
	Workspace       string
	Host            string
	Actor           contracts.Actor
	ReadOnly        bool
	Project         string
	ProjectExplicit bool `json:"-"`
	Query           string
	CSRFToken       string     `json:"-"`
	Form            url.Values `json:"-"`
	LocationName    string
	SelectedDate    string
	DateHeading     string
	PrevURL         string
	NextURL         string
	TodayURL        string
	Week            []ScheduleDay
	Hours           []ScheduleHour
	Entries         int
	History         []ScheduleHistoryItem
	TicketOptions   []TicketOption
	AgentOptions    []AgentOption
	Flash           string
	Error           string
	Data            service.ScheduleView
}

func (p SchedulePage) FormValue(key string, fallback string) string {
	return formValue(p.Form, key, fallback)
}

type ScheduleDay struct {
	Label  string
	Number string
	URL    string
	Active bool
	Today  bool
	Count  int
}

type ScheduleHour struct {
	Label   string
	ID      string
	Current bool
	Entries []ScheduleCard
}

type ScheduleCard struct {
	Entry       service.ScheduleEntry
	Time        string
	AtRFC3339   string
	RunnerLabel string
	RunnerMeta  string
	StateLabel  string
	StateClass  string
	TicketURL   string
}

type ScheduleHistoryItem struct {
	Entry     service.CompletionEntry
	Time      string
	Day       string
	TicketURL string
}

type TicketOption struct {
	ID    string
	Title string
}

type AgentOption struct {
	Actor string
	Label string
}

func (s *Server) buildSchedulePage(ctx context.Context, r *http.Request) (SchedulePage, error) {
	query := r.URL.Query()
	project := firstNonEmpty(query.Get("project"), s.cfg.Project)
	selected, err := s.selectedScheduleDate(query.Get("date"))
	page := SchedulePage{
		Page:            "schedule",
		Workspace:       s.cfg.Workspace,
		Host:            s.cfg.Host,
		Actor:           s.cfg.Actor,
		ReadOnly:        s.cfg.ReadOnly,
		Project:         project,
		ProjectExplicit: strings.TrimSpace(query.Get("project")) != "",
		Query:           strings.TrimSpace(query.Get("q")),
		CSRFToken:       s.cfg.CSRFToken,
		LocationName:    locationName(s.cfg.Location, s.cfg.Clock()),
		Flash:           strings.TrimSpace(query.Get("flash")),
		Error:           strings.TrimSpace(query.Get("error_flash")),
	}
	if err != nil {
		return page, err
	}

	weekStart := selected.AddDate(0, 0, -int(selected.Weekday()))
	weekEnd := weekStart.AddDate(0, 0, 7)
	view, err := s.queries.Schedule(ctx, service.ScheduleQuery{
		Project: project,
		From:    weekStart.UTC(),
		To:      weekEnd.UTC(),
	})
	if err != nil {
		return page, err
	}
	page.Data = view
	page.SelectedDate = selected.Format("2006-01-02")
	page.DateHeading = selected.Format("Monday, January 2")
	page.PrevURL = scheduleURL(selected.AddDate(0, 0, -7), project, page.ProjectExplicit)
	page.NextURL = scheduleURL(selected.AddDate(0, 0, 7), project, page.ProjectExplicit)
	page.TodayURL = scheduleURL(scheduleDate(s.cfg.Clock(), s.cfg.Location), project, page.ProjectExplicit)
	page.Week = s.scheduleWeek(weekStart, selected, view.Entries, project, page.ProjectExplicit)
	page.Hours, page.Entries = s.scheduleHours(selected, view.Entries, project, page.ProjectExplicit)
	page.History = s.scheduleHistory(view.History, project, page.ProjectExplicit)
	page.TicketOptions, err = s.scheduleTicketOptions(ctx, project)
	if err != nil {
		return page, err
	}
	page.AgentOptions, err = s.scheduleAgentOptions(ctx)
	if err != nil {
		return page, err
	}
	return page, nil
}

func (s *Server) selectedScheduleDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return scheduleDate(s.cfg.Clock(), s.cfg.Location), nil
	}
	selected, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), s.cfg.Location)
	if err != nil {
		return time.Time{}, apperr.New(apperr.CodeInvalidInput, "schedule date must use YYYY-MM-DD")
	}
	return selected, nil
}

func scheduleDate(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func (s *Server) scheduleWeek(start time.Time, selected time.Time, entries []service.ScheduleEntry, project string, explicit bool) []ScheduleDay {
	counts := map[string]int{}
	for _, entry := range entries {
		key := entry.Ticket.Schedule.At.In(s.cfg.Location).Format("2006-01-02")
		counts[key]++
	}
	today := scheduleDate(s.cfg.Clock(), s.cfg.Location)
	days := make([]ScheduleDay, 0, 7)
	for offset := 0; offset < 7; offset++ {
		day := start.AddDate(0, 0, offset)
		key := day.Format("2006-01-02")
		days = append(days, ScheduleDay{
			Label:  day.Format("Mon"),
			Number: day.Format("2"),
			URL:    scheduleURL(day, project, explicit),
			Active: sameScheduleDate(day, selected),
			Today:  sameScheduleDate(day, today),
			Count:  counts[key],
		})
	}
	return days
}

func (s *Server) scheduleHours(selected time.Time, entries []service.ScheduleEntry, project string, explicit bool) ([]ScheduleHour, int) {
	grouped := map[int][]ScheduleCard{}
	count := 0
	firstHour := 24
	lastHour := -1
	for _, entry := range entries {
		local := entry.Ticket.Schedule.At.In(s.cfg.Location)
		if !sameScheduleDate(local, selected) {
			continue
		}
		grouped[local.Hour()] = append(grouped[local.Hour()], s.scheduleCard(entry, project, explicit))
		if local.Hour() < firstHour {
			firstHour = local.Hour()
		}
		if local.Hour() > lastHour {
			lastHour = local.Hour()
		}
		count++
	}
	if count == 0 {
		return nil, 0
	}
	now := s.cfg.Clock().In(s.cfg.Location)
	hours := make([]ScheduleHour, 0, lastHour-firstHour+1)
	for hour := firstHour; hour <= lastHour; hour++ {
		at := time.Date(selected.Year(), selected.Month(), selected.Day(), hour, 0, 0, 0, s.cfg.Location)
		hours = append(hours, ScheduleHour{
			Label:   at.Format("3 PM"),
			ID:      fmt.Sprintf("hour-%02d", hour),
			Current: sameScheduleDate(selected, now) && now.Hour() == hour,
			Entries: grouped[hour],
		})
	}
	return hours, count
}

func (s *Server) scheduleCard(entry service.ScheduleEntry, project string, explicit bool) ScheduleCard {
	local := entry.Ticket.Schedule.At.In(s.cfg.Location)
	runnerMeta := "Human reminder"
	if entry.RunnerKind == "agent" {
		runnerMeta = "Agent"
		if entry.Agent != nil {
			parts := []string{}
			if entry.Agent.Provider != "" {
				parts = append(parts, string(entry.Agent.Provider))
			}
			if strings.TrimSpace(entry.Agent.DisplayName) != "" {
				parts = append(parts, entry.Agent.DisplayName)
			}
			if len(parts) > 0 {
				runnerMeta = strings.Join(parts, " · ")
			}
		}
	}
	return ScheduleCard{
		Entry:       entry,
		Time:        local.Format("3:04 PM"),
		AtRFC3339:   entry.Ticket.Schedule.At.UTC().Format(time.RFC3339),
		RunnerLabel: string(entry.Runner),
		RunnerMeta:  runnerMeta,
		StateLabel:  scheduleStateLabel(entry.State),
		StateClass:  scheduleStateClass(entry.State),
		TicketURL:   boardTicketURL(entry.Ticket.ID, project, explicit),
	}
}

func (s *Server) scheduleHistory(entries []service.CompletionEntry, project string, explicit bool) []ScheduleHistoryItem {
	items := make([]ScheduleHistoryItem, 0, len(entries))
	for _, entry := range entries {
		local := entry.CompletedAt.In(s.cfg.Location)
		items = append(items, ScheduleHistoryItem{
			Entry:     entry,
			Time:      local.Format("3:04 PM"),
			Day:       local.Format("Mon, Jan 2"),
			TicketURL: boardTicketURL(entry.Ticket.ID, project, explicit),
		})
	}
	return items
}

func (s *Server) scheduleTicketOptions(ctx context.Context, project string) ([]TicketOption, error) {
	tickets, err := s.actions.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: project, IncludeArchived: false})
	if err != nil {
		return nil, err
	}
	items := make([]TicketOption, 0, len(tickets))
	for _, ticket := range tickets {
		if contracts.IsTerminalStatus(ticket.Status) {
			continue
		}
		items = append(items, TicketOption{ID: ticket.ID, Title: ticket.Title})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (s *Server) scheduleAgentOptions(ctx context.Context) ([]AgentOption, error) {
	agents, err := s.queries.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AgentOption, 0, len(agents))
	for _, agent := range agents {
		if !agent.Profile.Enabled {
			continue
		}
		items = append(items, AgentOption{
			Actor: "agent:" + agent.Profile.AgentID,
			Label: fmt.Sprintf("%s · %s", agent.Profile.DisplayName, agent.Profile.Provider),
		})
	}
	return items, nil
}

func scheduleStateLabel(state service.ScheduleState) string {
	switch state {
	case service.ScheduleStateScheduled:
		return "Scheduled"
	case service.ScheduleStateDue:
		return "Due"
	case service.ScheduleStateOverdue:
		return "Overdue"
	case service.ScheduleStateAgentReady:
		return "Agent ready"
	case service.ScheduleStateAgentLaunched:
		return "Agent launched"
	case service.ScheduleStateNotified:
		return "Human notified"
	case service.ScheduleStateAcknowledged:
		return "Acknowledged"
	case service.ScheduleStateFailed:
		return "Failed"
	case service.ScheduleStateCompleted:
		return "Completed"
	case service.ScheduleStateCanceled:
		return "Canceled"
	default:
		return string(state)
	}
}

func scheduleStateClass(state service.ScheduleState) string {
	switch state {
	case service.ScheduleStateFailed, service.ScheduleStateOverdue:
		return "danger"
	case service.ScheduleStateCompleted, service.ScheduleStateAcknowledged:
		return "done"
	case service.ScheduleStateAgentLaunched, service.ScheduleStateAgentReady, service.ScheduleStateNotified:
		return "active"
	case service.ScheduleStateCanceled:
		return "muted"
	default:
		return "queued"
	}
}

func sameScheduleDate(left time.Time, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}

func scheduleURL(day time.Time, project string, explicit bool) string {
	query := url.Values{"date": {day.Format("2006-01-02")}}
	if explicit && project != "" {
		query.Set("project", project)
	}
	return "/schedule?" + query.Encode()
}

func boardTicketURL(ticketID string, project string, explicit bool) string {
	query := url.Values{"ticket": {ticketID}}
	if explicit && project != "" {
		query.Set("project", project)
	}
	return "/board?" + query.Encode()
}

func locationName(location *time.Location, at time.Time) string {
	if location.String() != "Local" {
		return location.String()
	}
	zone, _ := at.In(location).Zone()
	if zone == "" {
		return "Local time"
	}
	return "Local time (" + zone + ")"
}
