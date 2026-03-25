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

const ticketSelectColumns = `
	id, project, title, type, status, priority, parent, labels_json, assignee, reviewer,
	blocked_by_json, blocks_json, created_at, updated_at, schema_version, archived,
	summary, description, acceptance_json, notes, policy_json, review_state,
	lease_actor, lease_kind, lease_acquired_at, lease_expires_at, lease_heartbeat_at,
	template, skill_hint, blueprint, progress_json,
	required_capabilities_json, dispatch_mode, allow_parallel_runs, runbook,
	latest_run_id, latest_handoff_id, open_gate_ids_json, last_dispatch_at
`

// Store is a SQLite-backed projection and query engine.
type Store struct {
	Path         string
	DB           *sql.DB
	TicketSource contracts.TicketStore
	EventSource  contracts.EventLog
}

var _ contracts.ProjectionStore = (*Store)(nil)

func Open(path string, ticketSource contracts.TicketStore, eventSource contracts.EventLog) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	store := &Store{Path: path, DB: db, TicketSource: ticketSource, EventSource: eventSource}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal mode: %w", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set sync mode: %w", err)
	}
	return db, nil
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
			notes TEXT,
			policy_json TEXT NOT NULL DEFAULT '{}',
			review_state TEXT NOT NULL DEFAULT 'none',
			lease_actor TEXT,
			lease_kind TEXT,
			lease_acquired_at TEXT,
			lease_expires_at TEXT,
			lease_heartbeat_at TEXT,
			template TEXT,
			skill_hint TEXT,
			blueprint TEXT,
			progress_json TEXT NOT NULL DEFAULT '{}',
			required_capabilities_json TEXT NOT NULL DEFAULT '[]',
			dispatch_mode TEXT NOT NULL DEFAULT 'manual',
			allow_parallel_runs INTEGER NOT NULL DEFAULT 0,
			runbook TEXT,
			latest_run_id TEXT,
			latest_handoff_id TEXT,
			open_gate_ids_json TEXT NOT NULL DEFAULT '[]',
			last_dispatch_at TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_status ON tickets(project, status);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_updated ON tickets(project, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_assignee ON tickets(assignee);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_review_state ON tickets(review_state);`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_lease_expires ON tickets(lease_expires_at);`,
		`CREATE TABLE IF NOT EXISTS agents (
			agent_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			provider TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			capabilities_json TEXT NOT NULL DEFAULT '[]',
			allowed_ticket_types_json TEXT NOT NULL DEFAULT '[]',
			default_runbook TEXT,
			max_active_runs INTEGER NOT NULL DEFAULT 0,
			preferred_roles_json TEXT NOT NULL DEFAULT '[]',
			routing_weight INTEGER NOT NULL DEFAULT 0,
			instruction_profile TEXT,
			launch_target TEXT,
			integration_template TEXT,
			notes TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL,
			project TEXT NOT NULL,
			agent_id TEXT,
			provider TEXT,
			status TEXT NOT NULL,
			kind TEXT NOT NULL,
			blueprint_stage TEXT,
			worktree_path TEXT,
			branch_name TEXT,
			created_at TEXT NOT NULL,
			started_at TEXT,
			completed_at TEXT,
			last_heartbeat_at TEXT,
			result TEXT,
			summary TEXT,
			handoff_to TEXT,
			supersedes_run_id TEXT,
			evidence_count INTEGER NOT NULL DEFAULT 0,
			session_provider TEXT,
			session_ref TEXT,
			schema_version INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_ticket_status ON runs(ticket_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_agent_status ON runs(agent_id, status);`,
		`CREATE TABLE IF NOT EXISTS gates (
			gate_id TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL,
			run_id TEXT,
			kind TEXT NOT NULL,
			state TEXT NOT NULL,
			required_role TEXT,
			required_agent_id TEXT,
			created_by TEXT NOT NULL,
			decided_by TEXT,
			decision_reason TEXT,
			evidence_requirements_json TEXT NOT NULL DEFAULT '[]',
			related_run_ids_json TEXT NOT NULL DEFAULT '[]',
			replaces_gate_id TEXT,
			created_at TEXT NOT NULL,
			decided_at TEXT,
			schema_version INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_gates_ticket_state ON gates(ticket_id, state);`,
		`CREATE TABLE IF NOT EXISTS evidence (
			evidence_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL,
			type TEXT NOT NULL,
			title TEXT,
			body TEXT,
			artifact_path TEXT,
			supersedes_evidence_id TEXT,
			actor TEXT NOT NULL,
			created_at TEXT NOT NULL,
			schema_version INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_evidence_run_created ON evidence(run_id, created_at);`,
		`CREATE TABLE IF NOT EXISTS handoffs (
			handoff_id TEXT PRIMARY KEY,
			source_run_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL,
			actor TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			schema_version INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_handoffs_ticket_generated ON handoffs(ticket_id, generated_at);`,
		`CREATE TABLE IF NOT EXISTS events (
			project TEXT NOT NULL,
			event_id INTEGER NOT NULL,
			ticket_id TEXT,
			ts TEXT NOT NULL,
			actor TEXT NOT NULL,
			reason TEXT,
			type TEXT NOT NULL,
			payload_json TEXT,
			metadata_json TEXT NOT NULL DEFAULT '{}',
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

	columns := []struct {
		name       string
		definition string
	}{
		{name: "policy_json", definition: `TEXT NOT NULL DEFAULT '{}'`},
		{name: "review_state", definition: `TEXT NOT NULL DEFAULT 'none'`},
		{name: "lease_actor", definition: `TEXT`},
		{name: "lease_kind", definition: `TEXT`},
		{name: "lease_acquired_at", definition: `TEXT`},
		{name: "lease_expires_at", definition: `TEXT`},
		{name: "lease_heartbeat_at", definition: `TEXT`},
		{name: "template", definition: `TEXT`},
		{name: "skill_hint", definition: `TEXT`},
		{name: "blueprint", definition: `TEXT`},
		{name: "progress_json", definition: `TEXT NOT NULL DEFAULT '{}'`},
		{name: "required_capabilities_json", definition: `TEXT NOT NULL DEFAULT '[]'`},
		{name: "dispatch_mode", definition: `TEXT NOT NULL DEFAULT 'manual'`},
		{name: "allow_parallel_runs", definition: `INTEGER NOT NULL DEFAULT 0`},
		{name: "runbook", definition: `TEXT`},
		{name: "latest_run_id", definition: `TEXT`},
		{name: "latest_handoff_id", definition: `TEXT`},
		{name: "open_gate_ids_json", definition: `TEXT NOT NULL DEFAULT '[]'`},
		{name: "last_dispatch_at", definition: `TEXT`},
	}
	for _, column := range columns {
		if err := s.ensureTicketColumn(column.name, column.definition); err != nil {
			return err
		}
	}
	if err := s.ensureEventColumn("metadata_json", `TEXT NOT NULL DEFAULT '{}'`); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureTicketColumn(name string, definition string) error {
	rows, err := s.DB.Query(`PRAGMA table_info(tickets)`)
	if err != nil {
		return fmt.Errorf("inspect tickets schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			columnName string
			typ        string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &columnName, &typ, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table info: %w", err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE tickets ADD COLUMN ` + name + ` ` + definition); err != nil {
		return fmt.Errorf("add tickets.%s: %w", name, err)
	}
	return nil
}

func (s *Store) ensureEventColumn(name string, definition string) error {
	rows, err := s.DB.Query(`PRAGMA table_info(events)`)
	if err != nil {
		return fmt.Errorf("inspect events schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			columnName string
			typ        string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &columnName, &typ, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan events table info: %w", err)
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate events table info: %w", err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE events ADD COLUMN ` + name + ` ` + definition); err != nil {
		return fmt.Errorf("add events.%s: %w", name, err)
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
	for _, agent := range extractAgentProfiles(event.Payload) {
		if err := s.upsertAgent(ctx, agent); err != nil {
			return err
		}
	}
	for _, run := range extractRunSnapshots(event.Payload) {
		if err := s.upsertRun(ctx, run); err != nil {
			return err
		}
	}
	for _, evidence := range extractEvidenceItems(event.Payload) {
		if err := s.upsertEvidence(ctx, evidence); err != nil {
			return err
		}
	}
	for _, handoff := range extractHandoffPackets(event.Payload) {
		if err := s.upsertHandoff(ctx, handoff); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Rebuild(ctx context.Context, project string) error {
	if project == "" && s.Path != "" {
		return s.rebuildBySwap(ctx)
	}
	return s.rebuildInPlace(ctx, project)
}

func (s *Store) rebuildInPlace(ctx context.Context, project string) error {
	if s.TicketSource == nil || s.EventSource == nil {
		return fmt.Errorf("rebuild requires ticket and event sources")
	}
	if project == "" {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM tickets`); err != nil {
			return fmt.Errorf("clear tickets: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM agents`); err != nil {
			return fmt.Errorf("clear agents: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM runs`); err != nil {
			return fmt.Errorf("clear runs: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM gates`); err != nil {
			return fmt.Errorf("clear gates: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM evidence`); err != nil {
			return fmt.Errorf("clear evidence: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM handoffs`); err != nil {
			return fmt.Errorf("clear handoffs: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM events`); err != nil {
			return fmt.Errorf("clear events: %w", err)
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
		for _, agent := range extractAgentProfiles(event.Payload) {
			if err := s.upsertAgent(ctx, agent); err != nil {
				return err
			}
		}
		for _, run := range extractRunSnapshots(event.Payload) {
			if err := s.upsertRun(ctx, run); err != nil {
				return err
			}
		}
		for _, evidence := range extractEvidenceItems(event.Payload) {
			if err := s.upsertEvidence(ctx, evidence); err != nil {
				return err
			}
		}
		for _, handoff := range extractHandoffPackets(event.Payload) {
			if err := s.upsertHandoff(ctx, handoff); err != nil {
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

func (s *Store) rebuildBySwap(ctx context.Context) error {
	if s.TicketSource == nil || s.EventSource == nil {
		return fmt.Errorf("rebuild requires ticket and event sources")
	}
	tempPath := s.Path + ".rebuild"
	for _, candidate := range []string{tempPath, tempPath + "-wal", tempPath + "-shm"} {
		_ = os.Remove(candidate)
	}
	tempStore, err := Open(tempPath, s.TicketSource, s.EventSource)
	if err != nil {
		return err
	}
	if err := tempStore.rebuildInPlace(ctx, ""); err != nil {
		_ = tempStore.Close()
		return err
	}
	if err := tempStore.Close(); err != nil {
		return fmt.Errorf("close rebuilt temp projection: %w", err)
	}
	if err := s.Close(); err != nil {
		return fmt.Errorf("close existing projection: %w", err)
	}
	for _, candidate := range []string{s.Path, s.Path + "-wal", s.Path + "-shm"} {
		_ = os.Remove(candidate)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := tempPath + suffix
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, s.Path+suffix); err != nil {
				return fmt.Errorf("swap projection file %s: %w", filepath.Base(src), err)
			}
		}
	}
	db, err := openDB(s.Path)
	if err != nil {
		return err
	}
	s.DB = db
	return s.migrate()
}

func (s *Store) QueryBoard(ctx context.Context, opts contracts.BoardQueryOptions) (contracts.BoardView, error) {
	query := `SELECT ` + ticketSelectColumns + ` FROM tickets WHERE archived = 0`
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

func (s *Store) QueryTicket(ctx context.Context, ticketID string) (contracts.TicketSnapshot, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+ticketSelectColumns+` FROM tickets WHERE id = ?`, ticketID)
	ticket, err := scanTicket(row)
	if err != nil {
		return contracts.TicketSnapshot{}, fmt.Errorf("query ticket %s: %w", ticketID, err)
	}
	return ticket, nil
}

func (s *Store) QuerySearch(ctx context.Context, query contracts.SearchQuery) ([]contracts.TicketSnapshot, error) {
	base := `SELECT ` + ticketSelectColumns + ` FROM tickets WHERE 1=1`
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
		SELECT event_id, ts, actor, reason, type, project, ticket_id, payload_json, metadata_json, schema_version
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
			event        contracts.Event
			ts           string
			actor        string
			eventType    string
			payloadJSON  sql.NullString
			metadataJSON sql.NullString
		)
		if err := rows.Scan(&event.EventID, &ts, &actor, &event.Reason, &eventType, &event.Project, &event.TicketID, &payloadJSON, &metadataJSON, &event.SchemaVersion); err != nil {
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
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &event.Metadata); err != nil {
				return nil, fmt.Errorf("decode event metadata: %w", err)
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) upsertTicket(ctx context.Context, ticket contracts.TicketSnapshot) error {
	ticket = contracts.NormalizeTicketSnapshot(ticket)
	labelsJSON, blockedByJSON, blocksJSON, acceptanceJSON, policyJSON, progressJSON, requiredCapabilitiesJSON, openGateIDsJSON, err := marshalTicketJSON(ticket)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO tickets (
			id, project, title, type, status, priority, parent, labels_json, assignee, reviewer,
			blocked_by_json, blocks_json, created_at, updated_at, schema_version, archived,
			summary, description, acceptance_json, notes, policy_json, review_state,
			lease_actor, lease_kind, lease_acquired_at, lease_expires_at, lease_heartbeat_at,
			template, skill_hint, blueprint, progress_json,
			required_capabilities_json, dispatch_mode, allow_parallel_runs, runbook,
			latest_run_id, latest_handoff_id, open_gate_ids_json, last_dispatch_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			notes=excluded.notes,
			policy_json=excluded.policy_json,
			review_state=excluded.review_state,
			lease_actor=excluded.lease_actor,
			lease_kind=excluded.lease_kind,
			lease_acquired_at=excluded.lease_acquired_at,
			lease_expires_at=excluded.lease_expires_at,
			lease_heartbeat_at=excluded.lease_heartbeat_at,
			template=excluded.template,
			skill_hint=excluded.skill_hint,
			blueprint=excluded.blueprint,
			progress_json=excluded.progress_json,
			required_capabilities_json=excluded.required_capabilities_json,
			dispatch_mode=excluded.dispatch_mode,
			allow_parallel_runs=excluded.allow_parallel_runs,
			runbook=excluded.runbook,
			latest_run_id=excluded.latest_run_id,
			latest_handoff_id=excluded.latest_handoff_id,
			open_gate_ids_json=excluded.open_gate_ids_json,
			last_dispatch_at=excluded.last_dispatch_at
	`,
		ticket.ID, ticket.Project, ticket.Title, string(ticket.Type), string(ticket.Status), string(ticket.Priority), nullable(ticket.Parent),
		labelsJSON, nullable(string(ticket.Assignee)), nullable(string(ticket.Reviewer)), blockedByJSON, blocksJSON,
		ticket.CreatedAt.UTC().Format(time.RFC3339Nano), ticket.UpdatedAt.UTC().Format(time.RFC3339Nano), ticket.SchemaVersion, boolToInt(ticket.Archived),
		nullable(ticket.Summary), nullable(ticket.Description), acceptanceJSON, nullable(ticket.Notes), policyJSON, string(ticket.ReviewState),
		nullable(string(ticket.Lease.Actor)), nullable(string(ticket.Lease.Kind)), nullableTime(ticket.Lease.AcquiredAt), nullableTime(ticket.Lease.ExpiresAt), nullableTime(ticket.Lease.LastHeartbeatAt),
		nullable(ticket.Template), nullable(ticket.SkillHint), nullable(ticket.Blueprint), progressJSON,
		requiredCapabilitiesJSON, string(ticket.DispatchMode), boolToInt(ticket.AllowParallelRuns), nullable(ticket.Runbook),
		nullable(ticket.LatestRunID), nullable(ticket.LatestHandoffID), openGateIDsJSON, nullableTime(ticket.LastDispatchAt),
	)
	if err != nil {
		return fmt.Errorf("upsert ticket %s: %w", ticket.ID, err)
	}
	return nil
}

func (s *Store) insertTicketIfMissing(ctx context.Context, ticket contracts.TicketSnapshot) error {
	ticket = contracts.NormalizeTicketSnapshot(ticket)
	labelsJSON, blockedByJSON, blocksJSON, acceptanceJSON, policyJSON, progressJSON, requiredCapabilitiesJSON, openGateIDsJSON, err := marshalTicketJSON(ticket)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO tickets (
			id, project, title, type, status, priority, parent, labels_json, assignee, reviewer,
			blocked_by_json, blocks_json, created_at, updated_at, schema_version, archived,
			summary, description, acceptance_json, notes, policy_json, review_state,
			lease_actor, lease_kind, lease_acquired_at, lease_expires_at, lease_heartbeat_at,
			template, skill_hint, blueprint, progress_json,
			required_capabilities_json, dispatch_mode, allow_parallel_runs, runbook,
			latest_run_id, latest_handoff_id, open_gate_ids_json, last_dispatch_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`,
		ticket.ID, ticket.Project, ticket.Title, string(ticket.Type), string(ticket.Status), string(ticket.Priority), nullable(ticket.Parent),
		labelsJSON, nullable(string(ticket.Assignee)), nullable(string(ticket.Reviewer)), blockedByJSON, blocksJSON,
		ticket.CreatedAt.UTC().Format(time.RFC3339Nano), ticket.UpdatedAt.UTC().Format(time.RFC3339Nano), ticket.SchemaVersion, boolToInt(ticket.Archived),
		nullable(ticket.Summary), nullable(ticket.Description), acceptanceJSON, nullable(ticket.Notes), policyJSON, string(ticket.ReviewState),
		nullable(string(ticket.Lease.Actor)), nullable(string(ticket.Lease.Kind)), nullableTime(ticket.Lease.AcquiredAt), nullableTime(ticket.Lease.ExpiresAt), nullableTime(ticket.Lease.LastHeartbeatAt),
		nullable(ticket.Template), nullable(ticket.SkillHint), nullable(ticket.Blueprint), progressJSON,
		requiredCapabilitiesJSON, string(ticket.DispatchMode), boolToInt(ticket.AllowParallelRuns), nullable(ticket.Runbook),
		nullable(ticket.LatestRunID), nullable(ticket.LatestHandoffID), openGateIDsJSON, nullableTime(ticket.LastDispatchAt),
	)
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
	metadataJSON := "{}"
	if raw, err := json.Marshal(event.Metadata); err == nil {
		metadataJSON = string(raw)
	} else {
		return fmt.Errorf("marshal event metadata: %w", err)
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO events (project, event_id, ticket_id, ts, actor, reason, type, payload_json, metadata_json, schema_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, event_id) DO NOTHING
	`, event.Project, event.EventID, event.TicketID, event.Timestamp.UTC().Format(time.RFC3339Nano), string(event.Actor), event.Reason, string(event.Type), payloadJSON, metadataJSON, event.SchemaVersion)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *Store) upsertAgent(ctx context.Context, profile contracts.AgentProfile) error {
	capabilitiesJSON, err := json.Marshal(profile.Capabilities)
	if err != nil {
		return fmt.Errorf("marshal agent capabilities: %w", err)
	}
	ticketTypesJSON, err := json.Marshal(profile.AllowedTicketTypes)
	if err != nil {
		return fmt.Errorf("marshal agent ticket types: %w", err)
	}
	rolesJSON, err := json.Marshal(profile.PreferredRoles)
	if err != nil {
		return fmt.Errorf("marshal agent preferred roles: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO agents (
			agent_id, display_name, provider, enabled, capabilities_json, allowed_ticket_types_json,
			default_runbook, max_active_runs, preferred_roles_json, routing_weight,
			instruction_profile, launch_target, integration_template, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			display_name=excluded.display_name,
			provider=excluded.provider,
			enabled=excluded.enabled,
			capabilities_json=excluded.capabilities_json,
			allowed_ticket_types_json=excluded.allowed_ticket_types_json,
			default_runbook=excluded.default_runbook,
			max_active_runs=excluded.max_active_runs,
			preferred_roles_json=excluded.preferred_roles_json,
			routing_weight=excluded.routing_weight,
			instruction_profile=excluded.instruction_profile,
			launch_target=excluded.launch_target,
			integration_template=excluded.integration_template,
			notes=excluded.notes
	`, profile.AgentID, profile.DisplayName, string(profile.Provider), boolToInt(profile.Enabled), string(capabilitiesJSON), string(ticketTypesJSON),
		nullable(profile.DefaultRunbook), profile.MaxActiveRuns, string(rolesJSON), profile.RoutingWeight, nullable(profile.InstructionProfile),
		nullable(profile.LaunchTarget), nullable(profile.IntegrationTemplate), nullable(profile.Notes))
	if err != nil {
		return fmt.Errorf("upsert agent %s: %w", profile.AgentID, err)
	}
	return nil
}

func (s *Store) upsertRun(ctx context.Context, run contracts.RunSnapshot) error {
	if err := run.Validate(); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO runs (
			run_id, ticket_id, project, agent_id, provider, status, kind, blueprint_stage,
			worktree_path, branch_name, created_at, started_at, completed_at, last_heartbeat_at,
			result, summary, handoff_to, supersedes_run_id, evidence_count, session_provider,
			session_ref, schema_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			ticket_id=excluded.ticket_id,
			project=excluded.project,
			agent_id=excluded.agent_id,
			provider=excluded.provider,
			status=excluded.status,
			kind=excluded.kind,
			blueprint_stage=excluded.blueprint_stage,
			worktree_path=excluded.worktree_path,
			branch_name=excluded.branch_name,
			created_at=excluded.created_at,
			started_at=excluded.started_at,
			completed_at=excluded.completed_at,
			last_heartbeat_at=excluded.last_heartbeat_at,
			result=excluded.result,
			summary=excluded.summary,
			handoff_to=excluded.handoff_to,
			supersedes_run_id=excluded.supersedes_run_id,
			evidence_count=excluded.evidence_count,
			session_provider=excluded.session_provider,
			session_ref=excluded.session_ref,
			schema_version=excluded.schema_version
	`,
		run.RunID,
		run.TicketID,
		run.Project,
		nullable(run.AgentID),
		nullable(string(run.Provider)),
		string(run.Status),
		string(run.Kind),
		nullable(run.BlueprintStage),
		nullable(run.WorktreePath),
		nullable(run.BranchName),
		run.CreatedAt.UTC().Format(time.RFC3339Nano),
		nullableTime(run.StartedAt),
		nullableTime(run.CompletedAt),
		nullableTime(run.LastHeartbeatAt),
		nullable(run.Result),
		nullable(run.Summary),
		nullable(run.HandoffTo),
		nullable(run.SupersedesRunID),
		run.EvidenceCount,
		nullable(string(run.SessionProvider)),
		nullable(run.SessionRef),
		run.SchemaVersion,
	)
	if err != nil {
		return fmt.Errorf("upsert run %s: %w", run.RunID, err)
	}
	return nil
}

func (s *Store) upsertEvidence(ctx context.Context, evidence contracts.EvidenceItem) error {
	if err := evidence.Validate(); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO evidence (
			evidence_id, run_id, ticket_id, type, title, body, artifact_path,
			supersedes_evidence_id, actor, created_at, schema_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(evidence_id) DO UPDATE SET
			run_id=excluded.run_id,
			ticket_id=excluded.ticket_id,
			type=excluded.type,
			title=excluded.title,
			body=excluded.body,
			artifact_path=excluded.artifact_path,
			supersedes_evidence_id=excluded.supersedes_evidence_id,
			actor=excluded.actor,
			created_at=excluded.created_at,
			schema_version=excluded.schema_version
	`,
		evidence.EvidenceID,
		evidence.RunID,
		evidence.TicketID,
		string(evidence.Type),
		nullable(evidence.Title),
		nullable(evidence.Body),
		nullable(evidence.ArtifactPath),
		nullable(evidence.SupersedesEvidenceID),
		string(evidence.Actor),
		evidence.CreatedAt.UTC().Format(time.RFC3339Nano),
		evidence.SchemaVersion,
	)
	if err != nil {
		return fmt.Errorf("upsert evidence %s: %w", evidence.EvidenceID, err)
	}
	return nil
}

func (s *Store) upsertHandoff(ctx context.Context, handoff contracts.HandoffPacket) error {
	if err := handoff.Validate(); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(handoff)
	if err != nil {
		return fmt.Errorf("marshal handoff payload: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO handoffs (
			handoff_id, source_run_id, ticket_id, actor, payload_json, generated_at, schema_version
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(handoff_id) DO UPDATE SET
			source_run_id=excluded.source_run_id,
			ticket_id=excluded.ticket_id,
			actor=excluded.actor,
			payload_json=excluded.payload_json,
			generated_at=excluded.generated_at,
			schema_version=excluded.schema_version
	`,
		handoff.HandoffID,
		handoff.SourceRunID,
		handoff.TicketID,
		string(handoff.Actor),
		string(payloadJSON),
		handoff.GeneratedAt.UTC().Format(time.RFC3339Nano),
		handoff.SchemaVersion,
	)
	if err != nil {
		return fmt.Errorf("upsert handoff %s: %w", handoff.HandoffID, err)
	}
	return nil
}

type ticketScanner interface {
	Scan(dest ...any) error
}

func scanTicket(scanner ticketScanner) (contracts.TicketSnapshot, error) {
	var (
		ticket                   contracts.TicketSnapshot
		typeValue                string
		statusValue              string
		priorityValue            string
		createdAt                string
		updatedAt                string
		archived                 int
		labelsJSON               string
		blockedByJSON            string
		blocksJSON               string
		acceptanceJSON           string
		policyJSON               string
		progressJSON             string
		requiredCapabilitiesJSON string
		dispatchMode             sql.NullString
		allowParallelRuns        int
		runbook                  sql.NullString
		latestRunID              sql.NullString
		latestHandoffID          sql.NullString
		openGateIDsJSON          string
		lastDispatchAt           sql.NullString
		parent                   sql.NullString
		assignee                 sql.NullString
		reviewer                 sql.NullString
		summary                  sql.NullString
		description              sql.NullString
		notes                    sql.NullString
		reviewState              sql.NullString
		leaseActor               sql.NullString
		leaseKind                sql.NullString
		leaseAcquiredAt          sql.NullString
		leaseExpiresAt           sql.NullString
		leaseHeartbeatAt         sql.NullString
		template                 sql.NullString
		skillHint                sql.NullString
		blueprint                sql.NullString
	)
	if err := scanner.Scan(
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
		&policyJSON,
		&reviewState,
		&leaseActor,
		&leaseKind,
		&leaseAcquiredAt,
		&leaseExpiresAt,
		&leaseHeartbeatAt,
		&template,
		&skillHint,
		&blueprint,
		&progressJSON,
		&requiredCapabilitiesJSON,
		&dispatchMode,
		&allowParallelRuns,
		&runbook,
		&latestRunID,
		&latestHandoffID,
		&openGateIDsJSON,
		&lastDispatchAt,
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
	if strings.TrimSpace(policyJSON) == "" {
		policyJSON = `{}`
	}
	if err := json.Unmarshal([]byte(policyJSON), &ticket.Policy); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if strings.TrimSpace(progressJSON) == "" {
		progressJSON = `{}`
	}
	if err := json.Unmarshal([]byte(progressJSON), &ticket.Progress); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if strings.TrimSpace(requiredCapabilitiesJSON) == "" {
		requiredCapabilitiesJSON = `[]`
	}
	if err := json.Unmarshal([]byte(requiredCapabilitiesJSON), &ticket.RequiredCapabilities); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if strings.TrimSpace(openGateIDsJSON) == "" {
		openGateIDsJSON = `[]`
	}
	if err := json.Unmarshal([]byte(openGateIDsJSON), &ticket.OpenGateIDs); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	ticket.Type = contracts.TicketType(typeValue)
	ticket.Status = contracts.Status(statusValue)
	ticket.Priority = contracts.Priority(priorityValue)
	if dispatchMode.Valid {
		ticket.DispatchMode = contracts.DispatchMode(dispatchMode.String)
	}
	ticket.AllowParallelRuns = allowParallelRuns == 1
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
	if reviewState.Valid {
		ticket.ReviewState = contracts.ReviewState(reviewState.String)
	}
	if leaseActor.Valid {
		ticket.Lease.Actor = contracts.Actor(leaseActor.String)
	}
	if leaseKind.Valid {
		ticket.Lease.Kind = contracts.LeaseKind(leaseKind.String)
	}
	if leaseAcquiredAt.Valid {
		ticket.Lease.AcquiredAt, err = time.Parse(time.RFC3339Nano, leaseAcquiredAt.String)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
	}
	if leaseExpiresAt.Valid {
		ticket.Lease.ExpiresAt, err = time.Parse(time.RFC3339Nano, leaseExpiresAt.String)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
	}
	if leaseHeartbeatAt.Valid {
		ticket.Lease.LastHeartbeatAt, err = time.Parse(time.RFC3339Nano, leaseHeartbeatAt.String)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
	}
	if template.Valid {
		ticket.Template = template.String
	}
	if skillHint.Valid {
		ticket.SkillHint = skillHint.String
	}
	if blueprint.Valid {
		ticket.Blueprint = blueprint.String
	}
	if runbook.Valid {
		ticket.Runbook = runbook.String
	}
	if latestRunID.Valid {
		ticket.LatestRunID = latestRunID.String
	}
	if latestHandoffID.Valid {
		ticket.LatestHandoffID = latestHandoffID.String
	}
	if lastDispatchAt.Valid {
		ticket.LastDispatchAt, err = time.Parse(time.RFC3339Nano, lastDispatchAt.String)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
	}
	return contracts.NormalizeTicketSnapshot(ticket), nil
}

func marshalTicketJSON(ticket contracts.TicketSnapshot) (labelsJSON string, blockedByJSON string, blocksJSON string, acceptanceJSON string, policyJSON string, progressJSON string, requiredCapabilitiesJSON string, openGateIDsJSON string, err error) {
	labelsRaw, err := json.Marshal(ticket.Labels)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal labels: %w", err)
	}
	blockedByRaw, err := json.Marshal(ticket.BlockedBy)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal blocked_by: %w", err)
	}
	blocksRaw, err := json.Marshal(ticket.Blocks)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal blocks: %w", err)
	}
	acceptanceRaw, err := json.Marshal(ticket.AcceptanceCriteria)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal acceptance criteria: %w", err)
	}
	policyRaw, err := json.Marshal(ticket.Policy)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal policy: %w", err)
	}
	progressRaw, err := json.Marshal(ticket.Progress)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal progress: %w", err)
	}
	requiredCapabilitiesRaw, err := json.Marshal(ticket.RequiredCapabilities)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal required capabilities: %w", err)
	}
	openGateIDsRaw, err := json.Marshal(ticket.OpenGateIDs)
	if err != nil {
		return "", "", "", "", "", "", "", "", fmt.Errorf("marshal open gate ids: %w", err)
	}
	return string(labelsRaw), string(blockedByRaw), string(blocksRaw), string(acceptanceRaw), string(policyRaw), string(progressRaw), string(requiredCapabilitiesRaw), string(openGateIDsRaw), nil
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
		ticket = contracts.NormalizeTicketSnapshot(ticket)
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

func extractAgentProfiles(payload any) []contracts.AgentProfile {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var profile contracts.AgentProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil
	}
	if profile.Validate() != nil {
		return nil
	}
	return []contracts.AgentProfile{profile}
}

func extractRunSnapshots(payload any) []contracts.RunSnapshot {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	result := make([]contracts.RunSnapshot, 0, 1)
	seen := map[string]struct{}{}
	appendRun := func(run contracts.RunSnapshot) {
		if run.SchemaVersion == 0 {
			run.SchemaVersion = contracts.CurrentSchemaVersion
		}
		if run.Status == "" {
			run.Status = contracts.RunStatusPlanned
		}
		if run.Kind == "" {
			run.Kind = contracts.RunKindWork
		}
		if run.Validate() != nil {
			return
		}
		if _, ok := seen[run.RunID]; ok {
			return
		}
		seen[run.RunID] = struct{}{}
		result = append(result, run)
	}

	var wrapped struct {
		Run contracts.RunSnapshot `json:"run"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		appendRun(wrapped.Run)
	}

	var run contracts.RunSnapshot
	if err := json.Unmarshal(raw, &run); err == nil {
		appendRun(run)
	}

	return result
}

func extractEvidenceItems(payload any) []contracts.EvidenceItem {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var wrapped struct {
		Evidence contracts.EvidenceItem `json:"evidence"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Evidence.Validate() == nil {
		return []contracts.EvidenceItem{wrapped.Evidence}
	}
	var evidence contracts.EvidenceItem
	if err := json.Unmarshal(raw, &evidence); err == nil && evidence.Validate() == nil {
		return []contracts.EvidenceItem{evidence}
	}
	return nil
}

func extractHandoffPackets(payload any) []contracts.HandoffPacket {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var wrapped struct {
		Handoff contracts.HandoffPacket `json:"handoff"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Handoff.Validate() == nil {
		return []contracts.HandoffPacket{wrapped.Handoff}
	}
	var handoff contracts.HandoffPacket
	if err := json.Unmarshal(raw, &handoff); err == nil && handoff.Validate() == nil {
		return []contracts.HandoffPacket{handoff}
	}
	return nil
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
