package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newImportCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "import", Short: "Preview and apply Atlas imports"}
	preview := &cobra.Command{Use: "preview <PATH>", Args: cobra.ExactArgs(1), Short: "Preview an import source", RunE: runImportPreview}
	apply := &cobra.Command{Use: "apply <JOB-ID>", Args: cobra.ExactArgs(1), Short: "Apply a previewed import job", RunE: runImportApply}
	list := &cobra.Command{Use: "list", Short: "List import jobs", RunE: runImportList}
	view := &cobra.Command{Use: "view <JOB-ID>", Args: cobra.ExactArgs(1), Short: "View one import job", RunE: runImportView}
	for _, sub := range []*cobra.Command{preview, apply, list, view} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	addMutationFlags(preview, &mutationFlags{Actor: "human:owner"})
	addMutationFlags(apply, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(preview, apply, list, view)
	return cmd
}

func newExportCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "export", Short: "Create and verify Atlas export bundles"}
	create := &cobra.Command{Use: "create", Short: "Create an export bundle", RunE: runExportCreate}
	list := &cobra.Command{Use: "list", Short: "List export bundles", RunE: runExportList}
	view := &cobra.Command{Use: "view <BUNDLE-ID>", Args: cobra.ExactArgs(1), Short: "View one export bundle", RunE: runExportView}
	verify := &cobra.Command{Use: "verify <PATH|BUNDLE-ID>", Args: cobra.ExactArgs(1), Short: "Verify an export bundle", RunE: runExportVerify}
	for _, sub := range []*cobra.Command{create, list, view, verify} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	create.Flags().String("scope", "workspace", "Export scope")
	addMutationFlags(create, &mutationFlags{Actor: "human:owner"})
	addMutationFlags(verify, &mutationFlags{Actor: "human:owner"})
	cmd.AddCommand(create, list, view, verify)
	return cmd
}

func runImportPreview(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.PreviewImport(ctx, args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatImportJobDetail(view)
	data := map[string]any{"kind": "import_preview", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runImportApply(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.ApplyImport(ctx, args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatImportJobDetail(view)
	data := map[string]any{"kind": "import_apply_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runImportList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.ListImportJobs(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatImportJobList(items)
	data := map[string]any{"kind": "import_job_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runImportView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.ImportJobDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatImportJobDetail(view)
	data := map[string]any{"kind": "import_job_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runExportCreate(cmd *cobra.Command, _ []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	scope, _ := cmd.Flags().GetString("scope")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.CreateExportBundle(ctx, scope, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatExportBundleDetail(view)
	data := map[string]any{"kind": "export_bundle_create_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runExportList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.ListExportBundles(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatExportBundleList(items)
	data := map[string]any{"kind": "export_bundle_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runExportView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.ExportBundleDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatExportBundleDetail(view)
	data := map[string]any{"kind": "export_bundle_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runExportVerify(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.VerifyExportBundle(ctx, args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatExportVerify(view)
	data := map[string]any{"kind": "export_verify_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatImportJobList(items []contracts.ImportJob) string {
	if len(items) == 0 {
		return "no import jobs"
	}
	lines := []string{"import jobs:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s type=%s status=%s", item.JobID, item.SourceType, item.Status))
	}
	return strings.Join(lines, "\n")
}

func formatImportJobDetail(view service.ImportJobDetailView) string {
	lines := []string{
		fmt.Sprintf("import job %s", view.Job.JobID),
		fmt.Sprintf("status=%s source=%s", view.Job.Status, view.Plan.SourceType),
		fmt.Sprintf("path=%s", view.Plan.SourcePath),
		fmt.Sprintf("items=%d", len(view.Plan.Items)),
	}
	if len(view.Plan.Conflicts) > 0 {
		lines = append(lines, "conflicts="+strings.Join(view.Plan.Conflicts, ","))
	}
	if len(view.Plan.Errors) > 0 {
		lines = append(lines, "errors="+strings.Join(view.Plan.Errors, ","))
	}
	return strings.Join(lines, "\n")
}

func formatExportBundleList(items []contracts.ExportBundle) string {
	if len(items) == 0 {
		return "no export bundles"
	}
	lines := []string{"export bundles:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s status=%s format=%s", item.BundleID, item.Status, item.Format))
	}
	return strings.Join(lines, "\n")
}

func formatExportBundleDetail(view service.ExportBundleDetailView) string {
	return fmt.Sprintf("export bundle %s status=%s files=%d artifact=%s", view.Bundle.BundleID, view.Bundle.Status, view.FileCount, filepath.Base(view.Bundle.ArtifactPath))
}

func formatExportVerify(view service.ExportVerifyView) string {
	line := fmt.Sprintf("export verify %s verified=%t", filepath.Base(view.Path), view.Verified)
	if len(view.Errors) > 0 {
		line += " errors=" + strings.Join(view.Errors, ",")
	}
	return line
}
