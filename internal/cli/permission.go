package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newPermissionProfileCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "permission-profile", Short: "Manage permission profiles"}
	list := &cobra.Command{Use: "list", Short: "List permission profiles", RunE: runPermissionProfileList}
	view := &cobra.Command{Use: "view <PROFILE-ID>", Args: cobra.ExactArgs(1), Short: "Show one permission profile", RunE: runPermissionProfileView}
	create := &cobra.Command{Use: "create <PROFILE-ID>", Args: cobra.ExactArgs(1), Short: "Create a permission profile", RunE: runPermissionProfileCreate}
	edit := &cobra.Command{Use: "edit <PROFILE-ID>", Args: cobra.ExactArgs(1), Short: "Edit a permission profile", RunE: runPermissionProfileEdit}
	bind := &cobra.Command{Use: "bind <PROFILE-ID>", Args: cobra.ExactArgs(1), Short: "Bind a permission profile to a target", RunE: runPermissionProfileBind}
	unbind := &cobra.Command{Use: "unbind <PROFILE-ID>", Args: cobra.ExactArgs(1), Short: "Unbind a permission profile from a target", RunE: runPermissionProfileUnbind}
	for _, sub := range []*cobra.Command{list, view, create, edit, bind, unbind} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{create, edit} {
		sub.Flags().String("name", "", "Display name")
		sub.Flags().Int("priority", 0, "Priority within the binding layer")
		sub.Flags().Bool("workspace-default", false, "Apply to every workspace action by default")
		sub.Flags().StringArray("project", nil, "Bound project key")
		sub.Flags().StringArray("agent", nil, "Bound agent id")
		sub.Flags().StringArray("runbook", nil, "Bound runbook name")
		sub.Flags().StringArray("allow-project", nil, "Allowed project key")
		sub.Flags().StringArray("allow-ticket-type", nil, "Allowed ticket type")
		sub.Flags().StringArray("allow-runbook", nil, "Allowed runbook name")
		sub.Flags().StringArray("allow-capability", nil, "Allowed capability")
		sub.Flags().StringArray("allow-action", nil, "Allowed action")
		sub.Flags().StringArray("deny-action", nil, "Denied action")
		sub.Flags().StringArray("allow-path", nil, "Allowed repo-root-relative path or glob")
		sub.Flags().StringArray("forbid-path", nil, "Forbidden repo-root-relative path or glob")
		sub.Flags().Bool("require-owner-for-sensitive-ops", false, "Require human:owner for protected or sensitive ticket actions")
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
	}
	for _, sub := range []*cobra.Command{bind, unbind} {
		sub.Flags().Bool("workspace", false, "Bind at the workspace-default layer")
		sub.Flags().String("project", "", "Project target")
		sub.Flags().String("agent", "", "Agent target")
		sub.Flags().String("runbook", "", "Runbook target")
		sub.Flags().String("ticket", "", "Ticket target")
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
	}
	cmd.AddCommand(list, view, create, edit, bind, unbind)
	return cmd
}

func newPermissionsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "permissions", Short: "Explain effective permissions for a target"}
	view := &cobra.Command{Use: "view <TARGET>", Args: cobra.ExactArgs(1), Short: "Show effective permissions for a ticket, run, change, or gate target", RunE: runPermissionsView}
	view.Flags().String("actor", "", "Actor to evaluate")
	view.Flags().String("action", "", "Optional action filter")
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)
	return cmd
}

func runPermissionProfileList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.ListPermissionProfiles(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatPermissionProfileList(items)
	data := map[string]any{"kind": "permission_profile_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runPermissionProfileView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	profile, err := workspace.queries.PermissionProfileDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatPermissionProfileDetail(profile)
	data := map[string]any{"kind": "permission_profile_detail", "generated_at": time.Now().UTC(), "payload": profile}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runPermissionProfileCreate(cmd *cobra.Command, args []string) error {
	return savePermissionProfile(cmd, args[0], true)
}

func runPermissionProfileEdit(cmd *cobra.Command, args []string) error {
	return savePermissionProfile(cmd, args[0], false)
}

func savePermissionProfile(cmd *cobra.Command, profileID string, create bool) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	profile := contracts.PermissionProfile{ProfileID: profileID, SchemaVersion: contracts.CurrentSchemaVersion}
	if !create {
		loaded, err := workspace.queries.PermissionProfileDetail(ctx, profileID)
		if err != nil {
			return err
		}
		profile = loaded
	}
	if err := applyPermissionProfileFlags(cmd, &profile, create); err != nil {
		return err
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	saved, err := workspace.actions.SavePermissionProfile(ctx, profile, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatPermissionProfileDetail(saved)
	data := map[string]any{"kind": "permission_profile_detail", "generated_at": time.Now().UTC(), "payload": saved}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runPermissionProfileBind(cmd *cobra.Command, args []string) error {
	return mutatePermissionBinding(cmd, args[0], true)
}

func runPermissionProfileUnbind(cmd *cobra.Command, args []string) error {
	return mutatePermissionBinding(cmd, args[0], false)
}

func mutatePermissionBinding(cmd *cobra.Command, profileID string, bind bool) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	targetKind, targetID, err := permissionBindingTargetFromFlags(cmd)
	if err != nil {
		return err
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	var result service.PermissionBindingResult
	if bind {
		result, err = workspace.actions.BindPermissionProfile(ctx, profileID, targetKind, targetID, normalizeActor(actorRaw), reason)
	} else {
		result, err = workspace.actions.UnbindPermissionProfile(ctx, profileID, targetKind, targetID, normalizeActor(actorRaw), reason)
	}
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("%s %s -> %s %s", map[bool]string{true: "bound", false: "unbound"}[bind], result.Profile.ProfileID, result.TargetKind, result.TargetID)
	data := map[string]any{"kind": "permission_profile_detail", "generated_at": result.GeneratedAt, "payload": result}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runPermissionsView(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	actionRaw, _ := cmd.Flags().GetString("action")
	actor := contracts.Actor(strings.TrimSpace(actorRaw))
	if actor != "" && !actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", actor)
	}
	action := contracts.PermissionAction(strings.TrimSpace(actionRaw))
	if action != "" && !action.IsValid() {
		return fmt.Errorf("invalid action: %s", action)
	}
	view, err := workspace.queries.PermissionsView(ctx, args[0], actor, action)
	if err != nil {
		return err
	}
	pretty := formatPermissionsView(view)
	data := map[string]any{"kind": "permissions_effective_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func applyPermissionProfileFlags(cmd *cobra.Command, profile *contracts.PermissionProfile, create bool) error {
	if create || cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		profile.DisplayName = strings.TrimSpace(name)
	}
	if create || cmd.Flags().Changed("priority") {
		priority, _ := cmd.Flags().GetInt("priority")
		profile.Priority = priority
	}
	if create || cmd.Flags().Changed("workspace-default") {
		value, _ := cmd.Flags().GetBool("workspace-default")
		profile.WorkspaceDefault = value
	}
	if create || cmd.Flags().Changed("project") {
		values, _ := cmd.Flags().GetStringArray("project")
		profile.Projects = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("agent") {
		values, _ := cmd.Flags().GetStringArray("agent")
		profile.Agents = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("runbook") {
		values, _ := cmd.Flags().GetStringArray("runbook")
		profile.Runbooks = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("allow-project") {
		values, _ := cmd.Flags().GetStringArray("allow-project")
		profile.AllowedProjects = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("allow-ticket-type") {
		values, _ := cmd.Flags().GetStringArray("allow-ticket-type")
		profile.AllowedTicketTypes = profile.AllowedTicketTypes[:0]
		for _, value := range values {
			kind := contracts.TicketType(strings.TrimSpace(value))
			if kind == "" {
				continue
			}
			profile.AllowedTicketTypes = append(profile.AllowedTicketTypes, kind)
		}
	}
	if create || cmd.Flags().Changed("allow-runbook") {
		values, _ := cmd.Flags().GetStringArray("allow-runbook")
		profile.AllowedRunbooks = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("allow-capability") {
		values, _ := cmd.Flags().GetStringArray("allow-capability")
		profile.AllowedCapabilities = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("allow-action") {
		values, _ := cmd.Flags().GetStringArray("allow-action")
		profile.AllowActions = profile.AllowActions[:0]
		for _, value := range values {
			action := contracts.PermissionAction(strings.TrimSpace(value))
			if action == "" {
				continue
			}
			profile.AllowActions = append(profile.AllowActions, action)
		}
	}
	if create || cmd.Flags().Changed("deny-action") {
		values, _ := cmd.Flags().GetStringArray("deny-action")
		profile.DenyActions = profile.DenyActions[:0]
		for _, value := range values {
			action := contracts.PermissionAction(strings.TrimSpace(value))
			if action == "" {
				continue
			}
			profile.DenyActions = append(profile.DenyActions, action)
		}
	}
	if create || cmd.Flags().Changed("allow-path") {
		values, _ := cmd.Flags().GetStringArray("allow-path")
		profile.AllowedPaths = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("forbid-path") {
		values, _ := cmd.Flags().GetStringArray("forbid-path")
		profile.ForbiddenPaths = append([]string{}, values...)
	}
	if create || cmd.Flags().Changed("require-owner-for-sensitive-ops") {
		value, _ := cmd.Flags().GetBool("require-owner-for-sensitive-ops")
		profile.RequiresOwnerForSensitiveOps = value
	}
	return profile.Validate()
}

func permissionBindingTargetFromFlags(cmd *cobra.Command) (string, string, error) {
	workspaceSelected, _ := cmd.Flags().GetBool("workspace")
	targets := []struct {
		kind string
		id   string
	}{
		{kind: "project", id: strings.TrimSpace(flagString(cmd, "project"))},
		{kind: "agent", id: strings.TrimSpace(flagString(cmd, "agent"))},
		{kind: "runbook", id: strings.TrimSpace(flagString(cmd, "runbook"))},
		{kind: "ticket", id: strings.TrimSpace(flagString(cmd, "ticket"))},
	}
	selected := ""
	selectedID := ""
	for _, candidate := range targets {
		if candidate.id == "" {
			continue
		}
		if selected != "" {
			return "", "", fmt.Errorf("choose exactly one binding target")
		}
		selected = candidate.kind
		selectedID = candidate.id
	}
	if workspaceSelected {
		if selected != "" {
			return "", "", fmt.Errorf("choose exactly one binding target")
		}
		return "workspace", "", nil
	}
	if selected == "" {
		return "", "", fmt.Errorf("binding target is required")
	}
	return selected, selectedID, nil
}

func flagString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}

func formatPermissionProfileList(items []contracts.PermissionProfile) string {
	if len(items) == 0 {
		return "no permission profiles"
	}
	lines := []string{"permission profiles:"}
	for _, item := range items {
		line := fmt.Sprintf("- %s priority=%d", item.ProfileID, item.Priority)
		if strings.TrimSpace(item.DisplayName) != "" {
			line += fmt.Sprintf(" name=%s", item.DisplayName)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatPermissionProfileDetail(profile contracts.PermissionProfile) string {
	lines := []string{fmt.Sprintf("permission profile %s", profile.ProfileID)}
	if strings.TrimSpace(profile.DisplayName) != "" {
		lines = append(lines, fmt.Sprintf("name=%s", profile.DisplayName))
	}
	lines = append(lines, fmt.Sprintf("priority=%d workspace_default=%t", profile.Priority, profile.WorkspaceDefault))
	if len(profile.Projects) > 0 {
		lines = append(lines, "projects="+strings.Join(profile.Projects, ","))
	}
	if len(profile.Agents) > 0 {
		lines = append(lines, "agents="+strings.Join(profile.Agents, ","))
	}
	if len(profile.Runbooks) > 0 {
		lines = append(lines, "runbooks="+strings.Join(profile.Runbooks, ","))
	}
	if len(profile.AllowActions) > 0 {
		values := make([]string, 0, len(profile.AllowActions))
		for _, value := range profile.AllowActions {
			values = append(values, string(value))
		}
		lines = append(lines, "allow_actions="+strings.Join(values, ","))
	}
	if len(profile.DenyActions) > 0 {
		values := make([]string, 0, len(profile.DenyActions))
		for _, value := range profile.DenyActions {
			values = append(values, string(value))
		}
		lines = append(lines, "deny_actions="+strings.Join(values, ","))
	}
	if len(profile.AllowedPaths) > 0 {
		lines = append(lines, "allow_paths="+strings.Join(profile.AllowedPaths, ","))
	}
	if len(profile.ForbiddenPaths) > 0 {
		lines = append(lines, "forbidden_paths="+strings.Join(profile.ForbiddenPaths, ","))
	}
	if profile.RequiresOwnerForSensitiveOps {
		lines = append(lines, "requires_owner_for_sensitive_ops=true")
	}
	return strings.Join(lines, "\n")
}

func formatPermissionsView(view service.PermissionsView) string {
	lines := []string{fmt.Sprintf("permissions %s actor=%s", view.Target, view.Actor)}
	for _, decision := range view.Decisions {
		line := fmt.Sprintf("- %s allowed=%t", decision.Action, decision.Allowed)
		if decision.RequiresOwnerOverride {
			line += fmt.Sprintf(" owner_override=%t", decision.OverrideApplied)
		}
		if len(decision.ReasonCodes) > 0 {
			line += fmt.Sprintf(" reasons=%s", strings.Join(decision.ReasonCodes, ","))
		}
		lines = append(lines, line)
		for _, profile := range decision.Profiles {
			lines = append(lines, fmt.Sprintf("  · %s (%s)", profile.ProfileID, profile.Layer))
		}
	}
	return strings.Join(lines, "\n")
}
