package mcp

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestProjectAndTicketWorkflowMutationTools(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer workspace.Close()
	ctx := context.Background()
	server := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())

	projectSpec, ok := ToolSpecByName("atlas.project.create")
	if !ok {
		t.Fatal("missing atlas.project.create")
	}
	if projectSpec.RequiresActor || projectSpec.RequiresReason || projectSpec.RequiresApproval {
		t.Fatalf("project container creation should not require event metadata or approval: %#v", projectSpec)
	}
	if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{"key": "APP", "name": "MCP App"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	project, err := workspace.Actions.Projects.GetProject(ctx, "APP")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if project.Name != "MCP App" || project.SchemaVersion != contracts.CurrentSchemaVersion || !project.CreatedAt.Equal(now) {
		t.Fatalf("unexpected normalized project: %#v", project)
	}
	if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{"key": "APP", "name": "Duplicate"}); err == nil {
		t.Fatal("expected duplicate project creation to fail")
	}
	if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{"key": "../BAD", "name": "Bad"}); err == nil {
		t.Fatal("expected invalid project key to fail")
	}

	created, err := server.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"project": "APP", "title": "Original", "type": "task", "status": "ready",
		"priority": "medium", "labels": []any{"existing"}, "assignee": "human:owner",
		"reviewer": "agent:reviewer-1", "description": "keep me", "acceptance": []any{"original"},
		"actor": "human:owner", "reason": "seed through MCP",
	})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	ticketID := payloadTicketID(t, created)
	mutationCases := map[string]map[string]any{
		"atlas.ticket.heartbeat":    {"ticket_id": ticketID},
		"atlas.ticket.priority":     {"ticket_id": ticketID, "priority": "high"},
		"atlas.ticket.label.add":    {"ticket_id": ticketID, "label": "boundary"},
		"atlas.ticket.label.remove": {"ticket_id": ticketID, "label": "boundary"},
		"atlas.ticket.edit":         {"ticket_id": ticketID, "description": "boundary"},
	}
	readServer := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())
	if _, err := readServer.CallTool(ctx, "atlas.project.create", map[string]any{"key": "READ", "name": "Denied"}); err == nil || !strings.Contains(err.Error(), "profile_not_selected") {
		t.Fatalf("expected read profile to reject project create, got %v", err)
	}
	for name, required := range mutationCases {
		missingActor := cloneToolArgs(required)
		missingActor["reason"] = "boundary check"
		if _, err := server.CallTool(ctx, name, missingActor); err == nil || !strings.Contains(err.Error(), "actor is required") {
			t.Fatalf("expected %s missing actor rejection, got %v", name, err)
		}
		missingReason := cloneToolArgs(required)
		missingReason["actor"] = "human:owner"
		if _, err := server.CallTool(ctx, name, missingReason); err == nil || !strings.Contains(err.Error(), "reason is required") {
			t.Fatalf("expected %s missing reason rejection, got %v", name, err)
		}
		readArgs := cloneToolArgs(required)
		readArgs["actor"] = "human:owner"
		readArgs["reason"] = "boundary check"
		if _, err := readServer.CallTool(ctx, name, readArgs); err == nil || !strings.Contains(err.Error(), "profile_not_selected") {
			t.Fatalf("expected read profile to reject %s, got %v", name, err)
		}
	}

	if _, err := server.CallTool(ctx, "atlas.ticket.priority", map[string]any{
		"ticket_id": ticketID, "priority": "high", "actor": "human:owner", "reason": "raise priority",
	}); err != nil {
		t.Fatalf("set priority: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := server.CallTool(ctx, "atlas.ticket.label.add", map[string]any{
			"ticket_id": ticketID, "label": "workflow", "actor": "human:owner", "reason": "add label",
		}); err != nil {
			t.Fatalf("add label attempt %d: %v", i+1, err)
		}
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.label.remove", map[string]any{
		"ticket_id": ticketID, "label": "existing", "actor": "human:owner", "reason": "remove label",
	}); err != nil {
		t.Fatalf("remove label: %v", err)
	}

	rawDescription := "  leading whitespace\n\ntrailing whitespace  "
	editResult, err := server.CallTool(ctx, "atlas.ticket.edit", map[string]any{
		"ticket_id":   ticketID,
		"title":       "Updated\n title",
		"description": rawDescription,
		"acceptance":  []any{},
		"priority":    "critical",
		"labels":      []any{},
		"assignee":    "",
		"reviewer":    "",
		"actor":       "human:owner",
		"reason":      "apply one atomic patch",
	})
	if err != nil {
		t.Fatalf("edit ticket: %v", err)
	}
	responseTicket, ok := editResult["payload"].(contracts.TicketSnapshot)
	if !ok || responseTicket.Description != rawDescription {
		t.Fatalf("MCP mutation response did not preserve raw description: %#v", editResult["payload"])
	}
	updated, err := workspace.Actions.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		t.Fatalf("reload edited ticket: %v", err)
	}
	if updated.Title != "Updated title" || updated.Summary != "Updated title" || updated.Description != strings.TrimSpace(rawDescription) {
		t.Fatalf("title/summary/persisted description mismatch: %#v", updated)
	}
	if updated.Priority != contracts.PriorityCritical || len(updated.Labels) != 0 || len(updated.AcceptanceCriteria) != 0 || updated.Assignee != "" || updated.Reviewer != "" {
		t.Fatalf("explicit edit clear/replacement mismatch: %#v", updated)
	}

	beforeInvalid := updated
	invalidEdits := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "array item", args: map[string]any{"acceptance": []any{42}}, want: "acceptance items must be strings"},
		{name: "blank title", args: map[string]any{"title": " \n\t "}, want: "title cannot be blank"},
		{name: "assignee", args: map[string]any{"assignee": "not-an-actor"}, want: "invalid assignee actor"},
	}
	for _, testCase := range invalidEdits {
		args := cloneToolArgs(testCase.args)
		args["ticket_id"] = ticketID
		args["actor"] = "human:owner"
		args["reason"] = "invalid " + testCase.name
		if _, err := server.CallTool(ctx, "atlas.ticket.edit", args); err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Fatalf("expected invalid %s error containing %q, got %v", testCase.name, testCase.want, err)
		}
		after, loadErr := workspace.Actions.Tickets.GetTicket(ctx, ticketID)
		if loadErr != nil {
			t.Fatalf("reload after invalid %s: %v", testCase.name, loadErr)
		}
		if !reflect.DeepEqual(after, beforeInvalid) {
			t.Fatalf("invalid %s edit partially persisted:\nbefore=%#v\nafter=%#v", testCase.name, beforeInvalid, after)
		}
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.edit", map[string]any{
		"ticket_id": ticketID, "title": "must not persist", "priority": "urgent",
		"actor": "human:owner", "reason": "invalid atomic patch",
	}); err == nil || !strings.Contains(err.Error(), "invalid priority") {
		t.Fatalf("expected invalid priority error, got %v", err)
	}
	afterInvalid, err := workspace.Actions.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		t.Fatalf("reload after invalid edit: %v", err)
	}
	if !reflect.DeepEqual(afterInvalid, beforeInvalid) {
		t.Fatalf("invalid edit partially persisted:\nbefore=%#v\nafter=%#v", beforeInvalid, afterInvalid)
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.edit", map[string]any{
		"ticket_id": ticketID, "description": nil, "actor": "human:owner", "reason": "reject null",
	}); err == nil || !strings.Contains(err.Error(), "description cannot be null") {
		t.Fatalf("expected null rejection, got %v", err)
	}

	if _, err := server.CallTool(ctx, "atlas.ticket.claim", map[string]any{
		"ticket_id": ticketID, "actor": "human:owner", "reason": "claim for heartbeat",
	}); err != nil {
		t.Fatalf("claim ticket: %v", err)
	}
	now = now.Add(5 * time.Minute)
	if _, err := server.CallTool(ctx, "atlas.ticket.heartbeat", map[string]any{
		"ticket_id": ticketID, "actor": "human:owner", "reason": "still working",
	}); err != nil {
		t.Fatalf("heartbeat ticket: %v", err)
	}
	heartbeat, err := workspace.Actions.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		t.Fatalf("reload heartbeat: %v", err)
	}
	if !heartbeat.Lease.LastHeartbeatAt.Equal(now) || !heartbeat.Lease.ExpiresAt.Equal(now.Add(contracts.DefaultLeaseTTL)) {
		t.Fatalf("heartbeat did not renew from the service clock: %#v", heartbeat.Lease)
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.heartbeat", map[string]any{
		"ticket_id": ticketID, "actor": "agent:other", "reason": "wrong holder",
	}); err == nil {
		t.Fatal("expected wrong-holder heartbeat to fail")
	}
	history, err := workspace.Queries.History(ctx, ticketID)
	if err != nil {
		t.Fatalf("ticket history: %v", err)
	}
	last := history.Events[len(history.Events)-1]
	if last.Type != contracts.EventTicketHeartbeat || last.Metadata.Surface != contracts.EventSurfaceMCP || last.Actor != contracts.Actor("human:owner") {
		t.Fatalf("unexpected heartbeat event metadata: %#v", last)
	}
}

func cloneToolArgs(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func TestNewWorkflowMutationToolsRequireWorkflowActorAndReason(t *testing.T) {
	for _, name := range []string{
		"atlas.ticket.heartbeat",
		"atlas.ticket.priority",
		"atlas.ticket.label.add",
		"atlas.ticket.label.remove",
		"atlas.ticket.edit",
	} {
		spec, ok := ToolSpecByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if spec.Class != ClassWorkflow || !spec.RequiresActor || !spec.RequiresReason || spec.RequiresApproval || spec.HighImpact {
			t.Fatalf("unexpected workflow metadata for %s: %#v", name, spec)
		}
	}
	readInventory := Inventory(Options{Profile: ProfileRead}.Normalized())
	for _, item := range readInventory {
		if strings.HasPrefix(item.Name, "atlas.ticket.") && (item.Name == "atlas.ticket.heartbeat" || item.Name == "atlas.ticket.priority" || item.Name == "atlas.ticket.label.add" || item.Name == "atlas.ticket.label.remove" || item.Name == "atlas.ticket.edit") && item.Enabled {
			t.Fatalf("read profile unexpectedly enabled %s", item.Name)
		}
	}
}
