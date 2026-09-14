package mcp

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func extraWorkspaceSpecs() []ToolSpec {
	readProfiles := []ToolProfile{ProfileRead, ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	workflowProfiles := []ToolProfile{ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	adminProfiles := []ToolProfile{ProfileAdmin}

	attention := readTool("atlas.attention", "Read attention items for the bound workspace, or across registered workspaces when no workspace is bound.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"workspace_id": stringProp("Optional workspace ID.")})), "QueryService.Inbox/Approvals", attentionTool)
	attention.Scope = ScopeOptional

	specs := []ToolSpec{
		attention,
		readTool("atlas.activity", "Read recent workspace activity.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"project": stringProp("Optional project key.")})), "QueryService.RecentEvents", activityTool),
		readTool("atlas.project.list", "List projects in the bound workspace.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), nil)), "ProjectStore.ListProjects", projectListTool),
		writeTool("atlas.project.update", ClassWorkflow, workflowProfiles, false, "Update a project name.", objectSchema([]string{"key", "name", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"key": stringProp("Project key."), "name": stringProp("Project name.")})), "ActionService.UpdateProject", "key", projectUpdateTool),
		readTool("atlas.views.list", "List saved views.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), nil)), "QueryService.ListSavedViews", viewsListTool),
		readTool("atlas.views.get", "Read one saved view.", readProfiles, objectSchema([]string{"name"}, map[string]any{"name": stringProp("Saved view name.")}), "QueryService.SavedView", viewsGetTool),
		readTool("atlas.views.run", "Run a saved view.", readProfiles, objectSchema([]string{"name"}, map[string]any{"name": stringProp("Saved view name."), "actor": stringProp("Optional actor override.")}), "QueryService.RunSavedView", viewsRunTool),
		writeTool("atlas.views.save", ClassWorkflow, workflowProfiles, false, "Create or replace a saved view.", objectSchema([]string{"name", "kind", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"name": stringProp("Saved view name."), "kind": stringProp("Kind: board, search, queue, or next."), "query": stringProp("Search query."), "project": stringProp("Optional project key."), "title": stringProp("Optional title.")})), "ViewStore.SaveView", "name", viewsSaveTool),
		writeTool("atlas.views.delete", ClassWorkflow, workflowProfiles, false, "Delete a saved view.", objectSchema([]string{"name", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"name": stringProp("Saved view name.")})), "ViewStore.DeleteView", "name", viewsDeleteTool),
		writeTool("atlas.ticket.bulk", ClassWorkflow, workflowProfiles, false, "Preview or apply a bulk ticket operation.", objectSchema([]string{"kind", "ticket_ids", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"kind": stringProp("Bulk kind: move, assign, request_review, complete, claim, or release."), "ticket_ids": stringArrayProp("Ticket IDs."), "status": stringProp("Target status for move."), "assignee": stringProp("Assignee for assign."), "dry_run": boolProp("Preview without writing."), "confirm": boolProp("Required to apply."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.RunBulk", "kind", ticketBulkTool),
		readTool("atlas.backup.list", "List backup snapshots without target URLs or credentials.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), nil)), "ActionService.ListBackups", backupListTool),
		readTool("atlas.backup.history", "Read local checkpoint identifiers and automatic backup health.", readProfiles, objectSchema(nil, map[string]any{}), "QueryService.AutoBackupStatus", backupHistoryTool),
		readTool("atlas.backup.verify", "Verify a backup snapshot. Push is not treated as remote verification.", readProfiles, objectSchema([]string{"ref"}, map[string]any{"ref": stringProp("Backup ID or snapshot ref.")}), "ActionService.VerifyBackupSnapshot", backupVerifyTool),
		readTool("atlas.backup.targets", "List backup targets with redacted URLs.", readProfiles, objectSchema(nil, map[string]any{}), "ActionService.ListBackupTargets", backupTargetsTool),
		writeTool("atlas.backup.run", ClassWorkflow, workflowProfiles, false, "Run a local checkpoint now.", objectSchema([]string{"actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"force": boolProp("Bypass coalescing delay.")})), "ActionService.BackupTick", "workspace", backupRunTool),
		highImpactTool("atlas.backup.configure", adminProfiles, "Add, edit, enable, or disable a backup target. Remote URLs stay high-impact.", objectSchema([]string{"action", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("action"), map[string]any{"action": stringProp("enable, disable, add, edit, or remove."), "target_id": stringProp("Target ID."), "url": stringProp("Remote URL for add/edit."), "allow_local_file": boolProp("Required for file:// targets."), "acknowledge_boundary": boolProp("Required when adding a remote."), "attest_private": boolProp("Required when adding a remote.")})), "ActionService.AddBackupTarget", "action", backupConfigureTool),
		writeTool("atlas.restore.plan", ClassWorkflow, workflowProfiles, false, "Create a restore plan and persist its ID and digest.", objectSchema([]string{"ref", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ref": stringProp("Backup ID.")})), "ActionService.CreateRestorePlan", "ref", restorePlanTool),
		highImpactTool("atlas.restore.apply", adminProfiles, "Apply a previously created restore plan by ID and digest.", objectSchema([]string{"plan_id", "digest", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Restore plan ID."), map[string]any{"plan_id": stringProp("Restore plan ID."), "digest": stringProp("Digest returned by atlas.restore.plan.")})), "ActionService.ApplyRestorePlan", "plan_id", restoreApplyTool),
		readTool("atlas.settings.get", "Read machine or workspace settings without filesystem secrets.", readProfiles, objectSchema(nil, map[string]any{"workspace_id": stringProp("Optional workspace ID for workspace settings.")}), "Machine.Settings", settingsGetTool),
		writeTool("atlas.settings.update", ClassWorkflow, workflowProfiles, false, "Update safe settings. Discovery-root widening is rejected on this tool.", objectSchema([]string{"actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"auto_register": boolProp("Auto-register the current workspace."), "local_checkpoints": boolProp("Enable local checkpoints."), "default_project": boolProp("Create a default project on init."), "open_home": boolProp("Open Home in a browser."), "show_hidden": boolProp("Show hidden workspaces.")})), "Machine.UpdateSettings", "settings", settingsUpdateTool),
	}
	for i := range specs {
		switch specs[i].Name {
		case "atlas.settings.get", "atlas.attention":
			specs[i].Scope = ScopeOptional
		case "atlas.settings.update":
			specs[i].Scope = ScopeMachine
		}
	}
	return specs
}

func attentionTool(tc ToolContext, args map[string]any) (any, error) {
	limit := intArg(args, "limit", tc.Server.Options.MaxItems)
	if machine, err := requireMachine(tc); err == nil {
		report, err := machine.Attention(tc.Context, app.AttentionOptions{Limit: limit})
		if err != nil {
			return nil, err
		}
		want := stringArg(args, "workspace_id")
		if want != "" {
			filtered := report.Items[:0]
			for _, item := range report.Items {
				if item.WorkspaceID == want {
					filtered = append(filtered, item)
				}
			}
			report.Items = filtered
		}
		return publicAttention(report, tc.Server.Options.IncludeLocalOnlyPaths), nil
	}
	if tc.Server.Workspace == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "workspace_id is required")
	}
	id, _ := loadWorkspaceIdentity(tc)
	items := []map[string]any{}
	inbox, err := tc.Server.Workspace.Queries.Inbox(tc.Context, "")
	if err != nil {
		return nil, err
	}
	for _, item := range inbox {
		items = append(items, map[string]any{"kind": "inbox", "workspace_id": id, "ticket_id": item.TicketID, "summary": item.Summary})
	}
	if approvals, err := tc.Server.Workspace.Queries.Approvals(tc.Context, ""); err == nil {
		for _, item := range approvals {
			items = append(items, map[string]any{"kind": "approval", "workspace_id": id, "ticket_id": item.Ticket.ID, "project": item.Ticket.Project, "summary": item.Summary})
		}
	}
	if backup, err := tc.Server.Workspace.Queries.AutoBackupStatus(tc.Context); err == nil && strings.TrimSpace(backup.HealthWarning) != "" {
		items = append(items, map[string]any{"kind": "backup", "workspace_id": id, "summary": backup.HealthWarning})
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return map[string]any{"kind": "attention", "items": items, "total": len(items)}, nil
}

func appSearchOptions(args map[string]any, maxItems int) app.SearchOptions {
	return app.SearchOptions{Query: stringArg(args, "query"), Limit: intArg(args, "limit", maxItems)}
}

func activityTool(tc ToolContext, args map[string]any) (any, error) {
	if tc.Server.Workspace == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "workspace_id is required")
	}
	limit := intArg(args, "limit", tc.Server.Options.MaxItems)
	events, err := tc.Server.Workspace.Queries.RecentEvents(tc.Context, limit)
	if err != nil {
		return nil, err
	}
	return paginateSlice(events, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func projectListTool(tc ToolContext, args map[string]any) (any, error) {
	projects, err := listProjectRefs(tc)
	if err != nil {
		return nil, err
	}
	return paginateSlice(projects, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func projectUpdateTool(tc ToolContext, args map[string]any) (any, error) {
	key := stringArg(args, "key")
	existing, err := tc.Server.Workspace.Actions.Projects.GetProject(tc.Context, key)
	if err != nil {
		return nil, err
	}
	existing.Name = stringArg(args, "name")
	if err := tc.Server.Workspace.Actions.UpdateProject(tc.Context, existing); err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.Projects.GetProject(tc.Context, key)
}

func viewsListTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.ListSavedViews()
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func viewsGetTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.SavedView(stringArg(args, "name"))
}

func viewsRunTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.RunSavedView(tc.Context, stringArg(args, "name"), contracts.Actor(stringArg(args, "actor")))
}

func viewsSaveTool(tc ToolContext, args map[string]any) (any, error) {
	view := contracts.SavedView{
		Name:    stringArg(args, "name"),
		Title:   stringArg(args, "title"),
		Kind:    contracts.SavedViewKind(stringArg(args, "kind")),
		Query:   stringArg(args, "query"),
		Project: stringArg(args, "project"),
	}
	if err := tc.Server.Workspace.Queries.Views.SaveView(view); err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Queries.SavedView(view.Name)
}

func viewsDeleteTool(tc ToolContext, args map[string]any) (any, error) {
	name := stringArg(args, "name")
	if err := tc.Server.Workspace.Queries.Views.DeleteView(name); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "name": name}, nil
}

func ticketBulkTool(tc ToolContext, args map[string]any) (any, error) {
	op := service.BulkOperation{
		Kind:         service.BulkOperationKind(stringArg(args, "kind")),
		Actor:        contracts.Actor(tc.Actor),
		Assignee:     contracts.Actor(stringArg(args, "assignee")),
		Status:       contracts.Status(stringArg(args, "status")),
		Reason:       tc.Reason,
		TicketIDs:    stringSliceArg(args, "ticket_ids"),
		DryRun:       boolArg(args, "dry_run"),
		Confirm:      boolArg(args, "confirm"),
		OverrideDeps: boolArg(args, "override_deps"),
	}
	return tc.Server.Workspace.Actions.RunBulk(tc.Context, op)
}

func backupListTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Actions.ListBackups(tc.Context)
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	view.Items = page.Items.([]contracts.BackupSnapshot)
	return map[string]any{"backups": view, "total": page.Total, "next_cursor": page.NextCursor}, nil
}

func backupHistoryTool(tc ToolContext, _ map[string]any) (any, error) {
	auto, err := tc.Server.Workspace.Queries.AutoBackupStatus(tc.Context)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"automatic":                auto,
		"last_local_checkpoint":    auto.LastLocalCheckpointID,
		"push_is_not_verification": true,
	}, nil
}

func backupVerifyTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.VerifyBackupSnapshot(tc.Context, stringArg(args, "ref"))
}

func backupTargetsTool(tc ToolContext, _ map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ListBackupTargets(tc.Context)
}

func backupRunTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.BackupTick(tc.Context, boolArg(args, "force"))
}

func backupConfigureTool(tc ToolContext, args map[string]any) (any, error) {
	action := strings.ToLower(stringArg(args, "action"))
	switch action {
	case "enable":
		return tc.Server.Workspace.Actions.EnableAutoBackup(tc.Context, stringArg(args, "target_id"))
	case "disable":
		return tc.Server.Workspace.Actions.DisableAutoBackup(tc.Context)
	case "add":
		return tc.Server.Workspace.Actions.AddBackupTarget(tc.Context, service.BackupTargetAddOptions{
			TargetID:            stringArg(args, "target_id"),
			URL:                 stringArg(args, "url"),
			Enabled:             true,
			AllowLocalFile:      boolArg(args, "allow_local_file"),
			AcknowledgeBoundary: boolArg(args, "acknowledge_boundary"),
			AttestPrivate:       boolArg(args, "attest_private"),
		})
	case "edit":
		return tc.Server.Workspace.Actions.EditBackupTarget(tc.Context, stringArg(args, "target_id"), service.BackupTargetEditOptions{})
	case "remove":
		return tc.Server.Workspace.Actions.RemoveBackupTarget(tc.Context, stringArg(args, "target_id"))
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, "action must be enable, disable, add, edit, or remove")
	}
}

func restorePlanTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Actions.CreateRestorePlan(tc.Context, stringArg(args, "ref"), contracts.Actor(tc.Actor))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"plan":    view,
		"plan_id": view.PlanID,
		"digest":  view.PlanDigest,
	}, nil
}

func restoreApplyTool(tc ToolContext, args map[string]any) (any, error) {
	planID := stringArg(args, "plan_id")
	digest := stringArg(args, "digest")
	plan, err := tc.Server.Workspace.Actions.RestorePlans.LoadRestorePlan(tc.Context, planID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInvalidInput, err, "restore apply requires a previously created plan")
	}
	stored := service.RestorePlanBindingDigest(plan)
	if stored != digest {
		return nil, apperr.New(apperr.CodeConflict, "restore plan digest does not match the stored plan")
	}
	return tc.Server.Workspace.Actions.ApplyRestorePlan(tc.Context, planID, contracts.Actor(tc.Actor), tc.Reason, true)
}

func settingsGetTool(tc ToolContext, args map[string]any) (any, error) {
	if stringArg(args, "workspace_id") != "" || (tc.Server.Workspace != nil && !tc.Server.Options.Global) {
		if tc.Server.Workspace == nil {
			return nil, apperr.New(apperr.CodeInvalidInput, "workspace_id is required")
		}
		cfg, err := loadWorkspaceSettings(tc)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	settings := machine.Settings()
	return publicMachineSettings(settings), nil
}

func settingsUpdateTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	patch := app.MachineSettingsPatch{}
	if _, ok := args["auto_register"]; ok {
		v := boolArg(args, "auto_register")
		patch.AutoRegister = &v
	}
	if _, ok := args["local_checkpoints"]; ok {
		v := boolArg(args, "local_checkpoints")
		patch.LocalCheckpoints = &v
	}
	if _, ok := args["default_project"]; ok {
		v := boolArg(args, "default_project")
		patch.DefaultProject = &v
	}
	if _, ok := args["open_home"]; ok {
		patch.Browser = &app.BrowserSettings{OpenHome: boolArg(args, "open_home")}
	}
	if _, ok := args["show_hidden"]; ok {
		patch.Home = &app.HomeSettings{ShowHidden: boolArg(args, "show_hidden")}
	}
	updated, err := machine.UpdateSettings(tc.Context, patch)
	if err != nil {
		return nil, err
	}
	return publicMachineSettings(updated), nil
}

func loadWorkspaceSettings(tc ToolContext) (map[string]any, error) {
	id, err := loadWorkspaceIdentity(tc)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace_id": id,
		"kind":         "workspace_settings",
	}, nil
}

func requireMachine(tc ToolContext) (Machine, error) {
	if tc.Server != nil && tc.Server.Options.Machine != nil {
		return tc.Server.Options.Machine, nil
	}
	return nil, apperr.New(apperr.CodeInvalidInput, "machine MCP surface is not available")
}

func requireWorkspace(tc ToolContext, what string) error {
	if tc.Server.Workspace != nil {
		return nil
	}
	return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s requires a bound workspace", what))
}
