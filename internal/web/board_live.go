package web

import (
	"bytes"
	"context"
	"encoding/json"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

const liveTailLimit = 256 * 1024

type liveCardPatch struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
	HTML   string `json:"html,omitempty"`
	Remove bool   `json:"remove,omitempty"`
}

type liveDrawer struct {
	ID          string `json:"id"`
	Revision    string `json:"revision"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	Acceptance  string `json:"acceptance"`
	Priority    string `json:"priority"`
	Assignee    string `json:"assignee"`
	Reviewer    string `json:"reviewer"`
	Labels      string `json:"labels"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
}

type liveBoardBody struct {
	Cards        []liveCardPatch `json:"cards"`
	CommentsHTML string          `json:"comments_html,omitempty"`
	HistoryHTML  string          `json:"history_html,omitempty"`
	Resync       bool            `json:"resync,omitempty"`
	Drawer       *liveDrawer     `json:"drawer,omitempty"`
}

type parsedLiveStamp struct {
	query string
	files map[string]int64
	db    map[string]string
}

func quotedETag(body string) string {
	return `"` + body + `"`
}

// boardLiveStamp is a stat of the event logs, the sqlite index, and the board
// query. It does not read ticket rows. A matching stamp means the visible
// board cannot have changed, including a reindex or a git pull that rebuilt
// the index without appending an event.
func (s *Server) boardLiveStamp(r *http.Request) string {
	q := r.URL.Query()
	q.Del("flash")
	q.Del("error_flash")
	q.Del("token")
	sum := fnv.New64a()
	_, _ = sum.Write([]byte(q.Encode()))
	parts := []string{"v1", strconv.FormatUint(sum.Sum64(), 16)}
	files := eventFileSizes(s.cfg.Root)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parts = append(parts, name+"="+strconv.FormatInt(files[name], 10))
	}
	parts = append(parts, projectionStampParts(s.cfg.Root)...)
	return strings.Join(parts, "|")
}

func projectionStampParts(root string) []string {
	dir := storage.TrackerDir(root)
	parts := make([]string, 0, 2)
	for _, name := range []string{"index.sqlite", "index.sqlite-wal"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			parts = append(parts, "db:"+name+"=0:0")
			continue
		}
		parts = append(parts, "db:"+name+"="+strconv.FormatInt(info.Size(), 10)+":"+strconv.FormatInt(info.ModTime().UnixNano(), 10))
	}
	return parts
}

func eventFileSizes(root string) map[string]int64 {
	out := map[string]int64{}
	if strings.TrimSpace(root) == "" {
		return out
	}
	entries, err := os.ReadDir(storage.EventsDir(root))
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out[entry.Name()] = info.Size()
	}
	return out
}

func parseLiveStamp(raw string) (parsedLiveStamp, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "W/")
	raw = strings.Trim(raw, `"`)
	if !strings.HasPrefix(raw, "v1|") {
		return parsedLiveStamp{}, false
	}
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		return parsedLiveStamp{}, false
	}
	parsed := parsedLiveStamp{query: parts[1], files: map[string]int64{}, db: map[string]string{}}
	for _, part := range parts[2:] {
		if strings.HasPrefix(part, "db:") {
			name, rest, ok := strings.Cut(strings.TrimPrefix(part, "db:"), "=")
			if !ok || name == "" || strings.Contains(rest, "|") {
				return parsedLiveStamp{}, false
			}
			parsed.db[name] = rest
			continue
		}
		name, sizeText, ok := strings.Cut(part, "=")
		if !ok || !strings.HasSuffix(name, ".jsonl") {
			return parsedLiveStamp{}, false
		}
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil || size < 0 {
			return parsedLiveStamp{}, false
		}
		parsed.files[name] = size
	}
	return parsed, true
}

func projectionChanged(prev, cur map[string]string) bool {
	if len(prev) != len(cur) {
		return true
	}
	for name, value := range cur {
		if prev[name] != value {
			return true
		}
	}
	return false
}

func (s *Server) liveBoardPatch(r *http.Request, stamp string) (liveBoardBody, bool) {
	if s.cfg.Root == "" || s.actions == nil || s.queries == nil {
		return liveBoardBody{}, false
	}
	prev, ok := parseLiveStamp(r.Header.Get("If-None-Match"))
	if !ok {
		return liveBoardBody{}, false
	}
	cur, ok := parseLiveStamp(stamp)
	if !ok || prev.query != cur.query {
		return liveBoardBody{}, false
	}
	// A tail we cannot apply id-by-id, or an index change with no new events
	// (reindex, git pull), still has to land. One resync, then the new stamp
	// makes later polls 304. Falling through to a full HTML page wedges a
	// large board, which discards that page and asks again forever.
	tail, tailOK := readGrownTails(s.cfg.Root, prev.files, cur.files)
	if !tailOK || (projectionChanged(prev.db, cur.db) && len(bytes.TrimSpace(tail)) == 0) {
		return s.liveResync(r)
	}
	ids := ticketIDsFromTail(tail)
	page := s.boardFilterPage(r)
	colors := map[string]string{}
	if cfg, err := config.Load(s.cfg.Root); err == nil {
		colors = cfg.Web.AgentColors
	}
	body := liveBoardBody{Cards: make([]liveCardPatch, 0, len(ids))}
	openID := strings.TrimSpace(r.URL.Query().Get("ticket"))
	for _, id := range ids {
		patch, includeDetail := s.liveCard(r.Context(), r, page, colors, id)
		body.Cards = append(body.Cards, patch)
		if id == openID && !patch.Remove {
			if ticket, err := s.actions.Tickets.GetTicket(r.Context(), id); err == nil && !ticket.Archived {
				drawer := drawerFromTicket(ticket)
				body.Drawer = &drawer
			}
		}
		if includeDetail && id == openID {
			if comments, history, err := s.liveActivityHTML(r, id); err == nil {
				body.CommentsHTML = comments
				body.HistoryHTML = history
			}
		}
	}
	return body, true
}

func (s *Server) liveResync(r *http.Request) (liveBoardBody, bool) {
	page := s.boardFilterPage(r)
	board, err := s.loadBoard(r.Context(), page)
	if err != nil {
		return liveBoardBody{}, false
	}
	colors := map[string]string{}
	if cfg, err := config.Load(s.cfg.Root); err == nil {
		colors = cfg.Web.AgentColors
	}
	openID := strings.TrimSpace(r.URL.Query().Get("ticket"))
	body := liveBoardBody{Resync: true}
	for _, column := range s.columnsFromBoard(r.Context(), board, page, colors) {
		for _, card := range column.Tickets {
			html, err := s.renderFragment(r, "card", card)
			if err != nil || strings.TrimSpace(html) == "" {
				continue
			}
			body.Cards = append(body.Cards, liveCardPatch{
				ID:     card.Ticket.ID,
				Status: string(card.BoardStatus),
				HTML:   html,
			})
			if card.Ticket.ID == openID {
				drawer := drawerFromTicket(card.Ticket)
				body.Drawer = &drawer
			}
		}
	}
	return body, true
}

func drawerFromTicket(ticket contracts.TicketSnapshot) liveDrawer {
	return liveDrawer{
		ID:          ticket.ID,
		Revision:    TicketRevision(ticket),
		Title:       ticket.Title,
		Description: ticket.Description,
		Notes:       ticket.Notes,
		Acceptance:  strings.Join(ticket.AcceptanceCriteria, "\n"),
		Priority:    string(ticket.Priority),
		Assignee:    string(ticket.Assignee),
		Reviewer:    string(ticket.Reviewer),
		Labels:      strings.Join(ticket.Labels, ", "),
		Status:      statusKey(ticket.Status),
		StatusLabel: statusLabel(ticket.Status),
	}
}

func (s *Server) liveCard(ctx context.Context, r *http.Request, page BoardPage, colors map[string]string, id string) (liveCardPatch, bool) {
	ticket, err := s.actions.Tickets.GetTicket(ctx, id)
	if err != nil || ticket.Archived {
		return liveCardPatch{ID: id, Remove: true}, false
	}
	if page.Project != "" && !strings.EqualFold(ticket.Project, page.Project) {
		return liveCardPatch{ID: id, Remove: true}, false
	}
	filtered := filterBoard(contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		ticket.Status: {ticket},
	}}, page)
	if len(filtered.Columns[ticket.Status]) == 0 {
		return liveCardPatch{ID: id, Remove: true}, false
	}
	count := 0
	if counts, err := s.queries.CommentCounts(ctx, []string{id}); err == nil {
		count = counts[id]
	}
	agentName, colorClass := agentChipForAssignee(ticket.Assignee, colors)
	card := TicketCard{
		Ticket:            ticket,
		EffectiveReviewer: ticket.Reviewer,
		Warnings:          cardWarnings(ticket),
		CommentCount:      count,
		BoardStatus:       ticket.Status,
		StatusLabel:       statusLabel(ticket.Status),
		AgentName:         agentName,
		AgentColorClass:   colorClass,
		BoardPath:         page.BoardPath,
	}
	html, err := s.renderFragment(r, "card", card)
	if err != nil {
		return liveCardPatch{ID: id, Remove: true}, false
	}
	return liveCardPatch{ID: id, Status: string(ticket.Status), HTML: html}, true
}

func (s *Server) liveActivityHTML(r *http.Request, id string) (string, string, error) {
	detail, err := s.ticketDetail(r.Context(), id)
	if err != nil {
		return "", "", err
	}
	home, board, schedule, newTicket, prefix := s.navPaths()
	page := BoardPage{
		Page:          "board",
		Actor:         s.cfg.Actor,
		ReadOnly:      s.cfg.ReadOnly,
		CSRFToken:     s.cfg.CSRFToken,
		BoardPath:     board,
		ActionPrefix:  prefix,
		HomePath:      home,
		SchedulePath:  schedule,
		NewTicketPath: newTicket,
		Detail:        &detail,
	}
	comments, err := s.renderFragment(r, "commentThread", page)
	if err != nil {
		return "", "", err
	}
	history, err := s.renderFragment(r, "historyList", page)
	if err != nil {
		return "", "", err
	}
	return comments, history, nil
}

func (s *Server) renderFragment(r *http.Request, name string, data any) (string, error) {
	templates, err := s.templatesForRequest(r)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) boardFilterPage(r *http.Request) BoardPage {
	q := r.URL.Query()
	home, board, schedule, newTicket, prefix := s.navPaths()
	page := BoardPage{
		Project:         firstNonEmpty(strings.TrimSpace(q.Get("project")), s.cfg.Project),
		ProjectExplicit: strings.TrimSpace(q.Get("project")) != "",
		View:            strings.TrimSpace(q.Get("view")),
		Query:           strings.TrimSpace(q.Get("q")),
		Assignee:        strings.TrimSpace(q.Get("assignee")),
		Reviewer:        strings.TrimSpace(q.Get("reviewer")),
		Label:           strings.TrimSpace(q.Get("label")),
		Priority:        strings.TrimSpace(q.Get("priority")),
		Type:            strings.TrimSpace(q.Get("type")),
		BoardPath:       board,
		ActionPrefix:    prefix,
		HomePath:        home,
		SchedulePath:    schedule,
		NewTicketPath:   newTicket,
	}
	if page.ProjectExplicit {
		if canon, ok := s.canonicalProject(r.Context(), page.Project); ok {
			page.Project = canon
		}
	}
	return page
}

func (s *Server) canonicalProject(ctx context.Context, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || s.queries == nil || s.queries.Projects == nil {
		return "", false
	}
	projects, err := s.queries.Projects.ListProjects(ctx)
	if err != nil {
		return "", false
	}
	for _, project := range projects {
		if project.Key == raw || strings.EqualFold(project.Key, raw) {
			return project.Key, true
		}
	}
	return "", false
}

func readGrownTails(root string, prev, cur map[string]int64) ([]byte, bool) {
	names := make([]string, 0, len(cur))
	for name := range cur {
		names = append(names, name)
	}
	sort.Strings(names)
	var total int64
	var buf bytes.Buffer
	for _, name := range names {
		size := cur[name]
		old := prev[name]
		if _, seen := prev[name]; !seen {
			old = 0
		}
		if size < old {
			return nil, false
		}
		if size == old {
			continue
		}
		delta := size - old
		if delta > liveTailLimit || total+delta > liveTailLimit {
			return nil, false
		}
		file, err := os.Open(filepath.Join(storage.EventsDir(root), name))
		if err != nil {
			return nil, false
		}
		if _, err := file.Seek(old, io.SeekStart); err != nil {
			file.Close()
			return nil, false
		}
		n, err := io.CopyN(&buf, file, delta)
		file.Close()
		if err != nil || n != delta {
			return nil, false
		}
		total += n
	}
	return buf.Bytes(), true
}

func ticketIDsFromTail(raw []byte) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, 4)
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event struct {
			TicketID string `json:"ticket_id"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		id := strings.TrimSpace(event.TicketID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
