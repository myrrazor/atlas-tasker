package contracts

import (
	"fmt"
	"strings"
)

// SubscriptionTargetKind identifies what a watcher is attached to.
type SubscriptionTargetKind string

const (
	SubscriptionTargetTicket    SubscriptionTargetKind = "ticket"
	SubscriptionTargetProject   SubscriptionTargetKind = "project"
	SubscriptionTargetSavedView SubscriptionTargetKind = "saved_view"
)

var validSubscriptionTargetKinds = map[SubscriptionTargetKind]struct{}{
	SubscriptionTargetTicket:    {},
	SubscriptionTargetProject:   {},
	SubscriptionTargetSavedView: {},
}

// IsValid reports whether the subscription target kind is supported.
func (k SubscriptionTargetKind) IsValid() bool {
	_, ok := validSubscriptionTargetKinds[k]
	return ok
}

// Subscription defines one local watcher rule.
type Subscription struct {
	Actor      Actor                  `json:"actor" toml:"actor"`
	TargetKind SubscriptionTargetKind `json:"target_kind" toml:"target_kind"`
	Target     string                 `json:"target" toml:"target"`
	EventTypes []EventType            `json:"event_types,omitempty" toml:"event_types"`
}

// Validate checks the watcher payload before it is persisted or evaluated.
func (s Subscription) Validate() error {
	if !s.Actor.IsValid() {
		return fmt.Errorf("invalid subscription actor: %s", s.Actor)
	}
	if !s.TargetKind.IsValid() {
		return fmt.Errorf("invalid subscription target kind: %s", s.TargetKind)
	}
	if strings.TrimSpace(s.Target) == "" {
		return fmt.Errorf("subscription target is required")
	}
	for _, eventType := range s.EventTypes {
		if !eventType.IsValid() {
			return fmt.Errorf("invalid subscription event type: %s", eventType)
		}
	}
	return nil
}
