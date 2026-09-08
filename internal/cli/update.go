package cli

import (
	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
	"github.com/myrrazor/atlas-tasker/internal/update"
	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and install the latest tracker release",
		Long: `Check GitHub releases for a newer tracker binary, verify checksums.txt
(and attestations by default), then atomically replace this executable.

Examples:
  tracker update --check
  tracker update --dry-run
  tracker update --yes
  tracker update --version v1.10.0 --yes --skip-attestations`,
		RunE: runUpdateCommand,
	}
	cmd.Flags().Bool("check", false, "Only report whether an update is available")
	cmd.Flags().Bool("dry-run", false, "Resolve and report the update plan without replacing the binary")
	cmd.Flags().Bool("yes", false, "Replace the current binary without further confirmation")
	cmd.Flags().Bool("force", false, "Allow reinstalling the same version or installing an older version")
	cmd.Flags().String("version", "", "Install a specific release tag instead of latest")
	cmd.Flags().Bool("skip-attestations", false, "Skip gh attestation verify (not recommended)")
	cmd.Flags().Bool("json", false, "Print the update result as JSON")
	return cmd
}

func runUpdateCommand(cmd *cobra.Command, _ []string) error {
	checkOnly, _ := cmd.Flags().GetBool("check")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	version, _ := cmd.Flags().GetString("version")
	skipAttestations, _ := cmd.Flags().GetBool("skip-attestations")

	result, err := update.Run(cmd.Context(), update.Options{
		CurrentVersion:   buildinfo.Version,
		TargetVersion:    version,
		CheckOnly:        checkOnly,
		DryRun:           dryRun,
		Yes:              yes,
		Force:            force,
		SkipAttestations: skipAttestations,
	})
	if err != nil {
		return err
	}
	pretty := update.FormatPretty(result)
	return writeCommandOutput(cmd, result, pretty, pretty)
}
