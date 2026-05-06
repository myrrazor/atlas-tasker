package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newKeyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "key", Short: "Manage v1.7 signing keys"}
	list := &cobra.Command{Use: "list", Short: "List signing keys", Args: cobra.NoArgs, RunE: runKeyList}
	view := &cobra.Command{Use: "view <KEY-ID>", Short: "Show one signing key", Args: cobra.ExactArgs(1), RunE: runKeyView}
	generate := &cobra.Command{Use: "generate", Short: "Generate a local signing key", Args: cobra.NoArgs, RunE: runKeyGenerate}
	generate.Flags().String("scope", "workspace", "Key scope: workspace|collaborator|admin|release")
	generate.Flags().String("owner-id", "", "Explicit key owner id")
	exportPublic := &cobra.Command{Use: "export-public <KEY-ID>", Short: "Export a public key record", Args: cobra.ExactArgs(1), RunE: runKeyExportPublic}
	importPublic := &cobra.Command{Use: "import-public <PATH>", Short: "Import an untrusted public key record", Args: cobra.ExactArgs(1), RunE: runKeyImportPublic}
	rotate := &cobra.Command{Use: "rotate <KEY-ID>", Short: "Rotate a signing key", Args: cobra.ExactArgs(1), RunE: runKeyRotate}
	revoke := &cobra.Command{Use: "revoke <KEY-ID>", Short: "Revoke a signing key", Args: cobra.ExactArgs(1), RunE: runKeyRevoke}
	verify := &cobra.Command{Use: "verify <KEY-ID>", Short: "Verify key health and public material", Args: cobra.ExactArgs(1), RunE: runKeyView}
	for _, sub := range []*cobra.Command{list, view, exportPublic, verify} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{generate, importPublic, rotate, revoke} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
	}
	cmd.AddCommand(
		list,
		view,
		generate,
		exportPublic,
		importPublic,
		rotate,
		revoke,
		verify,
	)
	return cmd
}

func newTrustCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "trust", Short: "Inspect and bind local v1.7 trust decisions"}
	status := &cobra.Command{Use: "status", Short: "Show trust store status", Args: cobra.NoArgs, RunE: runTrustStatus}
	list := &cobra.Command{Use: "list", Short: "List local trust bindings", Args: cobra.NoArgs, RunE: runTrustList}
	collaborator := &cobra.Command{Use: "collaborator <COLLABORATOR-ID>", Short: "Show trust for one collaborator", Args: cobra.ExactArgs(1), RunE: runTrustCollaborator}
	bindKey := &cobra.Command{Use: "bind-key <COLLABORATOR-ID> <PUBLIC-KEY-ID>", Short: "Trust a key for a collaborator", Args: cobra.ExactArgs(2), RunE: runTrustBindKey}
	revokeKey := &cobra.Command{Use: "revoke-key <PUBLIC-KEY-ID>", Short: "Revoke local trust for a key", Args: cobra.ExactArgs(1), RunE: runTrustRevokeKey}
	explain := &cobra.Command{Use: "explain <TARGET>", Short: "Explain trust for a target", Args: cobra.ExactArgs(1), RunE: runTrustExplain}
	for _, sub := range []*cobra.Command{status, list, collaborator, explain} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{bindKey, revokeKey} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
	}
	cmd.AddCommand(
		status,
		list,
		collaborator,
		bindKey,
		revokeKey,
		explain,
	)
	return cmd
}

func newSignCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "sign", Short: "Sign Atlas artifacts"}
	bundle := &cobra.Command{Use: "bundle <BUNDLE-ID>", Short: "Sign an export bundle", Args: cobra.ExactArgs(1), RunE: runSignBundle}
	syncPublication := &cobra.Command{Use: "sync-publication <BUNDLE-ID|PATH>", Short: "Sign a sync publication", Args: cobra.ExactArgs(1), RunE: runSignSyncPublication}
	for _, sub := range []*cobra.Command{bundle, syncPublication} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
		sub.Flags().String("signing-key", "", "Signing key id")
		cmd.AddCommand(sub)
	}
	for _, sub := range []*cobra.Command{
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
	bundle := &cobra.Command{Use: "bundle <BUNDLE-ID|PATH>", Short: "Verify bundle integrity and signature state", Args: cobra.ExactArgs(1), RunE: runVerifyBundleSignature}
	syncPublication := &cobra.Command{Use: "sync-publication <BUNDLE-ID|PATH>", Short: "Verify sync publication signature state", Args: cobra.ExactArgs(1), RunE: runVerifySyncPublicationSignature}
	for _, sub := range []*cobra.Command{bundle, syncPublication} {
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
	cmd.AddCommand(
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
	packList := &cobra.Command{Use: "list", Short: "List governance packs", Args: cobra.NoArgs, RunE: runGovernancePackList}
	packView := &cobra.Command{Use: "view <PACK-ID>", Short: "Show one governance pack", Args: cobra.ExactArgs(1), RunE: runGovernancePackView}
	packCreate := &cobra.Command{Use: "create <NAME>", Short: "Create a governance pack", Args: cobra.ExactArgs(1), RunE: runGovernancePackCreate}
	packCreate.Flags().String("policy-id", "", "Policy id; defaults to the pack name")
	packCreate.Flags().String("scope", "workspace", "Policy scope: workspace|project:<KEY>|runbook:<NAME>|ticket_type:<TYPE>|classification:<LEVEL>")
	packCreate.Flags().StringArray("protected-action", nil, "Protected action; repeat to protect multiple actions")
	packCreate.Flags().Int("required-signatures", 0, "Trusted signatures required before the action can proceed")
	packCreate.Flags().Int("quorum-count", 0, "Required approver count for each protected action")
	packCreate.Flags().StringArray("quorum-role", nil, "Allowed quorum membership role; repeat to allow multiple roles")
	packCreate.Flags().StringArray("separation-event", nil, "Prior event type that current actor cannot also own")
	packCreate.Flags().Bool("allow-owner-override", false, "Allow human:owner to override quorum/separation denials")
	packCreate.Flags().Bool("require-override-reason", true, "Require a non-empty reason for owner override")
	packCreate.Flags().Bool("require-trusted-signature", false, "Require trusted signature evidence for quorum/override rules")
	packApply := &cobra.Command{Use: "apply <PACK-ID>", Short: "Apply a governance pack to a scope", Args: cobra.ExactArgs(1), RunE: runGovernancePackApply}
	packApply.Flags().String("scope", "", "Policy scope, e.g. project:APP")
	for _, sub := range []*cobra.Command{packList, packView} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{packCreate, packApply} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
	}
	pack.AddCommand(
		packList,
		packView,
		packCreate,
		packApply,
	)
	cmd.AddCommand(pack)
	validate := &cobra.Command{Use: "validate", Short: "Validate governance packs", Args: cobra.NoArgs, RunE: runGovernanceValidate}
	explain := &cobra.Command{Use: "explain <TARGET>", Short: "Explain governance for a target", Args: cobra.ExactArgs(1), RunE: runGovernanceExplain}
	explain.Flags().String("action", string(contracts.ProtectedActionTicketComplete), "Protected action to explain")
	explain.Flags().String("actor", "human:owner", "Actor to explain")
	explain.Flags().String("reason", "", "Reason to include in the evaluation")
	explain.Flags().StringArray("approval-actor", nil, "Approval actor to include in the evaluation")
	explain.Flags().Int("trusted-signatures", 0, "Trusted signature count to include in the evaluation")
	simulate := &cobra.Command{Use: "simulate <ACTION>", Short: "Simulate a protected action", Args: cobra.ExactArgs(1), RunE: runGovernanceSimulate}
	simulate.Flags().String("actor", "", "Actor to simulate")
	simulate.Flags().String("reason", "", "Reason to include in the evaluation")
	simulate.Flags().String("ticket", "", "Ticket id")
	simulate.Flags().String("run", "", "Run id")
	simulate.Flags().String("change", "", "Change id")
	simulate.Flags().String("gate", "", "Gate id")
	simulate.Flags().StringArray("approval-actor", nil, "Approval actor to include in the evaluation")
	simulate.Flags().Int("trusted-signatures", 0, "Trusted signature count to include in the evaluation")
	for _, sub := range []*cobra.Command{validate, explain, simulate} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	cmd.AddCommand(validate, explain, simulate)
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

func runKeyGenerate(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	scopeRaw, _ := cmd.Flags().GetString("scope")
	ownerID, _ := cmd.Flags().GetString("owner-id")
	view, err := w.actions.GenerateKey(cmd.Context(), service.KeyGenerateOptions{
		Scope:   contracts.KeyScope(strings.TrimSpace(scopeRaw)),
		OwnerID: strings.TrimSpace(ownerID),
	}, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, keyDetailMarkdown(view), keyDetailPretty(view))
}

func runKeyList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ListKeys(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, keyListMarkdown(view), keyListPretty(view))
}

func runKeyView(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.KeyDetail(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, keyDetailMarkdown(view), keyDetailPretty(view))
}

func runKeyExportPublic(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.KeyDetail(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	exportDoc, err := publicKeyExportDocument(view.PublicKey)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view.PublicKey, exportDoc, exportDoc)
}

func runKeyImportPublic(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.ImportPublicKey(cmd.Context(), args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, keyDetailMarkdown(view), keyDetailPretty(view))
}

func runKeyRotate(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.RotateKey(cmd.Context(), args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, keyDetailMarkdown(view), keyDetailPretty(view))
}

func runKeyRevoke(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.RevokeKey(cmd.Context(), args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, keyDetailMarkdown(view), keyDetailPretty(view))
}

func runTrustStatus(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.TrustStatus(cmd.Context())
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("public keys: %d\ntrusted bindings: %d\nrevoked bindings: %d\nimported untrusted keys: %d", view.PublicKeys, view.TrustedBindings, view.RevokedBindings, view.ImportedUntrusted)
	return writeCommandOutput(cmd, view, pretty, pretty)
}

func runTrustList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ListTrust(cmd.Context(), "")
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, trustListMarkdown(view), trustListPretty(view))
}

func runTrustCollaborator(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ListTrust(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, trustListMarkdown(view), trustListPretty(view))
}

func runTrustBindKey(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	binding, err := w.actions.BindTrust(cmd.Context(), args[0], args[1], actor, reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("trusted %s for %s", binding.PublicKeyID, binding.TrustedOwnerID)
	return writeCommandOutput(cmd, map[string]any{"kind": "trust_binding_result", "generated_at": time.Now().UTC(), "binding": binding}, pretty, pretty)
}

func runTrustRevokeKey(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.RevokeTrustForKey(cmd.Context(), args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, trustListMarkdown(view), trustListPretty(view))
}

func runTrustExplain(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ExplainTrust(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("%s: %d trust bindings", view.Target, len(view.Bindings))
	if len(view.ReasonCodes) > 0 {
		pretty += "\n" + strings.Join(view.ReasonCodes, "\n")
	}
	return writeCommandOutput(cmd, view, pretty, pretty)
}

func runSignBundle(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignExportBundle(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runSignSyncPublication(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignSyncPublication(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runVerifyBundleSignature(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyExportBundleSignature(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runVerifySyncPublicationSignature(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifySyncPublicationSignature(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runGovernancePackCreate(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	opts, err := governancePackCreateOptionsFromFlags(cmd, args[0])
	if err != nil {
		return err
	}
	view, err := w.actions.CreateGovernancePack(cmd.Context(), opts, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, governancePackMarkdown(view), governancePackPretty(view))
}

func runGovernancePackApply(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	view, err := w.actions.ApplyGovernancePack(cmd.Context(), args[0], scope, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, governancePackMarkdown(view), governancePackPretty(view))
}

func runGovernancePackList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ListGovernancePacks(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, governancePackListMarkdown(view), governancePackListPretty(view))
}

func runGovernancePackView(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.GovernancePackDetail(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, governancePackMarkdown(view), governancePackPretty(view))
}

func runGovernanceValidate(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ValidateGovernance(cmd.Context())
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("governance valid=%t policies=%d packs=%d", view.Valid, view.Policies, view.Packs)
	if len(view.Errors) > 0 {
		pretty += "\n" + strings.Join(view.Errors, "\n")
	}
	if err := writeCommandOutput(cmd, view, governanceValidationMarkdown(view), pretty); err != nil {
		return err
	}
	if !view.Valid {
		return apperr.New(apperr.CodeConflict, "governance validation failed")
	}
	return nil
}

func runGovernanceExplain(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actionRaw, _ := cmd.Flags().GetString("action")
	actorRaw, _ := cmd.Flags().GetString("actor")
	input, err := governanceEvaluationInputFromFlags(cmd, contracts.ProtectedAction(strings.TrimSpace(actionRaw)), contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	input.Target = args[0]
	view, err := w.actions.ExplainGovernance(cmd.Context(), input)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, governanceExplanationMarkdown(view), governanceExplanationPretty(view))
}

func runGovernanceSimulate(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	if strings.TrimSpace(actorRaw) == "" {
		actorRaw = "human:owner"
	}
	input, err := governanceEvaluationInputFromFlags(cmd, contracts.ProtectedAction(strings.TrimSpace(args[0])), contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	view, err := w.actions.SimulateGovernance(cmd.Context(), input)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, governanceExplanationMarkdown(view.Explanation), governanceExplanationPretty(view.Explanation))
}

func mutationActorReason(cmd *cobra.Command) (contracts.Actor, string) {
	actor, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	return contracts.Actor(strings.TrimSpace(actor)), strings.TrimSpace(reason)
}

func signatureDetailPretty(view service.SignatureDetailView) string {
	return fmt.Sprintf("%s signed %s %s with %s", view.Signature.SignatureID, view.ArtifactKind, view.ArtifactUID, view.Signature.PublicKeyID)
}

func signatureDetailMarkdown(view service.SignatureDetailView) string {
	return fmt.Sprintf("# Signature\n\n- Signature: `%s`\n- Artifact: `%s` `%s`\n- Key: `%s`\n", view.Signature.SignatureID, view.ArtifactKind, view.ArtifactUID, view.Signature.PublicKeyID)
}

func signatureVerifyPretty(view service.ArtifactSignatureVerifyView) string {
	pretty := fmt.Sprintf("%s %s", view.Kind, view.Signature.State)
	if present, verified, errors, warnings := signatureIntegrityStatus(view.Integrity); present {
		pretty += fmt.Sprintf(" integrity=%t", verified)
		if len(errors) > 0 {
			pretty += " errors=" + strings.Join(errors, ",")
		}
		if len(warnings) > 0 {
			pretty += " warnings=" + strings.Join(warnings, ",")
		}
	}
	return pretty
}

func signatureVerifyMarkdown(view service.ArtifactSignatureVerifyView) string {
	lines := []string{
		"# Signature Verification",
		"",
		fmt.Sprintf("- State: `%s`", view.Signature.State),
		fmt.Sprintf("- Artifact: `%s` `%s`", view.Signature.ArtifactKind, view.Signature.ArtifactUID),
	}
	if present, verified, errors, warnings := signatureIntegrityStatus(view.Integrity); present {
		lines = append(lines, fmt.Sprintf("- Integrity verified: `%t`", verified))
		for _, item := range errors {
			lines = append(lines, fmt.Sprintf("- Integrity error: `%s`", item))
		}
		for _, item := range warnings {
			lines = append(lines, fmt.Sprintf("- Integrity warning: `%s`", item))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func signatureIntegrityStatus(integrity any) (bool, bool, []string, []string) {
	switch view := integrity.(type) {
	case service.ExportVerifyView:
		return true, view.Verified, append([]string{}, view.Errors...), append([]string{}, view.Warnings...)
	case service.SyncBundleVerifyView:
		return true, view.Verified, append([]string{}, view.Errors...), append([]string{}, view.Warnings...)
	default:
		return false, false, nil, nil
	}
}

func governancePackCreateOptionsFromFlags(cmd *cobra.Command, name string) (service.GovernancePackCreateOptions, error) {
	policyID, _ := cmd.Flags().GetString("policy-id")
	scope, _ := cmd.Flags().GetString("scope")
	actionRaw, _ := cmd.Flags().GetStringArray("protected-action")
	actions := make([]contracts.ProtectedAction, 0, len(actionRaw))
	for _, raw := range actionRaw {
		for _, item := range strings.Split(raw, ",") {
			action := contracts.ProtectedAction(strings.TrimSpace(item))
			if action == "" {
				continue
			}
			if !action.IsValid() {
				return service.GovernancePackCreateOptions{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid protected action: %s", action))
			}
			actions = append(actions, action)
		}
	}
	roleRaw, _ := cmd.Flags().GetStringArray("quorum-role")
	roles := make([]contracts.MembershipRole, 0, len(roleRaw))
	for _, raw := range roleRaw {
		for _, item := range strings.Split(raw, ",") {
			role := contracts.MembershipRole(strings.TrimSpace(item))
			if role == "" {
				continue
			}
			if !role.IsValid() {
				return service.GovernancePackCreateOptions{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid quorum role: %s", role))
			}
			roles = append(roles, role)
		}
	}
	eventRaw, _ := cmd.Flags().GetStringArray("separation-event")
	events := make([]contracts.EventType, 0, len(eventRaw))
	for _, raw := range eventRaw {
		for _, item := range strings.Split(raw, ",") {
			eventType := contracts.EventType(strings.TrimSpace(item))
			if eventType == "" {
				continue
			}
			if !eventType.IsValid() {
				return service.GovernancePackCreateOptions{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid separation event: %s", eventType))
			}
			events = append(events, eventType)
		}
	}
	requiredSignatures, _ := cmd.Flags().GetInt("required-signatures")
	quorumCount, _ := cmd.Flags().GetInt("quorum-count")
	allowOverride, _ := cmd.Flags().GetBool("allow-owner-override")
	requireOverrideReason, _ := cmd.Flags().GetBool("require-override-reason")
	requireTrustedSignature, _ := cmd.Flags().GetBool("require-trusted-signature")
	return service.GovernancePackCreateOptions{
		Name:                    strings.TrimSpace(name),
		PolicyID:                strings.TrimSpace(policyID),
		Scope:                   strings.TrimSpace(scope),
		ProtectedActions:        actions,
		RequiredSignatures:      requiredSignatures,
		QuorumCount:             quorumCount,
		QuorumRoles:             roles,
		SeparationEventTypes:    events,
		AllowOwnerOverride:      allowOverride,
		RequireOverrideReason:   requireOverrideReason,
		RequireTrustedSignature: requireTrustedSignature,
	}, nil
}

func governanceEvaluationInputFromFlags(cmd *cobra.Command, action contracts.ProtectedAction, actor contracts.Actor) (service.GovernanceEvaluationInput, error) {
	if !action.IsValid() {
		return service.GovernanceEvaluationInput{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid protected action: %s", action))
	}
	if !actor.IsValid() {
		return service.GovernanceEvaluationInput{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
	}
	ticketID, _ := cmd.Flags().GetString("ticket")
	runID, _ := cmd.Flags().GetString("run")
	changeID, _ := cmd.Flags().GetString("change")
	gateID, _ := cmd.Flags().GetString("gate")
	reason, _ := cmd.Flags().GetString("reason")
	approvalRaw, _ := cmd.Flags().GetStringArray("approval-actor")
	approvals := make([]contracts.Actor, 0, len(approvalRaw))
	for _, raw := range approvalRaw {
		for _, item := range strings.Split(raw, ",") {
			approval := contracts.Actor(strings.TrimSpace(item))
			if approval == "" {
				continue
			}
			if !approval.IsValid() {
				return service.GovernanceEvaluationInput{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid approval actor: %s", approval))
			}
			approvals = append(approvals, approval)
		}
	}
	trustedSignatures, _ := cmd.Flags().GetInt("trusted-signatures")
	return service.GovernanceEvaluationInput{
		Action:                action,
		Actor:                 actor,
		TicketID:              strings.TrimSpace(ticketID),
		RunID:                 strings.TrimSpace(runID),
		ChangeID:              strings.TrimSpace(changeID),
		GateID:                strings.TrimSpace(gateID),
		Reason:                strings.TrimSpace(reason),
		ApprovalActors:        approvals,
		TrustedSignatureCount: trustedSignatures,
	}, nil
}

func governancePackPretty(view service.GovernancePackDetailView) string {
	return fmt.Sprintf("%s policies=%d", view.Pack.PackID, len(view.Pack.Policies))
}

func governancePackMarkdown(view service.GovernancePackDetailView) string {
	lines := []string{"# Governance Pack", "", fmt.Sprintf("- Pack: `%s`", view.Pack.PackID), fmt.Sprintf("- Policies: `%d`", len(view.Pack.Policies))}
	for _, policy := range view.Pack.Policies {
		lines = append(lines, fmt.Sprintf("- Policy `%s`: `%s` `%s`", policy.PolicyID, policy.ScopeKind, policy.ScopeID))
	}
	return strings.Join(lines, "\n") + "\n"
}

func governancePackListPretty(view service.GovernancePackListView) string {
	if len(view.Items) == 0 {
		return "no governance packs"
	}
	lines := make([]string, 0, len(view.Items))
	for _, pack := range view.Items {
		lines = append(lines, fmt.Sprintf("%s policies=%d", pack.PackID, len(pack.Policies)))
	}
	return strings.Join(lines, "\n")
}

func governancePackListMarkdown(view service.GovernancePackListView) string {
	if len(view.Items) == 0 {
		return "# Governance Packs\n\nNo governance packs.\n"
	}
	lines := []string{"# Governance Packs", ""}
	for _, pack := range view.Items {
		lines = append(lines, fmt.Sprintf("- `%s` policies=%d", pack.PackID, len(pack.Policies)))
	}
	return strings.Join(lines, "\n") + "\n"
}

func governanceValidationMarkdown(view service.GovernanceValidationView) string {
	lines := []string{"# Governance Validation", "", fmt.Sprintf("- Valid: `%t`", view.Valid), fmt.Sprintf("- Policies: `%d`", view.Policies), fmt.Sprintf("- Packs: `%d`", view.Packs)}
	for _, item := range view.Errors {
		lines = append(lines, fmt.Sprintf("- Error: `%s`", item))
	}
	return strings.Join(lines, "\n") + "\n"
}

func governanceExplanationPretty(view contracts.GovernanceExplanation) string {
	out := fmt.Sprintf("%s %s allowed=%t", view.Action, view.Target, view.Allowed)
	if len(view.ReasonCodes) > 0 {
		out += " reasons=" + strings.Join(view.ReasonCodes, ",")
	}
	return out
}

func governanceExplanationMarkdown(view contracts.GovernanceExplanation) string {
	lines := []string{"# Governance", "", fmt.Sprintf("- Action: `%s`", view.Action), fmt.Sprintf("- Target: `%s`", view.Target), fmt.Sprintf("- Actor: `%s`", view.Actor), fmt.Sprintf("- Allowed: `%t`", view.Allowed)}
	for _, policy := range view.MatchedPolicies {
		lines = append(lines, fmt.Sprintf("- Matched policy: `%s`", policy))
	}
	for _, reason := range view.ReasonCodes {
		lines = append(lines, fmt.Sprintf("- Reason: `%s`", reason))
	}
	return strings.Join(lines, "\n") + "\n"
}

func keyListPretty(view service.KeyListView) string {
	if len(view.Items) == 0 {
		return "no signing keys"
	}
	lines := make([]string, 0, len(view.Items))
	for _, item := range view.Items {
		lines = append(lines, keyDetailPretty(item))
	}
	return strings.Join(lines, "\n")
}

func keyListMarkdown(view service.KeyListView) string {
	if len(view.Items) == 0 {
		return "# Signing Keys\n\nNo signing keys.\n"
	}
	lines := []string{"# Signing Keys", ""}
	for _, item := range view.Items {
		lines = append(lines, "- "+keyDetailPretty(item))
	}
	return strings.Join(lines, "\n")
}

func keyDetailPretty(view service.KeyDetailView) string {
	state := "cannot sign"
	if view.CanSign {
		state = "can sign"
	}
	return fmt.Sprintf("%s %s %s:%s %s", view.PublicKey.PublicKeyID, view.PublicKey.Status, view.PublicKey.OwnerKind, view.PublicKey.OwnerID, state)
}

func keyDetailMarkdown(view service.KeyDetailView) string {
	return fmt.Sprintf("# %s\n\n- Status: %s\n- Owner: %s:%s\n- Fingerprint: `%s`\n- Can sign: %t\n", view.PublicKey.PublicKeyID, view.PublicKey.Status, view.PublicKey.OwnerKind, view.PublicKey.OwnerID, view.PublicKey.Fingerprint, view.CanSign)
}

func publicKeyExportDocument(record contracts.PublicKeyRecord) (string, error) {
	raw, err := yaml.Marshal(struct {
		contracts.PublicKeyRecord `yaml:",inline"`
	}{PublicKeyRecord: record})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("---\n%s---\n\nAtlas public key `%s` for `%s:%s`.\n", string(raw), record.PublicKeyID, record.OwnerKind, record.OwnerID), nil
}

func trustListPretty(view service.TrustListView) string {
	if len(view.Items) == 0 {
		return "no local trust bindings"
	}
	lines := make([]string, 0, len(view.Items))
	for _, binding := range view.Items {
		lines = append(lines, fmt.Sprintf("%s %s -> %s:%s", binding.TrustLevel, binding.PublicKeyID, binding.TrustedOwnerKind, binding.TrustedOwnerID))
	}
	return strings.Join(lines, "\n")
}

func trustListMarkdown(view service.TrustListView) string {
	if len(view.Items) == 0 {
		return "# Trust Bindings\n\nNo local trust bindings.\n"
	}
	lines := []string{"# Trust Bindings", ""}
	for _, binding := range view.Items {
		lines = append(lines, fmt.Sprintf("- `%s` %s -> %s:%s", binding.PublicKeyID, binding.TrustLevel, binding.TrustedOwnerKind, binding.TrustedOwnerID))
	}
	return strings.Join(lines, "\n")
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
