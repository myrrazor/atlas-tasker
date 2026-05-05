package contracts

import (
	"fmt"
	"strings"
)

type AutomationActionKind string

const (
	AutomationActionComment       AutomationActionKind = "comment"
	AutomationActionMove          AutomationActionKind = "move"
	AutomationActionRequestReview AutomationActionKind = "request_review"
	AutomationActionNotify        AutomationActionKind = "notify"
)

var validAutomationActionKinds = map[AutomationActionKind]struct{}{
	AutomationActionComment:       {},
	AutomationActionMove:          {},
	AutomationActionRequestReview: {},
	AutomationActionNotify:        {},
}

func (k AutomationActionKind) IsValid() bool {
	_, ok := validAutomationActionKinds[k]
	return ok
}

type AutomationTrigger struct {
	EventTypes []EventType `json:"event_types" toml:"event_types"`
}

func (t AutomationTrigger) Validate() error {
	if len(t.EventTypes) == 0 {
		return fmt.Errorf("automation trigger requires at least one event type")
	}
	for _, eventType := range t.EventTypes {
		if !eventType.IsValid() {
			return fmt.Errorf("invalid automation event type: %s", eventType)
		}
	}
	return nil
}

type AutomationCondition struct {
	Project     string      `json:"project,omitempty" toml:"project"`
	Status      Status      `json:"status,omitempty" toml:"status"`
	Type        TicketType  `json:"type,omitempty" toml:"type"`
	Assignee    Actor       `json:"assignee,omitempty" toml:"assignee"`
	Reviewer    Actor       `json:"reviewer,omitempty" toml:"reviewer"`
	ReviewState ReviewState `json:"review_state,omitempty" toml:"review_state"`
	Labels      []string    `json:"labels,omitempty" toml:"labels"`
}

func (c AutomationCondition) Validate() error {
	if c.Status != "" && !c.Status.IsValid() {
		return fmt.Errorf("invalid automation status condition: %s", c.Status)
	}
	if c.Type != "" && !c.Type.IsValid() {
		return fmt.Errorf("invalid automation type condition: %s", c.Type)
	}
	if c.Assignee != "" && !c.Assignee.IsValid() {
		return fmt.Errorf("invalid automation assignee condition: %s", c.Assignee)
	}
	if c.Reviewer != "" && !c.Reviewer.IsValid() {
		return fmt.Errorf("invalid automation reviewer condition: %s", c.Reviewer)
	}
	if c.ReviewState != "" && !c.ReviewState.IsValid() {
		return fmt.Errorf("invalid automation review_state condition: %s", c.ReviewState)
	}
	return nil
}

type AutomationAction struct {
	Kind    AutomationActionKind `json:"kind" toml:"kind"`
	Body    string               `json:"body,omitempty" toml:"body"`
	Status  Status               `json:"status,omitempty" toml:"status"`
	Message string               `json:"message,omitempty" toml:"message"`
}

func (a AutomationAction) Validate() error {
	if !a.Kind.IsValid() {
		return fmt.Errorf("invalid automation action kind: %s", a.Kind)
	}
	switch a.Kind {
	case AutomationActionComment:
		if strings.TrimSpace(a.Body) == "" {
			return fmt.Errorf("comment action requires body")
		}
	case AutomationActionMove:
		if !a.Status.IsValid() {
			return fmt.Errorf("move action requires valid status")
		}
	case AutomationActionRequestReview:
	case AutomationActionNotify:
		if strings.TrimSpace(a.Message) == "" {
			return fmt.Errorf("notify action requires message")
		}
	}
	return nil
}

type AutomationRule struct {
	Name       string              `json:"name" toml:"name"`
	Enabled    bool                `json:"enabled" toml:"enabled"`
	Trigger    AutomationTrigger   `json:"trigger" toml:"trigger"`
	Conditions AutomationCondition `json:"conditions" toml:"conditions"`
	Actions    []AutomationAction  `json:"actions" toml:"actions"`
}

func (r AutomationRule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("automation name is required")
	}
	if err := r.Trigger.Validate(); err != nil {
		return err
	}
	if err := r.Conditions.Validate(); err != nil {
		return err
	}
	if len(r.Actions) == 0 {
		return fmt.Errorf("automation requires at least one action")
	}
	for _, action := range r.Actions {
		if err := action.Validate(); err != nil {
			return err
		}
	}
	return nil
}
