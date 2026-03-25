package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
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
	root.AddCommand(newAutomationCommand())
	root.AddCommand(newNotifyCommand())
	root.AddCommand(newGitCommand())
	root.AddCommand(newViewsCommand())
	root.AddCommand(newWatchCommand())
	root.AddCommand(newUnwatchCommand())
	root.AddCommand(newBulkCommand())
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
			rootDir, err = service.CanonicalWorkspaceRoot(rootDir)
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

func newAutomationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "automation", Short: "Manage local automation rules"}
	list := &cobra.Command{Use: "list", Short: "List automation rules", RunE: runAutomationList}
	view := &cobra.Command{Use: "view <NAME>", Args: cobra.ExactArgs(1), Short: "Show one automation rule", RunE: runAutomationView}
	create := &cobra.Command{Use: "create <NAME>", Args: cobra.ExactArgs(1), Short: "Create an automation rule", RunE: runAutomationCreate}
	edit := &cobra.Command{Use: "edit <NAME>", Args: cobra.ExactArgs(1), Short: "Replace an automation rule", RunE: runAutomationEdit}
	remove := &cobra.Command{Use: "delete <NAME>", Args: cobra.ExactArgs(1), Short: "Delete an automation rule", RunE: runAutomationDelete}
	dryRun := &cobra.Command{Use: "dry-run <NAME>", Args: cobra.ExactArgs(1), Short: "Evaluate a rule without mutating", RunE: runAutomationDryRun}
	explain := &cobra.Command{Use: "explain <NAME>", Args: cobra.ExactArgs(1), Short: "Explain why a rule does or does not match", RunE: runAutomationExplain}
	for _, sub := range []*cobra.Command{list, view, create, edit, remove, dryRun, explain} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{create, edit} {
		sub.Flags().StringArray("on", nil, "Trigger event types")
		sub.Flags().String("project", "", "Only match this project")
		sub.Flags().String("status", "", "Only match this ticket status")
		sub.Flags().String("type", "", "Only match this ticket type")
		sub.Flags().String("assignee", "", "Only match this assignee")
		sub.Flags().String("reviewer", "", "Only match this reviewer")
		sub.Flags().String("review-state", "", "Only match this review state")
		sub.Flags().StringArray("label", nil, "Require a label")
		sub.Flags().StringArray("action", nil, "Action definition: comment:body, move:status, request_review, notify:message")
		sub.Flags().Bool("disabled", false, "Create the rule disabled")
	}
	for _, sub := range []*cobra.Command{dryRun, explain} {
		sub.Flags().String("ticket", "", "Ticket to evaluate against")
		sub.Flags().String("event-type", "", "Synthetic triggering event type")
		sub.Flags().String("actor", "human:owner", "Actor that emitted the triggering event")
	}
	cmd.AddCommand(list, view, create, edit, remove, dryRun, explain)
	return cmd
}

func newNotifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "notify", Short: "Debug and inspect notifier delivery"}
	send := &cobra.Command{Use: "send", Short: "Send a synthetic notification event", RunE: runNotifySend}
	send.Flags().String("event-type", "", "Event type to send")
	send.Flags().String("ticket", "", "Ticket id for the event")
	send.Flags().String("project", "", "Project key when no ticket id is provided")
	send.Flags().String("actor", "human:owner", "Actor for the synthetic event")
	send.Flags().String("reason", "", "Reason/message for the synthetic event")
	_ = send.MarkFlagRequired("event-type")
	addReadOutputFlags(send, &outputFlags{})

	logCmd := &cobra.Command{Use: "log", Short: "Read notification delivery attempts", RunE: runNotifyLog}
	logCmd.Flags().Int("limit", 20, "Maximum number of log entries to show")
	addReadOutputFlags(logCmd, &outputFlags{})

	dead := &cobra.Command{Use: "dead-letter", Short: "Read notification dead letters", RunE: runNotifyDeadLetter}
	dead.Flags().Int("limit", 20, "Maximum number of dead letters to show")
	addReadOutputFlags(dead, &outputFlags{})

	cmd.AddCommand(send, logCmd, dead)
	return cmd
}

func newGitCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "git", Short: "Inspect local git context for Atlas tickets"}
	status := &cobra.Command{Use: "status", Short: "Show local git repository status", RunE: runGitStatus}
	branchName := &cobra.Command{Use: "branch-name <ID>", Args: cobra.ExactArgs(1), Short: "Suggest a branch name for a ticket", RunE: runGitBranchName}
	refs := &cobra.Command{Use: "refs <ID>", Args: cobra.ExactArgs(1), Short: "Show local commits referencing a ticket", RunE: runGitRefs}
	commit := &cobra.Command{Use: "commit <ID>", Args: cobra.ExactArgs(1), Short: "Create a local git commit tied to a ticket", RunE: runGitCommit}
	commit.Flags().String("message", "", "Commit message body")
	_ = commit.MarkFlagRequired("message")
	for _, sub := range []*cobra.Command{status, branchName, refs, commit} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	cmd.AddCommand(status, branchName, refs, commit)
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
	cmd.Flags().String("view", "", "Saved board view to run")
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
	cmd.Flags().String("view", "", "Saved next view to run")
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
	cmd.Flags().String("view", "", "Saved queue view to run")
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
	cmd := &cobra.Command{Use: "search [QUERY]", Args: cobra.MaximumNArgs(1), Short: "Search tickets", RunE: runSearch}
	cmd.Flags().String("view", "", "Saved search view to run")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newViewsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "views", Short: "Manage saved views"}
	list := &cobra.Command{Use: "list", Short: "List saved views", RunE: runViewsList}
	view := &cobra.Command{Use: "view <NAME>", Args: cobra.ExactArgs(1), Short: "Show one saved view", RunE: runViewsView}
	save := &cobra.Command{Use: "save <NAME>", Args: cobra.ExactArgs(1), Short: "Create or update a saved view", RunE: runViewsSave}
	remove := &cobra.Command{Use: "delete <NAME>", Args: cobra.ExactArgs(1), Short: "Delete a saved view", RunE: runViewsDelete}
	run := &cobra.Command{Use: "run <NAME>", Args: cobra.ExactArgs(1), Short: "Run a saved view", RunE: runViewsRun}
	for _, sub := range []*cobra.Command{list, view, save, remove, run} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	save.Flags().String("kind", "", "View kind: board, search, queue, next")
	save.Flags().String("title", "", "Optional display title")
	save.Flags().String("query", "", "Search query for search views")
	save.Flags().String("project", "", "Project filter")
	save.Flags().String("assignee", "", "Assignee filter")
	save.Flags().String("type", "", "Ticket type filter")
	save.Flags().String("actor", "", "Default actor for queue/next views")
	save.Flags().StringArray("column", nil, "Board columns to include")
	save.Flags().StringArray("queue-category", nil, "Queue categories to include for queue/next views")
	_ = save.MarkFlagRequired("kind")
	run.Flags().String("actor", "", "Actor override for queue/next views")
	cmd.AddCommand(list, view, save, remove, run)
	return cmd
}

func newWatchCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "watch", Short: "Manage notification watchers"}
	list := &cobra.Command{Use: "list", Short: "List watcher rules", RunE: runWatchList}
	list.Flags().String("actor", "", "Only show watchers for this actor")
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)
	for _, spec := range []struct {
		use        string
		short      string
		targetKind contracts.SubscriptionTargetKind
		run        func(*cobra.Command, []string) error
	}{
		{use: "ticket <ID>", short: "Watch one ticket", targetKind: contracts.SubscriptionTargetTicket, run: runWatchTicket},
		{use: "project <KEY>", short: "Watch one project", targetKind: contracts.SubscriptionTargetProject, run: runWatchProject},
		{use: "view <NAME>", short: "Watch one saved view", targetKind: contracts.SubscriptionTargetSavedView, run: runWatchView},
	} {
		sub := &cobra.Command{Use: spec.use, Args: cobra.ExactArgs(1), Short: spec.short, RunE: spec.run}
		sub.Flags().String("actor", "", "Watcher actor")
		sub.Flags().StringArray("event", nil, "Only notify on these event types")
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newUnwatchCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "unwatch", Short: "Remove notification watchers"}
	for _, spec := range []struct {
		use   string
		short string
		run   func(*cobra.Command, []string) error
	}{
		{use: "ticket <ID>", short: "Unwatch one ticket", run: runUnwatchTicket},
		{use: "project <KEY>", short: "Unwatch one project", run: runUnwatchProject},
		{use: "view <NAME>", short: "Unwatch one saved view", run: runUnwatchView},
	} {
		sub := &cobra.Command{Use: spec.use, Args: cobra.ExactArgs(1), Short: spec.short, RunE: spec.run}
		sub.Flags().String("actor", "", "Watcher actor")
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newBulkCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "bulk", Short: "Run one mutation across many tickets"}
	move := &cobra.Command{Use: "move <STATUS>", Args: cobra.ExactArgs(1), Short: "Move many tickets", RunE: runBulkMove}
	assign := &cobra.Command{Use: "assign <ACTOR>", Args: cobra.ExactArgs(1), Short: "Assign many tickets", RunE: runBulkAssign}
	requestReview := &cobra.Command{Use: "request-review", Short: "Request review for many tickets", RunE: runBulkRequestReview}
	complete := &cobra.Command{Use: "complete", Short: "Complete many tickets", RunE: runBulkComplete}
	claim := &cobra.Command{Use: "claim", Short: "Claim many tickets", RunE: runBulkClaim}
	release := &cobra.Command{Use: "release", Short: "Release many tickets", RunE: runBulkRelease}
	for _, sub := range []*cobra.Command{move, assign, requestReview, complete, claim, release} {
		addBulkTargetFlags(sub)
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func addBulkTargetFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("ticket", nil, "Ticket ID to include; repeatable")
	cmd.Flags().String("view", "", "Saved view used to resolve ticket IDs")
	cmd.Flags().Bool("dry-run", false, "Preview the batch without mutating")
	cmd.Flags().Bool("yes", false, "Apply the batch without prompting")
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
	return executeArgsWithSurface(args, contracts.EventSurfaceCLI)
}

func executeArgsWithSurface(args []string, surface contracts.EventSurface) error {
	root := NewRootCommand()
	root.SetContext(service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: surface}))
	root.SetArgs(args)
	return root.Execute()
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func runTicketCreate(cmd *cobra.Command, _ []string) error {
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	ctx := commandContext(cmd)
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
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err = service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return err
	}
	repair, _ := cmd.Flags().GetBool("repair")
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: defaultNow}
	eventLog := &eventstore.Log{RootDir: root}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	events, err := eventLog.StreamEvents(ctx, "", 0)
	if err != nil {
		return err
	}
	projects, err := projectStore.ListProjects(ctx)
	if err != nil {
		return err
	}
	tickets, err := ticketStore.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return err
	}
	projectionPath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	projection, err := sqlitestore.Open(projectionPath, ticketStore, eventLog)
	repairActions := make([]string, 0)
	if err != nil {
		if !repair {
			return err
		}
		for _, candidate := range []string{projectionPath, projectionPath + "-wal", projectionPath + "-shm"} {
			if removeErr := os.Remove(candidate); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
		}
		projection, err = sqlitestore.Open(projectionPath, ticketStore, eventLog)
		if err != nil {
			return err
		}
		repairActions = append(repairActions, "reset corrupted projection")
	}
	defer func() { _ = projection.Close() }()
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
	if _, err := projection.QueryBoard(ctx, contracts.BoardQueryOptions{}); err != nil {
		if !repair {
			return err
		}
		if rebuildErr := service.WithWriteLock(ctx, service.FileLockManager{Root: root}, "doctor repair", func(ctx context.Context) error {
			var err error
			repairReport, err = service.RepairWorkspace(ctx, root, defaultNow, eventLog, projection)
			return err
		}); rebuildErr != nil {
			return rebuildErr
		}
	} else {
		if repair {
			if err := service.WithWriteLock(ctx, service.FileLockManager{Root: root}, "doctor repair", func(ctx context.Context) error {
				var err error
				repairReport, err = service.RepairWorkspace(ctx, root, defaultNow, eventLog, projection)
				return err
			}); err != nil {
				return err
			}
		}
	}
	message := fmt.Sprintf("doctor ok: %d events scanned, %d projects, %d tickets", len(events), len(projects), len(tickets))
	payload := map[string]any{
		"ok":             true,
		"events_scanned": len(events),
		"projects":       len(projects),
		"tickets":        len(tickets),
		"repair_ran":     repair,
		"repair_actions": append(append([]string{}, repairActions...), repairReport.Actions...),
		"repair_pending": repairReport.Pending,
		"config":         config.MaskTrackerConfig(cfg),
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
	viewName, _ := cmd.Flags().GetString("view")
	actorRaw, _ := cmd.Flags().GetString("actor")
	if strings.TrimSpace(viewName) != "" {
		if _, err := requireSavedViewKind(workspace, viewName, contracts.SavedViewKindQueue); err != nil {
			return err
		}
		result, err := workspace.queries.RunSavedView(ctx, viewName, contracts.Actor(strings.TrimSpace(actorRaw)))
		if err != nil {
			return err
		}
		if result.Queue == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s is not a queue view", viewName))
		}
		return writeCommandOutput(cmd, result, queueMarkdownSelected(*result.Queue, result.View.Queue.Categories, savedViewTitle(result.View, "Queue")), queuePrettySelected(*result.Queue, result.View.Queue.Categories, savedViewTitle(result.View, "Queue")))
	}
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
	ctx := commandContext(cmd)
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
	viewName, _ := cmd.Flags().GetString("view")
	project, _ := cmd.Flags().GetString("project")
	assigneeRaw, _ := cmd.Flags().GetString("assignee")
	typeRaw, _ := cmd.Flags().GetString("type")
	if strings.TrimSpace(viewName) != "" {
		if _, err := requireSavedViewKind(workspace, viewName, contracts.SavedViewKindBoard); err != nil {
			return err
		}
		result, err := workspace.queries.RunSavedView(ctx, viewName, "")
		if err != nil {
			return err
		}
		if result.Board == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s is not a board view", viewName))
		}
		board := result.Board.Board
		markdown := boardMarkdown(savedViewTitle(result.View, "Board"), board, result.View.Board.Columns)
		return writeCommandOutput(cmd, result, markdown, render.BoardPretty(board))
	}
	boardVM, err := workspace.queries.Board(ctx, contracts.BoardQueryOptions{
		Project:  project,
		Assignee: contracts.Actor(strings.TrimSpace(assigneeRaw)),
		Type:     contracts.TicketType(strings.TrimSpace(typeRaw)),
	})
	if err != nil {
		return err
	}
	board := boardVM.Board
	markdown := boardMarkdown("Board", board, nil)
	pretty := render.BoardPretty(board)
	return writeCommandOutput(cmd, board, markdown, pretty)
}

func boardMarkdown(title string, board contracts.BoardView, columns []contracts.Status) string {
	markdown := fmt.Sprintf("## %s\n\n", title)
	ordered := orderedBoardStatuses()
	if len(columns) > 0 {
		ordered = columns
	}
	for _, status := range ordered {
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
	return markdown
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
	viewName, _ := cmd.Flags().GetString("view")
	actorRaw, _ := cmd.Flags().GetString("actor")
	if strings.TrimSpace(viewName) != "" {
		if _, err := requireSavedViewKind(workspace, viewName, contracts.SavedViewKindNext); err != nil {
			return err
		}
		result, err := workspace.queries.RunSavedView(ctx, viewName, contracts.Actor(strings.TrimSpace(actorRaw)))
		if err != nil {
			return err
		}
		if result.Next == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s is not a next view", viewName))
		}
		markdown, pretty := nextOutput(*result.Next, savedViewTitle(result.View, "Next"))
		return writeCommandOutput(cmd, result, markdown, pretty)
	}
	nextView, err := workspace.queries.Next(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	markdown, pretty := nextOutput(nextView, "Next")
	return writeCommandOutput(cmd, nextView, markdown, pretty)
}

func nextOutput(nextView service.NextView, title string) (string, string) {
	markdown := fmt.Sprintf("## %s\n\n", title)
	pretty := fmt.Sprintf("%s for %s:\n", strings.ToLower(title), nextView.Actor)
	for _, item := range nextView.Entries {
		markdown += fmt.Sprintf("- %s [%s/%s] %s (%s)\n", item.Entry.Ticket.ID, item.Entry.Ticket.Status, item.Entry.Ticket.Priority, item.Entry.Ticket.Title, item.Entry.Reason)
		pretty += fmt.Sprintf("- %s [%s] %s -> %s\n", item.Entry.Ticket.ID, item.Category, item.Entry.Ticket.Title, item.Entry.Reason)
	}
	return markdown, pretty
}

func runAutomationList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	rules, err := workspace.actions.Automation.ListRules()
	if err != nil {
		return err
	}
	md := "## Automation Rules\n\n"
	pretty := "automation rules:\n"
	for _, rule := range rules {
		state := "enabled"
		if !rule.Enabled {
			state = "disabled"
		}
		md += fmt.Sprintf("- %s (%s)\n", rule.Name, state)
		pretty += fmt.Sprintf("- %s [%s]\n", rule.Name, state)
	}
	return writeCommandOutput(cmd, rules, md, pretty)
}

func runAutomationView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	rule, err := workspace.actions.Automation.LoadRule(args[0])
	if err != nil {
		return err
	}
	md := fmt.Sprintf("## %s\n\n- Enabled: %t\n- Triggers: %v\n- Actions: %d\n", rule.Name, rule.Enabled, rule.Trigger.EventTypes, len(rule.Actions))
	pretty := fmt.Sprintf("automation %s [%t]", rule.Name, rule.Enabled)
	return writeCommandOutput(cmd, rule, md, pretty)
}

func runAutomationCreate(cmd *cobra.Command, args []string) error {
	return saveAutomationRule(cmd, args[0], false)
}

func runAutomationEdit(cmd *cobra.Command, args []string) error {
	return saveAutomationRule(cmd, args[0], true)
}

func saveAutomationRule(cmd *cobra.Command, name string, mustExist bool) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	_, loadErr := workspace.actions.Automation.LoadRule(name)
	switch {
	case mustExist && loadErr != nil && (errors.Is(loadErr, os.ErrNotExist) || apperr.CodeOf(loadErr) == apperr.CodeNotFound):
		return apperr.New(apperr.CodeNotFound, fmt.Sprintf("automation %s not found", name))
	case !mustExist && loadErr == nil:
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("automation %s already exists", name))
	case loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) && apperr.CodeOf(loadErr) != apperr.CodeNotFound:
		return loadErr
	}
	rule, err := buildAutomationRuleFromFlags(cmd, name)
	if err != nil {
		return err
	}
	if err := workspace.withWriteLock(commandContext(cmd), "save automation rule", func(_ context.Context) error {
		return workspace.actions.Automation.SaveRule(rule)
	}); err != nil {
		return err
	}
	md := fmt.Sprintf("## %s\n\nsaved\n", rule.Name)
	pretty := fmt.Sprintf("saved automation %s", rule.Name)
	return writeCommandOutput(cmd, rule, md, pretty)
}

func runAutomationDelete(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	if err := workspace.withWriteLock(commandContext(cmd), "delete automation rule", func(_ context.Context) error {
		return workspace.actions.Automation.DeleteRule(args[0])
	}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "name": args[0]}
	return writeCommandOutput(cmd, result, fmt.Sprintf("## %s\n\ndeleted\n", args[0]), fmt.Sprintf("deleted automation %s", args[0]))
}

func runAutomationDryRun(cmd *cobra.Command, args []string) error {
	return evaluateAutomationRule(cmd, args[0], true)
}

func runAutomationExplain(cmd *cobra.Command, args []string) error {
	return evaluateAutomationRule(cmd, args[0], false)
}

func evaluateAutomationRule(cmd *cobra.Command, name string, dryRun bool) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	rule, err := workspace.actions.Automation.LoadRule(name)
	if err != nil {
		return err
	}
	event, ticketID, err := automationEventFromFlags(cmd)
	if err != nil {
		return err
	}
	var result service.AutomationResult
	if dryRun {
		result, err = workspace.actions.Automation.DryRun(ctx, workspace.queries, rule, event, ticketID)
	} else {
		result, err = workspace.actions.Automation.Explain(ctx, workspace.queries, rule, event, ticketID)
	}
	if err != nil {
		return err
	}
	md := fmt.Sprintf("## %s\n\n- Matched: %t\n", rule.Name, result.Matched)
	for _, reason := range result.Reasons {
		md += fmt.Sprintf("- %s\n", reason)
	}
	pretty := fmt.Sprintf("automation %s matched=%t", rule.Name, result.Matched)
	return writeCommandOutput(cmd, result, md, pretty)
}

func automationEventFromFlags(cmd *cobra.Command) (contracts.Event, string, error) {
	ticketID, _ := cmd.Flags().GetString("ticket")
	eventTypeRaw, _ := cmd.Flags().GetString("event-type")
	actorRaw, _ := cmd.Flags().GetString("actor")
	eventType := contracts.EventType(strings.TrimSpace(eventTypeRaw))
	if !eventType.IsValid() {
		return contracts.Event{}, "", fmt.Errorf("invalid event type: %s", eventTypeRaw)
	}
	actor := contracts.Actor(strings.TrimSpace(actorRaw))
	if !actor.IsValid() {
		return contracts.Event{}, "", fmt.Errorf("invalid actor: %s", actorRaw)
	}
	project := ""
	if strings.TrimSpace(ticketID) != "" {
		parts := strings.SplitN(ticketID, "-", 2)
		project = parts[0]
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     defaultNow(),
		Actor:         actor,
		Type:          eventType,
		Project:       project,
		TicketID:      strings.TrimSpace(ticketID),
		SchemaVersion: contracts.CurrentSchemaVersion,
		Metadata: contracts.EventMetadata{
			CorrelationID: "dry-run",
			MutationID:    "dry-run",
			Surface:       contracts.EventSurfaceCLI,
			RootActor:     actor,
		},
	}
	return event, strings.TrimSpace(ticketID), nil
}

func buildAutomationRuleFromFlags(cmd *cobra.Command, name string) (contracts.AutomationRule, error) {
	eventTypesRaw, _ := cmd.Flags().GetStringArray("on")
	actionDefs, _ := cmd.Flags().GetStringArray("action")
	project, _ := cmd.Flags().GetString("project")
	statusRaw, _ := cmd.Flags().GetString("status")
	typeRaw, _ := cmd.Flags().GetString("type")
	assigneeRaw, _ := cmd.Flags().GetString("assignee")
	reviewerRaw, _ := cmd.Flags().GetString("reviewer")
	reviewStateRaw, _ := cmd.Flags().GetString("review-state")
	labels, _ := cmd.Flags().GetStringArray("label")
	disabled, _ := cmd.Flags().GetBool("disabled")
	if len(eventTypesRaw) == 0 {
		return contracts.AutomationRule{}, fmt.Errorf("at least one --on trigger is required")
	}
	trigger := contracts.AutomationTrigger{EventTypes: make([]contracts.EventType, 0, len(eventTypesRaw))}
	for _, raw := range eventTypesRaw {
		eventType := contracts.EventType(strings.TrimSpace(raw))
		if !eventType.IsValid() {
			return contracts.AutomationRule{}, fmt.Errorf("invalid event type: %s", raw)
		}
		trigger.EventTypes = append(trigger.EventTypes, eventType)
	}
	actions := make([]contracts.AutomationAction, 0, len(actionDefs))
	for _, raw := range actionDefs {
		action, err := parseAutomationAction(raw)
		if err != nil {
			return contracts.AutomationRule{}, err
		}
		actions = append(actions, action)
	}
	rule := contracts.AutomationRule{
		Name:    name,
		Enabled: !disabled,
		Trigger: trigger,
		Conditions: contracts.AutomationCondition{
			Project:     strings.TrimSpace(project),
			Status:      contracts.Status(strings.TrimSpace(statusRaw)),
			Type:        contracts.TicketType(strings.TrimSpace(typeRaw)),
			Assignee:    contracts.Actor(strings.TrimSpace(assigneeRaw)),
			Reviewer:    contracts.Actor(strings.TrimSpace(reviewerRaw)),
			ReviewState: contracts.ReviewState(strings.TrimSpace(reviewStateRaw)),
			Labels:      labels,
		},
		Actions: actions,
	}
	return rule, rule.Validate()
}

func parseAutomationAction(raw string) (contracts.AutomationAction, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.EqualFold(raw, "request_review"):
		return contracts.AutomationAction{Kind: contracts.AutomationActionRequestReview}, nil
	case strings.HasPrefix(raw, "comment:"):
		return contracts.AutomationAction{Kind: contracts.AutomationActionComment, Body: strings.TrimSpace(strings.TrimPrefix(raw, "comment:"))}, nil
	case strings.HasPrefix(raw, "move:"):
		return contracts.AutomationAction{Kind: contracts.AutomationActionMove, Status: contracts.Status(strings.TrimSpace(strings.TrimPrefix(raw, "move:")))}, nil
	case strings.HasPrefix(raw, "notify:"):
		return contracts.AutomationAction{Kind: contracts.AutomationActionNotify, Message: strings.TrimSpace(strings.TrimPrefix(raw, "notify:"))}, nil
	default:
		return contracts.AutomationAction{}, fmt.Errorf("unsupported automation action: %s", raw)
	}
}

func buildSavedViewFromFlags(cmd *cobra.Command, name string) (contracts.SavedView, error) {
	kindRaw, _ := cmd.Flags().GetString("kind")
	title, _ := cmd.Flags().GetString("title")
	query, _ := cmd.Flags().GetString("query")
	project, _ := cmd.Flags().GetString("project")
	assigneeRaw, _ := cmd.Flags().GetString("assignee")
	typeRaw, _ := cmd.Flags().GetString("type")
	actorRaw, _ := cmd.Flags().GetString("actor")
	columnsRaw, _ := cmd.Flags().GetStringArray("column")
	categories, _ := cmd.Flags().GetStringArray("queue-category")
	view := contracts.SavedView{
		Name:     name,
		Title:    strings.TrimSpace(title),
		Kind:     contracts.SavedViewKind(strings.TrimSpace(kindRaw)),
		Query:    strings.TrimSpace(query),
		Project:  strings.TrimSpace(project),
		Assignee: normalizeActor(assigneeRaw),
		Type:     contracts.TicketType(strings.TrimSpace(typeRaw)),
		Actor:    normalizeActor(actorRaw),
		Queue:    contracts.SavedQueueConfig{Categories: categories},
	}
	for _, raw := range columnsRaw {
		column := contracts.Status(strings.TrimSpace(raw))
		if column == "" {
			continue
		}
		view.Board.Columns = append(view.Board.Columns, column)
	}
	return view, view.Validate()
}

func savedViewTitle(view contracts.SavedView, fallback string) string {
	if strings.TrimSpace(view.Title) != "" {
		return strings.TrimSpace(view.Title)
	}
	return fallback
}

func savedViewKindTitle(kind contracts.SavedViewKind) string {
	switch kind {
	case contracts.SavedViewKindBoard:
		return "Board"
	case contracts.SavedViewKindSearch:
		return "Search Results"
	case contracts.SavedViewKindQueue:
		return "Queue"
	case contracts.SavedViewKindNext:
		return "Next"
	default:
		return "Saved View"
	}
}

func requireSavedViewKind(workspace *workspace, name string, kind contracts.SavedViewKind) (contracts.SavedView, error) {
	view, err := workspace.queries.SavedView(name)
	if err != nil {
		return contracts.SavedView{}, err
	}
	if view.Kind != kind {
		return contracts.SavedView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s is not a %s view", name, kind))
	}
	return view, nil
}

func runNotifySend(cmd *cobra.Command, _ []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	cfg, err := config.Load(workspace.root)
	if err != nil {
		return err
	}
	notifier, err := service.BuildNotifier(workspace.root, cfg, cmd.ErrOrStderr(), service.SubscriptionResolver{
		Store:   service.SubscriptionStore{Root: workspace.root},
		Queries: workspace.queries,
	})
	if err != nil {
		return err
	}
	if notifier == nil {
		return apperr.New(apperr.CodeNotFound, "no notifier sinks are configured")
	}
	event, err := notifyEventFromFlags(cmd)
	if err != nil {
		return err
	}
	if err := notifier.Notify(ctx, event); err != nil {
		return err
	}
	pretty := fmt.Sprintf("notified %s via configured sinks", event.Type)
	md := fmt.Sprintf("## Notification Sent\n\n- Event: %s\n- Ticket: %s\n- Project: %s\n", event.Type, event.TicketID, event.Project)
	return writeCommandOutput(cmd, event, md, pretty)
}

func runNotifyLog(cmd *cobra.Command, _ []string) error {
	return runNotifyRecords(cmd, false)
}

func runNotifyDeadLetter(cmd *cobra.Command, _ []string) error {
	return runNotifyRecords(cmd, true)
}

func runNotifyRecords(cmd *cobra.Command, deadLetters bool) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	cfg, err := config.Load(workspace.root)
	if err != nil {
		return err
	}
	var records []service.NotificationDelivery
	if deadLetters {
		records, err = service.ReadDeadLetters(workspace.root, cfg)
	} else {
		records, err = service.ReadNotificationLog(workspace.root, cfg)
	}
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
	if limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
	}
	title := "Notification Log"
	if deadLetters {
		title = "Notification Dead Letters"
	}
	md := fmt.Sprintf("## %s\n\n", title)
	pretty := strings.ToLower(title) + ":\n"
	for _, record := range records {
		state := "ok"
		if !record.Delivered {
			state = "failed"
		}
		md += fmt.Sprintf("- %s %s %s (%s)\n", record.Timestamp.Format(timeRFC3339), record.Sink, record.Event.Type, state)
		pretty += fmt.Sprintf("- %s %s %s [%s]\n", record.Timestamp.Format(timeRFC3339), record.Sink, record.Event.Type, state)
	}
	return writeCommandOutput(cmd, records, md, pretty)
}

func notifyEventFromFlags(cmd *cobra.Command) (contracts.Event, error) {
	eventTypeRaw, _ := cmd.Flags().GetString("event-type")
	ticketID, _ := cmd.Flags().GetString("ticket")
	project, _ := cmd.Flags().GetString("project")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	eventType := contracts.EventType(strings.TrimSpace(eventTypeRaw))
	if !eventType.IsValid() {
		return contracts.Event{}, fmt.Errorf("invalid event type: %s", eventTypeRaw)
	}
	actor := normalizeActor(actorRaw)
	if !actor.IsValid() {
		return contracts.Event{}, fmt.Errorf("invalid actor: %s", actorRaw)
	}
	ticketID = strings.TrimSpace(ticketID)
	project = strings.TrimSpace(project)
	if ticketID != "" {
		parts := strings.SplitN(ticketID, "-", 2)
		project = parts[0]
	}
	if project == "" {
		return contracts.Event{}, fmt.Errorf("project is required when ticket is omitted")
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     defaultNow(),
		Actor:         actor,
		Reason:        strings.TrimSpace(reason),
		Type:          eventType,
		Project:       project,
		TicketID:      ticketID,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Metadata: contracts.EventMetadata{
			CorrelationID: "notify-debug",
			MutationID:    "notify-debug",
			Surface:       contracts.EventSurfaceCLI,
			RootActor:     actor,
		},
	}
	return event, event.Validate()
}

func runGitStatus(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	status, err := service.SCMService{Root: workspace.root}.RepoStatus(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := "git repo not detected"
	md := "## Git Status\n\n- Present: false\n"
	if status.Present {
		pretty = fmt.Sprintf("git %s dirty=%t", status.Branch, status.Dirty)
		md = fmt.Sprintf("## Git Status\n\n- Present: true\n- Root: %s\n- Branch: %s\n- Dirty: %t\n", status.Root, status.Branch, status.Dirty)
	}
	return writeCommandOutput(cmd, status, md, pretty)
}

func runGitBranchName(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticket, err := workspace.queries.TicketDetail(ctx, args[0])
	if err != nil {
		return err
	}
	branch := service.SCMService{Root: workspace.root}.SuggestedBranch(ticket.Ticket)
	payload := map[string]any{"ticket_id": args[0], "branch": branch}
	return writeCommandOutput(cmd, payload, fmt.Sprintf("## Branch Name\n\n- Ticket: %s\n- Branch: %s\n", args[0], branch), branch)
}

func runGitRefs(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	refs, err := service.SCMService{Root: workspace.root}.TicketRefs(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	md := "## Git Refs\n\n"
	pretty := "git refs:\n"
	for _, ref := range refs {
		md += fmt.Sprintf("- %s %s %s\n", ref.Hash, ref.AuthorDate.Format(timeRFC3339), ref.Subject)
		pretty += fmt.Sprintf("- %s %s\n", ref.Hash[:7], ref.Subject)
	}
	return writeCommandOutput(cmd, refs, md, pretty)
}

func runGitCommit(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticket, err := workspace.queries.TicketDetail(ctx, args[0])
	if err != nil {
		return err
	}
	message, _ := cmd.Flags().GetString("message")
	hash, err := service.SCMService{Root: workspace.root}.Commit(ctx, ticket.Ticket, message)
	if err != nil {
		return err
	}
	payload := map[string]any{"ticket_id": args[0], "commit": hash}
	return writeCommandOutput(cmd, payload, fmt.Sprintf("## Git Commit\n\n- Ticket: %s\n- Commit: %s\n", args[0], hash), fmt.Sprintf("committed %s", hash))
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
	viewName, _ := cmd.Flags().GetString("view")
	if strings.TrimSpace(viewName) != "" {
		if len(args) > 0 {
			return apperr.New(apperr.CodeInvalidInput, "search accepts either a query argument or --view, not both")
		}
		if _, err := requireSavedViewKind(workspace, viewName, contracts.SavedViewKindSearch); err != nil {
			return err
		}
		result, err := workspace.queries.RunSavedView(ctx, viewName, "")
		if err != nil {
			return err
		}
		title := savedViewTitle(result.View, "Search Results")
		markdown := fmt.Sprintf("## %s\n\n", title)
		for _, ticket := range result.Tickets {
			markdown += fmt.Sprintf("- %s [%s] %s\n", ticket.ID, ticket.Status, ticket.Title)
		}
		return writeCommandOutput(cmd, result, markdown, render.TicketsPretty(title, result.Tickets))
	}
	if len(args) != 1 {
		return apperr.New(apperr.CodeInvalidInput, "search requires a query or --view")
	}
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

func runViewsList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	views, err := workspace.queries.ListSavedViews()
	if err != nil {
		return err
	}
	md := "## Saved Views\n\n"
	pretty := "saved views:\n"
	for _, view := range views {
		title := strings.TrimSpace(view.Title)
		if title == "" {
			title = view.Name
		}
		md += fmt.Sprintf("- %s [%s] %s\n", view.Name, view.Kind, title)
		pretty += fmt.Sprintf("- %s [%s] %s\n", view.Name, view.Kind, title)
	}
	return writeCommandOutput(cmd, views, md, pretty)
}

func runViewsView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.SavedView(args[0])
	if err != nil {
		return err
	}
	md := fmt.Sprintf("## %s\n\n- Kind: %s\n", view.Name, view.Kind)
	if strings.TrimSpace(view.Title) != "" {
		md += fmt.Sprintf("- Title: %s\n", view.Title)
	}
	if strings.TrimSpace(view.Query) != "" {
		md += fmt.Sprintf("- Query: %s\n", view.Query)
	}
	pretty := fmt.Sprintf("saved view %s [%s]", view.Name, view.Kind)
	return writeCommandOutput(cmd, view, md, pretty)
}

func runViewsSave(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := buildSavedViewFromFlags(cmd, args[0])
	if err != nil {
		return err
	}
	if err := workspace.withWriteLock(commandContext(cmd), "save saved view", func(_ context.Context) error {
		return workspace.queries.Views.SaveView(view)
	}); err != nil {
		return err
	}
	md := fmt.Sprintf("## %s\n\nsaved\n", view.Name)
	pretty := fmt.Sprintf("saved view %s", view.Name)
	return writeCommandOutput(cmd, view, md, pretty)
}

func runViewsDelete(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	if err := workspace.withWriteLock(commandContext(cmd), "delete saved view", func(_ context.Context) error {
		return workspace.queries.Views.DeleteView(args[0])
	}); err != nil {
		return err
	}
	payload := map[string]any{"ok": true, "name": args[0]}
	return writeCommandOutput(cmd, payload, fmt.Sprintf("## %s\n\ndeleted\n", args[0]), fmt.Sprintf("deleted view %s", args[0]))
}

func runViewsRun(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	result, err := workspace.queries.RunSavedView(ctx, args[0], contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	title := savedViewTitle(result.View, savedViewKindTitle(result.View.Kind))
	switch result.View.Kind {
	case contracts.SavedViewKindBoard:
		if result.Board == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s returned no board payload", result.View.Name))
		}
		return writeCommandOutput(cmd, result, boardMarkdown(title, result.Board.Board, result.View.Board.Columns), render.BoardPretty(result.Board.Board))
	case contracts.SavedViewKindSearch:
		md := fmt.Sprintf("## %s\n\n", title)
		for _, ticket := range result.Tickets {
			md += fmt.Sprintf("- %s [%s] %s\n", ticket.ID, ticket.Status, ticket.Title)
		}
		return writeCommandOutput(cmd, result, md, render.TicketsPretty(title, result.Tickets))
	case contracts.SavedViewKindQueue:
		if result.Queue == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s returned no queue payload", result.View.Name))
		}
		return writeCommandOutput(cmd, result, queueMarkdownSelected(*result.Queue, result.View.Queue.Categories, title), queuePrettySelected(*result.Queue, result.View.Queue.Categories, title))
	case contracts.SavedViewKindNext:
		if result.Next == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("saved view %s returned no next payload", result.View.Name))
		}
		md, pretty := nextOutput(*result.Next, title)
		return writeCommandOutput(cmd, result, md, pretty)
	default:
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported saved view kind: %s", result.View.Kind))
	}
}

func runWatchList(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	var actor contracts.Actor
	if strings.TrimSpace(actorRaw) != "" {
		actor, err = workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
		if err != nil {
			return err
		}
	}
	subscriptions, err := workspace.queries.ListSubscriptions(ctx, actor)
	if err != nil {
		return err
	}
	md := "## Watchers\n\n"
	pretty := "watchers:\n"
	for _, item := range subscriptions {
		subscription := item.Subscription
		events := "all notify-worthy events"
		if len(subscription.EventTypes) > 0 {
			events = strings.Join(eventTypesToStrings(subscription.EventTypes), ", ")
		}
		status := "active"
		if !item.Active {
			status = "inactive"
		}
		md += fmt.Sprintf("- %s watches %s `%s` (%s, %s)", subscription.Actor, subscription.TargetKind, subscription.Target, events, status)
		pretty += fmt.Sprintf("- %s -> %s %s [%s] (%s)", subscription.Actor, subscription.TargetKind, subscription.Target, events, status)
		if item.InactiveReason != "" {
			md += fmt.Sprintf(" - %s", item.InactiveReason)
			pretty += fmt.Sprintf(" - %s", item.InactiveReason)
		}
		md += "\n"
		pretty += "\n"
	}
	return writeCommandOutput(cmd, subscriptions, md, pretty)
}

func runWatchTicket(cmd *cobra.Command, args []string) error {
	return saveSubscription(cmd, contracts.SubscriptionTargetTicket, args[0])
}

func runWatchProject(cmd *cobra.Command, args []string) error {
	return saveSubscription(cmd, contracts.SubscriptionTargetProject, args[0])
}

func runWatchView(cmd *cobra.Command, args []string) error {
	return saveSubscription(cmd, contracts.SubscriptionTargetSavedView, args[0])
}

func saveSubscription(cmd *cobra.Command, kind contracts.SubscriptionTargetKind, target string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	subscription, err := buildSubscriptionFromFlags(ctx, workspace, kind, target, cmd)
	if err != nil {
		return err
	}
	store := service.SubscriptionStore{Root: workspace.root}
	if err := workspace.withWriteLock(ctx, "save watcher", func(_ context.Context) error {
		return store.SaveSubscription(subscription)
	}); err != nil {
		return err
	}
	pretty := fmt.Sprintf("%s watches %s %s", subscription.Actor, subscription.TargetKind, subscription.Target)
	md := fmt.Sprintf("## Watcher Saved\n\n- Actor: %s\n- Target: %s %s\n", subscription.Actor, subscription.TargetKind, subscription.Target)
	return writeCommandOutput(cmd, subscription, md, pretty)
}

func runUnwatchTicket(cmd *cobra.Command, args []string) error {
	return deleteSubscription(cmd, contracts.SubscriptionTargetTicket, args[0])
}

func runUnwatchProject(cmd *cobra.Command, args []string) error {
	return deleteSubscription(cmd, contracts.SubscriptionTargetProject, args[0])
}

func runUnwatchView(cmd *cobra.Command, args []string) error {
	return deleteSubscription(cmd, contracts.SubscriptionTargetSavedView, args[0])
}

func deleteSubscription(cmd *cobra.Command, kind contracts.SubscriptionTargetKind, target string) error {
	ctx := commandContext(cmd)
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
	subscription := contracts.Subscription{
		Actor:      actor,
		TargetKind: kind,
		Target:     strings.TrimSpace(target),
	}
	store := service.SubscriptionStore{Root: workspace.root}
	if err := workspace.withWriteLock(ctx, "delete watcher", func(_ context.Context) error {
		return store.DeleteSubscription(subscription)
	}); err != nil {
		return err
	}
	payload := map[string]any{"ok": true, "actor": subscription.Actor, "target_kind": subscription.TargetKind, "target": subscription.Target}
	return writeCommandOutput(cmd, payload, fmt.Sprintf("## Watcher Removed\n\n- Actor: %s\n- Target: %s %s\n", subscription.Actor, subscription.TargetKind, subscription.Target), fmt.Sprintf("removed watcher %s %s %s", subscription.Actor, subscription.TargetKind, subscription.Target))
}

func runBulkMove(cmd *cobra.Command, args []string) error {
	status := contracts.Status(strings.TrimSpace(args[0]))
	if !status.IsValid() {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid status: %s", args[0]))
	}
	return runBulkOperation(cmd, service.BulkOperation{Kind: service.BulkOperationMove, Status: status})
}

func runBulkAssign(cmd *cobra.Command, args []string) error {
	assignee := normalizeActor(args[0])
	if args[0] != "" && !assignee.IsValid() {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid assignee actor: %s", args[0]))
	}
	return runBulkOperation(cmd, service.BulkOperation{Kind: service.BulkOperationAssign, Assignee: assignee})
}

func runBulkRequestReview(cmd *cobra.Command, _ []string) error {
	return runBulkOperation(cmd, service.BulkOperation{Kind: service.BulkOperationRequestReview})
}

func runBulkComplete(cmd *cobra.Command, _ []string) error {
	return runBulkOperation(cmd, service.BulkOperation{Kind: service.BulkOperationComplete})
}

func runBulkClaim(cmd *cobra.Command, _ []string) error {
	return runBulkOperation(cmd, service.BulkOperation{Kind: service.BulkOperationClaim})
}

func runBulkRelease(cmd *cobra.Command, _ []string) error {
	return runBulkOperation(cmd, service.BulkOperation{Kind: service.BulkOperationRelease})
}

func runBulkOperation(cmd *cobra.Command, base service.BulkOperation) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	confirm, _ := cmd.Flags().GetBool("yes")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	ticketIDs, source, err := bulkTargetTicketIDs(ctx, workspace, cmd)
	if err != nil {
		return err
	}
	base.Actor = actor
	base.Reason = reason
	base.TicketIDs = ticketIDs
	base.DryRun = dryRun
	base.Confirm = confirm
	result, err := workspace.actions.RunBulk(ctx, base)
	if err != nil {
		return err
	}
	md, pretty := bulkOutput(result, source)
	return writeCommandOutput(cmd, result, md, pretty)
}

func bulkTargetTicketIDs(ctx context.Context, workspace *workspace, cmd *cobra.Command) ([]string, string, error) {
	ticketIDs, _ := cmd.Flags().GetStringArray("ticket")
	viewName, _ := cmd.Flags().GetString("view")
	targets := append([]string{}, ticketIDs...)
	source := fmt.Sprintf("%d explicit tickets", len(ticketIDs))
	if strings.TrimSpace(viewName) != "" {
		result, err := workspace.queries.RunSavedView(ctx, strings.TrimSpace(viewName), "")
		if err != nil {
			return nil, "", err
		}
		targets = append(targets, savedViewTicketIDs(result)...)
		if len(ticketIDs) == 0 {
			source = fmt.Sprintf("saved view %s", viewName)
		} else {
			source = fmt.Sprintf("%d explicit tickets + saved view %s", len(ticketIDs), viewName)
		}
	}
	normalized := uniqueStrings(targets)
	if len(normalized) == 0 {
		return nil, "", apperr.New(apperr.CodeInvalidInput, "bulk operations require --ticket or --view with at least one ticket")
	}
	return normalized, source, nil
}

func savedViewTicketIDs(result service.SavedViewResult) []string {
	ticketIDs := make([]string, 0)
	switch result.View.Kind {
	case contracts.SavedViewKindSearch:
		for _, ticket := range result.Tickets {
			ticketIDs = append(ticketIDs, ticket.ID)
		}
	case contracts.SavedViewKindQueue:
		if result.Queue == nil {
			return ticketIDs
		}
		for _, entries := range result.Queue.Categories {
			for _, entry := range entries {
				ticketIDs = append(ticketIDs, entry.Ticket.ID)
			}
		}
	case contracts.SavedViewKindNext:
		if result.Next == nil {
			return ticketIDs
		}
		for _, entry := range result.Next.Entries {
			ticketIDs = append(ticketIDs, entry.Entry.Ticket.ID)
		}
	case contracts.SavedViewKindBoard:
		if result.Board == nil {
			return ticketIDs
		}
		for _, column := range result.Board.Board.Columns {
			for _, ticket := range column {
				ticketIDs = append(ticketIDs, ticket.ID)
			}
		}
	}
	return ticketIDs
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func bulkOutput(result service.BulkOperationResult, source string) (string, string) {
	md := fmt.Sprintf("## Bulk %s\n\n- Batch: `%s`\n- Source: %s\n- Dry Run: %t\n- Tickets: %d\n- Succeeded: %d\n- Failed: %d\n- Skipped: %d\n\n", result.Preview.Kind, result.BatchID, source, result.Preview.DryRun, result.Preview.TicketCount, result.Summary.Succeeded, result.Summary.Failed, result.Summary.Skipped)
	pretty := fmt.Sprintf("bulk %s batch=%s tickets=%d ok=%d failed=%d skipped=%d", result.Preview.Kind, result.BatchID, result.Preview.TicketCount, result.Summary.Succeeded, result.Summary.Failed, result.Summary.Skipped)
	for _, entry := range result.Results {
		status := "ok"
		if !entry.OK {
			status = "failed"
		} else if entry.DryRun {
			status = "preview"
		}
		md += fmt.Sprintf("- `%s` %s", entry.TicketID, status)
		pretty += fmt.Sprintf("\n- %s %s", entry.TicketID, status)
		if entry.Code != "" {
			md += fmt.Sprintf(" [%s]", entry.Code)
			pretty += fmt.Sprintf(" [%s]", entry.Code)
		}
		if strings.TrimSpace(entry.Reason) != "" {
			md += fmt.Sprintf(": %s", entry.Reason)
			pretty += fmt.Sprintf(": %s", entry.Reason)
		}
		if strings.TrimSpace(entry.Error) != "" {
			md += fmt.Sprintf(" (%s)", entry.Error)
			pretty += fmt.Sprintf(" (%s)", entry.Error)
		}
		md += "\n"
	}
	return md, pretty
}

func buildSubscriptionFromFlags(ctx context.Context, workspace *workspace, kind contracts.SubscriptionTargetKind, target string, cmd *cobra.Command) (contracts.Subscription, error) {
	actorRaw, _ := cmd.Flags().GetString("actor")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return contracts.Subscription{}, err
	}
	if kind == contracts.SubscriptionTargetSavedView {
		if _, err := workspace.queries.SavedView(strings.TrimSpace(target)); err != nil {
			return contracts.Subscription{}, err
		}
	}
	if kind == contracts.SubscriptionTargetTicket {
		if _, err := workspace.ticket.GetTicket(ctx, strings.TrimSpace(target)); err != nil {
			return contracts.Subscription{}, err
		}
	}
	if kind == contracts.SubscriptionTargetProject {
		if _, err := workspace.project.GetProject(ctx, strings.TrimSpace(target)); err != nil {
			return contracts.Subscription{}, err
		}
	}
	eventTypesRaw, _ := cmd.Flags().GetStringArray("event")
	subscription := contracts.Subscription{
		Actor:      actor,
		TargetKind: kind,
		Target:     strings.TrimSpace(target),
		EventTypes: make([]contracts.EventType, 0, len(eventTypesRaw)),
	}
	for _, raw := range eventTypesRaw {
		eventType := contracts.EventType(strings.TrimSpace(raw))
		if !eventType.IsValid() {
			return contracts.Subscription{}, fmt.Errorf("invalid event type: %s", raw)
		}
		subscription.EventTypes = append(subscription.EventTypes, eventType)
	}
	return subscription, subscription.Validate()
}

func eventTypesToStrings(values []contracts.EventType) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return items
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
	return queueMarkdownSelected(queue, nil, "Queue")
}

func queueMarkdownSelected(queue service.QueueView, categories []string, title string) string {
	md := fmt.Sprintf("## %s for %s\n\n", title, queue.Actor)
	for _, category := range orderedQueueCategoriesSelected(categories) {
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
	return queuePrettySelected(queue, nil, "Queue")
}

func queuePrettySelected(queue service.QueueView, categories []string, title string) string {
	pretty := fmt.Sprintf("%s for %s:\n", strings.ToLower(title), queue.Actor)
	for _, category := range orderedQueueCategoriesSelected(categories) {
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

func orderedQueueCategoriesSelected(categories []string) []service.QueueCategory {
	if len(categories) == 0 {
		return orderedQueueCategories()
	}
	allowed := make(map[service.QueueCategory]struct{}, len(categories))
	for _, raw := range categories {
		category := service.QueueCategory(strings.TrimSpace(raw))
		if category == "" {
			continue
		}
		allowed[category] = struct{}{}
	}
	selected := make([]service.QueueCategory, 0, len(allowed))
	for _, category := range orderedQueueCategories() {
		if _, ok := allowed[category]; ok {
			selected = append(selected, category)
		}
	}
	return selected
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
