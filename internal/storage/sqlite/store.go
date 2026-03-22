package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed projection and query engine.
type Store struct {
	DB           *sql.DB
	TicketSource contracts.TicketStore
	EventSource  contracts.EventLog
}

var _ contracts.ProjectionStore = (*Store)(nil)

func Open(path string, ticketSource contracts.TicketStore, eventSource contracts.EventLog) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	store := &Store{DB: db, TicketSource: ticketSource, EventSource: eventSource}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal mode: %w", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set sync mode: %w", err)
	}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS tickets (
			id TEXT PRIMARY KEY,
			project TEXT NOT NULL,
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			parent TEXT,
			labels_json TEXT NOT NULL,
			assignee TEXT,
			reviewer TEXT,
			blocked_by_json TEXT NOT NULL,
			blocks_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			archived INTEGER NOT NULL DEFAULT 0,
			summary TEXT,
			description TEXT,
			acceptance_json TEXT NOT NULL,
			notes TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_status ON tickets(project, status);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_updated ON tickets(project, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_assignee ON tickets(assignee);`,
		`CREATE TABLE IF NOT EXISTS events (
			project TEXT NOT NULL,
			event_id INTEGER NOT NULL,
			ticket_id TEXT,
			ts TEXT NOT NULL,
			actor TEXT NOT NULL,
			reason TEXT,
			type TEXT NOT NULL,
			payload_json TEXT,
			schema_version INTEGER NOT NULL,
			PRIMARY KEY (project, event_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_events_ticket ON events(ticket_id);`,
	}
	for _, stmt := range statements {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite migrate failed: %w", err)
		}
	}
	return nil
}

func (s *Store) ApplyEvent(ctx context.Context, event contracts.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if err := s.insertEventOnly(ctx, event); err != nil {
		return err
	}

	for _, ticket := range extractTicketSnapshots(event.Payload) {
		if err := s.upsertTicket(ctx, ticket); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Rebuild(ctx context.Context, project string) error {
	if s.TicketSource == nil || s.EventSource == nil {
		return fmt.Errorf("rebuild requires ticket and event sources")
	}
	if project == "" {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM tickets; DELETE FROM events;`); err != nil {
			return fmt.Errorf("clear projection: %w", err)
		}
	} else {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM tickets WHERE project = ?`, project); err != nil {
			return fmt.Errorf("clear project tickets: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM events WHERE project = ?`, project); err != nil {
			return fmt.Errorf("clear project events: %w", err)
		}
	}

	events, err := s.EventSource.StreamEvents(ctx, project, 0)
	if err != nil {
		return fmt.Errorf("load events from source: %w", err)
	}
	for _, event := range events {
		if err := s.insertEventOnly(ctx, event); err != nil {
			return err
		}
		for _, ticket := range extractTicketSnapshots(event.Payload) {
			if err := s.upsertTicket(ctx, ticket); err != nil {
				return err
			}
		}
	}

	tickets, err := s.TicketSource.ListTickets(ctx, contracts.TicketListOptions{Project: project, IncludeArchived: true})
	if err != nil {
		return fmt.Errorf("load tickets from source: %w", err)
	}
	for _, ticket := range tickets {
		if err := s.insertTicketIfMissing(ctx, ticket); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) QueryBoard(ctx context.Context, opts contracts.BoardQueryOptions) (contracts.BoardView, error) {
	query := `SELECT id, project, title, type, status, priority, parent, labels_json, assignee, reviewer, blocked_by_json, blocks_json,
	created_at, updated_at, schema_version, archived, summary, description, acceptance_json, notes
	FROM tickets WHERE archived = 0`
	args := make([]any, 0)
	if opts.Project != "" {
		query += ` AND project = ?`
		args = append(args, opts.Project)
	}
	if opts.Assignee != "" {
		query += ` AND assignee = ?`
		args = append(args, string(opts.Assignee))
	}
	if opts.Type != "" {
		query += ` AND type = ?`
		args = append(args, string(opts.Type))
	}
	query += ` ORDER BY updated_at ASC, id ASC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return contracts.BoardView{}, fmt.Errorf("query board: %w", err)
	}
	defer rows.Close()

	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return contracts.BoardView{}, err
		}
		boardTicket := ticket
		boardTicket.Status = contracts.BoardStatus(ticket)
		columns[boardTicket.Status] = append(columns[boardTicket.Status], boardTicket)
	}
	if err := rows.Err(); err != nil {
		return contracts.BoardView{}, err
	}
	return contracts.BoardView{Columns: columns}, nil
}

func (s *Store) QuerySearch(ctx context.Context, query contracts.SearchQuery) ([]contracts.TicketSnapshot, error) {
	base := `SELECT id, project, title, type, status, priority, parent, labels_json, assignee, reviewer, blocked_by_json, blocks_json,
	created_at, updated_at, schema_version, archived, summary, description, acceptance_json, notes
	FROM tickets WHERE 1=1`
	args := make([]any, 0)
	for _, term := range query.Terms {
		switch term.Kind {
		case contracts.SearchTermStatus:
			base += ` AND status = ?`
			args = append(args, term.Value)
		case contracts.SearchTermType:
			base += ` AND type = ?`
			args = append(args, term.Value)
		case contracts.SearchTermProject:
			base += ` AND project = ?`
			args = append(args, term.Value)
		case contracts.SearchTermAssignee:
			base += ` AND assignee = ?`
			args = append(args, term.Value)
		case contracts.SearchTermLabel:
			base += ` AND labels_json LIKE ?`
			args = append(args, "%\""+term.Value+"\"%")
		case contracts.SearchTermTextLike:
			base += ` AND LOWER(COALESCE(title,'') || ' ' || COALESCE(summary,'') || ' ' || COALESCE(description,'') || ' ' || COALESCE(notes,'')) LIKE ?`
			args = append(args, "%"+strings.ToLower(term.Value)+"%")
		default:
			return nil, fmt.Errorf("unsupported search term kind: %s", term.Kind)
		}
	}
	base += ` ORDER BY updated_at DESC, id ASC`

	rows, err := s.DB.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("query search: %w", err)
	}
	defer rows.Close()

	result := make([]contracts.TicketSnapshot, 0)
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) QueryHistory(ctx context.Context, ticketID string) ([]contracts.Event, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT event_id, ts, actor, reason, type, project, ticket_id, payload_json, schema_version
		FROM events
		WHERE ticket_id = ?
		ORDER BY event_id ASC
	`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	events := make([]contracts.Event, 0)
	for rows.Next() {
		var (
			event       contracts.Event
			ts          string
			actor       string
			eventType   string
			payloadJSON sql.NullString
		)
		if err := rows.Scan(&event.EventID, &ts, &actor, &event.Reason, &eventType, &event.Project, &event.TicketID, &payloadJSON, &event.SchemaVersion); err != nil {
			return nil, err
		}
		parsedTS, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, fmt.Errorf("parse event timestamp: %w", err)
		}
		event.Timestamp = parsedTS
		event.Actor = contracts.Actor(actor)
		event.Type = contracts.EventType(eventType)
		if payloadJSON.Valid && payloadJSON.String != "" {
			var payload any
			if err := json.Unmarshal([]byte(payloadJSON.String), &payload); err != nil {
				return nil, fmt.Errorf("decode event payload: %w", err)
			}
			event.Payload = payload
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) upsertTicket(ctx context.Context, ticket contracts.TicketSnapshot) error {
	labelsJSON, err := json.Marshal(ticket.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	blockedByJSON, err := json.Marshal(ticket.BlockedBy)
	if err != nil {
		return fmt.Errorf("marshal blocked_by: %w", err)
	}
	blocksJSON, err := json.Marshal(ticket.Blocks)
	if err != nil {
		return fmt.Errorf("marshal blocks: %w", err)
	}
	acceptanceJSON, err := json.Marshal(ticket.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("marshal acceptance criteria: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO tickets (
			id, project, title, type, status, priority, parent, labels_json, assignee, reviewer,
			blocked_by_json, blocks_json, created_at, updated_at, schema_version, archived,
			summary, description, acceptance_json, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title,
			type=excluded.type,
			status=excluded.status,
			priority=excluded.priority,
			parent=excluded.parent,
			labels_json=excluded.labels_json,
			assignee=excluded.assignee,
			reviewer=excluded.reviewer,
			blocked_by_json=excluded.blocked_by_json,
			blocks_json=excluded.blocks_json,
			updated_at=excluded.updated_at,
			schema_version=excluded.schema_version,
			archived=excluded.archived,
			summary=excluded.summary,
			description=excluded.description,
			acceptance_json=excluded.acceptance_json,
			notes=excluded.notes
	`, ticket.ID, ticket.Project, ticket.Title, string(ticket.Type), string(ticket.Status), string(ticket.Priority), nullable(ticket.Parent), string(labelsJSON), nullable(string(ticket.Assignee)), nullable(string(ticket.Reviewer)), string(blockedByJSON), string(blocksJSON), ticket.CreatedAt.UTC().Format(time.RFC3339Nano), ticket.UpdatedAt.UTC().Format(time.RFC3339Nano), ticket.SchemaVersion, boolToInt(ticket.Archived), nullable(ticket.Summary), nullable(ticket.Description), string(acceptanceJSON), nullable(ticket.Notes))
	if err != nil {
		return fmt.Errorf("upsert ticket %s: %w", ticket.ID, err)
	}
	return nil
}

func (s *Store) insertTicketIfMissing(ctx context.Context, ticket contracts.TicketSnapshot) error {
	labelsJSON, err := json.Marshal(ticket.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	blockedByJSON, err := json.Marshal(ticket.BlockedBy)
	if err != nil {
		return fmt.Errorf("marshal blocked_by: %w", err)
	}
	blocksJSON, err := json.Marshal(ticket.Blocks)
	if err != nil {
		return fmt.Errorf("marshal blocks: %w", err)
	}
	acceptanceJSON, err := json.Marshal(ticket.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("marshal acceptance criteria: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO tickets (
			id, project, title, type, status, priority, parent, labels_json, assignee, reviewer,
			blocked_by_json, blocks_json, created_at, updated_at, schema_version, archived,
			summary, description, acceptance_json, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, ticket.ID, ticket.Project, ticket.Title, string(ticket.Type), string(ticket.Status), string(ticket.Priority), nullable(ticket.Parent), string(labelsJSON), nullable(string(ticket.Assignee)), nullable(string(ticket.Reviewer)), string(blockedByJSON), string(blocksJSON), ticket.CreatedAt.UTC().Format(time.RFC3339Nano), ticket.UpdatedAt.UTC().Format(time.RFC3339Nano), ticket.SchemaVersion, boolToInt(ticket.Archived), nullable(ticket.Summary), nullable(ticket.Description), string(acceptanceJSON), nullable(ticket.Notes))
	if err != nil {
		return fmt.Errorf("insert missing ticket %s: %w", ticket.ID, err)
	}
	return nil
}

func (s *Store) insertEventOnly(ctx context.Context, event contracts.Event) error {
	payloadJSON := ""
	if event.Payload != nil {
		raw, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("marshal event payload: %w", err)
		}
		payloadJSON = string(raw)
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO events (project, event_id, ticket_id, ts, actor, reason, type, payload_json, schema_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, event_id) DO NOTHING
	`, event.Project, event.EventID, event.TicketID, event.Timestamp.UTC().Format(time.RFC3339Nano), string(event.Actor), event.Reason, string(event.Type), payloadJSON, event.SchemaVersion)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func scanTicket(rows *sql.Rows) (contracts.TicketSnapshot, error) {
	var (
		ticket         contracts.TicketSnapshot
		typeValue      string
		statusValue    string
		priorityValue  string
		createdAt      string
		updatedAt      string
		archived       int
		labelsJSON     string
		blockedByJSON  string
		blocksJSON     string
		acceptanceJSON string
		parent         sql.NullString
		assignee       sql.NullString
		reviewer       sql.NullString
		summary        sql.NullString
		description    sql.NullString
		notes          sql.NullString
	)
	if err := rows.Scan(
		&ticket.ID,
		&ticket.Project,
		&ticket.Title,
		&typeValue,
		&statusValue,
		&priorityValue,
		&parent,
		&labelsJSON,
		&assignee,
		&reviewer,
		&blockedByJSON,
		&blocksJSON,
		&createdAt,
		&updatedAt,
		&ticket.SchemaVersion,
		&archived,
		&summary,
		&description,
		&acceptanceJSON,
		&notes,
	); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(labelsJSON), &ticket.Labels); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(blockedByJSON), &ticket.BlockedBy); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(blocksJSON), &ticket.Blocks); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(acceptanceJSON), &ticket.AcceptanceCriteria); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	ticket.Type = contracts.TicketType(typeValue)
	ticket.Status = contracts.Status(statusValue)
	ticket.Priority = contracts.Priority(priorityValue)
	ticket.CreatedAt = parsedCreatedAt
	ticket.UpdatedAt = parsedUpdatedAt
	ticket.Archived = archived == 1
	if parent.Valid {
		ticket.Parent = parent.String
	}
	if assignee.Valid {
		ticket.Assignee = contracts.Actor(assignee.String)
	}
	if reviewer.Valid {
		ticket.Reviewer = contracts.Actor(reviewer.String)
	}
	if summary.Valid {
		ticket.Summary = summary.String
	}
	if description.Valid {
		ticket.Description = description.String
	}
	if notes.Valid {
		ticket.Notes = notes.String
	}
	return ticket, nil
}

func extractTicketSnapshots(payload any) []contracts.TicketSnapshot {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	result := make([]contracts.TicketSnapshot, 0, 2)
	seen := map[string]struct{}{}
	appendTicket := func(ticket contracts.TicketSnapshot) {
		if ticket.ValidateForCreate() != nil {
			return
		}
		if _, ok := seen[ticket.ID]; ok {
			return
		}
		seen[ticket.ID] = struct{}{}
		result = append(result, ticket)
	}

	var wrapped struct {
		Ticket      contracts.TicketSnapshot `json:"ticket"`
		OtherTicket contracts.TicketSnapshot `json:"other_ticket"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		appendTicket(wrapped.Ticket)
		appendTicket(wrapped.OtherTicket)
	}

	var ticket contracts.TicketSnapshot
	if err := json.Unmarshal(raw, &ticket); err == nil {
		appendTicket(ticket)
	}

	return result
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
