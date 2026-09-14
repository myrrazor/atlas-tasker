package cli

import (
	"os"

	"github.com/myrrazor/atlas-tasker/internal/uninstall"
	"github.com/spf13/cobra"
)

// tests inject a fake service runner; production uses HostCommandRunner.
var uninstallCommandOverride uninstall.CommandRunner

func newUninstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Atlas Tasker software only",
		Long: `Preview or apply a software-only uninstall.

Default is a digest-bound preview of exact Atlas-owned paths. Pass --yes or
--apply to execute. Workspaces, backups, registry pointers, and unrelated
client config are kept. Without a verifiable install receipt, uninstall
refuses to delete files. Source and go-install builds write a receipt on
successful init/setup for the running tracker only.

Examples:
  tracker uninstall
  tracker uninstall --json
  tracker uninstall --yes`,
		RunE: runUninstallCommand,
	}
	cmd.Flags().Bool("yes", false, "Apply the uninstall plan without further confirmation")
	cmd.Flags().Bool("apply", false, "Alias for --yes")
	cmd.Flags().Bool("json", false, "Print the plan or result as JSON")
	cmd.Flags().Bool("md", false, "Markdown output")
	return cmd
}

func runUninstallCommand(cmd *cobra.Command, _ []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	apply, _ := cmd.Flags().GetBool("apply")
	opts := uninstall.Options{
		Home:    os.Getenv("HOME"),
		Getenv:  os.Getenv,
		Command: uninstall.HostCommandRunner{},
	}
	if uninstallCommandOverride != nil {
		opts.Command = uninstallCommandOverride
	}
	if yes || apply {
		result, err := uninstall.Apply(cmd.Context(), opts, true)
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, uninstall.PrettyResult(result), uninstall.PrettyResult(result))
	}
	plan, err := uninstall.PlanUninstall(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, plan, uninstall.Pretty(plan), uninstall.Pretty(plan))
}
