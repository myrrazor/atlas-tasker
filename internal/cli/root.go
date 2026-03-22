package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	"github.com/spf13/cobra"
)

type outputFlags struct {
	Pretty bool
	MD     bool
	JSON   bool
}

type mutationFlags struct {
	Actor  string
	Reason string
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "tracker",
		Short: "Local-first markdown issue tracker for AI coding agents",
	}

	root.AddCommand(newInitCommand())
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newReindexCommand())
	root.AddCommand(newConfigCommand())
	root.AddCommand(newProjectCommand())
	root.AddCommand(newTicketCommand())
	root.AddCommand(newBoardCommand())
	root.AddCommand(newBacklogCommand())
	root.AddCommand(newNextCommand())
	root.AddCommand(newBlockedCommand())
	root.AddCommand(newSearchCommand())
	root.AddCommand(newRenderCommand())
	root.AddCommand(newShellCommand())

	return root
}

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize tracker workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := ensureInitArtifacts(root); err != nil {
				return err
			}
			workspace, err := openWorkspace()
			if err != nil {
				return err
			}
			workspace.close()
			fmt.Fprintln(cmd.OutOrStdout(), "initialized")
			return nil
		},
	}
	return cmd
}

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run consistency checks",
		RunE:  notImplemented("tracker doctor", "PR-009"),
	}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newReindexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild SQLite projection from markdown and events",
		RunE:  notImplemented("tracker reindex", "PR-009"),
	}
	return cmd
}

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Read or update tracker config"}
	cmd.AddCommand(&cobra.Command{
		Use:   "get [KEY]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Get config values",
		RunE: func(_ *cobra.Command, args []string) error {
			key := ""
			if len(args) == 1 {
				key = args[0]
			}
			rootDir, err := os.Getwd()
			if err != nil {
				return err
			}
			value, err := config.Get(rootDir, key)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "%s\n", value)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <KEY> <VALUE>",
		Args:  cobra.ExactArgs(2),
		Short: "Set config values",
		RunE: func(_ *cobra.Command, args []string) error {
			rootDir, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := config.Set(rootDir, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "ok\n")
			return nil
		},
	})
	return cmd
}

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Project management commands"}
	cmd.AddCommand(&cobra.Command{
		Use:   "create <KEY> <NAME>",
		Args:  cobra.ExactArgs(2),
		Short: "Create a project",
		RunE: func(command *cobra.Command, args []string) error {
			ctx := context.Background()
			workspace, err := openWorkspace()
			if err != nil {
				return err
			}
			defer workspace.close()

			project := contracts.Project{Key: strings.TrimSpace(args[0]), Name: strings.TrimSpace(args[1]), CreatedAt: defaultNow()}
			if err := workspace.project.CreateProject(ctx, project); err != nil {
				return err
			}
			return writeCommandOutput(command, project, fmt.Sprintf("# %s\n\n%s", project.Key, project.Name), fmt.Sprintf("created project %s", project.Key))
		},
	})
	list := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(command *cobra.Command, _ []string) error {
			workspace, err := openWorkspace()
			if err != nil {
				return err
			}
			defer workspace.close()
			projects, err := workspace.project.ListProjects(context.Background())
			if err != nil {
				return err
			}
			pretty := "projects:\n"
			for _, project := range projects {
				pretty += fmt.Sprintf("- %s %s\n", project.Key, project.Name)
			}
			return writeCommandOutput(command, projects, pretty, pretty)
		},
	}
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)
	view := &cobra.Command{
		Use:   "view <KEY>",
		Args:  cobra.ExactArgs(1),
		Short: "View project details",
		RunE: func(command *cobra.Command, args []string) error {
			workspace, err := openWorkspace()
			if err != nil {
				return err
			}
			defer workspace.close()
			project, err := workspace.project.GetProject(context.Background(), args[0])
			if err != nil {
				return err
			}
			md := fmt.Sprintf("# %s\n\n- Name: %s\n- Created: %s", project.Key, project.Name, project.CreatedAt.Format(timeRFC3339))
			pretty := fmt.Sprintf("%s %s", project.Key, project.Name)
			return writeCommandOutput(command, project, md, pretty)
		},
	}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)
	return cmd
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func newTicketCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "ticket", Short: "Ticket CRUD and workflow commands"}

	create := &cobra.Command{Use: "create", Short: "Create a ticket", RunE: runTicketCreate}
	addMutationFlags(create, &mutationFlags{Actor: "human:owner"})
	create.Flags().String("project", "", "Project key (required)")
	create.Flags().String("title", "", "Ticket title (required)")
	create.Flags().String("type", "", "Ticket type: epic|task|bug|subtask (required)")
	create.Flags().String("status", "backlog", "Initial status")
	create.Flags().String("priority", "medium", "Ticket priority")
	create.Flags().String("parent", "", "Parent ticket id")
	create.Flags().String("labels", "", "Comma-separated labels")
	create.Flags().String("assignee", "", "Assignee actor")
	create.Flags().String("reviewer", "", "Reviewer actor")
	create.Flags().String("description", "", "Ticket description")
	create.Flags().StringArray("acceptance", []string{}, "Acceptance criterion (repeatable)")
	_ = create.MarkFlagRequired("project")
	_ = create.MarkFlagRequired("title")
	_ = create.MarkFlagRequired("type")
	cmd.AddCommand(create)

	view := &cobra.Command{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View ticket", RunE: runTicketView}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)

	edit := &cobra.Command{Use: "edit <ID>", Args: cobra.ExactArgs(1), Short: "Edit ticket fields", RunE: runTicketEdit}
	edit.Flags().String("title", "", "New title")
	edit.Flags().String("description", "", "New description")
	edit.Flags().StringArray("acceptance", nil, "Replace acceptance criteria")
	edit.Flags().String("priority", "", "New priority")
	edit.Flags().String("labels", "", "Comma-separated labels")
	edit.Flags().String("assignee", "", "Assignee actor")
	edit.Flags().String("reviewer", "", "Reviewer actor")
	addMutationFlags(edit, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(edit)

	deleteCmd := &cobra.Command{Use: "delete <ID>", Args: cobra.ExactArgs(1), Short: "Soft-delete ticket", RunE: runTicketDelete}
	addMutationFlags(deleteCmd, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(deleteCmd)

	list := &cobra.Command{Use: "list", Short: "List tickets", RunE: runTicketList}
	list.Flags().String("project", "", "Project filter")
	list.Flags().String("status", "", "Status filter")
	list.Flags().String("assignee", "", "Assignee filter")
	list.Flags().String("type", "", "Type filter")
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)

	move := &cobra.Command{Use: "move <ID> <STATUS>", Args: cobra.ExactArgs(2), Short: "Move ticket status", RunE: runTicketMove}
	addMutationFlags(move, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(move)

	assign := &cobra.Command{Use: "assign <ID> <ACTOR>", Args: cobra.ExactArgs(2), Short: "Assign ticket actor", RunE: runTicketAssign}
	addMutationFlags(assign, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(assign)

	priority := &cobra.Command{Use: "priority <ID> <PRIORITY>", Args: cobra.ExactArgs(2), Short: "Set ticket priority", RunE: runTicketPriority}
	addMutationFlags(priority, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(priority)

	label := &cobra.Command{Use: "label", Short: "Manage ticket labels"}
	labelAdd := &cobra.Command{Use: "add <ID> <LABEL>", Args: cobra.ExactArgs(2), Short: "Add label", RunE: runTicketLabelAdd}
	addMutationFlags(labelAdd, &mutationFlags{Actor: "human:owner"})
	label.AddCommand(labelAdd)
	labelRemove := &cobra.Command{Use: "remove <ID> <LABEL>", Args: cobra.ExactArgs(2), Short: "Remove label", RunE: runTicketLabelRemove}
	addMutationFlags(labelRemove, &mutationFlags{Actor: "human:owner"})
	label.AddCommand(labelRemove)
	cmd.AddCommand(label)

	link := &cobra.Command{Use: "link <ID>", Args: cobra.ExactArgs(1), Short: "Create ticket relationship", RunE: runTicketLink}
	link.Flags().String("blocks", "", "Link as blocks relationship")
	link.Flags().String("blocked-by", "", "Link as blocked_by relationship")
	link.Flags().String("parent", "", "Link as parent relationship")
	addMutationFlags(link, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(link)

	unlink := &cobra.Command{Use: "unlink <ID> <OTHER_ID>", Args: cobra.ExactArgs(2), Short: "Remove relationship", RunE: runTicketUnlink}
	addMutationFlags(unlink, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(unlink)

	comment := &cobra.Command{Use: "comment <ID>", Args: cobra.ExactArgs(1), Short: "Add comment to ticket", RunE: runTicketComment}
	comment.Flags().String("body", "", "Comment body")
	_ = comment.MarkFlagRequired("body")
	addMutationFlags(comment, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(comment)

	history := &cobra.Command{Use: "history <ID>", Args: cobra.ExactArgs(1), Short: "Show ticket history", RunE: runTicketHistory}
	addReadOutputFlags(history, &outputFlags{})
	cmd.AddCommand(history)

	return cmd
}

func newBoardCommand() *cobra.Command {
	flags := &outputFlags{}
	cmd := &cobra.Command{Use: "board", Short: "Show board view", RunE: notImplemented("tracker board", "PR-008")}
	cmd.Flags().String("project", "", "Filter by project")
	cmd.Flags().String("assignee", "", "Filter by assignee")
	cmd.Flags().String("type", "", "Filter by ticket type")
	addReadOutputFlags(cmd, flags)
	return cmd
}

func newBacklogCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "backlog", Short: "Show backlog tickets", RunE: notImplemented("tracker backlog", "PR-008")}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newNextCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "next", Short: "Show next-up queue", RunE: notImplemented("tracker next", "PR-008")}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newBlockedCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "blocked", Short: "Show blocked tickets", RunE: notImplemented("tracker blocked", "PR-008")}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newSearchCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "search <QUERY>", Args: cobra.ExactArgs(1), Short: "Search tickets", RunE: notImplemented("tracker search", "PR-008")}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newRenderCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "render <ID>", Args: cobra.ExactArgs(1), Short: "Render markdown ticket details", RunE: notImplemented("tracker render", "PR-008")}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func addReadOutputFlags(cmd *cobra.Command, flags *outputFlags) {
	cmd.Flags().BoolVar(&flags.Pretty, "pretty", true, "Pretty terminal output")
	cmd.Flags().BoolVar(&flags.MD, "md", false, "Markdown output")
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "JSON output")
}

func addMutationFlags(cmd *cobra.Command, flags *mutationFlags) {
	cmd.Flags().StringVar(&flags.Actor, "actor", flags.Actor, "Mutation actor (e.g. human:owner)")
	cmd.Flags().StringVar(&flags.Reason, "reason", "", "Optional reason for change")
}

func notImplemented(commandName string, milestone string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("%s is not implemented yet (%s)", commandName, milestone)
	}
}

func executeArgs(args []string) error {
	root := NewRootCommand()
	root.SetArgs(args)
	return root.Execute()
}

func runTicketCreate(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()

	project, _ := cmd.Flags().GetString("project")
	title, _ := cmd.Flags().GetString("title")
	typeValue, _ := cmd.Flags().GetString("type")
	statusValue, _ := cmd.Flags().GetString("status")
	priorityValue, _ := cmd.Flags().GetString("priority")
	parent, _ := cmd.Flags().GetString("parent")
	labelsRaw, _ := cmd.Flags().GetString("labels")
	assigneeRaw, _ := cmd.Flags().GetString("assignee")
	reviewerRaw, _ := cmd.Flags().GetString("reviewer")
	description, _ := cmd.Flags().GetString("description")
	acceptance, _ := cmd.Flags().GetStringArray("acceptance")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor := normalizeActor(actorRaw)

	if _, err := workspace.project.GetProject(ctx, project); err != nil {
		return err
	}
	existing, err := workspace.ticket.ListTickets(ctx, contracts.TicketListOptions{Project: project, IncludeArchived: true})
	if err != nil {
		return err
	}
	ticketType := contracts.TicketType(typeValue)
	if !ticketType.IsValid() {
		return fmt.Errorf("invalid ticket type: %s", typeValue)
	}
	status := contracts.Status(statusValue)
	if !status.IsValid() {
		return fmt.Errorf("invalid status: %s", statusValue)
	}
	if status == contracts.StatusDone || status == contracts.StatusCanceled {
		return fmt.Errorf("status %s is not allowed on ticket create", status)
	}
	priority := contracts.Priority(priorityValue)
	if !priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", priorityValue)
	}
	if !actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", actorRaw)
	}
	id := nextTicketID(project, existing)
	now := defaultNow()
	ticket := contracts.TicketSnapshot{
		ID:                 id,
		Project:            project,
		Title:              title,
		Type:               ticketType,
		Status:             status,
		Priority:           priority,
		Parent:             parent,
		Labels:             parseLabels(labelsRaw),
		Assignee:           normalizeActor(assigneeRaw),
		Reviewer:           normalizeActor(reviewerRaw),
		CreatedAt:          now,
		UpdatedAt:          now,
		SchemaVersion:      contracts.CurrentSchemaVersion,
		Summary:            title,
		Description:        description,
		AcceptanceCriteria: acceptance,
	}
	if assigneeRaw != "" && !ticket.Assignee.IsValid() {
		return fmt.Errorf("invalid assignee actor: %s", assigneeRaw)
	}
	if reviewerRaw != "" && !ticket.Reviewer.IsValid() {
		return fmt.Errorf("invalid reviewer actor: %s", reviewerRaw)
	}
	if assigneeRaw == "" {
		ticket.Assignee = ""
	}
	if reviewerRaw == "" {
		ticket.Reviewer = ""
	}
	if err := workspace.ticket.CreateTicket(ctx, ticket); err != nil {
		return err
	}
	eventID, err := workspace.nextEventID(ctx, project)
	if err != nil {
		return err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketCreated,
		Project:       project,
		TicketID:      id,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\n%s", ticket.ID, ticket.Title), fmt.Sprintf("created %s", ticket.ID))
}

func runTicketView(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticket, err := workspace.ticket.GetTicket(ctx, args[0])
	if err != nil {
		return err
	}
	events, err := listTicketEvents(ctx, workspace, args[0])
	if err != nil {
		return err
	}
	comments := make([]string, 0)
	for _, event := range events {
		if event.Type == contracts.EventTicketCommented {
			if payloadMap, ok := event.Payload.(map[string]any); ok {
				if body, ok := payloadMap["body"].(string); ok {
					comments = append(comments, body)
				}
			}
		}
	}
	rawMD, _ := mdstore.EncodeTicketMarkdown(ticket)
	if len(comments) > 0 {
		rawMD += "\n## Recent Comments\n\n"
		for _, comment := range comments {
			rawMD += "- " + comment + "\n"
		}
	}
	pretty := fmt.Sprintf("%s [%s] %s", ticket.ID, ticket.Status, ticket.Title)
	payload := map[string]any{"ticket": ticket, "comments": comments}
	return writeCommandOutput(cmd, payload, rawMD, pretty)
}

func runTicketEdit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	ticket, err := workspace.ticket.GetTicket(ctx, args[0])
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("title") {
		title, _ := cmd.Flags().GetString("title")
		ticket.Title = title
		ticket.Summary = title
	}
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		ticket.Description = description
	}
	if cmd.Flags().Changed("acceptance") {
		acceptance, _ := cmd.Flags().GetStringArray("acceptance")
		ticket.AcceptanceCriteria = acceptance
	}
	if cmd.Flags().Changed("priority") {
		priority, _ := cmd.Flags().GetString("priority")
		ticketPriority := contracts.Priority(priority)
		if !ticketPriority.IsValid() {
			return fmt.Errorf("invalid priority: %s", priority)
		}
		ticket.Priority = ticketPriority
	}
	if cmd.Flags().Changed("labels") {
		labels, _ := cmd.Flags().GetString("labels")
		ticket.Labels = parseLabels(labels)
	}
	if cmd.Flags().Changed("assignee") {
		assignee, _ := cmd.Flags().GetString("assignee")
		ticket.Assignee = normalizeActor(assignee)
		if assignee != "" && !ticket.Assignee.IsValid() {
			return fmt.Errorf("invalid assignee actor: %s", assignee)
		}
		if assignee == "" {
			ticket.Assignee = ""
		}
	}
	if cmd.Flags().Changed("reviewer") {
		reviewer, _ := cmd.Flags().GetString("reviewer")
		ticket.Reviewer = normalizeActor(reviewer)
		if reviewer != "" && !ticket.Reviewer.IsValid() {
			return fmt.Errorf("invalid reviewer actor: %s", reviewer)
		}
		if reviewer == "" {
			ticket.Reviewer = ""
		}
	}
	ticket.UpdatedAt = defaultNow()
	if err := workspace.ticket.UpdateTicket(ctx, ticket); err != nil {
		return err
	}
	eventID, err := workspace.nextEventID(ctx, ticket.Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: ticket.UpdatedAt, Actor: normalizeActor(actorRaw), Reason: reason, Type: contracts.EventTicketUpdated, Project: ticket.Project, TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\n%s", ticket.ID, ticket.Title), fmt.Sprintf("updated %s", ticket.ID))
}

func runTicketDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor := normalizeActor(actorRaw)
	if err := workspace.ticket.SoftDeleteTicket(ctx, args[0], actor, reason); err != nil {
		return err
	}
	ticket, err := workspace.ticket.GetTicket(ctx, args[0])
	if err != nil {
		return err
	}
	eventID, err := workspace.nextEventID(ctx, ticket.Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: defaultNow(), Actor: actor, Reason: reason, Type: contracts.EventTicketClosed, Project: ticket.Project, TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\narchived", ticket.ID), fmt.Sprintf("archived %s", ticket.ID))
}

func runTicketList(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	project, _ := cmd.Flags().GetString("project")
	status, _ := cmd.Flags().GetString("status")
	assignee, _ := cmd.Flags().GetString("assignee")
	typeValue, _ := cmd.Flags().GetString("type")
	opts := contracts.TicketListOptions{Project: project}
	if status != "" {
		opts.Statuses = []contracts.Status{contracts.Status(status)}
	}
	if assignee != "" {
		opts.Assignee = contracts.Actor(assignee)
	}
	if typeValue != "" {
		opts.Type = contracts.TicketType(typeValue)
	}
	tickets, err := workspace.ticket.ListTickets(ctx, opts)
	if err != nil {
		return err
	}
	pretty := "tickets:\n"
	for _, ticket := range tickets {
		pretty += fmt.Sprintf("- %s [%s] %s\n", ticket.ID, ticket.Status, ticket.Title)
	}
	return writeCommandOutput(cmd, tickets, pretty, pretty)
}

func runTicketMove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor := normalizeActor(actorRaw)
	ticket, err := workspace.ticket.GetTicket(ctx, args[0])
	if err != nil {
		return err
	}
	cfg, err := config.Load(workspace.root)
	if err != nil {
		return err
	}
	to := contracts.Status(args[1])
	if err := domain.ValidateMove(cfg.Workflow.CompletionMode, ticket.Status, to, actor, ticket.Reviewer); err != nil {
		return err
	}
	from := ticket.Status
	ticket.Status = to
	ticket.UpdatedAt = defaultNow()
	if err := workspace.ticket.UpdateTicket(ctx, ticket); err != nil {
		return err
	}
	eventID, err := workspace.nextEventID(ctx, ticket.Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: ticket.UpdatedAt, Actor: actor, Reason: reason, Type: contracts.EventTicketMoved, Project: ticket.Project, TicketID: ticket.ID, Payload: map[string]any{"from": from, "to": to, "ticket": ticket}, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\n%s -> %s", ticket.ID, from, to), fmt.Sprintf("moved %s to %s", ticket.ID, to))
}

func runTicketAssign(cmd *cobra.Command, args []string) error {
	assignee := normalizeActor(args[1])
	if !assignee.IsValid() {
		return fmt.Errorf("invalid assignee actor: %s", args[1])
	}
	return runTicketFieldUpdate(cmd, args[0], func(ticket *contracts.TicketSnapshot) {
		ticket.Assignee = assignee
	}, "assigned")
}

func runTicketPriority(cmd *cobra.Command, args []string) error {
	priority := contracts.Priority(args[1])
	if !priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", args[1])
	}
	return runTicketFieldUpdate(cmd, args[0], func(ticket *contracts.TicketSnapshot) {
		ticket.Priority = priority
	}, "priority updated")
}

func runTicketLabelAdd(cmd *cobra.Command, args []string) error {
	return runTicketFieldUpdate(cmd, args[0], func(ticket *contracts.TicketSnapshot) {
		ticket.Labels = addUniqueLabel(ticket.Labels, args[1])
	}, "label added")
}

func runTicketLabelRemove(cmd *cobra.Command, args []string) error {
	return runTicketFieldUpdate(cmd, args[0], func(ticket *contracts.TicketSnapshot) {
		ticket.Labels = removeLabel(ticket.Labels, args[1])
	}, "label removed")
}

func runTicketFieldUpdate(cmd *cobra.Command, ticketID string, mutate func(*contracts.TicketSnapshot), message string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor := normalizeActor(actorRaw)
	ticket, err := workspace.ticket.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	mutate(&ticket)
	ticket.UpdatedAt = defaultNow()
	if err := workspace.ticket.UpdateTicket(ctx, ticket); err != nil {
		return err
	}
	eventID, err := workspace.nextEventID(ctx, ticket.Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: ticket.UpdatedAt, Actor: actor, Reason: reason, Type: contracts.EventTicketUpdated, Project: ticket.Project, TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\n%s", ticket.ID, message), fmt.Sprintf("%s: %s", message, ticket.ID))
}

func runTicketLink(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	blocks, _ := cmd.Flags().GetString("blocks")
	blockedBy, _ := cmd.Flags().GetString("blocked-by")
	parent, _ := cmd.Flags().GetString("parent")

	setCount := 0
	if strings.TrimSpace(blocks) != "" {
		setCount++
	}
	if strings.TrimSpace(blockedBy) != "" {
		setCount++
	}
	if strings.TrimSpace(parent) != "" {
		setCount++
	}
	if setCount != 1 {
		return fmt.Errorf("exactly one of --blocks, --blocked-by, --parent is required")
	}
	otherID := ""
	kind := domain.LinkBlocks
	if blocks != "" {
		otherID = blocks
		kind = domain.LinkBlocks
	} else if blockedBy != "" {
		otherID = blockedBy
		kind = domain.LinkBlockedBy
	} else {
		otherID = parent
		kind = domain.LinkParent
	}
	mapped, err := loadTicketsMap(ctx, workspace)
	if err != nil {
		return err
	}
	if err := domain.ApplyLink(mapped, args[0], otherID, kind); err != nil {
		return err
	}
	now := defaultNow()
	for _, id := range []string{args[0], strings.TrimSpace(otherID)} {
		ticket := mapped[id]
		ticket.UpdatedAt = now
		if err := workspace.ticket.UpdateTicket(ctx, ticket); err != nil {
			return err
		}
	}
	eventID, err := workspace.nextEventID(ctx, mapped[args[0]].Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: now, Actor: normalizeActor(actorRaw), Reason: reason, Type: contracts.EventTicketLinked, Project: mapped[args[0]].Project, TicketID: args[0], Payload: map[string]any{"id": args[0], "other_id": strings.TrimSpace(otherID), "kind": kind, "ticket": mapped[args[0]], "other_ticket": mapped[strings.TrimSpace(otherID)]}, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, event, fmt.Sprintf("linked %s %s %s", args[0], kind, strings.TrimSpace(otherID)), fmt.Sprintf("linked %s", args[0]))
}

func runTicketUnlink(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	mapped, err := loadTicketsMap(ctx, workspace)
	if err != nil {
		return err
	}
	if err := domain.RemoveLink(mapped, args[0], args[1]); err != nil {
		return err
	}
	now := defaultNow()
	for _, id := range []string{strings.TrimSpace(args[0]), strings.TrimSpace(args[1])} {
		ticket := mapped[id]
		ticket.UpdatedAt = now
		if err := workspace.ticket.UpdateTicket(ctx, ticket); err != nil {
			return err
		}
	}
	eventID, err := workspace.nextEventID(ctx, mapped[strings.TrimSpace(args[0])].Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: now, Actor: normalizeActor(actorRaw), Reason: reason, Type: contracts.EventTicketUnlinked, Project: mapped[strings.TrimSpace(args[0])].Project, TicketID: strings.TrimSpace(args[0]), Payload: map[string]any{"id": strings.TrimSpace(args[0]), "other_id": strings.TrimSpace(args[1]), "ticket": mapped[strings.TrimSpace(args[0])], "other_ticket": mapped[strings.TrimSpace(args[1])]}, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, event, fmt.Sprintf("unlinked %s %s", args[0], args[1]), fmt.Sprintf("unlinked %s", args[0]))
}

func runTicketComment(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	body, _ := cmd.Flags().GetString("body")
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("comment body is required in v1 non-interactive mode")
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	ticket, err := workspace.ticket.GetTicket(ctx, args[0])
	if err != nil {
		return err
	}
	now := defaultNow()
	eventID, err := workspace.nextEventID(ctx, ticket.Project)
	if err != nil {
		return err
	}
	event := contracts.Event{EventID: eventID, Timestamp: now, Actor: normalizeActor(actorRaw), Reason: reason, Type: contracts.EventTicketCommented, Project: ticket.Project, TicketID: ticket.ID, Payload: map[string]any{"body": strings.TrimSpace(body)}, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := workspace.appendAndProject(ctx, event); err != nil {
		return err
	}
	return writeCommandOutput(cmd, event, body, fmt.Sprintf("comment added to %s", ticket.ID))
}

func runTicketHistory(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	events, err := listTicketEvents(ctx, workspace, args[0])
	if err != nil {
		return err
	}
	pretty := "history:\n"
	for _, event := range events {
		pretty += fmt.Sprintf("- #%d %s %s %s\n", event.EventID, event.Timestamp.Format(timeRFC3339), event.Type, event.Actor)
	}
	return writeCommandOutput(cmd, events, pretty, pretty)
}

func addUniqueLabel(values []string, label string) []string {
	for _, existing := range values {
		if existing == label {
			return values
		}
	}
	return append(values, label)
}

func removeLabel(values []string, label string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != label {
			result = append(result, value)
		}
	}
	return result
}
