package service

import (
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestDecodeTicketConflictSnapshotMatchesMarkdownCodec(t *testing.T) {
	now := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	ticket := contracts.NormalizeTicketSnapshot(contracts.TicketSnapshot{
		ID:                 "APP-42",
		TicketUID:          contracts.TicketUID("APP", "APP-42"),
		Project:            "APP",
		Title:              "Fix sync conflict projection",
		Type:               contracts.TicketTypeTask,
		Status:             contracts.StatusReady,
		Priority:           contracts.PriorityHigh,
		Labels:             []string{"sync", "conflicts"},
		Assignee:           contracts.Actor("agent:builder-1"),
		Reviewer:           contracts.Actor("agent:reviewer-1"),
		CreatedAt:          now,
		UpdatedAt:          now.Add(5 * time.Minute),
		SchemaVersion:      contracts.CurrentSchemaVersion,
		Summary:            "keep the projection honest",
		Description:        "rebuild should reflect the chosen side after conflict resolution",
		AcceptanceCriteria: []string{"projection matches canonical markdown", "no import cycle"},
		Notes:              "this used to regress after sync resolve",
		PermissionProfiles: []string{"audit-ops"},
	})

	doc, err := mdstore.EncodeTicketMarkdown(ticket)
	if err != nil {
		t.Fatalf("encode ticket markdown: %v", err)
	}
	decoded, err := decodeTicketConflictSnapshot(doc)
	if err != nil {
		t.Fatalf("decode ticket conflict snapshot: %v", err)
	}
	if decoded.ID != ticket.ID || decoded.Title != ticket.Title || decoded.Description != ticket.Description {
		t.Fatalf("decoded ticket mismatch: %#v", decoded)
	}
	if len(decoded.AcceptanceCriteria) != len(ticket.AcceptanceCriteria) || decoded.AcceptanceCriteria[0] != ticket.AcceptanceCriteria[0] {
		t.Fatalf("decoded acceptance criteria mismatch: %#v", decoded.AcceptanceCriteria)
	}
}
