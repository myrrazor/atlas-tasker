package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/spf13/cobra"
)

func newKeyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "key", Short: "Manage v1.7 signing keys"}
	generate := v17MutationCommand("generate", "Generate a local signing key", "key_generate_result", cobra.NoArgs)
	generate.Flags().String("scope", "workspace", "Key scope: workspace|collaborator|admin|release")
	cmd.AddCommand(
		v17ReadCommand("list", "List signing keys", "key_list", cobra.NoArgs),
		v17ReadCommand("view <KEY-ID>", "Show one signing key", "key_detail", cobra.ExactArgs(1)),
		generate,
		v17ReadCommand("export-public <KEY-ID>", "Export a public key record", "key_detail", cobra.ExactArgs(1)),
		v17MutationCommand("import-public <PATH>", "Import an untrusted public key record", "key_import_result", cobra.ExactArgs(1)),
		v17MutationCommand("rotate <KEY-ID>", "Rotate a signing key", "key_rotation_result", cobra.ExactArgs(1)),
		v17MutationCommand("revoke <KEY-ID>", "Revoke a signing key", "key_revocation_result", cobra.ExactArgs(1)),
		v17ReadCommand("verify <KEY-ID>", "Verify key health and public material", "key_detail", cobra.ExactArgs(1)),
	)
	return cmd
}

func newTrustCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "trust", Short: "Inspect and bind local v1.7 trust decisions"}
	cmd.AddCommand(
		v17ReadCommand("status", "Show trust store status", "trust_status", cobra.NoArgs),
		v17ReadCommand("list", "List local trust bindings", "trust_list", cobra.NoArgs),
		v17ReadCommand("collaborator <COLLABORATOR-ID>", "Show trust for one collaborator", "trust_collaborator_detail", cobra.ExactArgs(1)),
		v17MutationCommand("bind-key <COLLABORATOR-ID> <PUBLIC-KEY-ID>", "Trust a key for a collaborator", "trust_binding_result", cobra.ExactArgs(2)),
		v17MutationCommand("revoke-key <PUBLIC-KEY-ID>", "Revoke local trust for a key", "trust_revocation_result", cobra.ExactArgs(1)),
		v17ReadCommand("explain <TARGET>", "Explain trust for a target", "trust_explanation", cobra.ExactArgs(1)),
	)
	return cmd
}

func newSignCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "sign", Short: "Sign Atlas artifacts"}
	for _, sub := range []*cobra.Command{
		v17MutationCommand("bundle <BUNDLE-ID>", "Sign an export or sync bundle", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("sync-publication <PUBLICATION-ID>", "Sign a sync publication", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("approval <GATE-ID>", "Sign an approval artifact", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("handoff <HANDOFF-ID>", "Sign a handoff artifact", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("evidence <EVIDENCE-ID>", "Sign an evidence packet", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("audit <AUDIT-REPORT-ID>", "Sign an audit report", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("audit-packet <PACKET-ID>", "Sign an audit packet", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("backup <BACKUP-ID>", "Sign a backup snapshot", "signature_detail", cobra.ExactArgs(1)),
		v17MutationCommand("goal <MANIFEST-ID>", "Sign a goal manifest", "signature_detail", cobra.ExactArgs(1)),
	} {
		sub.Flags().String("signing-key", "", "Signing key id")
		cmd.AddCommand(sub)
	}
	return cmd
}

func newVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "verify", Short: "Verify signed Atlas artifacts without mutating by default"}
	cmd.AddCommand(
		v17ReadCommand("bundle <PATH>", "Verify bundle integrity and signature state", "signature_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("sync-publication <PATH>", "Verify sync publication signature state", "signature_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("approval <GATE-ID>", "Verify approval signature state", "signature_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("handoff <HANDOFF-ID>", "Verify handoff signature state", "signature_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("evidence <EVIDENCE-ID|PATH>", "Verify evidence packet signature state", "evidence_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("audit <REPORT-ID|PATH>", "Verify audit report signature state", "audit_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("audit-packet <PACKET-ID|PATH>", "Verify audit packet signature state", "audit_packet_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("backup <BACKUP-ID|PATH>", "Verify backup signature state", "backup_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("goal <MANIFEST-ID|PATH>", "Verify goal manifest signature state", "goal_manifest_verify_result", cobra.ExactArgs(1)),
	)
	return cmd
}

func newGovernanceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "governance", Short: "Manage v1.7 governance policy packs"}
	pack := &cobra.Command{Use: "pack", Short: "Manage governance packs"}
	packApply := v17MutationCommand("apply <PACK-ID>", "Apply a governance pack to a scope", "governance_pack_detail", cobra.ExactArgs(1))
	packApply.Flags().String("scope", "", "Policy scope, e.g. project:APP")
	pack.AddCommand(
		v17ReadCommand("list", "List governance packs", "governance_pack_list", cobra.NoArgs),
		v17ReadCommand("view <PACK-ID>", "Show one governance pack", "governance_pack_detail", cobra.ExactArgs(1)),
		v17MutationCommand("create <NAME>", "Create a governance pack", "governance_pack_detail", cobra.ExactArgs(1)),
		packApply,
	)
	cmd.AddCommand(pack)
	simulate := v17ReadCommand("simulate <ACTION>", "Simulate a protected action", "governance_simulation_result", cobra.ExactArgs(1))
	simulate.Flags().String("actor", "", "Actor to simulate")
	simulate.Flags().String("ticket", "", "Ticket id")
	simulate.Flags().String("run", "", "Run id")
	simulate.Flags().String("change", "", "Change id")
	cmd.AddCommand(
		v17ReadCommand("validate", "Validate governance packs", "governance_validation_result", cobra.NoArgs),
		v17ReadCommand("explain <TARGET>", "Explain governance for a target", "governance_explanation", cobra.ExactArgs(1)),
		simulate,
	)
	return cmd
}

func newClassifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "classify", Short: "Manage classification labels"}
	list := v17ReadCommand("list", "List classification labels", "classification_list", cobra.NoArgs)
	list.Flags().String("project", "", "Filter by project")
	cmd.AddCommand(
		v17ReadCommand("get <ENTITY>", "Show classification for an entity", "classification_detail", cobra.ExactArgs(1)),
		v17MutationCommand("set <ENTITY> <LEVEL>", "Set classification for an entity", "classification_detail", cobra.ExactArgs(2)),
		list,
		v17ReadCommand("explain <ENTITY>", "Explain inherited classification", "classification_detail", cobra.ExactArgs(1)),
	)
	return cmd
}

func newRedactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "redact", Short: "Preview and create redacted artifacts"}
	preview := v17MutationCommand("preview", "Create an actor-bound redaction preview", "redaction_preview", cobra.NoArgs)
	preview.Flags().String("scope", "", "Scope to preview")
	preview.Flags().String("target", "", "Target: export|sync|audit|backup|goal")
	export := v17MutationCommand("export", "Create a redacted export artifact", "redaction_export_result", cobra.NoArgs)
	export.Flags().String("scope", "", "Scope to export")
	cmd.AddCommand(preview, export, v17ReadCommand("verify <ARTIFACT>", "Verify a redacted artifact", "redaction_verify_result", cobra.ExactArgs(1)))
	return cmd
}

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "audit", Short: "Generate and verify v1.7 audit reports"}
	report := v17MutationCommand("report", "Create an audit report snapshot", "audit_report_detail", cobra.NoArgs)
	report.Flags().String("scope", "", "Audit scope")
	cmd.AddCommand(
		report,
		v17ReadCommand("list", "List audit reports", "audit_report_list", cobra.NoArgs),
		v17ReadCommand("view <REPORT-ID>", "Show one audit report", "audit_report_detail", cobra.ExactArgs(1)),
		v17MutationCommand("export <REPORT-ID>", "Export an audit packet", "audit_report_export_result", cobra.ExactArgs(1)),
		v17ReadCommand("verify <REPORT-ID|PATH>", "Verify an audit report or packet", "audit_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("explain-policy <EVENT-ID|ACTION-ID>", "Explain policy decision provenance", "governance_explanation", cobra.ExactArgs(1)),
	)
	return cmd
}

func newBackupCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "backup", Short: "Create and restore Atlas-owned backups"}
	create := v17MutationCommand("create", "Create a backup snapshot", "backup_detail", cobra.NoArgs)
	create.Flags().String("scope", "workspace", "Backup scope: workspace|project:<KEY>")
	restoreApply := v17MutationCommand("restore-apply <BACKUP-ID|PATH>", "Apply a backup restore plan", "backup_restore_result", cobra.ExactArgs(1))
	restoreApply.Flags().Bool("yes", false, "Apply restore without prompting")
	cmd.AddCommand(
		create,
		v17ReadCommand("list", "List backup snapshots", "backup_list", cobra.NoArgs),
		v17ReadCommand("view <BACKUP-ID>", "Show one backup snapshot", "backup_detail", cobra.ExactArgs(1)),
		v17ReadCommand("verify <BACKUP-ID|PATH>", "Verify a backup snapshot", "backup_verify_result", cobra.ExactArgs(1)),
		v17ReadCommand("restore-plan <BACKUP-ID|PATH>", "Preview a backup restore", "backup_restore_plan", cobra.ExactArgs(1)),
		restoreApply,
		v17ReadCommand("drill", "Run a read-only recovery drill", "recovery_drill_result", cobra.NoArgs),
	)
	return cmd
}

func newAdminCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Admin diagnostics"}
	cmd.AddCommand(
		v17ReadCommand("security-status", "Show v1.7 security status", "admin_security_status", cobra.NoArgs),
		v17ReadCommand("trust-store", "Inspect trust store health", "trust_store_status", cobra.NoArgs),
		v17ReadCommand("recovery-status", "Inspect recovery readiness", "recovery_status", cobra.NoArgs),
	)
	return cmd
}

func newGoalCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "goal", Short: "Generate read-only agent goal briefs and manifests"}
	cmd.AddCommand(
		v17ReadCommand("brief <TICKET-ID|RUN-ID>", "Render a goal-ready brief", "goal_brief", cobra.ExactArgs(1)),
		v17ReadCommand("manifest <TICKET-ID|RUN-ID>", "Render a goal-ready manifest", "goal_manifest", cobra.ExactArgs(1)),
		v17ReadCommand("verify <MANIFEST-ID|PATH>", "Verify a signed goal manifest", "goal_manifest_verify_result", cobra.ExactArgs(1)),
	)
	return cmd
}

func v17ReadCommand(use string, short string, kind string, args cobra.PositionalArgs) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args, RunE: v17PendingRead(kind, v17ReadFailsClosed(use, kind))}
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func v17MutationCommand(use string, short string, kind string, args cobra.PositionalArgs) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args, RunE: v17PendingMutation(kind)}
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func v17ReadFailsClosed(use string, kind string) bool {
	verb := strings.Fields(strings.TrimSpace(use))
	if len(verb) > 0 && (verb[0] == "verify" || verb[0] == "validate" || verb[0] == "drill") {
		return true
	}
	return strings.Contains(kind, "verify") || strings.Contains(kind, "validation") || strings.Contains(kind, "drill")
}

func v17PendingRead(kind string, failClosed bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		message := fmt.Sprintf("%s is frozen for v1.7 follow-up implementation", kind)
		if failClosed {
			return apperr.New(apperr.CodeInternal, message)
		}
		data := map[string]any{
			"kind":         kind,
			"generated_at": time.Now().UTC(),
			"items":        []any{},
			"warnings": []map[string]any{{
				"code":    "v1_7_contract_only",
				"message": "v1.7 command contract is frozen; behavior lands in follow-up PRs",
			}},
		}
		if err := writeCommandOutput(cmd, data, "# Pending\n\nThis v1.7 command contract is frozen; behavior lands in follow-up PRs.", message); err != nil {
			return err
		}
		return nil
	}
}

func v17PendingMutation(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		actor, _ := cmd.Flags().GetString("actor")
		reason, _ := cmd.Flags().GetString("reason")
		if !contracts.Actor(strings.TrimSpace(actor)).IsValid() || strings.TrimSpace(reason) == "" {
			return apperr.New(apperr.CodeInvalidInput, "v1.7 protected mutations require a valid --actor and non-empty --reason")
		}
		return apperr.New(apperr.CodeInternal, fmt.Sprintf("%s is frozen for v1.7 follow-up implementation", kind))
	}
}
