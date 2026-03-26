package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newArchiveCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "archive", Short: "Plan, apply, and restore retained Atlas artifacts"}
	plan := &cobra.Command{Use: "plan", Short: "Preview archive candidates", RunE: runArchivePlan}
	apply := &cobra.Command{Use: "apply", Short: "Archive eligible artifacts", RunE: runArchiveApply}
	list := &cobra.Command{Use: "list", Short: "List archive records", RunE: runArchiveList}
	restore := &cobra.Command{Use: "restore <ARCHIVE-ID>", Args: cobra.ExactArgs(1), Short: "Restore one archive record", RunE: runArchiveRestore}
	for _, sub := range []*cobra.Command{plan, apply, list, restore} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{plan, apply, list} {
		sub.Flags().String("target", string(contracts.RetentionTargetRuntime), "Archive target")
		sub.Flags().String("project", "", "Project key filter")
	}
	apply.Flags().Bool("yes", false, "Apply the archive plan without prompting")
	addMutationFlags(apply, &mutationFlags{Actor: "human:owner"})
	addMutationFlags(restore, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(plan, apply, list, restore)
	return cmd
}

func runArchivePlan(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	target, projectKey, err := archiveTargetFlags(cmd)
	if err != nil {
		return err
	}
	view, err := workspace.queries.ArchivePlan(commandContext(cmd), target, projectKey)
	if err != nil {
		return err
	}
	pretty := formatArchivePlan(view)
	data := map[string]any{"kind": "archive_plan", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runArchiveApply(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	target, projectKey, err := archiveTargetFlags(cmd)
	if err != nil {
		return err
	}
	yes, _ := cmd.Flags().GetBool("yes")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.ApplyArchive(commandContext(cmd), target, projectKey, yes, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatArchiveApply(view)
	data := map[string]any{"kind": "archive_apply_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runArchiveList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	target, projectKey, err := archiveTargetFlags(cmd)
	if err != nil {
		return err
	}
	_ = projectKey
	items, err := workspace.queries.ListArchiveRecords(commandContext(cmd), target)
	if err != nil {
		return err
	}
	pretty := formatArchiveList(items)
	data := map[string]any{"kind": "archive_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runArchiveRestore(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.RestoreArchive(commandContext(cmd), args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatArchiveRestore(view)
	data := map[string]any{"kind": "archive_restore_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func archiveTargetFlags(cmd *cobra.Command) (contracts.RetentionTarget, string, error) {
	targetRaw, _ := cmd.Flags().GetString("target")
	target := contracts.RetentionTarget(strings.TrimSpace(targetRaw))
	if !target.IsValid() {
		return "", "", fmt.Errorf("invalid archive target: %s", targetRaw)
	}
	projectKey, _ := cmd.Flags().GetString("project")
	return target, strings.TrimSpace(projectKey), nil
}

func formatArchivePlan(view service.ArchivePlanView) string {
	if len(view.Items) == 0 {
		return fmt.Sprintf("archive plan %s items=0", view.Target)
	}
	lines := []string{fmt.Sprintf("archive plan %s items=%d bytes=%d policy=%s", view.Target, len(view.Items), view.TotalBytes, view.Policy.PolicyID)}
	for _, item := range view.Items {
		line := fmt.Sprintf("- %s bytes=%d", item.Path, item.SizeBytes)
		if item.ProjectKey != "" {
			line += " project=" + item.ProjectKey
		}
		if item.Reason != "" {
			line += " reason=" + item.Reason
		}
		lines = append(lines, line)
	}
	if len(view.Warnings) > 0 {
		lines = append(lines, "warnings="+strings.Join(view.Warnings, ","))
	}
	return strings.Join(lines, "\n")
}

func formatArchiveApply(view service.ArchiveApplyResult) string {
	line := fmt.Sprintf("archive apply %s target=%s items=%d bytes=%d", view.Record.ArchiveID, view.Record.Target, view.Record.ItemCount, view.Record.TotalBytes)
	if len(view.Warnings) > 0 {
		line += " warnings=" + strings.Join(view.Warnings, ",")
	}
	return line
}

func formatArchiveList(items []contracts.ArchiveRecord) string {
	if len(items) == 0 {
		return "no archives"
	}
	lines := []string{"archives:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s target=%s state=%s items=%d", item.ArchiveID, item.Target, item.State, item.ItemCount))
	}
	return strings.Join(lines, "\n")
}

func formatArchiveRestore(view service.ArchiveRestoreResult) string {
	line := fmt.Sprintf("archive restore %s state=%s", view.Record.ArchiveID, view.Record.State)
	if len(view.Warnings) > 0 {
		line += " warnings=" + strings.Join(view.Warnings, ",")
	}
	return line
}
