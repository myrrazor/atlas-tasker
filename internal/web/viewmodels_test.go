package web

import (
	"errors"
	"os/user"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestFirstTicketIDForColumnPrefersActiveColumn(t *testing.T) {
	columns := []BoardColumn{
		{
			Status:  contracts.StatusBacklog,
			Tickets: []TicketCard{{Ticket: contracts.TicketSnapshot{ID: "WEB-1"}}},
		},
		{
			Status:  contracts.StatusReady,
			Tickets: []TicketCard{{Ticket: contracts.TicketSnapshot{ID: "WEB-2"}}},
		},
	}
	if got := firstTicketIDForColumn(columns, contracts.StatusReady); got != "WEB-2" {
		t.Fatalf("expected active column ticket WEB-2, got %q", got)
	}
	if got := firstTicketIDForColumn(columns, contracts.StatusInReview); got != "WEB-1" {
		t.Fatalf("expected fallback ticket WEB-1, got %q", got)
	}
}

func TestResolveOwnerNamePrecedence(t *testing.T) {
	original := currentOSUser
	currentOSUser = func() (*user.User, error) {
		return &user.User{Username: "fallback_user"}, nil
	}
	t.Cleanup(func() { currentOSUser = original })

	cfg := contracts.TrackerConfig{
		Web:   contracts.WebConfig{OwnerName: "Ada Lovelace"},
		Actor: contracts.ActorConfig{Default: contracts.Actor("human:grace_hopper")},
	}
	if got := resolveOwnerName(cfg); got != "Ada Lovelace" {
		t.Fatalf("expected explicit owner name, got %q", got)
	}
	cfg.Web.OwnerName = ""
	if got := resolveOwnerName(cfg); got != "Grace Hopper" {
		t.Fatalf("expected humanized actor name, got %q", got)
	}
	cfg.Actor.Default = ""
	if got := resolveOwnerName(cfg); got != "Fallback User" {
		t.Fatalf("expected OS user fallback, got %q", got)
	}
	currentOSUser = func() (*user.User, error) { return nil, errors.New("no user") }
	if got := resolveOwnerName(cfg); got != "" {
		t.Fatalf("expected empty fallback name, got %q", got)
	}
}

func TestRecentChangeDescribeDecodesMoveMap(t *testing.T) {
	event := contracts.Event{
		Type:      contracts.EventTicketMoved,
		Project:   "APP",
		TicketID:  "APP-42",
		Timestamp: time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"from":   "in_progress",
			"to":     contracts.StatusDone,
			"ticket": map[string]any{"id": "APP-42"},
		},
	}
	change := recentChangeFromEvent(event)
	if got := change.Describe(); got != "APP-42 moved in_progress → done" {
		t.Fatalf("unexpected move description: %q", got)
	}
}

func TestAgentColorClassAllowsKnownNamesOnly(t *testing.T) {
	if got := agentColorClass("orange"); got != "chip--orange" {
		t.Fatalf("unexpected orange class: %q", got)
	}
	if got := agentColorClass("chartreuse"); got != "chip--plain" {
		t.Fatalf("unknown colors must stay uncolored, got %q", got)
	}
}
