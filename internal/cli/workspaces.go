package cli

import (
	"fmt"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/spf13/cobra"
)

func newWorkspacesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "Machine workspace registry helpers",
	}
	grant := &cobra.Command{
		Use:   "grant <absolute-directory>",
		Short: "Issue a one-time path grant for Atlas Home",
		Long: `Mint a short-lived, single-use grant for an existing directory.

Home cannot invent filesystem paths. Run this in a terminal, then paste the
grant id into Atlas Home. Purpose must be init, register, or repair.

The directory must already exist. Atlas will not create it.`,
		Args: cobra.ExactArgs(1),
		RunE: runWorkspacesGrant,
	}
	grant.Flags().String("purpose", app.PathGrantInit, "init, register, or repair")
	addReadOutputFlags(grant, &outputFlags{})
	cmd.AddCommand(grant)
	return cmd
}

func runWorkspacesGrant(cmd *cobra.Command, args []string) error {
	purpose, _ := cmd.Flags().GetString("purpose")
	dir := args[0]
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		dir = abs
	}
	a, err := openApp()
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()
	grant, err := a.GrantPath(commandContext(cmd), dir, purpose)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("grant %s purpose=%s\n%s", grant.ID, grant.Purpose, grant.Path)
	md := fmt.Sprintf("# Path grant\n\n- ID: `%s`\n- Purpose: %s\n- Path: `%s`\n", grant.ID, grant.Purpose, grant.Path)
	if grant.ID == "" {
		return apperr.New(apperr.CodeConflict, "grant was not created")
	}
	return writeCommandOutput(cmd, grant, md, pretty)
}
