package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/slashcmd"
)

func (m model) now() time.Time {
	if m.actions != nil && m.actions.Clock != nil {
		return m.actions.Clock().UTC()
	}
	return time.Now().UTC()
}

func (m model) toggleClaimSelected() tea.Cmd {
	ticket, ok := m.selectedTicket()
	if !ok {
		return nil
	}
	return m.runMutation(ticket.ID, func(ctx context.Context, actor contracts.Actor) (string, error) {
		if ticket.Lease.Actor == actor && ticket.Lease.Active(m.now()) {
			_, err := m.actions.ReleaseTicket(ctx, ticket.ID, actor, "released from TUI")
			return fmt.Sprintf("released %s", ticket.ID), err
		}
		updated, err := m.actions.ClaimTicket(ctx, ticket.ID, actor, "claimed from TUI")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("claimed %s as %s", updated.ID, updated.Lease.Kind), nil
	})
}

func (m model) requestReviewSelected() tea.Cmd {
	id := m.selectedTicketID()
	if id == "" {
		return nil
	}
	return m.runMutation(id, func(ctx context.Context, actor contracts.Actor) (string, error) {
		_, err := m.actions.RequestReview(ctx, id, actor, "review requested from TUI")
		return fmt.Sprintf("requested review for %s", id), err
	})
}

func (m model) approveSelected() tea.Cmd {
	id := m.selectedTicketID()
	if id == "" {
		return nil
	}
	return m.runMutation(id, func(ctx context.Context, actor contracts.Actor) (string, error) {
		_, err := m.actions.ApproveTicket(ctx, id, actor, "approved from TUI")
		return fmt.Sprintf("approved %s", id), err
	})
}

func (m model) completeSelected() tea.Cmd {
	id := m.selectedTicketID()
	if id == "" {
		return nil
	}
	return m.runMutation(id, func(ctx context.Context, actor contracts.Actor) (string, error) {
		_, err := m.actions.CompleteTicket(ctx, id, actor, "completed from TUI")
		return fmt.Sprintf("completed %s", id), err
	})
}

func (m model) runPromptMutation(dialog dialogState) tea.Cmd {
	value := strings.TrimSpace(dialog.Input.Value())
	switch dialog.Action {
	case dialogMove:
		status := contracts.Status(value)
		if !status.IsValid() {
			return failMutation(fmt.Errorf("invalid status: %s", value))
		}
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			_, err := m.actions.MoveTicket(ctx, dialog.TicketID, status, actor, "moved from TUI")
			return fmt.Sprintf("moved %s to %s", dialog.TicketID, status), err
		})
	case dialogAssign:
		assignee := contracts.Actor(value)
		if value == "" {
			assignee = ""
		} else if !assignee.IsValid() {
			return failMutation(fmt.Errorf("invalid assignee actor: %s", value))
		}
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			_, err := m.actions.AssignTicket(ctx, dialog.TicketID, assignee, actor, "assigned from TUI")
			if assignee == "" {
				return fmt.Sprintf("cleared assignee on %s", dialog.TicketID), err
			}
			return fmt.Sprintf("assigned %s to %s", dialog.TicketID, assignee), err
		})
	case dialogComment:
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			err := m.actions.CommentTicket(ctx, dialog.TicketID, value, actor, "commented from TUI")
			return fmt.Sprintf("commented on %s", dialog.TicketID), err
		})
	case dialogReject:
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			_, err := m.actions.RejectTicket(ctx, dialog.TicketID, actor, value)
			return fmt.Sprintf("rejected %s", dialog.TicketID), err
		})
	case dialogLink:
		parts := strings.Fields(value)
		if len(parts) != 2 {
			return failMutation(fmt.Errorf("link prompt expects '<kind> <TICKET_ID>'"))
		}
		kind, err := parseLinkKind(parts[0])
		if err != nil {
			return failMutation(err)
		}
		otherID := parts[1]
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			_, err := m.actions.LinkTickets(ctx, dialog.TicketID, otherID, kind, actor, "linked from TUI")
			return fmt.Sprintf("linked %s to %s", dialog.TicketID, otherID), err
		})
	case dialogUnlink:
		otherID := value
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			_, err := m.actions.UnlinkTickets(ctx, dialog.TicketID, otherID, actor, "unlinked from TUI")
			return fmt.Sprintf("unlinked %s from %s", dialog.TicketID, otherID), err
		})
	case dialogBulk:
		return m.previewBulkAction(value)
	default:
		return failMutation(fmt.Errorf("unsupported dialog action: %s", dialog.Action))
	}
}

func (m model) runFormMutation(dialog dialogState) tea.Cmd {
	values := map[string]string{}
	for _, field := range dialog.Fields {
		values[field.Key] = strings.TrimSpace(field.Input.Value())
		if field.Required && values[field.Key] == "" {
			return failMutation(fmt.Errorf("%s is required", strings.ToLower(field.Label)))
		}
	}
	switch dialog.Action {
	case dialogCreate:
		typeValue := contracts.TicketType(values["type"])
		if !typeValue.IsValid() {
			return failMutation(fmt.Errorf("invalid ticket type: %s", values["type"]))
		}
		return m.runMutation("", func(ctx context.Context, actor contracts.Actor) (string, error) {
			now := m.now()
			ticket := contracts.TicketSnapshot{
				Project:            values["project"],
				Title:              values["title"],
				Summary:            values["title"],
				Description:        values["description"],
				Type:               typeValue,
				Status:             contracts.StatusBacklog,
				Priority:           contracts.PriorityMedium,
				CreatedAt:          now,
				UpdatedAt:          now,
				SchemaVersion:      contracts.CurrentSchemaVersion,
				AcceptanceCriteria: []string{},
			}
			created, err := m.actions.CreateTrackedTicket(ctx, ticket, actor, "created from TUI")
			if err != nil {
				return "", err
			}
			return created.ID, nil
		})
	case dialogEdit:
		return m.runMutation(dialog.TicketID, func(ctx context.Context, actor contracts.Actor) (string, error) {
			ticket, err := m.actions.Tickets.GetTicket(ctx, dialog.TicketID)
			if err != nil {
				return "", err
			}
			ticket.Title = values["title"]
			ticket.Summary = values["title"]
			ticket.Description = values["description"]
			ticket.UpdatedAt = m.now()
			_, err = m.actions.SaveTrackedTicket(ctx, ticket, actor, "edited from TUI")
			if err != nil {
				return "", err
			}
			return ticket.ID, nil
		})
	default:
		return failMutation(fmt.Errorf("unsupported form action: %s", dialog.Action))
	}
}

func (m model) runSlashMutation(input string) tea.Cmd {
	args, err := slashcmd.Parse(input)
	if err != nil {
		return failMutation(err)
	}
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "ticket":
		return m.runMutation("", func(ctx context.Context, actor contracts.Actor) (string, error) {
			return m.executeSlash(ctx, args, actor)
		})
	case "bulk":
		return m.runBulkSlash(args[1:])
	case "views":
		if len(args) == 3 && args[1] == "run" {
			return m.loadSavedView(args[2])
		}
		return failMutation(fmt.Errorf("supported view command: /views run <NAME>"))
	default:
		return failMutation(fmt.Errorf("TUI command palette currently supports /ticket, /bulk, and /views run"))
	}
}

func (m model) runMutation(fallbackSelectedID string, fn func(context.Context, contracts.Actor) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx := service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: contracts.EventSurfaceTUI})
		actor, err := m.queries.ResolveActor(ctx, m.actor)
		if err != nil {
			return loadedMsg{err: err}
		}
		affectedID, err := fn(ctx, actor)
		if err != nil {
			return loadedMsg{err: err}
		}
		status := "mutation applied"
		if strings.TrimSpace(affectedID) != "" && !strings.Contains(affectedID, " ") {
			fallbackSelectedID = affectedID
			status = affectedID
		}
		if strings.TrimSpace(fallbackSelectedID) == "" {
			fallbackSelectedID = m.selectedTicketID()
		}
		msg := m.reload(fallbackSelectedID, strings.TrimSpace(m.search.Value()), status)()
		if loaded, ok := msg.(loadedMsg); ok {
			loaded.status = status
			return loaded
		}
		return msg
	}
}

func failMutation(err error) tea.Cmd {
	return func() tea.Msg {
		return loadedMsg{err: err}
	}
}

func (m model) previewBulkAction(raw string) tea.Cmd {
	op, err := m.buildBulkOperation(strings.Fields(strings.TrimSpace(raw)))
	if err != nil {
		return failMutation(err)
	}
	return m.runBulk(op, false)
}

func (m model) applyPendingBulk() tea.Cmd {
	if m.pendingBulk == nil {
		return nil
	}
	op := *m.pendingBulk
	return m.runBulk(op, true)
}

func (m model) runBulkSlash(args []string) tea.Cmd {
	op, err := m.buildBulkOperation(args)
	if err != nil {
		return failMutation(err)
	}
	return m.runBulk(op, false)
}

func (m model) runBulk(op service.BulkOperation, apply bool) tea.Cmd {
	return func() tea.Msg {
		ctx := service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: contracts.EventSurfaceTUI})
		actor, err := m.queries.ResolveActor(ctx, m.actor)
		if err != nil {
			return bulkMsg{err: err}
		}
		op.Actor = actor
		if len(op.TicketIDs) == 0 {
			op.TicketIDs = m.currentBulkTicketIDs()
		}
		op.BatchID = service.NewOpaqueID()
		if apply {
			op.DryRun = false
			op.Confirm = true
		} else {
			op.DryRun = true
			op.Confirm = false
		}
		result, err := m.actions.RunBulk(ctx, op)
		return bulkMsg{result: result, op: op, applied: apply, err: err}
	}
}

func (m model) buildBulkOperation(args []string) (service.BulkOperation, error) {
	if len(args) == 0 {
		return service.BulkOperation{}, fmt.Errorf("bulk action is required")
	}
	op := service.BulkOperation{Reason: "bulk action from TUI"}
	switch strings.TrimSpace(args[0]) {
	case "move":
		if len(args) != 2 {
			return service.BulkOperation{}, fmt.Errorf("usage: move <STATUS>")
		}
		status := contracts.Status(args[1])
		if !status.IsValid() {
			return service.BulkOperation{}, fmt.Errorf("invalid status: %s", args[1])
		}
		op.Kind = service.BulkOperationMove
		op.Status = status
	case "assign":
		if len(args) != 2 {
			return service.BulkOperation{}, fmt.Errorf("usage: assign <ACTOR>")
		}
		assignee := contracts.Actor(args[1])
		if args[1] != "" && !assignee.IsValid() {
			return service.BulkOperation{}, fmt.Errorf("invalid assignee actor: %s", args[1])
		}
		op.Kind = service.BulkOperationAssign
		op.Assignee = assignee
	case "request-review":
		op.Kind = service.BulkOperationRequestReview
	case "complete":
		op.Kind = service.BulkOperationComplete
	case "claim":
		op.Kind = service.BulkOperationClaim
	case "release":
		op.Kind = service.BulkOperationRelease
	default:
		return service.BulkOperation{}, fmt.Errorf("unsupported bulk action: %s", args[0])
	}
	return op, nil
}

func (m model) currentBulkTicketIDs() []string {
	switch m.screen {
	case screenDetail:
		if m.detail.Ticket.ID != "" {
			return []string{m.detail.Ticket.ID}
		}
		return nil
	default:
		items := m.itemsForScreen()
		ids := make([]string, 0, len(items))
		for _, ticket := range items {
			ids = append(ids, ticket.ID)
		}
		return ids
	}
}

func (m model) executeSlash(ctx context.Context, args []string, actor contracts.Actor) (string, error) {
	if len(args) < 2 || args[0] != "ticket" {
		return "", fmt.Errorf("TUI command palette currently supports /ticket ... commands only")
	}
	cmd := args[1]
	positionals, flags := splitSlashArgs(args[2:])
	reason := firstFlag(flags, "reason")
	if actorFlag := firstFlag(flags, "actor"); actorFlag != "" {
		actor = contracts.Actor(actorFlag)
		if !actor.IsValid() {
			return "", fmt.Errorf("invalid actor: %s", actorFlag)
		}
	}
	switch cmd {
	case "claim":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket claim <ID>")
		}
		updated, err := m.actions.ClaimTicket(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "release":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket release <ID>")
		}
		updated, err := m.actions.ReleaseTicket(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "heartbeat":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket heartbeat <ID>")
		}
		updated, err := m.actions.HeartbeatTicket(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "move":
		if len(positionals) != 2 {
			return "", fmt.Errorf("usage: /ticket move <ID> <STATUS>")
		}
		status := contracts.Status(positionals[1])
		if !status.IsValid() {
			return "", fmt.Errorf("invalid status: %s", positionals[1])
		}
		updated, err := m.actions.MoveTicket(ctx, positionals[0], status, actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "assign":
		if len(positionals) != 2 {
			return "", fmt.Errorf("usage: /ticket assign <ID> <ACTOR>")
		}
		assignee := contracts.Actor(positionals[1])
		if !assignee.IsValid() {
			return "", fmt.Errorf("invalid assignee actor: %s", positionals[1])
		}
		updated, err := m.actions.AssignTicket(ctx, positionals[0], assignee, actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "comment":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket comment <ID> --body <TEXT>")
		}
		body := firstFlag(flags, "body")
		if err := m.actions.CommentTicket(ctx, positionals[0], body, actor, reason); err != nil {
			return "", err
		}
		return positionals[0], nil
	case "request-review":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket request-review <ID>")
		}
		updated, err := m.actions.RequestReview(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "approve":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket approve <ID>")
		}
		updated, err := m.actions.ApproveTicket(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "reject":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket reject <ID> --reason <TEXT>")
		}
		updated, err := m.actions.RejectTicket(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "complete":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket complete <ID>")
		}
		updated, err := m.actions.CompleteTicket(ctx, positionals[0], actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "create":
		project := firstFlag(flags, "project")
		title := firstFlag(flags, "title")
		typeValue := contracts.TicketType(firstFlag(flags, "type"))
		description := firstFlag(flags, "description")
		if project == "" || title == "" || !typeValue.IsValid() {
			return "", fmt.Errorf("usage: /ticket create --project <KEY> --title <TEXT> --type <TYPE> [--description <TEXT>]")
		}
		now := m.now()
		ticket := contracts.TicketSnapshot{Project: project, Title: title, Summary: title, Description: description, Type: typeValue, Status: contracts.StatusBacklog, Priority: contracts.PriorityMedium, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion, AcceptanceCriteria: []string{}}
		created, err := m.actions.CreateTrackedTicket(ctx, ticket, actor, reason)
		if err != nil {
			return "", err
		}
		return created.ID, nil
	case "edit":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket edit <ID> [--title <TEXT>] [--description <TEXT>]")
		}
		ticket, err := m.actions.Tickets.GetTicket(ctx, positionals[0])
		if err != nil {
			return "", err
		}
		if title := firstFlag(flags, "title"); title != "" {
			ticket.Title = title
			ticket.Summary = title
		}
		if flags["description"] != nil {
			ticket.Description = firstFlag(flags, "description")
		}
		ticket.UpdatedAt = m.now()
		updated, err := m.actions.SaveTrackedTicket(ctx, ticket, actor, reason)
		if err != nil {
			return "", err
		}
		return updated.ID, nil
	case "link":
		if len(positionals) != 1 {
			return "", fmt.Errorf("usage: /ticket link <ID> --blocks <OTHER_ID> | --blocked-by <OTHER_ID> | --parent <OTHER_ID>")
		}
		kind, otherID, err := linkArgsFromFlags(flags)
		if err != nil {
			return "", err
		}
		_, err = m.actions.LinkTickets(ctx, positionals[0], otherID, kind, actor, reason)
		if err != nil {
			return "", err
		}
		return positionals[0], nil
	case "unlink":
		if len(positionals) != 2 {
			return "", fmt.Errorf("usage: /ticket unlink <ID> <OTHER_ID>")
		}
		_, err := m.actions.UnlinkTickets(ctx, positionals[0], positionals[1], actor, reason)
		if err != nil {
			return "", err
		}
		return positionals[0], nil
	default:
		return "", fmt.Errorf("unsupported /ticket command: %s", cmd)
	}
}

func splitSlashArgs(args []string) ([]string, map[string][]string) {
	positionals := make([]string, 0)
	flags := map[string][]string{}
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			value := ""
			if idx+1 < len(args) && !strings.HasPrefix(args[idx+1], "--") {
				value = args[idx+1]
				idx++
			}
			flags[name] = append(flags[name], value)
			continue
		}
		positionals = append(positionals, arg)
	}
	return positionals, flags
}

func firstFlag(flags map[string][]string, key string) string {
	values := flags[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[len(values)-1])
}

func parseLinkKind(raw string) (domain.LinkKind, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "blocks":
		return domain.LinkBlocks, nil
	case "blocked-by", "blocked_by":
		return domain.LinkBlockedBy, nil
	case "parent":
		return domain.LinkParent, nil
	default:
		return "", fmt.Errorf("unsupported link kind: %s", raw)
	}
}

func linkArgsFromFlags(flags map[string][]string) (domain.LinkKind, string, error) {
	kindCount := 0
	var kind domain.LinkKind
	var otherID string
	for flagName, linkKind := range map[string]domain.LinkKind{"blocks": domain.LinkBlocks, "blocked-by": domain.LinkBlockedBy, "parent": domain.LinkParent} {
		if value := firstFlag(flags, flagName); value != "" {
			kindCount++
			kind = linkKind
			otherID = value
		}
	}
	if kindCount != 1 {
		return "", "", fmt.Errorf("exactly one of --blocks, --blocked-by, --parent is required")
	}
	return kind, otherID, nil
}
