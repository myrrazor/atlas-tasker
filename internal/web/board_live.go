package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
// stamp (sqlite+wal size:mtime plus the WAL commit counter). Idle polls
// reuse it and never open the index.
type liveFPCache struct {
	mu  sync.Mutex
	key string
	fp  string
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
	before := projectionStampParts(s.cfg.Root)
	key := strings.Join(before, "|")
	if key == "" {
		return nil
	}
	cache := s.fpCache()
	if fp, ok := cache.lookup(key); ok {
		return append(append([]string{}, before...), "db:fp="+fp)
	}
	fp, computed := s.computeLiveFP(r.Context())
	after := projectionStampParts(s.cfg.Root)
	afterKey := strings.Join(after, "|")
	if afterKey == "" {
		return nil
	}
	// The index moved while the rows were read, or a commit was in the
	// middle of publishing the WAL header. Caching that fingerprint under
	// the new stamp would 304 a stale board forever. Omit it so the client
	// resyncs, and let the next idle poll fill the cache.
	if !computed || afterKey != key || strings.Contains(afterKey, "walgen=pending") {
		return after
	}
	cache.store(afterKey, fp)
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

func (c *liveFPCache) store(key, fp string) {
	if key == "" || fp == "" {
		return
	}
	c.mu.Lock()
	c.key = key
	c.fp = fp
	c.mu.Unlock()
}

func (s *Server) computeLiveFP(ctx context.Context) (string, bool) {
	// The board is rendered from the sqlite projection. Fingerprinting the
	// markdown files instead races ahead of an in-flight reindex: the etag
	// says nothing changed while the cards on screen are still the old rows.
	if s.queries == nil || s.queries.Projection == nil {
		return "", false
	}
	tickets, err := s.queries.Projection.QuerySearch(ctx, contracts.SearchQuery{})
	if err != nil {
		return "", false
	}
	return fingerprintTickets(tickets), true
}

func fingerprintTickets(tickets []contracts.TicketSnapshot) string {
	sort.Slice(tickets, func(i, j int) bool { return tickets[i].ID < tickets[j].ID })
	sum := fnv.New64a()
	for _, ticket := range tickets {
		fmt.Fprintf(sum, "%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%t\x1f%d\n",
			ticket.ID,
			ticket.Status,
			ticket.Title,
			ticket.Description,
			ticket.Notes,
			ticket.Priority,
			ticket.Assignee,
			strings.Join(ticket.Labels, ","),
			ticket.Archived,
			ticket.UpdatedAt.UnixNano(),
		)
	}
	return strconv.FormatUint(sum.Sum64(), 16)
}

func projectionStampParts(root string) []string {
	dir := storage.TrackerDir(root)
	parts := make([]string, 0, 3)
	for _, name := range []string{"index.sqlite", "index.sqlite-wal"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			parts = append(parts, "db:"+name+"=0:0")
			continue
		}
		parts = append(parts, "db:"+name+"="+strconv.FormatInt(info.Size(), 10)+":"+strconv.FormatInt(info.ModTime().UnixNano(), 10))
	}
	// sqlite+wal size:mtime is not a commit counter. Frames land in the WAL
	// before readers can see them, and a later commit often keeps the same
	// mtime. The WAL index header is what a reader uses to notice a commit,
	// and idle polls do not change it. The shm file's own size and mtime
	// stay out of the key: they do not move on commit and must not be
	// treated as a generation.
	gen, ok := walCommitGeneration(filepath.Join(dir, "index.sqlite-shm"))
	if !ok {
		gen = "pending"
	}
	parts = append(parts, "db:index.sqlite-walgen="+gen)
	return parts
}

// walCommitGeneration reads the two copies of the WAL index header.
// Layout (host endian), from SQLite's wal-index header:
//
//	0  iVersion uint32
//	8  iChange  uint32  incremented on each commit
//	16 mxFrame  uint32
//	32 aSalt    uint32[2]
//
// ok is false when a commit is between the two header copies.
func walCommitGeneration(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "0", true
		}
		return "", false
	}
	defer f.Close()
	var buf [96]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return "", false
	}
	if !bytes.Equal(buf[:48], buf[48:96]) {
		return "", false
	}
	iChange := binary.NativeEndian.Uint32(buf[8:12])
	mxFrame := binary.NativeEndian.Uint32(buf[16:20])
	salt1 := binary.NativeEndian.Uint32(buf[32:36])
	salt2 := binary.NativeEndian.Uint32(buf[36:40])
	return strconv.FormatUint(uint64(iChange), 10) + ":" +
		strconv.FormatUint(uint64(mxFrame), 10) + ":" +
		strconv.FormatUint(uint64(salt1), 10) + ":" +
		strconv.FormatUint(uint64(salt2), 10), true
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
	// A tail we cannot apply id-by-id, or an index change with no new events
	// (reindex, git pull), still has to land. One resync, then the new stamp
	// makes later polls 304. Falling through to a full HTML page wedges a
	// large board, which discards that page and asks again forever.
	tail, tailOK := readGrownTails(s.cfg.Root, prev.files, cur.files)
	if !tailOK || (projectionChanged(prev.db, cur.db) && len(bytes.TrimSpace(tail)) == 0) {
		// Same rows, new file mtimes: reindex rewrote the index and appended
		// nothing. An empty patch advances the etag; a full resync would
		// reload every card.
		if tailOK && fingerprintUnchanged(prev.db, cur.db) {
			return liveBoardBody{Cards: []liveCardPatch{}}, true
		}
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
		CSRFToken:     s.cfg.CSRFToken,
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
