package domain

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// LinkKind identifies the relationship type between two tickets.
type LinkKind string

const (
	LinkBlocks    LinkKind = "blocks"
	LinkBlockedBy LinkKind = "blocked_by"
	LinkParent    LinkKind = "parent"
)

// ApplyLink updates ticket relationships in-memory with v1 validation rules.
func ApplyLink(tickets map[string]contracts.TicketSnapshot, id string, otherID string, kind LinkKind) error {
	id = strings.TrimSpace(id)
	otherID = strings.TrimSpace(otherID)
	if err := validateLinkInput(tickets, id, otherID, kind); err != nil {
		return err
	}
	self := tickets[id]
	other := tickets[otherID]

	switch kind {
	case LinkBlocks:
		self.Blocks = addUnique(self.Blocks, otherID)
		other.BlockedBy = addUnique(other.BlockedBy, id)
	case LinkBlockedBy:
		self.BlockedBy = addUnique(self.BlockedBy, otherID)
		other.Blocks = addUnique(other.Blocks, id)
	case LinkParent:
		if wouldCreateParentCycle(tickets, id, otherID) {
			return fmt.Errorf("parent link creates cycle: %s -> %s", id, otherID)
		}
		self.Parent = otherID
	default:
		return fmt.Errorf("unsupported link kind: %s", kind)
	}

	tickets[id] = self
	tickets[otherID] = other
	return nil
}

// RemoveLink removes relationships and keeps symmetric fields consistent.
func RemoveLink(tickets map[string]contracts.TicketSnapshot, id string, otherID string) error {
	id = strings.TrimSpace(id)
	otherID = strings.TrimSpace(otherID)
	if strings.TrimSpace(id) == "" || strings.TrimSpace(otherID) == "" {
		return fmt.Errorf("ticket ids are required")
	}
	self, ok := tickets[id]
	if !ok {
		return fmt.Errorf("ticket not found: %s", id)
	}
	other, ok := tickets[otherID]
	if !ok {
		return fmt.Errorf("ticket not found: %s", otherID)
	}

	self.Blocks = removeValue(self.Blocks, otherID)
	self.BlockedBy = removeValue(self.BlockedBy, otherID)
	other.Blocks = removeValue(other.Blocks, id)
	other.BlockedBy = removeValue(other.BlockedBy, id)

	if self.Parent == otherID {
		self.Parent = ""
	}
	if other.Parent == id {
		other.Parent = ""
	}

	tickets[id] = self
	tickets[otherID] = other
	return nil
}

func validateLinkInput(tickets map[string]contracts.TicketSnapshot, id string, otherID string, kind LinkKind) error {
	id = strings.TrimSpace(id)
	otherID = strings.TrimSpace(otherID)
	if id == "" || otherID == "" {
		return fmt.Errorf("ticket ids are required")
	}
	if id == otherID {
		return fmt.Errorf("self-link is not allowed")
	}
	if _, ok := tickets[id]; !ok {
		return fmt.Errorf("ticket not found: %s", id)
	}
	if _, ok := tickets[otherID]; !ok {
		return fmt.Errorf("ticket not found: %s", otherID)
	}
	switch kind {
	case LinkBlocks, LinkBlockedBy, LinkParent:
		return nil
	default:
		return fmt.Errorf("unsupported link kind: %s", kind)
	}
}

func wouldCreateParentCycle(tickets map[string]contracts.TicketSnapshot, childID string, parentID string) bool {
	current := parentID
	visited := map[string]struct{}{}
	for current != "" {
		if current == childID {
			return true
		}
		if _, seen := visited[current]; seen {
			return true
		}
		visited[current] = struct{}{}
		next := tickets[current].Parent
		current = strings.TrimSpace(next)
	}
	return false
}

func addUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeValue(values []string, value string) []string {
	trimmed := values[:0]
	for _, existing := range values {
		if existing != value {
			trimmed = append(trimmed, existing)
		}
	}
	result := make([]string, len(trimmed))
	copy(result, trimmed)
	return result
}
