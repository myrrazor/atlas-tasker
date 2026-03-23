package contracts

import (
	"fmt"
	"strings"
	"time"
)

// EventType captures every persisted mutation event.
type EventType string

const (
	EventTicketCreated         EventType = "ticket.created"
	EventTicketUpdated         EventType = "ticket.updated"
	EventTicketMoved           EventType = "ticket.moved"
	EventTicketCommented       EventType = "ticket.commented"
	EventTicketLinked          EventType = "ticket.linked"
	EventTicketUnlinked        EventType = "ticket.unlinked"
	EventTicketClosed          EventType = "ticket.closed"
	EventTicketClaimed         EventType = "ticket.claimed"
	EventTicketReleased        EventType = "ticket.released"
	EventTicketHeartbeat       EventType = "ticket.heartbeat"
	EventTicketLeaseExpired    EventType = "ticket.lease_expired"
	EventTicketReviewRequested EventType = "ticket.review_requested"
	EventTicketApproved        EventType = "ticket.approved"
	EventTicketRejected        EventType = "ticket.rejected"
	EventTicketPolicyUpdated   EventType = "ticket.policy_updated"
	EventTicketTemplateApplied EventType = "ticket.template_applied"
	EventOwnerAttentionRaised  EventType = "ticket.owner_attention_required"
	EventOwnerAttentionCleared EventType = "ticket.owner_attention_cleared"
	EventProjectPolicyUpdated  EventType = "project.policy_updated"
	EventConfigChanged         EventType = "config.changed"
)

var validEventTypes = map[EventType]struct{}{
	EventTicketCreated:         {},
	EventTicketUpdated:         {},
	EventTicketMoved:           {},
	EventTicketCommented:       {},
	EventTicketLinked:          {},
	EventTicketUnlinked:        {},
	EventTicketClosed:          {},
	EventTicketClaimed:         {},
	EventTicketReleased:        {},
	EventTicketHeartbeat:       {},
	EventTicketLeaseExpired:    {},
	EventTicketReviewRequested: {},
	EventTicketApproved:        {},
	EventTicketRejected:        {},
	EventTicketPolicyUpdated:   {},
	EventTicketTemplateApplied: {},
	EventOwnerAttentionRaised:  {},
	EventOwnerAttentionCleared: {},
	EventProjectPolicyUpdated:  {},
	EventConfigChanged:         {},
}

func (t EventType) IsValid() bool {
	_, ok := validEventTypes[t]
	return ok
}

// Event is the append-only JSONL record shape.
type Event struct {
	EventID       int64     `json:"event_id"`
	Timestamp     time.Time `json:"timestamp"`
	Actor         Actor     `json:"actor"`
	Reason        string    `json:"reason,omitempty"`
	Type          EventType `json:"type"`
	Project       string    `json:"project"`
	TicketID      string    `json:"ticket_id,omitempty"`
	Payload       any       `json:"payload,omitempty"`
	SchemaVersion int       `json:"schema_version"`
}

func (e Event) Validate() error {
	if e.EventID <= 0 {
		return fmt.Errorf("event_id must be positive")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if !e.Actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", e.Actor)
	}
	if !e.Type.IsValid() {
		return fmt.Errorf("invalid event type: %s", e.Type)
	}
	if strings.TrimSpace(e.Project) == "" {
		return fmt.Errorf("project is required")
	}
	if e.SchemaVersion <= 0 {
		return fmt.Errorf("schema_version is required")
	}
	return nil
}
