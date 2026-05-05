package cli

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newDashboardCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Show delivery dashboard summary", RunE: runDashboard}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newTimelineCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "timeline <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Show the event timeline for one ticket", RunE: runTimeline}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func runDashboard(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.Dashboard(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatDashboard(view)
	data := map[string]any{"kind": "dashboard_summary", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runTimeline(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.Timeline(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatTimeline(view)
	data := map[string]any{"kind": "timeline_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatDashboard(view service.DashboardSummaryView) string {
	lines := []string{
		fmt.Sprintf("dashboard active_runs=%d", view.ActiveRuns),
		fmt.Sprintf("awaiting_review=%d", view.AwaitingReview.Count),
		fmt.Sprintf("awaiting_owner=%d", view.AwaitingOwner.Count),
		fmt.Sprintf("merge_ready=%d", view.MergeReady.Count),
		fmt.Sprintf("blocked_by_checks=%d", view.BlockedByChecks.Count),
	}
	if len(view.StaleWorktrees) > 0 {
		lines = append(lines, "stale_worktrees="+strings.Join(view.StaleWorktrees, ","))
	}
	if len(view.RetentionTargets) > 0 {
		lines = append(lines, "retention_pressure="+strings.Join(view.RetentionTargets, ","))
	}
	return strings.Join(lines, "\n")
}

func formatTimeline(view service.TimelineView) string {
	if len(view.Entries) == 0 {
		return fmt.Sprintf("timeline %s entries=0", view.TicketID)
	}
	lines := []string{fmt.Sprintf("timeline %s entries=%d change_ready=%s", view.TicketID, len(view.Entries), view.ChangeReady)}
	for _, entry := range view.Entries {
		lines = append(lines, fmt.Sprintf("- %s %s %s", entry.Timestamp.Format(timeRFC3339), entry.Type, entry.Summary))
	}
	return strings.Join(lines, "\n")
}
