package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/spf13/cobra"
)

func newCollaboratorCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "collaborator", Short: "Manage collaborators and trust state"}
	for _, sub := range []*cobra.Command{
		{Use: "list", Short: "List collaborators", RunE: notImplementedRead("collaborator_list")},
		{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View collaborator details", RunE: notImplementedRead("collaborator_detail")},
		{Use: "add", Short: "Add a collaborator", RunE: notImplementedMutation("collaborator_detail")},
		{Use: "edit <ID>", Args: cobra.ExactArgs(1), Short: "Edit collaborator metadata", RunE: notImplementedMutation("collaborator_detail")},
		{Use: "trust <ID>", Args: cobra.ExactArgs(1), Short: "Mark a collaborator as trusted", RunE: notImplementedMutation("collaborator_detail")},
		{Use: "suspend <ID>", Args: cobra.ExactArgs(1), Short: "Suspend a collaborator", RunE: notImplementedMutation("collaborator_detail")},
		{Use: "remove <ID>", Args: cobra.ExactArgs(1), Short: "Tombstone a collaborator", RunE: notImplementedMutation("collaborator_detail")},
	} {
		if strings.Contains(sub.Use, "list") || strings.Contains(sub.Use, "view") {
			addReadOutputFlags(sub, &outputFlags{})
		} else {
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
			addReadOutputFlags(sub, &outputFlags{})
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func newMembershipCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "membership", Short: "Manage collaborator memberships"}
	list := &cobra.Command{Use: "list", Short: "List memberships", RunE: notImplementedRead("membership_list")}
	bind := &cobra.Command{Use: "bind", Short: "Bind a collaborator to a scope", RunE: notImplementedMutation("membership_list")}
	unbind := &cobra.Command{Use: "unbind", Short: "Unbind a collaborator membership", RunE: notImplementedMutation("membership_list")}
	for _, sub := range []*cobra.Command{list, bind, unbind} {
		if sub == list {
			addReadOutputFlags(sub, &outputFlags{})
		} else {
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
			addReadOutputFlags(sub, &outputFlags{})
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func newRemoteCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "remote", Short: "Manage Atlas sync remotes"}
	for _, sub := range []*cobra.Command{
		{Use: "list", Short: "List sync remotes", RunE: notImplementedRead("remote_list")},
		{Use: "view <ID>", Args: cobra.ExactArgs(1), Short: "View sync remote details", RunE: notImplementedRead("remote_detail")},
		{Use: "add", Short: "Add a sync remote", RunE: notImplementedMutation("remote_detail")},
		{Use: "edit <ID>", Args: cobra.ExactArgs(1), Short: "Edit a sync remote", RunE: notImplementedMutation("remote_detail")},
		{Use: "remove <ID>", Args: cobra.ExactArgs(1), Short: "Remove a sync remote", RunE: notImplementedMutation("remote_detail")},
	} {
		if strings.HasPrefix(sub.Use, "list") || strings.HasPrefix(sub.Use, "view") {
			addReadOutputFlags(sub, &outputFlags{})
		} else {
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
			addReadOutputFlags(sub, &outputFlags{})
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func newSyncCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Inspect and run Atlas sync jobs"}
	contracts := []struct {
		use  string
		kind string
		mut  bool
		args cobra.PositionalArgs
	}{
		{use: "status", kind: "sync_status"},
		{use: "jobs", kind: "sync_job_list"},
		{use: "view <JOB-ID>", kind: "sync_job_detail", args: cobra.ExactArgs(1)},
		{use: "fetch", kind: "sync_status", mut: true},
		{use: "pull", kind: "sync_status", mut: true},
		{use: "push", kind: "sync_status", mut: true},
		{use: "run", kind: "sync_status", mut: true},
	}
	for _, item := range contracts {
		sub := &cobra.Command{Use: item.use, Short: "v1.6 sync command", Args: item.args}
		if item.mut {
			sub.RunE = notImplementedMutation(item.kind)
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		} else {
			sub.RunE = notImplementedRead(item.kind)
		}
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newBundleCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "bundle", Short: "Create, verify, and import Atlas sync bundles"}
	for _, item := range []struct {
		use  string
		kind string
		mut  bool
		args cobra.PositionalArgs
	}{
		{use: "create", kind: "bundle_create_result", mut: true},
		{use: "list", kind: "bundle_list"},
		{use: "view <ID>", kind: "bundle_detail", args: cobra.ExactArgs(1)},
		{use: "verify <ID>", kind: "bundle_verify_result", mut: true, args: cobra.ExactArgs(1)},
		{use: "import", kind: "bundle_import_result", mut: true},
	} {
		sub := &cobra.Command{Use: item.use, Short: "v1.6 bundle command", Args: item.args}
		if item.mut {
			sub.RunE = notImplementedMutation(item.kind)
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		} else {
			sub.RunE = notImplementedRead(item.kind)
		}
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newConflictCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "conflict", Short: "Inspect and resolve sync conflicts"}
	for _, item := range []struct {
		use  string
		kind string
		mut  bool
		args cobra.PositionalArgs
	}{
		{use: "list", kind: "conflict_list"},
		{use: "view <ID>", kind: "conflict_detail", args: cobra.ExactArgs(1)},
		{use: "resolve <ID>", kind: "conflict_resolve_result", mut: true, args: cobra.ExactArgs(1)},
	} {
		sub := &cobra.Command{Use: item.use, Short: "v1.6 conflict command", Args: item.args}
		if item.mut {
			sub.RunE = notImplementedMutation(item.kind)
			addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		} else {
			sub.RunE = notImplementedRead(item.kind)
		}
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newMentionsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mentions", Short: "Inspect collaborator mentions"}
	list := &cobra.Command{Use: "list", Short: "List mentions", RunE: notImplementedRead("mentions_list")}
	view := &cobra.Command{Use: "view <MENTION-UID>", Args: cobra.ExactArgs(1), Short: "View a mention", RunE: notImplementedRead("mention_detail")}
	for _, sub := range []*cobra.Command{list, view} {
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	return cmd
}

func newProjectCodeownersCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "codeowners", Short: "Preview or write CODEOWNERS scaffolding"}
	render := &cobra.Command{Use: "render <KEY>", Args: cobra.ExactArgs(1), Short: "Render CODEOWNERS preview", RunE: notImplementedRead("codeowners_preview")}
	write := &cobra.Command{Use: "write <KEY>", Args: cobra.ExactArgs(1), Short: "Write CODEOWNERS scaffolding", RunE: notImplementedMutation("codeowners_preview")}
	addReadOutputFlags(render, &outputFlags{})
	addMutationFlags(write, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(write, &outputFlags{})
	cmd.AddCommand(render, write)
	return cmd
}

func newProjectRulesCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "rules", Short: "Render provider rules scaffolding"}
	render := &cobra.Command{Use: "render <KEY>", Args: cobra.ExactArgs(1), Short: "Render provider rules preview", RunE: notImplementedRead("provider_rules_preview")}
	addReadOutputFlags(render, &outputFlags{})
	cmd.AddCommand(render)
	return cmd
}

func notImplementedRead(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{
			"kind":         kind,
			"generated_at": time.Now().UTC(),
			"warnings": []map[string]any{{
				"code":    "not_implemented",
				"message": "v1.6 contract frozen; implementation lands in follow-up PRs",
			}},
			"items": []any{},
		}
		return writeCommandOutput(cmd, payload, "# Pending\n\nThis v1.6 command is frozen but not implemented yet.", fmt.Sprintf("%s is frozen but not implemented yet", kind))
	}
}

func notImplementedMutation(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")
		mdMode, _ := cmd.Flags().GetBool("md")
		if jsonMode {
			return writeCommandOutput(cmd, map[string]any{
				"ok":             false,
				"kind":           kind,
				"generated_at":   time.Now().UTC(),
				"format_version": jsonFormatVersion,
				"error": map[string]any{
					"code":    apperr.CodeInternal,
					"message": "v1.6 command is frozen but not implemented yet",
				},
			}, "", "")
		}
		if mdMode {
			fmt.Fprintln(cmd.OutOrStdout(), "# Pending\n\nThis v1.6 mutating command is frozen but not implemented yet.")
			return apperr.New(apperr.CodeInternal, "v1.6 command is frozen but not implemented yet")
		}
		return apperr.New(apperr.CodeInternal, "v1.6 command is frozen but not implemented yet")
	}
}
