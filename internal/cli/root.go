package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	"github.com/myrrazor/atlas-tasker/internal/tui"
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
		Use:           "tracker",
		Short:         "Local-first markdown issue tracker for AI coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
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
	root.AddCommand(newQueueCommand())
	root.AddCommand(newReviewQueueCommand())
	root.AddCommand(newOwnerQueueCommand())
	root.AddCommand(newWhoCommand())
	root.AddCommand(newSweepCommand())
	root.AddCommand(newInspectCommand())
	root.AddCommand(newTemplatesCommand())
	root.AddCommand(newIntegrationsCommand())
	root.AddCommand(newSearchCommand())
	root.AddCommand(newRenderCommand())
	root.AddCommand(newShellCommand())
	root.AddCommand(newTUICommand())

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
		RunE:  runDoctor,
	}
	cmd.Flags().Bool("repair", false, "Rebuild the projection after checks")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newReindexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild SQLite projection from markdown and events",
		RunE:  runReindex,
	}
	return cmd
}

func newInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <ID>",
		Args:  cobra.ExactArgs(1),
		Short: "Inspect a ticket with policy, lease, and history context",
		RunE:  runInspect,
	}
	cmd.Flags().String("actor", "", "Actor used to resolve queue placement")
	addReadOutputFlags(cmd, &outputFlags{})
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
			workspace, err := openWorkspace()
			if err != nil {
				return err
			}
			defer workspace.close()
			if err := workspace.withWriteLock(context.Background(), "config set", func(_ context.Context) error {
				return config.Set(workspace.root, args[0], args[1])
			}); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "ok\n")
			return nil
		},
	})
	return cmd
}

func newIntegrationsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "integrations", Short: "Install agent guidance for Atlas Tasker"}
	install := &cobra.Command{Use: "install", Short: "Install Atlas Tasker guidance into agent files"}
	for _, target := range []string{"codex", "claude"} {
		target := target
		targetCmd := &cobra.Command{
			Use:   target,
			Short: fmt.Sprintf("Install %s guidance", target),
			RunE: func(command *cobra.Command, _ []string) error {
				rootDir, err := os.Getwd()
				if err != nil {
					return err
				}
				if err := ensureInitArtifacts(rootDir); err != nil {
					return err
				}
				force, _ := command.Flags().GetBool("force")
				result, err := integrations.Installer{Root: rootDir}.Install(integrations.Target(target), force)
				if err != nil {
					return err
				}
				pretty := fmt.Sprintf("installed %s guidance into %s", target, result.InstructionFile)
				md := fmt.Sprintf("# %s integration\n\n- Instructions: %s\n- Guide: %s", target, result.InstructionFile, result.GuideFile)
				return writeCommandOutput(command, result, md, pretty)
			},
		}
		targetCmd.Flags().Bool("force", false, "Replace the whole instruction file instead of only the Atlas Tasker managed block")
		addReadOutputFlags(targetCmd, &outputFlags{})
		install.AddCommand(targetCmd)
	}
	cmd.AddCommand(install)
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

			now := defaultNow()
			if workspace.actions.Clock != nil {
				now = workspace.actions.Clock().UTC()
			}
			project := contracts.Project{Key: strings.TrimSpace(args[0]), Name: strings.TrimSpace(args[1]), CreatedAt: now}
			if err := workspace.actions.CreateProject(ctx, project); err != nil {
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

	policy := &cobra.Command{Use: "policy", Short: "Read or update project policy"}
	policyGet := &cobra.Command{Use: "get <KEY>", Args: cobra.ExactArgs(1), Short: "Get project policy", RunE: runProjectPolicyGet}
	addReadOutputFlags(policyGet, &outputFlags{})
	policy.AddCommand(policyGet)
	policySet := &cobra.Command{Use: "set <KEY>", Args: cobra.ExactArgs(1), Short: "Set project policy", RunE: runProjectPolicySet}
	policySet.Flags().String("completion-mode", "", "Default completion mode")
	policySet.Flags().Int("lease-ttl", 0, "Default lease TTL in minutes")
	policySet.Flags().String("allowed-workers", "", "Comma-separated allowed actors")
	policySet.Flags().String("required-reviewer", "", "Default required reviewer actor")
	addMutationFlags(policySet, &mutationFlags{Actor: "human:owner"})
	policy.AddCommand(policySet)
	cmd.AddCommand(policy)
	return cmd
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func newTicketCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "ticket", Short: "Ticket CRUD and workflow commands"}

	create := &cobra.Command{Use: "create", Short: "Create a ticket", RunE: runTicketCreate}
	addMutationFlags(create, &mutationFlags{Actor: "human:owner"})
	create.Flags().String("project", "", "Project key (required)")
	create.Flags().String("title", "", "Ticket title (required)")
	create.Flags().String("type", "", "Ticket type: epic|task|bug|subtask")
	create.Flags().String("template", "", "Template name from .tracker/templates")
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

	claim := &cobra.Command{Use: "claim <ID>", Args: cobra.ExactArgs(1), Short: "Claim a ticket lease", RunE: runTicketClaim}
	addMutationFlags(claim, &mutationFlags{})
	cmd.AddCommand(claim)

	release := &cobra.Command{Use: "release <ID>", Args: cobra.ExactArgs(1), Short: "Release a ticket lease", RunE: runTicketRelease}
	addMutationFlags(release, &mutationFlags{})
	cmd.AddCommand(release)

	heartbeat := &cobra.Command{Use: "heartbeat <ID>", Args: cobra.ExactArgs(1), Short: "Extend an active ticket lease", RunE: runTicketHeartbeat}
	addMutationFlags(heartbeat, &mutationFlags{})
	cmd.AddCommand(heartbeat)

	requestReview := &cobra.Command{Use: "request-review <ID>", Args: cobra.ExactArgs(1), Short: "Move ticket into review", RunE: runTicketRequestReview}
	addMutationFlags(requestReview, &mutationFlags{})
	cmd.AddCommand(requestReview)

	approve := &cobra.Command{Use: "approve <ID>", Args: cobra.ExactArgs(1), Short: "Approve a ticket in review", RunE: runTicketApprove}
	addMutationFlags(approve, &mutationFlags{})
	cmd.AddCommand(approve)

	reject := &cobra.Command{Use: "reject <ID>", Args: cobra.ExactArgs(1), Short: "Reject a ticket in review", RunE: runTicketReject}
	addMutationFlags(reject, &mutationFlags{})
	_ = reject.MarkFlagRequired("reason")
	cmd.AddCommand(reject)

	complete := &cobra.Command{Use: "complete <ID>", Args: cobra.ExactArgs(1), Short: "Complete an approved ticket", RunE: runTicketComplete}
	addMutationFlags(complete, &mutationFlags{})
	cmd.AddCommand(complete)

	policy := &cobra.Command{Use: "policy", Short: "Read or update ticket policy"}
	ticketPolicyGet := &cobra.Command{Use: "get <ID>", Args: cobra.ExactArgs(1), Short: "Get ticket policy", RunE: runTicketPolicyGet}
	addReadOutputFlags(ticketPolicyGet, &outputFlags{})
	policy.AddCommand(ticketPolicyGet)
	ticketPolicySet := &cobra.Command{Use: "set <ID>", Args: cobra.ExactArgs(1), Short: "Set ticket policy", RunE: runTicketPolicySet}
	ticketPolicySet.Flags().Bool("inherit", false, "Ticket inherits upstream policy")
	ticketPolicySet.Flags().String("completion-mode", "", "Override completion mode")
	ticketPolicySet.Flags().String("allowed-workers", "", "Comma-separated allowed actors")
	ticketPolicySet.Flags().String("required-reviewer", "", "Required reviewer actor")
	ticketPolicySet.Flags().Bool("owner-override", false, "Allow owner override")
	addMutationFlags(ticketPolicySet, &mutationFlags{Actor: "human:owner"})
	policy.AddCommand(ticketPolicySet)
	cmd.AddCommand(policy)

	return cmd
}

func newBoardCommand() *cobra.Command {
	flags := &outputFlags{}
	cmd := &cobra.Command{Use: "board", Short: "Show board view", RunE: runBoard}
	cmd.Flags().String("project", "", "Filter by project")
	cmd.Flags().String("assignee", "", "Filter by assignee")
	cmd.Flags().String("type", "", "Filter by ticket type")
	addReadOutputFlags(cmd, flags)
	return cmd
}

func newBacklogCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "backlog", Short: "Show backlog tickets", RunE: runBacklog}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newNextCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "next", Short: "Show next-up queue", RunE: runNext}
	cmd.Flags().String("actor", "", "Actor used for queue-aware next")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newTemplatesCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "templates", Short: "List and inspect ticket templates"}
	list := &cobra.Command{Use: "list", Short: "List templates", RunE: runTemplatesList}
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)
	view := &cobra.Command{Use: "view <NAME>", Args: cobra.ExactArgs(1), Short: "Show template details", RunE: runTemplatesView}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)
	return cmd
}

func newTUICommand() *cobra.Command {
	cmd := &cobra.Command{Use: "tui", Short: "Launch the full-screen tracker TUI", RunE: runTUI}
	cmd.Flags().String("actor", "", "Actor used for queue-aware tabs")
	return cmd
}

func newBlockedCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "blocked", Short: "Show blocked tickets", RunE: runBlocked}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newQueueCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "Show actor queue", RunE: runQueue}
	cmd.Flags().String("actor", "", "Queue actor")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newReviewQueueCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "review-queue", Short: "Show review queue for actor", RunE: runReviewQueue}
	cmd.Flags().String("actor", "", "Reviewer actor")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newOwnerQueueCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "owner-queue", Short: "Show owner attention queue", RunE: runOwnerQueue}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newWhoCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "who", Short: "List claimed tickets", RunE: runWho}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newSweepCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "sweep", Short: "Expire stale leases", RunE: runSweep}
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newSearchCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "search <QUERY>", Args: cobra.ExactArgs(1), Short: "Search tickets", RunE: runSearch}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newRenderCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "render <ID>", Args: cobra.ExactArgs(1), Short: "Render markdown ticket details", RunE: runRender}
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
	templateName, _ := cmd.Flags().GetString("template")
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
	var template service.TemplateView
	if strings.TrimSpace(templateName) != "" {
		template, err = workspace.queries.Template(ctx, templateName)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(typeValue) == "" && template.Type != "" {
		typeValue = string(template.Type)
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
	now := defaultNow()
	if workspace.actions.Clock != nil {
		now = workspace.actions.Clock().UTC()
	}
	ticket := contracts.TicketSnapshot{
		Project:            project,
		Title:              title,
		Type:               ticketType,
		Status:             status,
		Priority:           priority,
		Parent:             parent,
		Labels:             parseLabels(labelsRaw),
		CreatedAt:          now,
		UpdatedAt:          now,
		SchemaVersion:      contracts.CurrentSchemaVersion,
		Summary:            title,
		Description:        description,
		AcceptanceCriteria: acceptance,
		Template:           strings.TrimSpace(templateName),
	}
	if len(ticket.Labels) == 0 && len(template.Labels) > 0 {
		ticket.Labels = append([]string{}, template.Labels...)
	}
	if ticket.Reviewer == "" && template.Reviewer != "" {
		ticket.Reviewer = template.Reviewer
	}
	if strings.TrimSpace(ticket.Description) == "" && strings.TrimSpace(template.Description) != "" {
		ticket.Description = template.Description
	}
	if len(ticket.AcceptanceCriteria) == 0 && len(template.Acceptance) > 0 {
		ticket.AcceptanceCriteria = append([]string{}, template.Acceptance...)
	}
	if !ticket.Policy.HasOverrides() && template.Policy.HasOverrides() {
		ticket.Policy = template.Policy
	}
	if ticket.Blueprint == "" {
		ticket.Blueprint = template.Blueprint
	}
	if ticket.SkillHint == "" {
		ticket.SkillHint = template.SkillHint
	}
	if strings.TrimSpace(assigneeRaw) != "" {
		ticket.Assignee = contracts.Actor(strings.TrimSpace(assigneeRaw))
		if !ticket.Assignee.IsValid() {
			return fmt.Errorf("invalid assignee actor: %s", assigneeRaw)
		}
	}
	if strings.TrimSpace(reviewerRaw) != "" {
		ticket.Reviewer = contracts.Actor(strings.TrimSpace(reviewerRaw))
		if !ticket.Reviewer.IsValid() {
			return fmt.Errorf("invalid reviewer actor: %s", reviewerRaw)
		}
	}
	ticket, err = workspace.actions.CreateTrackedTicket(ctx, ticket, actor, reason)
	if err != nil {
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
	detail, err := workspace.queries.TicketDetail(ctx, args[0])
	if err != nil {
		return err
	}
	rawMD, err := mdstore.EncodeTicketMarkdown(detail.Ticket)
	if err != nil {
		return err
	}
	if len(detail.Comments) > 0 {
		rawMD += "\n## Recent Comments\n\n"
		for _, comment := range detail.Comments {
			rawMD += "- " + comment + "\n"
		}
	}
	pretty := fmt.Sprintf("%s [%s] %s", detail.Ticket.ID, detail.Ticket.Status, detail.Ticket.Title)
	payload := map[string]any{"ticket": detail.Ticket, "comments": detail.Comments, "effective_policy": detail.EffectivePolicy}
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
	ticket, err := workspace.actions.MutateTrackedTicket(ctx, args[0], normalizeActor(actorRaw), reason, "edit ticket", func(ticket *contracts.TicketSnapshot) error {
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
		return nil
	})
	if err != nil {
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
	ticket, err := workspace.actions.DeleteTrackedTicket(ctx, args[0], actor, reason)
	if err != nil {
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
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	to := contracts.Status(args[1])
	if !to.IsValid() {
		return fmt.Errorf("invalid status: %s", to)
	}
	ticket, err := workspace.actions.MoveTicket(ctx, args[0], to, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\nmoved to %s", ticket.ID, ticket.Status), fmt.Sprintf("moved %s to %s", ticket.ID, ticket.Status))
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
	ticket, err := workspace.actions.MutateTrackedTicket(ctx, ticketID, actor, reason, message, func(ticket *contracts.TicketSnapshot) error {
		mutate(ticket)
		return nil
	})
	if err != nil {
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
	event, err := workspace.actions.LinkTickets(ctx, args[0], otherID, kind, normalizeActor(actorRaw), reason)
	if err != nil {
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
	event, err := workspace.actions.UnlinkTickets(ctx, args[0], args[1], normalizeActor(actorRaw), reason)
	if err != nil {
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
	if err := workspace.actions.CommentTicket(ctx, args[0], body, normalizeActor(actorRaw), reason); err != nil {
		return err
	}
	return writeCommandOutput(cmd, map[string]any{"ticket_id": args[0], "body": strings.TrimSpace(body)}, body, fmt.Sprintf("comment added to %s", args[0]))
}

func runTicketHistory(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	history, err := workspace.queries.History(ctx, args[0])
	if err != nil {
		return err
	}
	pretty := "history:\n"
	for _, event := range history.Events {
		pretty += fmt.Sprintf("- #%d %s %s %s\n", event.EventID, event.Timestamp.Format(timeRFC3339), event.Type, event.Actor)
	}
	return writeCommandOutput(cmd, history, pretty, pretty)
}

func runTicketClaim(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.ClaimTicket(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("claimed %s as %s until %s", ticket.ID, ticket.Lease.Kind, ticket.Lease.ExpiresAt.Format(timeRFC3339))
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\nclaimed by %s", ticket.ID, actor), pretty)
}

func runTicketRelease(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.ReleaseTicket(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\nlease released", ticket.ID), fmt.Sprintf("released %s", ticket.ID))
}

func runTicketHeartbeat(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.HeartbeatTicket(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("heartbeat %s -> %s", ticket.ID, ticket.Lease.ExpiresAt.Format(timeRFC3339))
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\nlease extended", ticket.ID), pretty)
}

func runTicketRequestReview(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.RequestReview(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\nreview requested", ticket.ID), fmt.Sprintf("requested review for %s", ticket.ID))
}

func runTicketApprove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.ApproveTicket(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\napproved", ticket.ID), fmt.Sprintf("approved %s", ticket.ID))
}

func runTicketReject(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.RejectTicket(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\nrejected", ticket.ID), fmt.Sprintf("rejected %s", ticket.ID))
}

func runTicketComplete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.CompleteTicket(ctx, args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\ndone", ticket.ID), fmt.Sprintf("completed %s", ticket.ID))
}

func runTicketPolicyGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	policy, effective, err := workspace.actions.GetTicketPolicy(ctx, args[0])
	if err != nil {
		return err
	}
	payload := map[string]any{"policy": policy, "effective_policy": effective}
	md := fmt.Sprintf("## Ticket Policy %s\n\n- Completion: %s\n- Effective Completion: %s\n", args[0], policy.CompletionMode, effective.CompletionMode)
	pretty := fmt.Sprintf("ticket policy %s -> %s (effective %s)", args[0], policy.CompletionMode, effective.CompletionMode)
	return writeCommandOutput(cmd, payload, md, pretty)
}

func runTicketPolicySet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	current, _, err := workspace.actions.GetTicketPolicy(ctx, args[0])
	if err != nil {
		return err
	}
	policy, err := ticketPolicyFromFlags(cmd, current)
	if err != nil {
		return err
	}
	ticket, err := workspace.actions.SetTicketPolicy(ctx, args[0], policy, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("# %s\n\npolicy updated", ticket.ID), fmt.Sprintf("updated ticket policy for %s", ticket.ID))
}

func runProjectPolicyGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	defaults, err := workspace.actions.GetProjectPolicy(ctx, args[0])
	if err != nil {
		return err
	}
	md := fmt.Sprintf("## Project Policy %s\n\n- Completion: %s\n- Lease TTL: %d\n", args[0], defaults.CompletionMode, defaults.LeaseTTLMinutes)
	pretty := fmt.Sprintf("project policy %s -> %s/%d", args[0], defaults.CompletionMode, defaults.LeaseTTLMinutes)
	return writeCommandOutput(cmd, defaults, md, pretty)
}

func runProjectPolicySet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	current, err := workspace.actions.GetProjectPolicy(ctx, args[0])
	if err != nil {
		return err
	}
	defaults, err := projectPolicyFromFlags(cmd, current)
	if err != nil {
		return err
	}
	project, err := workspace.actions.SetProjectPolicy(ctx, args[0], defaults, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, project, fmt.Sprintf("# %s\n\nproject policy updated", project.Key), fmt.Sprintf("updated project policy for %s", project.Key))
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	cfg, err := config.Load(workspace.root)
	if err != nil {
		return err
	}
	events, err := workspace.events.StreamEvents(ctx, "", 0)
	if err != nil {
		return err
	}
	projects, err := workspace.project.ListProjects(ctx)
	if err != nil {
		return err
	}
	tickets, err := workspace.ticket.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return err
	}
	projectIssues := 0
	for _, project := range projects {
		if err := project.Validate(); err != nil {
			return err
		}
	}
	ticketIssues := 0
	for _, ticket := range tickets {
		normalized := contracts.NormalizeTicketSnapshot(ticket)
		if strings.TrimSpace(normalized.ID) == "" || strings.TrimSpace(normalized.Project) == "" || strings.TrimSpace(normalized.Title) == "" {
			return fmt.Errorf("invalid ticket snapshot: %s", ticket.ID)
		}
		if !normalized.Type.IsValid() || !normalized.Status.IsValid() || !normalized.Priority.IsValid() {
			return fmt.Errorf("invalid ticket snapshot enums: %s", ticket.ID)
		}
		if err := normalized.Policy.Validate(); err != nil {
			return err
		}
		if err := normalized.Lease.Validate(); err != nil {
			return err
		}
		if err := normalized.Progress.Validate(); err != nil {
			return err
		}
		if normalized.Project == "" {
			ticketIssues++
		}
	}
	repairReport := service.RepairReport{}
	if _, err := workspace.projection.QueryBoard(ctx, contracts.BoardQueryOptions{}); err != nil {
		repair, _ := cmd.Flags().GetBool("repair")
		if !repair {
			return err
		}
		if rebuildErr := workspace.withWriteLock(ctx, "doctor repair", func(ctx context.Context) error {
			var err error
			repairReport, err = service.RepairWorkspace(ctx, workspace.root, workspace.actions.Clock, workspace.events, workspace.projection)
			return err
		}); rebuildErr != nil {
			return rebuildErr
		}
	} else {
		repair, _ := cmd.Flags().GetBool("repair")
		if repair {
			if err := workspace.withWriteLock(ctx, "doctor repair", func(ctx context.Context) error {
				var err error
				repairReport, err = service.RepairWorkspace(ctx, workspace.root, workspace.actions.Clock, workspace.events, workspace.projection)
				return err
			}); err != nil {
				return err
			}
		}
	}
	repair, _ := cmd.Flags().GetBool("repair")
	message := fmt.Sprintf("doctor ok: %d events scanned, %d projects, %d tickets", len(events), len(projects), len(tickets))
	payload := map[string]any{
		"ok":             true,
		"events_scanned": len(events),
		"projects":       len(projects),
		"tickets":        len(tickets),
		"repair_ran":     repair,
		"repair_actions": append([]string{}, repairReport.Actions...),
		"repair_pending": repairReport.Pending,
		"config":         cfg,
		"issue_codes":    []string{},
		"issues": map[string]any{
			"project_issues": projectIssues,
			"ticket_issues":  ticketIssues,
		},
	}
	return writeCommandOutput(cmd, payload, message, message)
}

func runInspect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	actor := contracts.Actor(strings.TrimSpace(actorRaw))
	if actor != "" && !actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", actor)
	}
	view, err := workspace.queries.InspectTicket(ctx, args[0], actor)
	if err != nil {
		return err
	}
	md := fmt.Sprintf("## Inspect %s\n\n- Board Status: %s\n- Lease Active: %t\n- Completion: %s\n", view.Ticket.ID, view.BoardStatus, view.LeaseActive, view.EffectivePolicy.CompletionMode)
	pretty := fmt.Sprintf("inspect %s -> board=%s lease_active=%t completion=%s", view.Ticket.ID, view.BoardStatus, view.LeaseActive, view.EffectivePolicy.CompletionMode)
	return writeCommandOutput(cmd, view, md, pretty)
}

func runReindex(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	if _, err := config.Load(workspace.root); err != nil {
		return err
	}
	if err := workspace.withWriteLock(ctx, "reindex projection", func(ctx context.Context) error {
		return workspace.projection.Rebuild(ctx, "")
	}); err != nil {
		return err
	}
	message := "reindex complete"
	return writeCommandOutput(cmd, map[string]any{"ok": true}, message, message)
}

func runQueue(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	queue, err := workspace.queries.Queue(ctx, actor)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, queue, queueMarkdown(queue), queuePretty(queue))
}

func runReviewQueue(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	queue, err := workspace.queries.Queue(ctx, actor)
	if err != nil {
		return err
	}
	filtered := service.QueueView{
		Actor:       queue.Actor,
		GeneratedAt: queue.GeneratedAt,
		Categories:  map[service.QueueCategory][]service.QueueEntry{service.QueueNeedsReview: queue.Categories[service.QueueNeedsReview]},
	}
	return writeCommandOutput(cmd, filtered, queueMarkdown(filtered), queuePretty(filtered))
}

func runOwnerQueue(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	queue, err := workspace.queries.Queue(ctx, contracts.Actor("human:owner"))
	if err != nil {
		return err
	}
	filtered := service.QueueView{
		Actor:       queue.Actor,
		GeneratedAt: queue.GeneratedAt,
		Categories: map[service.QueueCategory][]service.QueueEntry{
			service.QueueAwaitingOwner: queue.Categories[service.QueueAwaitingOwner],
			service.QueueStaleClaims:   queue.Categories[service.QueueStaleClaims],
		},
	}
	return writeCommandOutput(cmd, filtered, queueMarkdown(filtered), queuePretty(filtered))
}

func runWho(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	tickets, err := workspace.queries.Who(ctx)
	if err != nil {
		return err
	}
	md := "## Claimed Tickets\n\n"
	now := defaultNow()
	if workspace.queries.Clock != nil {
		now = workspace.queries.Clock().UTC()
	}
	for _, ticket := range tickets {
		state := "active"
		if !ticket.Lease.Active(now) {
			state = "stale"
		}
		md += fmt.Sprintf("- %s `%s` `%s` until %s (%s)\n", ticket.ID, ticket.Lease.Actor, ticket.Lease.Kind, ticket.Lease.ExpiresAt.Format(timeRFC3339), state)
	}
	pretty := "claimed tickets:\n"
	for _, ticket := range tickets {
		state := "active"
		if !ticket.Lease.Active(now) {
			state = "STALE"
		}
		pretty += fmt.Sprintf("- %s %s %s until %s [%s]\n", ticket.ID, ticket.Lease.Actor, ticket.Lease.Kind, ticket.Lease.ExpiresAt.Format(timeRFC3339), state)
	}
	return writeCommandOutput(cmd, tickets, md, pretty)
}

func runSweep(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	tickets, err := workspace.actions.SweepExpiredClaims(ctx, actor, reason)
	if err != nil {
		return err
	}
	md := "## Sweep\n\n"
	for _, ticket := range tickets {
		md += fmt.Sprintf("- expired %s\n", ticket.ID)
	}
	pretty := fmt.Sprintf("expired %d lease(s)", len(tickets))
	return writeCommandOutput(cmd, tickets, md, pretty)
}

func runBoard(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	project, _ := cmd.Flags().GetString("project")
	assigneeRaw, _ := cmd.Flags().GetString("assignee")
	typeRaw, _ := cmd.Flags().GetString("type")
	boardVM, err := workspace.queries.Board(ctx, contracts.BoardQueryOptions{
		Project:  project,
		Assignee: contracts.Actor(strings.TrimSpace(assigneeRaw)),
		Type:     contracts.TicketType(strings.TrimSpace(typeRaw)),
	})
	if err != nil {
		return err
	}
	board := boardVM.Board
	markdown := "## Board\n\n"
	for _, status := range orderedBoardStatuses() {
		tickets := board.Columns[status]
		sort.Slice(tickets, func(i, j int) bool {
			if tickets[i].UpdatedAt.Equal(tickets[j].UpdatedAt) {
				return tickets[i].ID < tickets[j].ID
			}
			return tickets[i].UpdatedAt.Before(tickets[j].UpdatedAt)
		})
		markdown += fmt.Sprintf("### %s\n", status)
		if len(tickets) == 0 {
			markdown += "- (empty)\n"
			continue
		}
		for _, ticket := range tickets {
			markdown += fmt.Sprintf("- %s %s\n", ticket.ID, ticket.Title)
		}
	}
	pretty := render.BoardPretty(board)
	return writeCommandOutput(cmd, board, markdown, pretty)
}

func runBacklog(cmd *cobra.Command, _ []string) error {
	return runListByStatus(cmd, "Backlog", contracts.StatusBacklog)
}

func runBlocked(cmd *cobra.Command, _ []string) error {
	return runListByStatus(cmd, "Blocked", contracts.StatusBlocked)
}

func runListByStatus(cmd *cobra.Command, title string, status contracts.Status) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	boardVM, err := workspace.queries.Board(ctx, contracts.BoardQueryOptions{})
	if err != nil {
		return err
	}
	tickets := append([]contracts.TicketSnapshot{}, boardVM.Board.Columns[status]...)
	sort.Slice(tickets, func(i, j int) bool {
		if tickets[i].UpdatedAt.Equal(tickets[j].UpdatedAt) {
			return tickets[i].ID < tickets[j].ID
		}
		return tickets[i].UpdatedAt.Before(tickets[j].UpdatedAt)
	})
	markdown := fmt.Sprintf("## %s\n\n", title)
	for _, ticket := range tickets {
		markdown += fmt.Sprintf("- %s [%s] %s\n", ticket.ID, ticket.Priority, ticket.Title)
	}
	return writeCommandOutput(cmd, tickets, markdown, render.TicketsPretty(title, tickets))
}

func runNext(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	nextView, err := workspace.queries.Next(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	markdown := "## Next\n\n"
	pretty := fmt.Sprintf("next for %s:\n", nextView.Actor)
	for _, item := range nextView.Entries {
		markdown += fmt.Sprintf("- %s [%s/%s] %s (%s)\n", item.Entry.Ticket.ID, item.Entry.Ticket.Status, item.Entry.Ticket.Priority, item.Entry.Ticket.Title, item.Entry.Reason)
		pretty += fmt.Sprintf("- %s [%s] %s -> %s\n", item.Entry.Ticket.ID, item.Category, item.Entry.Ticket.Title, item.Entry.Reason)
	}
	return writeCommandOutput(cmd, nextView, markdown, pretty)
}

func runTemplatesList(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	templates, err := workspace.queries.ListTemplates(ctx)
	if err != nil {
		return err
	}
	md := "## Templates\n\n"
	pretty := "templates:\n"
	for _, template := range templates {
		md += fmt.Sprintf("- %s [%s] %s\n", template.Name, template.Type, template.Blueprint)
		pretty += fmt.Sprintf("- %s [%s] %s\n", template.Name, template.Type, template.Blueprint)
	}
	return writeCommandOutput(cmd, templates, md, pretty)
}

func runTemplatesView(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	template, err := workspace.queries.Template(ctx, args[0])
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("template %s [%s] %s", template.Name, template.Type, template.Blueprint)
	return writeCommandOutput(cmd, template, template.TemplateBody, pretty)
}

func runTUI(cmd *cobra.Command, _ []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	return tui.Run(rootDir, contracts.Actor(strings.TrimSpace(actorRaw)))
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	query, err := contracts.ParseSearchQuery(args[0])
	if err != nil {
		return err
	}
	tickets, err := workspace.queries.Search(ctx, query)
	if err != nil {
		return err
	}
	markdown := "## Search Results\n\n"
	for _, ticket := range tickets {
		markdown += fmt.Sprintf("- %s [%s] %s\n", ticket.ID, ticket.Status, ticket.Title)
	}
	return writeCommandOutput(cmd, tickets, markdown, render.TicketsPretty("Search", tickets))
}

func runRender(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticket, err := workspace.queries.Projection.QueryTicket(ctx, args[0])
	if err != nil {
		ticket, err = workspace.ticket.GetTicket(ctx, args[0])
		if err != nil {
			return err
		}
	}
	rawMD, err := mdstore.EncodeTicketMarkdown(ticket)
	if err != nil {
		return err
	}
	pretty := render.Markdown(rawMD)
	return writeCommandOutput(cmd, ticket, rawMD, pretty)
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

func orderedBoardStatuses() []contracts.Status {
	return []contracts.Status{
		contracts.StatusBacklog,
		contracts.StatusReady,
		contracts.StatusInProgress,
		contracts.StatusInReview,
		contracts.StatusBlocked,
		contracts.StatusDone,
	}
}

func queueMarkdown(queue service.QueueView) string {
	md := fmt.Sprintf("## Queue for %s\n\n", queue.Actor)
	for _, category := range orderedQueueCategories() {
		entries := queue.Categories[category]
		md += fmt.Sprintf("### %s\n", category)
		if len(entries) == 0 {
			md += "- (empty)\n"
			continue
		}
		for _, entry := range entries {
			md += fmt.Sprintf("- %s [%s] %s — %s\n", entry.Ticket.ID, entry.Ticket.Priority, entry.Ticket.Title, entry.Reason)
		}
	}
	return md
}

func queuePretty(queue service.QueueView) string {
	pretty := fmt.Sprintf("queue for %s:\n", queue.Actor)
	for _, category := range orderedQueueCategories() {
		entries := queue.Categories[category]
		pretty += fmt.Sprintf("%s:\n", category)
		if len(entries) == 0 {
			pretty += "  (empty)\n"
			continue
		}
		for _, entry := range entries {
			pretty += fmt.Sprintf("  - %s [%s/%s] %s — %s\n", entry.Ticket.ID, entry.Ticket.Status, entry.Ticket.Priority, entry.Ticket.Title, entry.Reason)
		}
	}
	return pretty
}

func orderedQueueCategories() []service.QueueCategory {
	return []service.QueueCategory{
		service.QueueReadyForMe,
		service.QueueClaimedByMe,
		service.QueueBlockedForMe,
		service.QueueNeedsReview,
		service.QueueAwaitingOwner,
		service.QueueStaleClaims,
		service.QueuePolicyViolations,
	}
}

func parseActors(raw string) ([]contracts.Actor, error) {
	labels := parseLabels(raw)
	actors := make([]contracts.Actor, 0, len(labels))
	for _, value := range labels {
		actor := contracts.Actor(value)
		if !actor.IsValid() {
			return nil, fmt.Errorf("invalid actor: %s", value)
		}
		actors = append(actors, actor)
	}
	return actors, nil
}

func ticketPolicyFromFlags(cmd *cobra.Command, policy contracts.TicketPolicy) (contracts.TicketPolicy, error) {
	if cmd.Flags().Changed("inherit") {
		value, _ := cmd.Flags().GetBool("inherit")
		policy.Inherit = value
	}
	if cmd.Flags().Changed("completion-mode") {
		value, _ := cmd.Flags().GetString("completion-mode")
		policy.CompletionMode = contracts.CompletionMode(strings.TrimSpace(value))
	}
	if cmd.Flags().Changed("allowed-workers") {
		value, _ := cmd.Flags().GetString("allowed-workers")
		actors, err := parseActors(value)
		if err != nil {
			return contracts.TicketPolicy{}, err
		}
		policy.AllowedWorkers = actors
	}
	if cmd.Flags().Changed("required-reviewer") {
		value, _ := cmd.Flags().GetString("required-reviewer")
		if strings.TrimSpace(value) != "" {
			policy.RequiredReviewer = contracts.Actor(strings.TrimSpace(value))
			if !policy.RequiredReviewer.IsValid() {
				return contracts.TicketPolicy{}, fmt.Errorf("invalid required reviewer: %s", value)
			}
		}
	}
	if cmd.Flags().Changed("owner-override") {
		value, _ := cmd.Flags().GetBool("owner-override")
		policy.OwnerOverride = value
	}
	return policy, policy.Validate()
}

func projectPolicyFromFlags(cmd *cobra.Command, defaults contracts.ProjectDefaults) (contracts.ProjectDefaults, error) {
	if cmd.Flags().Changed("completion-mode") {
		value, _ := cmd.Flags().GetString("completion-mode")
		defaults.CompletionMode = contracts.CompletionMode(strings.TrimSpace(value))
	}
	if cmd.Flags().Changed("lease-ttl") {
		value, _ := cmd.Flags().GetInt("lease-ttl")
		defaults.LeaseTTLMinutes = value
	}
	if cmd.Flags().Changed("allowed-workers") {
		value, _ := cmd.Flags().GetString("allowed-workers")
		actors, err := parseActors(value)
		if err != nil {
			return contracts.ProjectDefaults{}, err
		}
		defaults.AllowedWorkers = actors
	}
	if cmd.Flags().Changed("required-reviewer") {
		value, _ := cmd.Flags().GetString("required-reviewer")
		if strings.TrimSpace(value) != "" {
			defaults.RequiredReviewer = contracts.Actor(strings.TrimSpace(value))
			if !defaults.RequiredReviewer.IsValid() {
				return contracts.ProjectDefaults{}, fmt.Errorf("invalid required reviewer: %s", value)
			}
		}
	}
	return defaults, defaults.Validate()
}
