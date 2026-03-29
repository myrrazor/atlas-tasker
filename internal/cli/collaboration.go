package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newCollaboratorCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "collaborator", Short: "Manage collaborators and trust state"}

	list := &cobra.Command{Use: "list", Short: "List collaborators", RunE: runCollaboratorList}
	list.Flags().String("status", "", "Filter by collaborator status")
	addReadOutputFlags(list, &outputFlags{})

	view := &cobra.Command{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View collaborator details", RunE: runCollaboratorView}
	addReadOutputFlags(view, &outputFlags{})

	add := &cobra.Command{Use: "add <ID>", Args: cobra.ExactArgs(1), Short: "Add a collaborator", RunE: runCollaboratorAdd}
	add.Flags().String("name", "", "Display name")
	add.Flags().StringArray("actor-map", nil, "Atlas actor mapped to this collaborator")
	add.Flags().StringArray("provider-handle", nil, "Provider handle mapping as provider:handle")
	addMutationFlags(add, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(add, &outputFlags{})

	edit := &cobra.Command{Use: "edit <ID>", Args: cobra.ExactArgs(1), Short: "Edit collaborator metadata", RunE: runCollaboratorEdit}
	edit.Flags().String("name", "", "Display name")
	edit.Flags().StringArray("actor-map", nil, "Replace Atlas actor mappings")
	edit.Flags().StringArray("provider-handle", nil, "Replace provider handle mappings as provider:handle")
	editMutationFlags(edit)

	trust := &cobra.Command{Use: "trust <ID>", Args: cobra.ExactArgs(1), Short: "Mark a collaborator as trusted", RunE: runCollaboratorTrust}
	trustMutationFlags(trust)

	suspend := &cobra.Command{Use: "suspend <ID>", Args: cobra.ExactArgs(1), Short: "Suspend a collaborator", RunE: runCollaboratorSuspend}
	suspendMutationFlags(suspend)

	remove := &cobra.Command{Use: "remove <ID>", Args: cobra.ExactArgs(1), Short: "Tombstone a collaborator", RunE: runCollaboratorRemove}
	removeMutationFlags(remove)

	cmd.AddCommand(list, view, add, edit, trust, suspend, remove)
	return cmd
}

func editMutationFlags(cmd *cobra.Command) {
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
}

func trustMutationFlags(cmd *cobra.Command) {
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
}

func suspendMutationFlags(cmd *cobra.Command) {
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
}

func removeMutationFlags(cmd *cobra.Command) {
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
}

func newMembershipCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "membership", Short: "Manage collaborator memberships"}

	list := &cobra.Command{Use: "list", Short: "List memberships", RunE: runMembershipList}
	list.Flags().String("collaborator", "", "Filter by collaborator ID")
	addReadOutputFlags(list, &outputFlags{})

	bind := &cobra.Command{Use: "bind <COLLABORATOR-ID>", Args: cobra.ExactArgs(1), Short: "Bind a collaborator to a scope", RunE: runMembershipBind}
	bind.Flags().String("scope-kind", "", "Scope kind: workspace or project")
	bind.Flags().String("scope-id", "", "Scope ID; defaults to the current workspace for workspace scope")
	bind.Flags().String("role", "", "Membership role")
	bind.Flags().StringArray("profile", nil, "Default permission profile to attach")
	addMutationFlags(bind, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(bind, &outputFlags{})

	unbind := &cobra.Command{Use: "unbind <MEMBERSHIP-UID>", Args: cobra.ExactArgs(1), Short: "Unbind a collaborator membership", RunE: runMembershipUnbind}
	addMutationFlags(unbind, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(unbind, &outputFlags{})

	cmd.AddCommand(list, bind, unbind)
	return cmd
}

func newRemoteCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "remote", Short: "Manage Atlas sync remotes"}
	list := &cobra.Command{Use: "list", Short: "List sync remotes", RunE: runRemoteList}
	view := &cobra.Command{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View sync remote details", RunE: runRemoteView}
	add := &cobra.Command{Use: "add <ID>", Args: cobra.ExactArgs(1), Short: "Add a sync remote", RunE: runRemoteAdd}
	add.Flags().String("kind", "path", "Remote kind: path or git")
	add.Flags().String("location", "", "Remote location")
	add.Flags().String("default-action", "fetch", "Default sync action: fetch, pull, or push")
	add.Flags().Bool("disabled", false, "Create the remote disabled")
	edit := &cobra.Command{Use: "edit <ID>", Args: cobra.ExactArgs(1), Short: "Edit a sync remote", RunE: runRemoteEdit}
	edit.Flags().String("kind", "", "Replace the remote kind")
	edit.Flags().String("location", "", "Replace the remote location")
	edit.Flags().String("default-action", "", "Replace the default sync action")
	edit.Flags().Bool("enabled", true, "Whether the remote is enabled")
	remove := &cobra.Command{Use: "remove <ID>", Args: cobra.ExactArgs(1), Short: "Remove a sync remote", RunE: runRemoteRemove}
	for _, sub := range []*cobra.Command{list, view, add, edit, remove} {
		addReadOutputFlags(sub, &outputFlags{})
		if sub == add || sub == edit || sub == remove {
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func newSyncCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Inspect and run Atlas sync jobs"}
	status := &cobra.Command{Use: "status", Short: "Inspect sync status", RunE: runSyncStatus}
	status.Flags().String("remote", "", "Filter by remote ID")
	jobs := &cobra.Command{Use: "jobs", Short: "List sync jobs", RunE: runSyncJobs}
	jobs.Flags().String("remote", "", "Filter by remote ID")
	view := &cobra.Command{Use: "view <JOB-ID>", Args: cobra.ExactArgs(1), Short: "View one sync job", RunE: runSyncView}
	fetch := &cobra.Command{Use: "fetch", Short: "Fetch remote sync publications", RunE: runSyncFetch}
	fetch.Flags().String("remote", "", "Remote ID")
	pull := &cobra.Command{Use: "pull", Short: "Pull remote state into this workspace", RunE: runSyncPull}
	pull.Flags().String("remote", "", "Remote ID")
	pull.Flags().String("workspace", "", "Source workspace ID when the remote has multiple publications")
	push := &cobra.Command{Use: "push", Short: "Publish local state to a remote", RunE: runSyncPush}
	push.Flags().String("remote", "", "Remote ID")
	run := &cobra.Command{Use: "run", Short: "Run the remote's default sync action", RunE: runSyncRun}
	run.Flags().String("remote", "", "Remote ID")
	run.Flags().String("workspace", "", "Source workspace ID when running a pull action")
	for _, sub := range []*cobra.Command{status, jobs, view, fetch, pull, push, run} {
		addReadOutputFlags(sub, &outputFlags{})
		if sub == fetch || sub == pull || sub == push || sub == run {
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func newBundleCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "bundle", Short: "Create, verify, and import Atlas sync bundles"}
	create := &cobra.Command{Use: "create", Short: "Create a local sync bundle", RunE: runBundleCreate}
	list := &cobra.Command{Use: "list", Short: "List local bundle jobs", RunE: runBundleList}
	view := &cobra.Command{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View one bundle", RunE: runBundleView}
	verify := &cobra.Command{Use: "verify <PATH|BUNDLE-ID>", Args: cobra.ExactArgs(1), Short: "Verify a sync bundle", RunE: runBundleVerify}
	importCmd := &cobra.Command{Use: "import <PATH|BUNDLE-ID>", Args: cobra.ExactArgs(1), Short: "Import a sync bundle into this workspace", RunE: runBundleImport}
	for _, sub := range []*cobra.Command{create, list, view, verify, importCmd} {
		addReadOutputFlags(sub, &outputFlags{})
		if sub == create || sub == verify || sub == importCmd {
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func newConflictCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "conflict", Short: "Inspect and resolve sync conflicts"}
	list := &cobra.Command{Use: "list", Short: "List sync conflicts", RunE: runConflictList}
	view := &cobra.Command{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View one sync conflict", RunE: runConflictView}
	resolve := &cobra.Command{Use: "resolve <ID>", Args: cobra.ExactArgs(1), Short: "Resolve a sync conflict", RunE: runConflictResolve}
	resolve.Flags().String("resolution", "", "Resolution: use_local or use_remote")
	addReadOutputFlags(list, &outputFlags{})
	addReadOutputFlags(view, &outputFlags{})
	addMutationFlags(resolve, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(resolve, &outputFlags{})
	cmd.AddCommand(list, view, resolve)
	return cmd
}

func newMentionsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mentions", Short: "Inspect collaborator mentions"}
	list := &cobra.Command{Use: "list", Short: "List mentions", RunE: runMentionsList}
	list.Flags().String("collaborator", "", "Filter mentions for one collaborator")
	view := &cobra.Command{Use: "view <MENTION-UID>", Args: cobra.ExactArgs(1), Short: "View a mention", RunE: runMentionView}
	for _, sub := range []*cobra.Command{list, view} {
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newProjectCodeownersCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "codeowners", Short: "Preview or write CODEOWNERS scaffolding"}
	render := &cobra.Command{Use: "render <KEY>", Args: cobra.ExactArgs(1), Short: "Render CODEOWNERS preview", RunE: runProjectCodeownersRender}
	write := &cobra.Command{Use: "write <KEY>", Args: cobra.ExactArgs(1), Short: "Write CODEOWNERS scaffolding", RunE: runProjectCodeownersWrite}
	addReadOutputFlags(render, &outputFlags{})
	addMutationFlags(write, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(write, &outputFlags{})
	cmd.AddCommand(render, write)
	return cmd
}

func newProjectRulesCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "rules", Short: "Render provider rules scaffolding"}
	render := &cobra.Command{Use: "render <KEY>", Args: cobra.ExactArgs(1), Short: "Render provider rules preview", RunE: runProjectRulesRender}
	addReadOutputFlags(render, &outputFlags{})
	cmd.AddCommand(render)
	return cmd
}

func runCollaboratorList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	statusRaw, _ := cmd.Flags().GetString("status")
	status := contracts.CollaboratorStatus(strings.TrimSpace(statusRaw))
	items, err := workspace.queries.ListCollaborators(commandContext(cmd))
	if err != nil {
		return err
	}
	if status != "" {
		filtered := make([]contracts.CollaboratorProfile, 0, len(items))
		for _, item := range items {
			if item.Status == status {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	pretty := formatCollaboratorList(items)
	data := map[string]any{"kind": "collaborator_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runCollaboratorView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.CollaboratorDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatCollaboratorDetail(view)
	data := map[string]any{"kind": "collaborator_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runCollaboratorAdd(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	name, _ := cmd.Flags().GetString("name")
	actorsRaw, _ := cmd.Flags().GetStringArray("actor-map")
	handlesRaw, _ := cmd.Flags().GetStringArray("provider-handle")
	atlasActors, err := parseCollaboratorActors(actorsRaw)
	if err != nil {
		return err
	}
	providerHandles, err := parseProviderHandles(handlesRaw)
	if err != nil {
		return err
	}
	collaborator, err := workspace.actions.AddCollaborator(commandContext(cmd), contracts.CollaboratorProfile{
		CollaboratorID:  strings.TrimSpace(args[0]),
		DisplayName:     strings.TrimSpace(name),
		AtlasActors:     atlasActors,
		ProviderHandles: providerHandles,
	}, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	view, err := workspace.queries.CollaboratorDetail(commandContext(cmd), collaborator.CollaboratorID)
	if err != nil {
		return err
	}
	pretty := formatCollaboratorDetail(view)
	data := map[string]any{"kind": "collaborator_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runCollaboratorEdit(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	name, _ := cmd.Flags().GetString("name")
	actorsRaw, _ := cmd.Flags().GetStringArray("actor-map")
	handlesRaw, _ := cmd.Flags().GetStringArray("provider-handle")
	atlasActors, err := parseCollaboratorActors(actorsRaw)
	if err != nil {
		return err
	}
	providerHandles, err := parseProviderHandles(handlesRaw)
	if err != nil {
		return err
	}
	_, err = workspace.actions.EditCollaborator(commandContext(cmd), args[0], name, atlasActors, providerHandles, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	view, err := workspace.queries.CollaboratorDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatCollaboratorDetail(view)
	data := map[string]any{"kind": "collaborator_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runCollaboratorTrust(cmd *cobra.Command, args []string) error {
	return mutateCollaboratorStatus(cmd, args[0], func(workspace *workspace, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
		return workspace.actions.SetCollaboratorTrust(commandContext(cmd), args[0], true, actor, reason)
	})
}

func runCollaboratorSuspend(cmd *cobra.Command, args []string) error {
	return mutateCollaboratorStatus(cmd, args[0], func(workspace *workspace, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
		return workspace.actions.SetCollaboratorStatus(commandContext(cmd), args[0], contracts.CollaboratorStatusSuspended, actor, reason)
	})
}

func runCollaboratorRemove(cmd *cobra.Command, args []string) error {
	return mutateCollaboratorStatus(cmd, args[0], func(workspace *workspace, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
		return workspace.actions.SetCollaboratorStatus(commandContext(cmd), args[0], contracts.CollaboratorStatusRemoved, actor, reason)
	})
}

func mutateCollaboratorStatus(cmd *cobra.Command, collaboratorID string, fn func(*workspace, contracts.Actor, string) (contracts.CollaboratorProfile, error)) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	if _, err := fn(workspace, normalizeActor(actorRaw), reason); err != nil {
		return err
	}
	view, err := workspace.queries.CollaboratorDetail(commandContext(cmd), collaboratorID)
	if err != nil {
		return err
	}
	pretty := formatCollaboratorDetail(view)
	data := map[string]any{"kind": "collaborator_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMembershipList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	collaboratorID, _ := cmd.Flags().GetString("collaborator")
	items, err := workspace.queries.ListMemberships(commandContext(cmd), collaboratorID)
	if err != nil {
		return err
	}
	pretty := formatMembershipList(items)
	data := map[string]any{"kind": "membership_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMembershipBind(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	scopeKindRaw, _ := cmd.Flags().GetString("scope-kind")
	scopeID, _ := cmd.Flags().GetString("scope-id")
	roleRaw, _ := cmd.Flags().GetString("role")
	profiles, _ := cmd.Flags().GetStringArray("profile")
	membership, err := workspace.actions.BindMembership(commandContext(cmd), contracts.MembershipBinding{
		CollaboratorID:            strings.TrimSpace(args[0]),
		ScopeKind:                 contracts.MembershipScopeKind(strings.TrimSpace(scopeKindRaw)),
		ScopeID:                   strings.TrimSpace(scopeID),
		Role:                      contracts.MembershipRole(strings.TrimSpace(roleRaw)),
		DefaultPermissionProfiles: profiles,
	}, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	data := map[string]any{"kind": "membership_list", "generated_at": membership.UpdatedAt, "items": []contracts.MembershipBinding{membership}}
	pretty := formatMembershipList([]contracts.MembershipBinding{membership})
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMembershipUnbind(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	membership, err := workspace.actions.UnbindMembership(commandContext(cmd), args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	data := map[string]any{"kind": "membership_list", "generated_at": membership.UpdatedAt, "items": []contracts.MembershipBinding{membership}}
	pretty := formatMembershipList([]contracts.MembershipBinding{membership})
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRemoteList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.ListSyncRemotes(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatRemoteList(items)
	data := map[string]any{"kind": "remote_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRemoteView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.SyncRemoteDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatRemoteDetail(view)
	data := map[string]any{"kind": "remote_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRemoteAdd(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	kindRaw, _ := cmd.Flags().GetString("kind")
	location, _ := cmd.Flags().GetString("location")
	defaultActionRaw, _ := cmd.Flags().GetString("default-action")
	disabled, _ := cmd.Flags().GetBool("disabled")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	remote, err := workspace.actions.AddSyncRemote(commandContext(cmd), contracts.SyncRemote{
		RemoteID:      strings.TrimSpace(args[0]),
		Kind:          contracts.SyncRemoteKind(strings.TrimSpace(kindRaw)),
		Location:      strings.TrimSpace(location),
		Enabled:       !disabled,
		DefaultAction: contracts.SyncDefaultAction(strings.TrimSpace(defaultActionRaw)),
	}, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	view, err := workspace.queries.SyncRemoteDetail(commandContext(cmd), remote.RemoteID)
	if err != nil {
		return err
	}
	pretty := formatRemoteDetail(view)
	data := map[string]any{"kind": "remote_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRemoteEdit(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	kindRaw, _ := cmd.Flags().GetString("kind")
	location, _ := cmd.Flags().GetString("location")
	defaultActionRaw, _ := cmd.Flags().GetString("default-action")
	enabled, _ := cmd.Flags().GetBool("enabled")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	remote, err := workspace.actions.EditSyncRemote(commandContext(cmd), args[0], contracts.SyncRemoteKind(strings.TrimSpace(kindRaw)), strings.TrimSpace(location), contracts.SyncDefaultAction(strings.TrimSpace(defaultActionRaw)), enabled, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	view, err := workspace.queries.SyncRemoteDetail(commandContext(cmd), remote.RemoteID)
	if err != nil {
		return err
	}
	pretty := formatRemoteDetail(view)
	data := map[string]any{"kind": "remote_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRemoteRemove(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	if err := workspace.actions.RemoveSyncRemote(commandContext(cmd), args[0], normalizeActor(actorRaw), reason); err != nil {
		return err
	}
	data := map[string]any{"kind": "remote_detail", "generated_at": time.Now().UTC(), "payload": map[string]any{"remote_id": args[0], "removed": true}}
	return writeCommandOutput(cmd, data, "remote removed", "remote removed")
}

func runSyncStatus(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	remoteID, _ := cmd.Flags().GetString("remote")
	view, err := workspace.queries.SyncStatus(commandContext(cmd), strings.TrimSpace(remoteID))
	if err != nil {
		return err
	}
	pretty := formatSyncStatus(view)
	data := map[string]any{"kind": "sync_status", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runSyncJobs(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	remoteID, _ := cmd.Flags().GetString("remote")
	items, err := workspace.queries.ListSyncJobs(commandContext(cmd), strings.TrimSpace(remoteID))
	if err != nil {
		return err
	}
	pretty := formatSyncJobs(items)
	data := map[string]any{"kind": "sync_job_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runSyncView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.SyncJobDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "sync_job_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runSyncFetch(cmd *cobra.Command, _ []string) error {
	view, err := mutateSyncJob(cmd, func(workspace *workspace, actor contracts.Actor, reason string, remoteID string, sourceWorkspaceID string) (service.SyncJobDetailView, error) {
		return workspace.actions.SyncFetch(commandContext(cmd), remoteID, actor, reason)
	})
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "sync_job_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runSyncPush(cmd *cobra.Command, _ []string) error {
	view, err := mutateSyncJob(cmd, func(workspace *workspace, actor contracts.Actor, reason string, remoteID string, sourceWorkspaceID string) (service.SyncJobDetailView, error) {
		return workspace.actions.SyncPush(commandContext(cmd), remoteID, actor, reason)
	})
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "sync_job_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runSyncPull(cmd *cobra.Command, _ []string) error {
	view, err := mutateSyncJob(cmd, func(workspace *workspace, actor contracts.Actor, reason string, remoteID string, sourceWorkspaceID string) (service.SyncJobDetailView, error) {
		return workspace.actions.SyncPull(commandContext(cmd), remoteID, sourceWorkspaceID, actor, reason)
	})
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "sync_job_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runSyncRun(cmd *cobra.Command, _ []string) error {
	view, err := mutateSyncJob(cmd, func(workspace *workspace, actor contracts.Actor, reason string, remoteID string, sourceWorkspaceID string) (service.SyncJobDetailView, error) {
		return workspace.actions.SyncRun(commandContext(cmd), remoteID, sourceWorkspaceID, actor, reason)
	})
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "sync_job_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runBundleCreate(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.CreateSyncBundle(commandContext(cmd), normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "bundle_create_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runBundleList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.ListBundleJobs(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatSyncJobs(items)
	data := map[string]any{"kind": "bundle_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runBundleView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.BundleDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "bundle_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runBundleVerify(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.VerifySyncBundle(commandContext(cmd), args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatBundleVerify(view)
	data := map[string]any{"kind": "bundle_verify_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runBundleImport(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.ImportSyncBundle(commandContext(cmd), args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatSyncJobDetail(view)
	data := map[string]any{"kind": "bundle_import_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runConflictList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.ListConflicts(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatConflictList(items)
	data := map[string]any{"kind": "conflict_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runConflictView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.ConflictDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatConflictDetail(view)
	data := map[string]any{"kind": "conflict_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runConflictResolve(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	resolutionRaw, _ := cmd.Flags().GetString("resolution")
	resolution := contracts.ConflictResolution(strings.TrimSpace(resolutionRaw))
	view, err := workspace.actions.ResolveConflict(commandContext(cmd), args[0], resolution, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatConflictDetail(view)
	data := map[string]any{"kind": "conflict_resolve_result", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func mutateSyncJob(cmd *cobra.Command, fn func(*workspace, contracts.Actor, string, string, string) (service.SyncJobDetailView, error)) (service.SyncJobDetailView, error) {
	workspace, err := openWorkspace()
	if err != nil {
		return service.SyncJobDetailView{}, err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	remoteID, err := syncRemoteIDForCommand(cmd, workspace)
	if err != nil {
		return service.SyncJobDetailView{}, err
	}
	sourceWorkspaceID, _ := cmd.Flags().GetString("workspace")
	return fn(workspace, normalizeActor(actorRaw), reason, remoteID, strings.TrimSpace(sourceWorkspaceID))
}

func syncRemoteIDForCommand(cmd *cobra.Command, workspace *workspace) (string, error) {
	remoteID, _ := cmd.Flags().GetString("remote")
	remoteID = strings.TrimSpace(remoteID)
	if remoteID != "" {
		return remoteID, nil
	}
	remotes, err := workspace.queries.ListSyncRemotes(commandContext(cmd))
	if err != nil {
		return "", err
	}
	if len(remotes) == 1 {
		return remotes[0].RemoteID, nil
	}
	if len(remotes) == 0 {
		return "", apperr.New(apperr.CodeNotFound, "no sync remotes configured")
	}
	return "", apperr.New(apperr.CodeInvalidInput, "multiple sync remotes configured; specify --remote")
}

func runMentionsList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	collaboratorID, _ := cmd.Flags().GetString("collaborator")
	items, err := workspace.queries.ListMentions(commandContext(cmd), collaboratorID)
	if err != nil {
		return err
	}
	pretty := formatMentionsList(items)
	data := map[string]any{"kind": "mentions_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runProjectCodeownersRender(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.CodeownersPreview(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatCodeownersPreview(view)
	data := map[string]any{"kind": "codeowners_preview", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runProjectCodeownersWrite(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.WriteCodeowners(commandContext(cmd), args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatCodeownersPreview(view)
	data := map[string]any{"kind": "codeowners_preview", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runProjectRulesRender(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.ProviderRulesPreview(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatProviderRulesPreview(view)
	data := map[string]any{"kind": "provider_rules_preview", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runMentionView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.MentionDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatMentionDetail(view)
	data := map[string]any{"kind": "mention_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatCollaboratorList(items []contracts.CollaboratorProfile) string {
	if len(items) == 0 {
		return "no collaborators"
	}
	lines := []string{"collaborators:"}
	for _, item := range items {
		name := item.DisplayName
		if name == "" {
			name = item.CollaboratorID
		}
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s", item.CollaboratorID, item.Status, item.TrustState, name))
	}
	return strings.Join(lines, "\n")
}

func formatCollaboratorDetail(view service.CollaboratorDetailView) string {
	lines := []string{
		fmt.Sprintf("collaborator %s", view.Collaborator.CollaboratorID),
		fmt.Sprintf("status=%s trust=%s", view.Collaborator.Status, view.Collaborator.TrustState),
	}
	if view.Collaborator.DisplayName != "" {
		lines = append(lines, "name="+view.Collaborator.DisplayName)
	}
	if len(view.Collaborator.AtlasActors) > 0 {
		actors := make([]string, 0, len(view.Collaborator.AtlasActors))
		for _, actor := range view.Collaborator.AtlasActors {
			actors = append(actors, string(actor))
		}
		lines = append(lines, "actors="+strings.Join(actors, ","))
	}
	if len(view.Memberships) > 0 {
		lines = append(lines, "", formatMembershipList(view.Memberships))
	}
	if len(view.Mentions) > 0 {
		lines = append(lines, "", formatMentionsList(view.Mentions))
	}
	return strings.Join(lines, "\n")
}

func formatMembershipList(items []contracts.MembershipBinding) string {
	if len(items) == 0 {
		return "no memberships"
	}
	lines := []string{"memberships:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s %s -> %s:%s (%s)", item.MembershipUID, item.CollaboratorID, item.ScopeKind, item.ScopeID, item.Role))
	}
	return strings.Join(lines, "\n")
}

func formatMentionsList(items []contracts.Mention) string {
	if len(items) == 0 {
		return "no mentions"
	}
	lines := []string{"mentions:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s @%s %s %s", item.MentionUID, item.CollaboratorID, item.SourceKind, item.SourceID))
	}
	return strings.Join(lines, "\n")
}

func formatMentionDetail(view service.MentionDetailView) string {
	lines := []string{
		fmt.Sprintf("mention %s", view.Mention.MentionUID),
		fmt.Sprintf("@%s in %s %s", view.Mention.CollaboratorID, view.Mention.SourceKind, view.Mention.SourceID),
	}
	if view.Mention.TicketID != "" {
		lines = append(lines, "ticket="+view.Mention.TicketID)
	}
	if view.Collaborator.CollaboratorID != "" {
		lines = append(lines, fmt.Sprintf("collaborator_status=%s trust=%s", view.Collaborator.Status, view.Collaborator.TrustState))
	}
	return strings.Join(lines, "\n")
}

func formatRemoteList(items []contracts.SyncRemote) string {
	if len(items) == 0 {
		return "no sync remotes"
	}
	lines := []string{"sync remotes:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s kind=%s enabled=%t default=%s", item.RemoteID, item.Kind, item.Enabled, item.DefaultAction))
	}
	return strings.Join(lines, "\n")
}

func formatRemoteDetail(view service.SyncRemoteDetailView) string {
	lines := []string{
		fmt.Sprintf("remote %s", view.Remote.RemoteID),
		fmt.Sprintf("kind=%s enabled=%t default=%s", view.Remote.Kind, view.Remote.Enabled, view.Remote.DefaultAction),
		"location=" + view.Remote.Location,
	}
	if len(view.Publications) > 0 {
		lines = append(lines, "", "publications:")
		for _, publication := range view.Publications {
			lines = append(lines, fmt.Sprintf("- %s bundle=%s files=%d", publication.WorkspaceID, publication.BundleID, publication.FileCount))
		}
	}
	return strings.Join(lines, "\n")
}

func formatSyncStatus(view service.SyncStatusView) string {
	lines := []string{
		"workspace=" + view.WorkspaceID,
		fmt.Sprintf("migration_complete=%t", view.MigrationComplete),
	}
	if len(view.ReasonCodes) > 0 {
		lines = append(lines, "reasons="+strings.Join(view.ReasonCodes, ","))
	}
	if len(view.Remotes) > 0 {
		lines = append(lines, "", "remotes:")
		for _, item := range view.Remotes {
			lines = append(lines, fmt.Sprintf("- %s enabled=%t publications=%d", item.Remote.RemoteID, item.Remote.Enabled, len(item.Publications)))
		}
	}
	return strings.Join(lines, "\n")
}

func formatSyncJobs(items []contracts.SyncJob) string {
	if len(items) == 0 {
		return "no sync jobs"
	}
	lines := []string{"sync jobs:"}
	for _, item := range items {
		line := fmt.Sprintf("- %s mode=%s state=%s", item.JobID, item.Mode, item.State)
		if item.RemoteID != "" {
			line += " remote=" + item.RemoteID
		}
		if item.BundleRef != "" {
			line += " bundle=" + item.BundleRef
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatSyncJobDetail(view service.SyncJobDetailView) string {
	lines := []string{
		fmt.Sprintf("job %s", view.Job.JobID),
		fmt.Sprintf("mode=%s state=%s", view.Job.Mode, view.Job.State),
	}
	if view.Job.RemoteID != "" {
		lines = append(lines, "remote="+view.Job.RemoteID)
	}
	if view.Job.BundleRef != "" {
		lines = append(lines, "bundle="+view.Job.BundleRef)
	}
	if view.Publication.BundleID != "" {
		lines = append(lines, fmt.Sprintf("publication=%s files=%d", view.Publication.BundleID, view.Publication.FileCount))
	}
	if len(view.Job.ReasonCodes) > 0 {
		lines = append(lines, "reasons="+strings.Join(view.Job.ReasonCodes, ","))
	}
	if len(view.Job.Warnings) > 0 {
		lines = append(lines, "warnings="+strings.Join(view.Job.Warnings, ","))
	}
	return strings.Join(lines, "\n")
}

func formatBundleVerify(view service.SyncBundleVerifyView) string {
	line := fmt.Sprintf("bundle verify %s verified=%t", view.BundleRef, view.Verified)
	if len(view.Errors) > 0 {
		line += " errors=" + strings.Join(view.Errors, ",")
	}
	if len(view.Warnings) > 0 {
		line += " warnings=" + strings.Join(view.Warnings, ",")
	}
	return line
}

func formatConflictList(items []contracts.ConflictRecord) string {
	if len(items) == 0 {
		return "no sync conflicts"
	}
	lines := []string{"sync conflicts:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s %s %s [%s]", item.ConflictID, item.EntityKind, item.ConflictType, item.Status))
	}
	return strings.Join(lines, "\n")
}

func formatConflictDetail(view service.ConflictDetailView) string {
	lines := []string{
		fmt.Sprintf("conflict %s", view.Conflict.ConflictID),
		fmt.Sprintf("entity=%s uid=%s", view.Conflict.EntityKind, view.Conflict.EntityUID),
		fmt.Sprintf("type=%s status=%s", view.Conflict.ConflictType, view.Conflict.Status),
	}
	if view.Conflict.LocalRef != "" {
		lines = append(lines, "local_ref="+view.Conflict.LocalRef)
	}
	if view.Conflict.RemoteRef != "" {
		lines = append(lines, "remote_ref="+view.Conflict.RemoteRef)
	}
	if view.Conflict.Resolution != "" {
		lines = append(lines, "resolution="+string(view.Conflict.Resolution))
	}
	return strings.Join(lines, "\n")
}

func formatCodeownersPreview(view service.CodeownersPreviewView) string {
	lines := []string{"codeowners preview for " + view.Project}
	if strings.TrimSpace(view.Path) != "" {
		lines = append(lines, "path="+view.Path)
	}
	lines = append(lines, "", view.Content)
	if len(view.Warnings) > 0 {
		lines = append(lines, "", "warnings:")
		for _, warning := range view.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	return strings.Join(lines, "\n")
}

func formatProviderRulesPreview(view service.ProviderRulesPreviewView) string {
	if len(view.Rules) == 0 {
		if len(view.Warnings) == 0 {
			return "no provider rules preview"
		}
		return "provider rules preview\nwarnings:\n- " + strings.Join(view.Warnings, "\n- ")
	}
	lines := []string{"provider rules preview for " + view.Project}
	for _, rule := range view.Rules {
		lines = append(lines, fmt.Sprintf("- %s paths=%s approvals=%d reviewers=%s", rule.Name, strings.Join(rule.Paths, ","), rule.RequiredApprovals, strings.Join(rule.Reviewers, ",")))
	}
	if len(view.Warnings) > 0 {
		lines = append(lines, "warnings:")
		for _, warning := range view.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	return strings.Join(lines, "\n")
}

func parseCollaboratorActors(values []string) ([]contracts.Actor, error) {
	items := make([]contracts.Actor, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		actor := contracts.Actor(value)
		if !actor.IsValid() {
			return nil, fmt.Errorf("invalid actor: %s", value)
		}
		items = append(items, actor)
	}
	return items, nil
}

func parseProviderHandles(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	items := make(map[string]string, len(values))
	for _, value := range values {
		provider, handle, ok := strings.Cut(strings.TrimSpace(value), ":")
		if !ok {
			return nil, fmt.Errorf("invalid provider handle mapping %q: want provider:handle", value)
		}
		provider = strings.TrimSpace(provider)
		handle = strings.TrimSpace(handle)
		if provider == "" || handle == "" {
			return nil, fmt.Errorf("invalid provider handle mapping %q: want provider:handle", value)
		}
		items[provider] = handle
	}
	return items, nil
}

func notImplementedRead(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		data := map[string]any{
			"kind":         kind,
			"generated_at": time.Now().UTC(),
			"warnings": []map[string]any{{
				"code":    "not_implemented",
				"message": "v1.6 contract frozen; implementation lands in follow-up PRs",
			}},
		}
		if stubUsesPayload(kind) {
			data["payload"] = map[string]any{}
		} else {
			data["items"] = []any{}
		}
		return writeCommandOutput(cmd, data, "# Pending\n\nThis v1.6 command is frozen but not implemented yet.", fmt.Sprintf("%s is frozen but not implemented yet", kind))
	}
}

func stubUsesPayload(kind string) bool {
	switch kind {
	case "collaborator_detail",
		"mention_detail",
		"remote_detail",
		"sync_status",
		"sync_job_detail",
		"bundle_create_result",
		"bundle_detail",
		"bundle_verify_result",
		"bundle_import_result",
		"conflict_detail",
		"conflict_resolve_result",
		"codeowners_preview",
		"provider_rules_preview":
		return true
	default:
		return false
	}
}

func notImplementedMutation(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		_, _ = cmd.Flags().GetBool("json")
		_, _ = cmd.Flags().GetBool("md")
		return apperr.New(apperr.CodeInternal, fmt.Sprintf("%s is frozen but not implemented yet", kind))
	}
}
