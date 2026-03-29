package contracts

import "context"

// TicketListOptions controls ticket list filtering for stores.
type TicketListOptions struct {
	Project         string
	Statuses        []Status
	Assignee        Actor
	Type            TicketType
	Label           string
	IncludeArchived bool
	Limit           int
}

// BoardQueryOptions controls board/backlog/blocked view filtering.
type BoardQueryOptions struct {
	Project  string
	Assignee Actor
	Type     TicketType
}

// BoardView is a grouped set of tickets by status column.
type BoardView struct {
	Columns map[Status][]TicketSnapshot
}

// ProjectStore is the source-of-truth project storage contract.
type ProjectStore interface {
	CreateProject(ctx context.Context, project Project) error
	UpdateProject(ctx context.Context, project Project) error
	ListProjects(ctx context.Context) ([]Project, error)
	GetProject(ctx context.Context, key string) (Project, error)
}

// TicketStore is the source-of-truth ticket snapshot storage contract.
type TicketStore interface {
	CreateTicket(ctx context.Context, ticket TicketSnapshot) error
	GetTicket(ctx context.Context, id string) (TicketSnapshot, error)
	UpdateTicket(ctx context.Context, ticket TicketSnapshot) error
	ListTickets(ctx context.Context, opts TicketListOptions) ([]TicketSnapshot, error)
	SoftDeleteTicket(ctx context.Context, id string, actor Actor, reason string) error
}

// EventLog appends and streams immutable mutation history.
type EventLog interface {
	AppendEvent(ctx context.Context, event Event) error
	StreamEvents(ctx context.Context, project string, afterEventID int64) ([]Event, error)
}

// ProjectionStore maintains query-optimized materialized state.
type ProjectionStore interface {
	ApplyEvent(ctx context.Context, event Event) error
	Rebuild(ctx context.Context, project string) error
	QueryBoard(ctx context.Context, opts BoardQueryOptions) (BoardView, error)
	QueryTicket(ctx context.Context, ticketID string) (TicketSnapshot, error)
	QuerySearch(ctx context.Context, query SearchQuery) ([]TicketSnapshot, error)
	QueryHistory(ctx context.Context, ticketID string) ([]Event, error)
}

type CollaboratorStore interface {
	SaveCollaborator(ctx context.Context, collaborator CollaboratorProfile) error
	LoadCollaborator(ctx context.Context, collaboratorID string) (CollaboratorProfile, error)
	ListCollaborators(ctx context.Context) ([]CollaboratorProfile, error)
}

type MembershipStore interface {
	SaveMembership(ctx context.Context, membership MembershipBinding) error
	LoadMembership(ctx context.Context, membershipUID string) (MembershipBinding, error)
	ListMemberships(ctx context.Context, collaboratorID string) ([]MembershipBinding, error)
}

type MentionStore interface {
	SaveMention(ctx context.Context, mention Mention) error
	LoadMention(ctx context.Context, mentionUID string) (Mention, error)
	ListMentions(ctx context.Context, collaboratorID string) ([]Mention, error)
}
