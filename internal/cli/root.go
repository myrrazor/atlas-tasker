package cli

import (
	"fmt"
	"strings"

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
		RunE:  notImplemented("tracker init", "PR-007"),
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
		RunE:  notImplemented("tracker reindex", "PR-004/PR-009"),
	}
	return cmd
}

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Read or update tracker config"}
	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Get config values",
		RunE:  notImplemented("tracker config get", "PR-006"),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set",
		Short: "Set config values",
		RunE:  notImplemented("tracker config set", "PR-006"),
	})
	return cmd
}

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Project management commands"}
	cmd.AddCommand(&cobra.Command{
		Use:   "create <KEY> <NAME>",
		Args:  cobra.ExactArgs(2),
		Short: "Create a project",
		RunE:  notImplemented("tracker project create", "PR-007"),
	})
	list := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE:  notImplemented("tracker project list", "PR-007"),
	}
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)
	view := &cobra.Command{
		Use:   "view <KEY>",
		Args:  cobra.ExactArgs(1),
		Short: "View project details",
		RunE:  notImplemented("tracker project view", "PR-007"),
	}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)
	return cmd
}

func newTicketCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "ticket", Short: "Ticket CRUD and workflow commands"}

	create := &cobra.Command{Use: "create", Short: "Create a ticket", RunE: notImplemented("tracker ticket create", "PR-007")}
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

	view := &cobra.Command{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View ticket", RunE: notImplemented("tracker ticket view", "PR-007")}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)

	edit := &cobra.Command{Use: "edit <ID>", Args: cobra.ExactArgs(1), Short: "Edit ticket fields", RunE: notImplemented("tracker ticket edit", "PR-007")}
	addMutationFlags(edit, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(edit)

	deleteCmd := &cobra.Command{Use: "delete <ID>", Args: cobra.ExactArgs(1), Short: "Soft-delete ticket", RunE: notImplemented("tracker ticket delete", "PR-007")}
	addMutationFlags(deleteCmd, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(deleteCmd)

	list := &cobra.Command{Use: "list", Short: "List tickets", RunE: notImplemented("tracker ticket list", "PR-007")}
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)

	move := &cobra.Command{Use: "move <ID> <STATUS>", Args: cobra.ExactArgs(2), Short: "Move ticket status", RunE: notImplemented("tracker ticket move", "PR-006/PR-007")}
	addMutationFlags(move, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(move)

	assign := &cobra.Command{Use: "assign <ID> <ACTOR>", Args: cobra.ExactArgs(2), Short: "Assign ticket actor", RunE: notImplemented("tracker ticket assign", "PR-007")}
	addMutationFlags(assign, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(assign)

	priority := &cobra.Command{Use: "priority <ID> <PRIORITY>", Args: cobra.ExactArgs(2), Short: "Set ticket priority", RunE: notImplemented("tracker ticket priority", "PR-007")}
	addMutationFlags(priority, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(priority)

	label := &cobra.Command{Use: "label", Short: "Manage ticket labels"}
	labelAdd := &cobra.Command{Use: "add <ID> <LABEL>", Args: cobra.ExactArgs(2), Short: "Add label", RunE: notImplemented("tracker ticket label add", "PR-007")}
	addMutationFlags(labelAdd, &mutationFlags{Actor: "human:owner"})
	label.AddCommand(labelAdd)
	labelRemove := &cobra.Command{Use: "remove <ID> <LABEL>", Args: cobra.ExactArgs(2), Short: "Remove label", RunE: notImplemented("tracker ticket label remove", "PR-007")}
	addMutationFlags(labelRemove, &mutationFlags{Actor: "human:owner"})
	label.AddCommand(labelRemove)
	cmd.AddCommand(label)

	link := &cobra.Command{Use: "link <ID>", Args: cobra.ExactArgs(1), Short: "Create ticket relationship", RunE: notImplemented("tracker ticket link", "PR-006")}
	link.Flags().String("blocks", "", "Link as blocks relationship")
	link.Flags().String("blocked-by", "", "Link as blocked_by relationship")
	link.Flags().String("parent", "", "Link as parent relationship")
	addMutationFlags(link, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(link)

	unlink := &cobra.Command{Use: "unlink <ID> <OTHER_ID>", Args: cobra.ExactArgs(2), Short: "Remove relationship", RunE: notImplemented("tracker ticket unlink", "PR-006")}
	addMutationFlags(unlink, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(unlink)

	comment := &cobra.Command{Use: "comment <ID>", Args: cobra.ExactArgs(1), Short: "Add comment to ticket", RunE: notImplemented("tracker ticket comment", "PR-007")}
	comment.Flags().String("body", "", "Comment body")
	addMutationFlags(comment, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(comment)

	history := &cobra.Command{Use: "history <ID>", Args: cobra.ExactArgs(1), Short: "Show ticket history", RunE: notImplemented("tracker ticket history", "PR-007")}
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
	return func(cmd *cobra.Command, _ []string) error {
		return fmt.Errorf("%s is not implemented yet (%s)", commandName, milestone)
	}
}

func executeArgs(args []string) error {
	root := NewRootCommand()
	root.SetArgs(args)
	return root.Execute()
}

func normalizeCommandName(input string) string {
	return strings.TrimSpace(strings.TrimPrefix(input, "/"))
}
