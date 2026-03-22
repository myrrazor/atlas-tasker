package markdown

import (
	"reflect"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestEncodeDecodeTicketMarkdownRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	ticket := contracts.TicketSnapshot{
		ID:                 "APP-1",
		Project:            "APP",
		Title:              "Add parser",
		Type:               contracts.TicketTypeTask,
		Status:             contracts.StatusInProgress,
		Priority:           contracts.PriorityHigh,
		Labels:             []string{"cli", "parser"},
		Assignee:           contracts.Actor("agent:builder-1"),
		Reviewer:           contracts.Actor("human:owner"),
		BlockedBy:          []string{"APP-2"},
		Blocks:             []string{"APP-3"},
		CreatedAt:          now,
		UpdatedAt:          now,
		SchemaVersion:      contracts.CurrentSchemaVersion,
		Summary:            "One line",
		Description:        "Long body",
		AcceptanceCriteria: []string{"works", "tested"},
		Notes:              "tracking notes",
	}
	raw, err := EncodeTicketMarkdown(ticket)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	decoded, err := DecodeTicketMarkdown(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !reflect.DeepEqual(decoded, ticket) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", decoded, ticket)
	}
}
