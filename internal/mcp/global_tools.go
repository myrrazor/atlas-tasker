package mcp

import (
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func globalMachineSpecs() []ToolSpec {
	readProfiles := []ToolProfile{ProfileRead, ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	workflowProfiles := []ToolProfile{ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	adminProfiles := []ToolProfile{ProfileAdmin}

	machine := func(spec ToolSpec) ToolSpec {
		spec.Scope = ScopeMachine
		return spec
	}

	return []ToolSpec{
		machine(readTool("atlas.workspace.list", "List registered workspaces and their health.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"include_hidden": boolProp("Include hidden workspaces.")})), "App.ListWorkspaces", workspaceListTool)),
		machine(readTool("atlas.workspace.get", "Get one registered workspace after revalidation.", readProfiles, objectSchema([]string{"workspace_id"}, map[string]any{"workspace_id": stringProp("Workspace ID.")}), "App.GetWorkspace", workspaceGetTool)),
		machine(readTool("atlas.workspace.status", "Read registry health for one workspace.", readProfiles, objectSchema([]string{"workspace_id"}, map[string]any{"workspace_id": stringProp("Workspace ID.")}), "App.GetWorkspace", workspaceGetTool)),
		machine(readTool("atlas.workspace.board_url", "Return the loopback Home URL for a workspace or project board.", readProfiles, objectSchema([]string{"workspace_id"}, map[string]any{"workspace_id": stringProp("Workspace ID."), "project": stringProp("Optional project key.")}), "App.BoardURL", workspaceBoardURLTool)),
		machine(writeTool("atlas.workspace.init", ClassWorkflow, workflowProfiles, false, "Initialize the current directory or a granted/discovery path through app.Init.", objectSchema([]string{"actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"path": stringProp("Directory to initialize. Defaults to the current working directory, or the granted path when grant_id is set without path."), "grant_id": stringProp("Previously issued path grant."), "git_mode": stringProp("shared, private, or unmanaged."), "register": boolProp("Register the workspace. Default true."), "agents": boolProp("Prepare agent entries. Default true."), "backup": boolProp("Enable local checkpoints. Default true."), "default_project": boolProp("Create a default project when none exist.")})), "App.Init", "path", workspaceInitTool)),
		machine(writeTool("atlas.workspace.register", ClassWorkflow, workflowProfiles, false, "Register an initialized workspace pointer. Files are not copied.", objectSchema([]string{"path", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"path": stringProp("Initialized workspace directory.")})), "App.Register", "path", workspaceRegisterTool)),
		machine(writeTool("atlas.workspace.repair", ClassWorkflow, workflowProfiles, false, "Repair a moved workspace pointer. fork_copy is a separate high-impact tool.", objectSchema([]string{"workspace_id", "action", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"workspace_id": stringProp("Workspace ID."), "action": stringProp("update_path."), "path": stringProp("New canonical path for update_path.")})), "App.Repair", "workspace_id", workspaceRepairTool)),
		machine(writeTool("atlas.workspace.hide", ClassWorkflow, workflowProfiles, false, "Hide a workspace pointer from default lists.", objectSchema([]string{"workspace_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"workspace_id": stringProp("Workspace ID.")})), "App.Repair", "workspace_id", workspaceHideTool)),
		machine(writeTool("atlas.workspace.unhide", ClassWorkflow, workflowProfiles, false, "Unhide a workspace pointer.", objectSchema([]string{"workspace_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"workspace_id": stringProp("Workspace ID.")})), "App.Repair", "workspace_id", workspaceUnhideTool)),
		machine(writeTool("atlas.workspace.remove_pointer", ClassWorkflow, workflowProfiles, false, "Remove the registry pointer. Workspace files and backups are preserved.", objectSchema([]string{"workspace_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"workspace_id": stringProp("Workspace ID.")})), "App.Repair", "workspace_id", workspaceRemovePointerTool)),
		machine(highImpactTool("atlas.workspace.fork_copy", adminProfiles, "Give a copied workspace a new identity. Does not alter the original.", objectSchema([]string{"workspace_id", "path", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Workspace ID."), map[string]any{"workspace_id": stringProp("Original workspace ID."), "path": stringProp("Path of the copy.")})), "App.Repair", "workspace_id", workspaceForkCopyTool)),
		machine(highImpactTool("atlas.settings.grant_discovery", adminProfiles, "Widen machine discovery roots. Ordinary settings.update cannot do this.", objectSchema([]string{"root", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Discovery root."), map[string]any{"root": stringProp("Absolute directory to allow for discovery."), "enabled": boolProp("Enable discovery.")})), "Machine.GrantDiscovery", "root", settingsGrantDiscoveryTool)),
	}
}

func workspaceListTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	items, err := machine.ListWorkspaces(tc.Context, app.ListOptions{IncludeHidden: boolArg(args, "include_hidden")})
	if err != nil {
		return nil, err
	}
	public := make([]map[string]any, 0, len(items))
	for _, item := range items {
		public = append(public, publicWorkspace(item, tc.Server.Options.IncludeLocalOnlyPaths))
	}
	return paginateSlice(public, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func workspaceGetTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	rec, err := machine.GetWorkspace(tc.Context, stringArg(args, "workspace_id"))
	out := publicWorkspace(rec, tc.Server.Options.IncludeLocalOnlyPaths)
	if err != nil {
		return out, err
	}
	return out, nil
}

func workspaceBoardURLTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	url, err := machine.BoardURL(stringArg(args, "workspace_id"), stringArg(args, "project"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"board_url": url}, nil
}

func workspaceInitTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	if tc.Server.Options.ReadOnly || tc.Server.Options.Profile == ProfileRead {
		return nil, apperr.New(apperr.CodePermissionDenied, "workspace init is not available in a read-only MCP profile")
	}
	path := stringArg(args, "path")
	grantID := stringArg(args, "grant_id")
	if path == "" && grantID == "" {
		path = tc.Server.Options.CWD
	}
	call := InitCall{
		Root:           path,
		GitMode:        stringArg(args, "git_mode"),
		Register:       true,
		Agents:         true,
		Backup:         true,
		DefaultProject: true,
		Actor:          contracts.Actor(tc.Actor),
		GrantID:        grantID,
		WriteClientCfg: true,
	}
	if _, ok := args["register"]; ok {
		call.Register = boolArg(args, "register")
	}
	if _, ok := args["agents"]; ok {
		call.Agents = boolArg(args, "agents")
	}
	if _, ok := args["backup"]; ok {
		call.Backup = boolArg(args, "backup")
	}
	if _, ok := args["default_project"]; ok {
		call.DefaultProject = boolArg(args, "default_project")
	}
	if !call.Agents {
		call.WriteClientCfg = false
	}
	return machine.Init(tc.Context, call)
}

func workspaceRegisterTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	rec, err := machine.Register(tc.Context, app.RegisterOptions{Root: stringArg(args, "path"), DisplayName: filepath.Base(stringArg(args, "path"))})
	if err != nil {
		return nil, err
	}
	return publicWorkspace(rec, tc.Server.Options.IncludeLocalOnlyPaths), nil
}

func workspaceRepairTool(tc ToolContext, args map[string]any) (any, error) {
	action := stringArg(args, "action")
	if action == "" {
		action = string(app.RepairUpdatePath)
	}
	if action != string(app.RepairUpdatePath) {
		return nil, apperr.New(apperr.CodeInvalidInput, "atlas.workspace.repair only supports update_path; use fork_copy/hide/unhide/remove_pointer")
	}
	return repairAction(tc, app.RepairOptions{
		WorkspaceID: stringArg(args, "workspace_id"),
		Action:      app.RepairUpdatePath,
		NewPath:     stringArg(args, "path"),
	})
}

func workspaceHideTool(tc ToolContext, args map[string]any) (any, error) {
	return repairAction(tc, app.RepairOptions{WorkspaceID: stringArg(args, "workspace_id"), Action: app.RepairHide})
}

func workspaceUnhideTool(tc ToolContext, args map[string]any) (any, error) {
	return repairAction(tc, app.RepairOptions{WorkspaceID: stringArg(args, "workspace_id"), Action: app.RepairUnhide})
}

func workspaceRemovePointerTool(tc ToolContext, args map[string]any) (any, error) {
	return repairAction(tc, app.RepairOptions{WorkspaceID: stringArg(args, "workspace_id"), Action: app.RepairRemovePointer})
}

func workspaceForkCopyTool(tc ToolContext, args map[string]any) (any, error) {
	return repairAction(tc, app.RepairOptions{
		WorkspaceID: stringArg(args, "workspace_id"),
		Action:      app.RepairForkCopy,
		NewPath:     stringArg(args, "path"),
	})
}

func repairAction(tc ToolContext, opts app.RepairOptions) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	result, err := machine.Repair(tc.Context, opts)
	out := map[string]any{"kind": result.Kind, "forked_workspace_id": result.ForkedID, "record": publicWorkspace(result.Record, tc.Server.Options.IncludeLocalOnlyPaths)}
	if err != nil {
		return out, err
	}
	return out, nil
}

func settingsGrantDiscoveryTool(tc ToolContext, args map[string]any) (any, error) {
	machine, err := requireMachine(tc)
	if err != nil {
		return nil, err
	}
	current := machine.Settings()
	roots := append([]string{}, current.Discovery.Roots...)
	root := stringArg(args, "root")
	found := false
	for _, existing := range roots {
		if existing == root {
			found = true
			break
		}
	}
	if !found {
		roots = append(roots, root)
	}
	enabled := current.Discovery.Enabled
	if _, ok := args["enabled"]; ok {
		enabled = boolArg(args, "enabled")
	} else {
		enabled = true
	}
	updated, err := machine.GrantDiscovery(tc.Context, app.DiscoverySettings{Roots: roots, Enabled: enabled, MaxDepth: current.Discovery.MaxDepth})
	if err != nil {
		return nil, err
	}
	return publicMachineSettings(updated), nil
}
