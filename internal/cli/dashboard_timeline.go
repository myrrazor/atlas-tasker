package cli

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newDashboardCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Show delivery dashboard summary", RunE: runDashboard}
	cmd.Flags().String("collaborator", "", "Filter dashboard summary for one collaborator")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newTimelineCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "timeline <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Show the event timeline for one ticket", RunE: runTimeline}
	cmd.Flags().String("collaborator", "", "Filter timeline context for one collaborator")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func runDashboard(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	collaboratorID, _ := cmd.Flags().GetString("collaborator")
	view, err := workspace.queries.Dashboard(commandContext(cmd), collaboratorID)
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
	collaboratorID, _ := cmd.Flags().GetString("collaborator")
	view, err := workspace.queries.Timeline(commandContext(cmd), args[0], collaboratorID)
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
	if strings.TrimSpace(view.CollaboratorFilter) != "" {
		lines = append(lines, "collaborator_filter="+view.CollaboratorFilter)
	}
	if len(view.StaleWorktrees) > 0 {
		lines = append(lines, "stale_worktrees="+strings.Join(view.StaleWorktrees, ","))
	}
	if len(view.RetentionTargets) > 0 {
		lines = append(lines, "retention_pressure="+strings.Join(view.RetentionTargets, ","))
	}
	if len(view.CollaboratorWorkload) > 0 {
		lines = append(lines, "", "collaborator_workload:")
		for _, item := range view.CollaboratorWorkload {
			lines = append(lines, fmt.Sprintf("- %s approvals=%d inbox=%d mentions=%d handoffs=%d", item.CollaboratorID, item.Approvals, item.InboxItems, item.Mentions, item.Handoffs))
		}
	}
	if len(view.MentionQueue) > 0 {
		lines = append(lines, "", "mention_queue:")
		for _, item := range view.MentionQueue {
			lines = append(lines, fmt.Sprintf("- %s @%s %s", item.MentionUID, item.CollaboratorID, item.Summary))
		}
	}
	if len(view.ConflictQueue) > 0 {
		lines = append(lines, "", "conflict_queue:")
		for _, item := range view.ConflictQueue {
			lines = append(lines, fmt.Sprintf("- %s %s %s", item.ConflictID, item.EntityKind, item.ConflictType))
		}
	}
	if len(view.RemoteHealth) > 0 {
		lines = append(lines, "", "remote_health:")
		for _, item := range view.RemoteHealth {
			lines = append(lines, fmt.Sprintf("- %s state=%s publications=%d failed=%d", item.RemoteID, item.State, item.PublicationCount, item.FailedJobs))
		}
	}
	if len(view.ProviderMappingWarnings) > 0 {
		lines = append(lines, "", "provider_mapping_warnings="+strings.Join(view.ProviderMappingWarnings, ","))
	}
	return strings.Join(lines, "\n")
}

func formatTimeline(view service.TimelineView) string {
	if len(view.Entries) == 0 {
		return fmt.Sprintf("timeline %s entries=0", view.TicketID)
	}
	lines := []string{fmt.Sprintf("timeline %s entries=%d change_ready=%s", view.TicketID, len(view.Entries), view.ChangeReady)}
	if strings.TrimSpace(view.CollaboratorFilter) != "" {
		lines = append(lines, "collaborator_filter="+view.CollaboratorFilter)
	}
	for _, entry := range view.Entries {
		label := string(entry.Type)
		if strings.TrimSpace(entry.Kind) != "" {
			label = entry.Kind + ":" + label
		}
		lines = append(lines, fmt.Sprintf("- %s %s %s", entry.Timestamp.Format(timeRFC3339), label, entry.Summary))
	}
	return strings.Join(lines, "\n")
}
