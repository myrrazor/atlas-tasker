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
	"sync"

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
	Deleted     bool   `json:"deleted,omitempty"`
	ActionsHTML string `json:"actions_html,omitempty"`
}

// liveFPCache remembers a logical ticket fingerprint for one projection
// stamp. Idle polls reuse it and do not list tickets. Row hashes let a
// later poll patch the tickets that actually changed, including a reindex
// that lands while other events are appended.
type liveFPCache struct {
	mu       sync.Mutex
	key      string
	fp       string
	rows     map[string]uint64
	prevFP   string
	prevRows map[string]uint64
}

// sharedLiveFP lives for the process, keyed by workspace root. Home builds a
// fresh Server per request; a cache on that Server never hits.
var sharedLiveFP sync.Map

func (s *Server) fpCache() *liveFPCache {
	if s == nil {
		return &liveFPCache{}
	}
	root := s.cfg.Root
	if root == "" {
		if s.liveFP == nil {
			s.liveFP = &liveFPCache{}
		}
		return s.liveFP
	}
	if found, ok := sharedLiveFP.Load(root); ok {
		return found.(*liveFPCache)
	}
	fresh := &liveFPCache{}
	actual, _ := sharedLiveFP.LoadOrStore(root, fresh)
	return actual.(*liveFPCache)
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
	parts = append(parts, s.liveProjectionParts(r)...)
	return strings.Join(parts, "|")
}

// liveProjectionParts stamps the index files plus a logical fingerprint of
// ticket rows. A no-op reindex changes file mtimes without changing the
// fingerprint, so open tabs can advance the etag without rebuilding the board.
func (s *Server) liveProjectionParts(r *http.Request) []string {
	before := s.projectionStampParts(r.Context())
	key := strings.Join(before, "|")
	if key == "" {
		return nil
	}
	cache := s.fpCache()
	if fp, ok := cache.lookup(key); ok {
		return append(append([]string{}, before...), "db:fp="+fp)
	}
	fp, rows, gen, computed := s.computeLiveFP(r.Context())
	after := s.projectionStampParts(r.Context())
	afterKey := strings.Join(after, "|")
	if afterKey == "" {
		return nil
	}
	// A commit landed while the rows were read, or the generation on the
	// stamp is not the generation those rows were read at. Caching that
	// fingerprint would 304 a stale board. Omit it and let the next poll
	// try again once the snapshot is stable.
	if !computed || afterKey != key || gen == "" || !stampCarriesGeneration(after, gen) {
		return after
	}
	cache.store(afterKey, fp, rows)
	return append(after, "db:fp="+fp)
}

func (c *liveFPCache) lookup(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key == "" || c.key != key || c.fp == "" {
		return "", false
	}
	return c.fp, true
}

func (c *liveFPCache) store(key, fp string, rows map[string]uint64) {
	if key == "" || fp == "" {
		return
	}
	c.mu.Lock()
	if c.fp != "" && c.rows != nil {
		c.prevFP = c.fp
		c.prevRows = c.rows
	}
	c.key = key
	c.fp = fp
	c.rows = rows
	c.mu.Unlock()
}

// changedSince reports tickets whose rendered row changed since the client
// stamp's fingerprint. known is false when that fingerprint is not the one
// this process last published, and the caller should resync.
func (c *liveFPCache) changedSince(fp string) (ids []string, known bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fp == "" || c.fp == "" {
		return nil, false
	}
	if fp == c.fp {
		return nil, true
	}
	if fp != c.prevFP || c.prevRows == nil || c.rows == nil {
		return nil, false
	}
	seen := map[string]struct{}{}
	for id, hash := range c.rows {
		if c.prevRows[id] != hash {
			ids = append(ids, id)
			seen[id] = struct{}{}
		}
	}
	for id := range c.prevRows {
		if _, ok := c.rows[id]; ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, true
}

type liveSnapshotter interface {
	LiveSnapshot(ctx context.Context) (string, []contracts.TicketSnapshot, error)
	ProjectionGeneration(ctx context.Context) (string, bool)
}

func (s *Server) computeLiveFP(ctx context.Context) (string, map[string]uint64, string, bool) {
	// The board is rendered from the sqlite projection. Fingerprinting the
	// markdown files instead races ahead of an in-flight reindex: the etag
	// says nothing changed while the cards on screen are still the old rows.
	if s.queries == nil || s.queries.Projection == nil {
		return "", nil, "", false
	}
	if snap, ok := s.queries.Projection.(liveSnapshotter); ok {
		gen, tickets, err := snap.LiveSnapshot(ctx)
		if err != nil || gen == "" {
			return "", nil, "", false
		}
		fp, rows := fingerprintTickets(tickets)
		if fp == "" {
			return "", nil, "", false
		}
		return fp, rows, gen, true
	}
	tickets, err := s.queries.Projection.QuerySearch(ctx, contracts.SearchQuery{})
	if err != nil {
		return "", nil, "", false
	}
	fp, rows := fingerprintTickets(tickets)
	if fp == "" {
		return "", nil, "", false
	}
	return fp, rows, "", true
}

func fingerprintTickets(tickets []contracts.TicketSnapshot) (string, map[string]uint64) {
	statuses := make(map[string]contracts.Status, len(tickets))
	for _, ticket := range tickets {
		statuses[ticket.ID] = ticket.Status
	}
	sort.Slice(tickets, func(i, j int) bool { return tickets[i].ID < tickets[j].ID })
	sum := fnv.New64a()
	rows := make(map[string]uint64, len(tickets))
	for _, ticket := range tickets {
		// The whole row covers reviewer, acceptance, relations and review
		// state. Board is the derived column, which changes when a blocker
		// is added or finished without rewriting this ticket.
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(struct {
			Ticket contracts.TicketSnapshot `json:"ticket"`
			Board  contracts.Status         `json:"board"`
		}{ticket, boardStatusForBlockers(ticket, statuses)})
		if err != nil {
			return "", nil
		}
		line := buf.Bytes()
		row := fnv.New64a()
		_, _ = row.Write(line)
		rows[ticket.ID] = row.Sum64()
		_, _ = sum.Write(line)
	}
	return strconv.FormatUint(sum.Sum64(), 16), rows
}

// boardStatusForBlockers matches the column QueryBoard renders. A blocker
// that is not done puts the dependent in Blocked without changing its row.
func boardStatusForBlockers(ticket contracts.TicketSnapshot, statuses map[string]contracts.Status) contracts.Status {
	if contracts.IsTerminalStatus(ticket.Status) {
		return ticket.Status
	}
	if ticket.Status == contracts.StatusBlocked {
		return contracts.StatusBlocked
	}
	for _, blockerID := range ticket.BlockedBy {
		blockerID = strings.TrimSpace(blockerID)
		if blockerID == "" {
			continue
		}
		status, ok := statuses[blockerID]
		if !ok || status != contracts.StatusDone {
			return contracts.StatusBlocked
		}
	}
	return ticket.Status
}

func stampCarriesGeneration(parts []string, gen string) bool {
	want := "db:data_version=" + gen
	for _, part := range parts {
		if part == want {
			return true
		}
	}
	return false
}

func (s *Server) projectionStampParts(ctx context.Context) []string {
	parts := projectionFileStamp(s.cfg.Root)
	gen, ok := s.projectionGeneration(ctx)
	if !ok {
		return parts
	}
	return append(parts, "db:data_version="+gen)
}

func (s *Server) projectionGeneration(ctx context.Context) (string, bool) {
	if s == nil || s.queries == nil || s.queries.Projection == nil {
		return "", false
	}
	src, ok := s.queries.Projection.(liveSnapshotter)
	if !ok {
		return "", false
	}
	return src.ProjectionGeneration(ctx)
}

func projectionFileStamp(root string) []string {
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

func fingerprintUnchanged(prev, cur map[string]string) bool {
	old := prev["fp"]
	next := cur["fp"]
	return old != "" && old == next
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
	// Saved views carry their own project, assignee, type and column scope.
	// Resolve that scope through loadBoard instead of patching raw tickets
	// with only the URL's filters.
	if strings.TrimSpace(r.URL.Query().Get("view")) != "" {
		return s.liveResync(r)
	}
	// A tail we cannot apply id-by-id still has to land as one resync.
	// Falling through to a full HTML page wedges a large board, which
	// discards that page and asks again forever.
	changed, known := s.fpCache().changedSince(prev.db["fp"])
	tail, tailOK := readGrownTails(s.cfg.Root, prev.files, cur.files)
	if !tailOK || !known {
		if tailOK && known && fingerprintUnchanged(prev.db, cur.db) && len(changed) == 0 && len(bytes.TrimSpace(tail)) == 0 {
			return liveBoardBody{Cards: []liveCardPatch{}}, true
		}
		return s.liveResync(r)
	}
	ids := unionIDs(ticketIDsFromTail(tail), changed)
	if len(ids) == 0 {
		// Same rows, new file generation: a no-op reindex. An empty patch
		// advances the etag; a full resync would reload every card.
		return liveBoardBody{Cards: []liveCardPatch{}}, true
	}
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
		if id == openID {
			body.Drawer = s.openDrawer(r, id)
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
	foundOpen := false
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
				drawer := s.drawerForTicket(r, card.Ticket)
				body.Drawer = &drawer
				foundOpen = true
			}
		}
	}
	if openID != "" && !foundOpen {
		body.Drawer = s.openDrawer(r, openID)
	}
	if body.Drawer != nil && !body.Drawer.Deleted {
		if comments, history, err := s.liveActivityHTML(r, openID); err == nil {
			body.CommentsHTML = comments
			body.HistoryHTML = history
		}
	}
	return body, true
}

func (s *Server) openDrawer(r *http.Request, id string) *liveDrawer {
	if s.actions == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	ticket, err := s.actions.Tickets.GetTicket(r.Context(), id)
	if err != nil || ticket.Archived {
		return &liveDrawer{ID: id, Deleted: true}
	}
	drawer := s.drawerForTicket(r, ticket)
	return &drawer
}

func (s *Server) drawerForTicket(r *http.Request, ticket contracts.TicketSnapshot) liveDrawer {
	drawer := drawerFromTicket(ticket)
	if html, err := s.liveActionsHTML(r, ticket.ID); err == nil {
		drawer.ActionsHTML = html
	}
	return drawer
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
	boardStatus := ticket.Status
	if s.queries != nil {
		if projected, err := s.queries.BoardStatus(ctx, ticket); err == nil && projected != "" {
			boardStatus = projected
		}
	}
	filtered := filterBoard(contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		boardStatus: {ticket},
	}}, page)
	if len(filtered.Columns[boardStatus]) == 0 {
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
		BoardStatus:       boardStatus,
		StatusLabel:       statusLabel(boardStatus),
		AgentName:         agentName,
		AgentColorClass:   colorClass,
		BoardPath:         page.BoardPath,
	}
	html, err := s.renderFragment(r, "card", card)
	if err != nil {
		return liveCardPatch{ID: id, Remove: true}, false
	}
	return liveCardPatch{ID: id, Status: string(boardStatus), HTML: html}, true
}

func unionIDs(groups ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, group := range groups {
		for _, id := range group {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
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
		CSRFToken:     s.csrfToken(),
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

func (s *Server) liveActionsHTML(r *http.Request, id string) (string, error) {
	detail, err := s.ticketDetail(r.Context(), id)
	if err != nil {
		return "", err
	}
	home, board, schedule, newTicket, prefix := s.navPaths()
	page := BoardPage{
		Page:          "board",
		Actor:         s.cfg.Actor,
		ReadOnly:      s.cfg.ReadOnly,
		CSRFToken:     s.csrfToken(),
		BoardPath:     board,
		ActionPrefix:  prefix,
		HomePath:      home,
		SchedulePath:  schedule,
		NewTicketPath: newTicket,
		Detail:        &detail,
	}
	return s.renderFragment(r, "drawerActions", page)
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
