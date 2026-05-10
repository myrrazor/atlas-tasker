package cli

import (
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
	"github.com/spf13/cobra"
)

type versionView struct {
	Kind      string `json:"kind"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build version metadata",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Current()
			view := versionView{
				Kind:      "tracker_version",
				Version:   info.Version,
				Commit:    info.Commit,
				BuildDate: info.BuildDate,
				GoVersion: info.GoVersion,
				Platform:  info.Platform,
			}
			pretty := fmt.Sprintf("tracker %s\ncommit: %s\nbuild date: %s\ngo: %s\nplatform: %s", view.Version, view.Commit, view.BuildDate, view.GoVersion, view.Platform)
			return writeCommandOutput(cmd, view, pretty, pretty)
		},
	}
	cmd.Flags().Bool("json", false, "Print version metadata as JSON")
	return cmd
}
