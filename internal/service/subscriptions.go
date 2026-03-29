package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// SubscriptionAudience is the resolved watcher set for one event.
type SubscriptionAudience struct {
	HasSubscriptions bool              `json:"has_subscriptions"`
	Recipients       []contracts.Actor `json:"recipients,omitempty"`
	Targets          []string          `json:"targets,omitempty"`
}

// SubscriptionResolver expands local watcher rules into a notification audience.
type SubscriptionResolver struct {
	Store   SubscriptionStore
	Queries *QueryService
}

// Audience resolves watcher recipients for one event.
func (r SubscriptionResolver) Audience(ctx context.Context, event contracts.Event) (SubscriptionAudience, error) {
	subscriptions, err := r.Store.ListSubscriptions()
	if err != nil {
		return SubscriptionAudience{}, err
	}
	audience := SubscriptionAudience{HasSubscriptions: len(subscriptions) > 0}
	if len(subscriptions) == 0 {
		return audience, nil
	}
	recipientSet := map[contracts.Actor]struct{}{}
	targetSet := map[string]struct{}{}
	for _, subscription := range subscriptions {
		matched, err := r.matches(ctx, subscription, event)
		if err != nil {
			continue
		}
		if !matched {
			continue
		}
		recipientSet[subscription.Actor] = struct{}{}
		targetSet[fmt.Sprintf("%s:%s", subscription.TargetKind, subscription.Target)] = struct{}{}
	}
	for actor := range recipientSet {
		audience.Recipients = append(audience.Recipients, actor)
	}
	for target := range targetSet {
		audience.Targets = append(audience.Targets, target)
	}
	sort.Slice(audience.Recipients, func(i, j int) bool {
		return audience.Recipients[i] < audience.Recipients[j]
	})
	sort.Strings(audience.Targets)
	return audience, nil
}

func (r SubscriptionResolver) matches(ctx context.Context, subscription contracts.Subscription, event contracts.Event) (bool, error) {
	if r.Queries != nil {
		view := r.Queries.subscriptionView(ctx, subscription)
		if !view.Active {
			return false, nil
		}
	}
	if len(subscription.EventTypes) > 0 && !eventTypeInList(event.Type, subscription.EventTypes) {
		return false, nil
	}
	switch subscription.TargetKind {
	case contracts.SubscriptionTargetTicket:
		return strings.TrimSpace(event.TicketID) != "" && event.TicketID == subscription.Target, nil
	case contracts.SubscriptionTargetProject:
		return strings.TrimSpace(event.Project) != "" && event.Project == subscription.Target, nil
	case contracts.SubscriptionTargetSavedView:
		if strings.TrimSpace(event.TicketID) == "" || r.Queries == nil {
			return false, nil
		}
		result, err := r.Queries.RunSavedView(ctx, subscription.Target, subscription.Actor)
		if err != nil {
			return false, err
		}
		return savedViewContainsTicket(result, event.TicketID), nil
	default:
		return false, nil
	}
}

func savedViewContainsTicket(result SavedViewResult, ticketID string) bool {
	for _, ticket := range result.Tickets {
		if ticket.ID == ticketID {
			return true
		}
	}
	if result.Board != nil {
		for _, tickets := range result.Board.Board.Columns {
			for _, ticket := range tickets {
				if ticket.ID == ticketID {
					return true
				}
			}
		}
	}
	if result.Queue != nil {
		for _, entries := range result.Queue.Categories {
			for _, entry := range entries {
				if entry.Ticket.ID == ticketID {
					return true
				}
			}
		}
	}
	if result.Next != nil {
		for _, entry := range result.Next.Entries {
			if entry.Entry.Ticket.ID == ticketID {
				return true
			}
		}
	}
	return false
}

func eventTypeInList(kind contracts.EventType, values []contracts.EventType) bool {
	for _, value := range values {
		if value == kind {
			return true
		}
	}
	return false
}
