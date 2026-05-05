package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type automationDepthKey struct{}
type automationRulesKey struct{}
type automationRootActorKey struct{}
type automationState struct {
	SeenByCorrelation map[string]map[string]struct{}
}

type AutomationResult struct {
	Rule      contracts.AutomationRule  `json:"rule"`
	Matched   bool                      `json:"matched"`
	Reasons   []string                  `json:"reasons"`
	Actions   []string                  `json:"actions"`
	DryRun    bool                      `json:"dry_run"`
	Ticket    *contracts.TicketSnapshot `json:"ticket,omitempty"`
	EventType contracts.EventType       `json:"event_type"`
}

type AutomationEngine struct {
	Store    AutomationStore
	Notifier Notifier
}

func (e AutomationEngine) ListRules() ([]contracts.AutomationRule, error) {
	return e.Store.ListRules()
}

func (e AutomationEngine) LoadRule(name string) (contracts.AutomationRule, error) {
	return e.Store.LoadRule(name)
}

func (e AutomationEngine) SaveRule(rule contracts.AutomationRule) error {
	return e.Store.SaveRule(rule)
}

func (e AutomationEngine) DeleteRule(name string) error {
	return e.Store.DeleteRule(name)
}

func (e AutomationEngine) Explain(ctx context.Context, queries *QueryService, rule contracts.AutomationRule, event contracts.Event, ticketID string) (AutomationResult, error) {
	return e.evaluate(ctx, queries, rule, event, ticketID, true)
}

func (e AutomationEngine) DryRun(ctx context.Context, queries *QueryService, rule contracts.AutomationRule, event contracts.Event, ticketID string) (AutomationResult, error) {
	return e.evaluate(ctx, queries, rule, event, ticketID, true)
}

func (e AutomationEngine) Run(ctx context.Context, actions *ActionService, queries *QueryService, event contracts.Event) ([]AutomationResult, error) {
	rules, err := e.ListRules()
	if err != nil {
		return nil, err
	}
	ctx = markAutomationRuleSeen(ctx, event, "")
	results := make([]AutomationResult, 0)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		result, err := e.evaluate(ctx, queries, rule, event, event.TicketID, false)
		if err != nil {
			return nil, err
		}
		if !result.Matched {
			continue
		}
		if err := e.apply(ctx, actions, rule, event, result.Ticket); err != nil {
			return nil, err
		}
		ctx = markAutomationRuleSeen(ctx, event, rule.Name)
		results = append(results, result)
	}
	return results, nil
}

func (e AutomationEngine) evaluate(ctx context.Context, queries *QueryService, rule contracts.AutomationRule, event contracts.Event, ticketID string, dryRun bool) (AutomationResult, error) {
	result := AutomationResult{Rule: rule, DryRun: dryRun, EventType: event.Type, Reasons: []string{}, Actions: []string{}}
	if !rule.Enabled {
		result.Reasons = append(result.Reasons, "rule is disabled")
		return result, nil
	}
	if !eventTypeMatches(rule.Trigger, event.Type) {
		result.Reasons = append(result.Reasons, fmt.Sprintf("trigger does not include %s", event.Type))
		return result, nil
	}
	var ticket *contracts.TicketSnapshot
	if strings.TrimSpace(ticketID) != "" {
		detail, err := queries.TicketDetail(ctx, ticketID)
		if err != nil {
			return result, err
		}
		ticket = &detail.Ticket
		result.Ticket = ticket
	}
	if ok, reasons := conditionsMatch(rule.Conditions, event, ticket); !ok {
		result.Reasons = append(result.Reasons, reasons...)
		return result, nil
	}
	result.Matched = true
	for _, action := range rule.Actions {
		switch action.Kind {
		case contracts.AutomationActionComment:
			result.Actions = append(result.Actions, fmt.Sprintf("comment %q", action.Body))
		case contracts.AutomationActionMove:
			result.Actions = append(result.Actions, fmt.Sprintf("move to %s", action.Status))
		case contracts.AutomationActionRequestReview:
			result.Actions = append(result.Actions, "request review")
		case contracts.AutomationActionNotify:
			result.Actions = append(result.Actions, fmt.Sprintf("notify %q", action.Message))
		}
	}
	result.Reasons = append(result.Reasons, "rule matched")
	return result, nil
}

func (e AutomationEngine) apply(ctx context.Context, actions *ActionService, rule contracts.AutomationRule, event contracts.Event, ticket *contracts.TicketSnapshot) error {
	if ticket == nil {
		return nil
	}
	depth, _ := ctx.Value(automationDepthKey{}).(int)
	if depth >= 3 {
		return nil
	}
	correlationID := firstNonEmpty(event.Metadata.CorrelationID, event.Metadata.MutationID)
	rootActor := firstActor(event.Metadata.RootActor, event.Actor)
	storedRootActor, _ := ctx.Value(automationRootActorKey{}).(contracts.Actor)
	if storedRootActor != "" && rootActor != "" && storedRootActor != rootActor {
		return nil
	}
	state := automationGuardState(ctx)
	if state.SeenByCorrelation[correlationID] == nil {
		state.SeenByCorrelation[correlationID] = map[string]struct{}{}
	}
	for name := range state.SeenByCorrelation[correlationID] {
		if name == rule.Name {
			return nil
		}
	}
	state.SeenByCorrelation[correlationID][rule.Name] = struct{}{}
	automationActor := contracts.Actor("agent:automation")
	childCtx := context.WithValue(ctx, automationDepthKey{}, depth+1)
	childCtx = context.WithValue(childCtx, automationRulesKey{}, state)
	childCtx = context.WithValue(childCtx, automationRootActorKey{}, rootActor)
	childCtx = WithEventMetadata(childCtx, EventMetaContext{
		Surface:          contracts.EventSurfaceAutomation,
		CorrelationID:    correlationID,
		CausationEventID: event.EventID,
		RootActor:        rootActor,
	})
	for _, action := range rule.Actions {
		switch action.Kind {
		case contracts.AutomationActionComment:
			if err := actions.CommentTicket(childCtx, ticket.ID, action.Body, automationActor, "automation:"+rule.Name); err != nil {
				return err
			}
		case contracts.AutomationActionMove:
			if _, err := actions.MoveTicket(childCtx, ticket.ID, action.Status, automationActor, "automation:"+rule.Name); err != nil {
				return err
			}
		case contracts.AutomationActionRequestReview:
			if _, err := actions.RequestReview(childCtx, ticket.ID, automationActor, "automation:"+rule.Name); err != nil {
				return err
			}
		case contracts.AutomationActionNotify:
			if e.Notifier != nil {
				_ = e.Notifier.Notify(childCtx, contracts.Event{
					EventID:       event.EventID,
					Timestamp:     event.Timestamp,
					Actor:         automationActor,
					Reason:        action.Message,
					Type:          contracts.EventOwnerAttentionRaised,
					Project:       event.Project,
					TicketID:      event.TicketID,
					SchemaVersion: contracts.CurrentSchemaVersion,
					Metadata: contracts.EventMetadata{
						CorrelationID:    event.Metadata.CorrelationID,
						CausationEventID: event.EventID,
						Surface:          contracts.EventSurfaceAutomation,
						RootActor:        firstActor(event.Metadata.RootActor, event.Actor),
					},
				})
			}
		}
	}
	return nil
}

func eventTypeMatches(trigger contracts.AutomationTrigger, eventType contracts.EventType) bool {
	for _, candidate := range trigger.EventTypes {
		if candidate == eventType {
			return true
		}
	}
	return false
}

func conditionsMatch(cond contracts.AutomationCondition, event contracts.Event, ticket *contracts.TicketSnapshot) (bool, []string) {
	reasons := make([]string, 0)
	if cond.Project != "" && !strings.EqualFold(strings.TrimSpace(cond.Project), strings.TrimSpace(event.Project)) {
		reasons = append(reasons, fmt.Sprintf("project %s does not match", event.Project))
	}
	if ticket == nil {
		if len(reasons) > 0 {
			return false, reasons
		}
		return true, []string{}
	}
	if cond.Status != "" && ticket.Status != cond.Status {
		reasons = append(reasons, fmt.Sprintf("status %s does not match", ticket.Status))
	}
	if cond.Type != "" && ticket.Type != cond.Type {
		reasons = append(reasons, fmt.Sprintf("type %s does not match", ticket.Type))
	}
	if cond.Assignee != "" && ticket.Assignee != cond.Assignee {
		reasons = append(reasons, fmt.Sprintf("assignee %s does not match", ticket.Assignee))
	}
	if cond.Reviewer != "" && ticket.Reviewer != cond.Reviewer {
		reasons = append(reasons, fmt.Sprintf("reviewer %s does not match", ticket.Reviewer))
	}
	if cond.ReviewState != "" && ticket.ReviewState != cond.ReviewState {
		reasons = append(reasons, fmt.Sprintf("review state %s does not match", ticket.ReviewState))
	}
	if len(cond.Labels) > 0 {
		for _, label := range cond.Labels {
			if !containsLabel(ticket.Labels, label) {
				reasons = append(reasons, fmt.Sprintf("missing label %s", label))
			}
		}
	}
	return len(reasons) == 0, reasons
}

func containsLabel(labels []string, wanted string) bool {
	for _, label := range labels {
		if label == wanted {
			return true
		}
	}
	return false
}

func firstActor(values ...contracts.Actor) contracts.Actor {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func markAutomationRuleSeen(ctx context.Context, event contracts.Event, ruleName string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	correlationID := firstNonEmpty(event.Metadata.CorrelationID, event.Metadata.MutationID)
	if correlationID == "" {
		return ctx
	}
	rootActor := firstActor(event.Metadata.RootActor, event.Actor)
	storedRootActor, _ := ctx.Value(automationRootActorKey{}).(contracts.Actor)
	if storedRootActor == "" && rootActor != "" {
		ctx = context.WithValue(ctx, automationRootActorKey{}, rootActor)
	}
	state := automationGuardState(ctx)
	if strings.TrimSpace(ruleName) != "" {
		if state.SeenByCorrelation[correlationID] == nil {
			state.SeenByCorrelation[correlationID] = map[string]struct{}{}
		}
		state.SeenByCorrelation[correlationID][ruleName] = struct{}{}
	}
	return context.WithValue(ctx, automationRulesKey{}, state)
}

func automationGuardState(ctx context.Context) *automationState {
	state, _ := ctx.Value(automationRulesKey{}).(*automationState)
	if state != nil {
		return state
	}
	return &automationState{SeenByCorrelation: map[string]map[string]struct{}{}}
}
