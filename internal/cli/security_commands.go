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
	approval := &cobra.Command{Use: "approval <GATE-ID>", Short: "Sign an approval artifact", Args: cobra.ExactArgs(1), RunE: runSignApproval}
	handoff := &cobra.Command{Use: "handoff <HANDOFF-ID>", Short: "Sign a handoff artifact", Args: cobra.ExactArgs(1), RunE: runSignHandoff}
	evidence := &cobra.Command{Use: "evidence <EVIDENCE-ID>", Short: "Sign an evidence packet", Args: cobra.ExactArgs(1), RunE: runSignEvidence}
	audit := &cobra.Command{Use: "audit <AUDIT-REPORT-ID>", Short: "Sign an audit report", Args: cobra.ExactArgs(1), RunE: runSignAudit}
	auditPacket := &cobra.Command{Use: "audit-packet <PACKET-ID>", Short: "Sign an audit packet", Args: cobra.ExactArgs(1), RunE: runSignAuditPacket}
	backup := &cobra.Command{Use: "backup <BACKUP-ID>", Short: "Sign a backup snapshot", Args: cobra.ExactArgs(1), RunE: runSignBackup}
	goal := &cobra.Command{Use: "goal <MANIFEST-ID>", Short: "Sign a goal manifest", Args: cobra.ExactArgs(1), RunE: runSignGoal}
	for _, sub := range []*cobra.Command{bundle, syncPublication, approval, handoff, evidence, audit, auditPacket, backup, goal} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
		sub.Flags().String("signing-key", "", "Signing key id")
		cmd.AddCommand(sub)
	}
	return cmd
}

func newVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "verify", Short: "Verify signed Atlas artifacts without mutating by default"}
	bundle := &cobra.Command{Use: "bundle <BUNDLE-ID|PATH>", Short: "Verify bundle integrity and signature state", Args: cobra.ExactArgs(1), RunE: runVerifyBundleSignature}
	syncPublication := &cobra.Command{Use: "sync-publication <BUNDLE-ID|PATH>", Short: "Verify sync publication signature state", Args: cobra.ExactArgs(1), RunE: runVerifySyncPublicationSignature}
	approval := &cobra.Command{Use: "approval <GATE-ID>", Short: "Verify approval signature state", Args: cobra.ExactArgs(1), RunE: runVerifyApprovalSignature}
	handoff := &cobra.Command{Use: "handoff <HANDOFF-ID>", Short: "Verify handoff signature state", Args: cobra.ExactArgs(1), RunE: runVerifyHandoffSignature}
	evidence := &cobra.Command{Use: "evidence <EVIDENCE-ID|PATH>", Short: "Verify evidence packet signature state", Args: cobra.ExactArgs(1), RunE: runVerifyEvidenceSignature}
	audit := &cobra.Command{Use: "audit <REPORT-ID|PATH>", Short: "Verify audit report signature state", Args: cobra.ExactArgs(1), RunE: runVerifyAuditReportArtifact}
	auditPacket := &cobra.Command{Use: "audit-packet <PACKET-ID|PATH>", Short: "Verify audit packet signature state", Args: cobra.ExactArgs(1), RunE: runVerifyAuditPacketArtifact}
	backup := &cobra.Command{Use: "backup <BACKUP-ID|PATH>", Short: "Verify backup signature state", Args: cobra.ExactArgs(1), RunE: runVerifyBackup}
	goal := &cobra.Command{Use: "goal <MANIFEST-ID|PATH>", Short: "Verify goal manifest signature state", Args: cobra.ExactArgs(1), RunE: runVerifyGoal}
	for _, sub := range []*cobra.Command{bundle, syncPublication, approval, handoff, evidence, audit, auditPacket, backup, goal} {
		addReadOutputFlags(sub, &outputFlags{})
		cmd.AddCommand(sub)
	}
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
	list := &cobra.Command{Use: "list", Short: "List classification labels", Args: cobra.NoArgs, RunE: runClassifyList}
	list.Flags().String("project", "", "Filter by project")
	get := &cobra.Command{Use: "get <ENTITY>", Short: "Show classification for an entity", Args: cobra.ExactArgs(1), RunE: runClassifyGet}
	set := &cobra.Command{Use: "set <ENTITY> <LEVEL>", Short: "Set classification for an entity", Args: cobra.ExactArgs(2), RunE: runClassifySet}
	explain := &cobra.Command{Use: "explain <ENTITY>", Short: "Explain inherited classification", Args: cobra.ExactArgs(1), RunE: runClassifyGet}
	for _, sub := range []*cobra.Command{list, get, explain} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	addMutationFlags(set, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(set, &outputFlags{})
	cmd.AddCommand(
		get,
		set,
		list,
		explain,
	)
	return cmd
}

func newRedactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "redact", Short: "Preview and create redacted artifacts"}
	preview := &cobra.Command{Use: "preview", Short: "Create an actor-bound redaction preview", Args: cobra.NoArgs, RunE: runRedactPreview}
	preview.Flags().String("scope", "", "Scope to preview")
	preview.Flags().String("target", "", "Target: export|sync|audit|backup|goal")
	export := &cobra.Command{Use: "export", Short: "Create a redacted export artifact", Args: cobra.NoArgs, RunE: runRedactExport}
	export.Flags().String("scope", "", "Scope to export")
	export.Flags().String("preview-id", "", "Redaction preview id")
	verify := &cobra.Command{Use: "verify <ARTIFACT>", Short: "Verify a redacted artifact", Args: cobra.ExactArgs(1), RunE: runRedactVerify}
	for _, sub := range []*cobra.Command{preview, export} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
	}
	addReadOutputFlags(verify, &outputFlags{})
	cmd.AddCommand(preview, export, verify)
	return cmd
}

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "audit", Short: "Generate and verify v1.7 audit reports"}
	report := &cobra.Command{Use: "report", Short: "Create an audit report snapshot", Args: cobra.NoArgs, RunE: runAuditReport}
	addMutationFlags(report, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(report, &outputFlags{})
	report.Flags().String("scope", "", "Audit scope")
	list := &cobra.Command{Use: "list", Short: "List audit reports", Args: cobra.NoArgs, RunE: runAuditList}
	view := &cobra.Command{Use: "view <REPORT-ID>", Short: "Show one audit report", Args: cobra.ExactArgs(1), RunE: runAuditView}
	export := &cobra.Command{Use: "export <REPORT-ID>", Short: "Export an audit packet", Args: cobra.ExactArgs(1), RunE: runAuditExport}
	verify := &cobra.Command{Use: "verify <REPORT-ID|PATH>", Short: "Verify an audit report or packet", Args: cobra.ExactArgs(1), RunE: runVerifyAuditArtifact}
	explainPolicy := &cobra.Command{Use: "explain-policy <EVENT-ID|ACTION-ID>", Short: "Explain policy decision provenance", Args: cobra.ExactArgs(1), RunE: runAuditExplainPolicy}
	for _, sub := range []*cobra.Command{list, view, verify, explainPolicy} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	addMutationFlags(export, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(export, &outputFlags{})
	cmd.AddCommand(
		report,
		list,
		view,
		export,
		verify,
		explainPolicy,
	)
	return cmd
}

func newBackupCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "backup", Short: "Create and restore Atlas-owned backups"}
	create := &cobra.Command{Use: "create", Short: "Create a backup snapshot", Args: cobra.NoArgs, RunE: runBackupCreate}
	create.Flags().String("scope", "workspace", "Backup scope: workspace|project:<KEY>")
	restorePlan := &cobra.Command{Use: "restore-plan <BACKUP-ID|PATH>", Short: "Preview a backup restore", Args: cobra.ExactArgs(1), RunE: runBackupRestorePlan}
	restoreApply := &cobra.Command{Use: "restore-apply <BACKUP-ID|PATH>", Short: "Apply a backup restore plan", Args: cobra.ExactArgs(1), RunE: runBackupRestoreApply}
	restoreApply.Flags().Bool("yes", false, "Apply restore without prompting")
	drill := &cobra.Command{Use: "drill", Short: "Run a read-only recovery drill", Args: cobra.NoArgs, RunE: runBackupDrill}
	for _, sub := range []*cobra.Command{create, restoreApply} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{restorePlan, drill} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	cmd.AddCommand(
		create,
		readCommand("list", "List backup snapshots", cobra.NoArgs, runBackupList),
		readCommand("view <BACKUP-ID>", "Show one backup snapshot", cobra.ExactArgs(1), runBackupView),
		readCommand("verify <BACKUP-ID|PATH>", "Verify a backup snapshot", cobra.ExactArgs(1), runVerifyBackup),
		restorePlan,
		restoreApply,
		drill,
	)
	return cmd
}

func newAdminCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Admin diagnostics"}
	cmd.AddCommand(
		readCommand("security-status", "Show v1.7 security status", cobra.NoArgs, runAdminSecurityStatus),
		readCommand("trust-store", "Inspect trust store health", cobra.NoArgs, runAdminTrustStore),
		readCommand("recovery-status", "Inspect recovery readiness", cobra.NoArgs, runAdminRecoveryStatus),
	)
	return cmd
}

func newGoalCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "goal", Short: "Generate read-only agent goal briefs and manifests"}
	brief := &cobra.Command{Use: "brief <TICKET-ID|RUN-ID>", Short: "Render a goal-ready brief", Args: cobra.ExactArgs(1), RunE: runGoalBrief}
	manifest := &cobra.Command{Use: "manifest <TICKET-ID|RUN-ID>", Short: "Write a goal-ready manifest", Args: cobra.ExactArgs(1), RunE: runGoalManifest}
	verify := &cobra.Command{Use: "verify <MANIFEST-ID|PATH>", Short: "Verify a signed goal manifest", Args: cobra.ExactArgs(1), RunE: runVerifyGoal}
	addReadOutputFlags(brief, &outputFlags{})
	addMutationFlags(manifest, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(manifest, &outputFlags{})
	addReadOutputFlags(verify, &outputFlags{})
	cmd.AddCommand(
		brief,
		manifest,
		verify,
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

func runSignApproval(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignApproval(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runSignHandoff(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignHandoff(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runSignEvidence(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignEvidencePacket(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runSignAudit(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignAuditReport(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runSignAuditPacket(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignAuditPacket(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runVerifyApprovalSignature(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyApprovalSignature(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runVerifyHandoffSignature(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyHandoffSignature(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runVerifyEvidenceSignature(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyEvidencePacketSignature(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runVerifyAuditArtifact(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyAuditArtifact(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runVerifyAuditReportArtifact(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyAuditReportArtifact(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runVerifyAuditPacketArtifact(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyAuditPacketArtifact(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runSignBackup(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignBackupSnapshot(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runVerifyBackup(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyBackupSnapshot(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runSignGoal(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	signingKey, _ := cmd.Flags().GetString("signing-key")
	view, err := w.actions.SignGoalManifest(cmd.Context(), args[0], strings.TrimSpace(signingKey), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureDetailMarkdown(view), signatureDetailPretty(view))
}

func runVerifyGoal(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyGoalManifest(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, signatureVerifyMarkdown(view), signatureVerifyPretty(view))
}

func runBackupCreate(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	view, err := w.actions.CreateBackup(cmd.Context(), scope, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, backupDetailMarkdown(view), backupDetailPretty(view))
}

func runBackupList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ListBackups(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, backupListMarkdown(view), backupListPretty(view))
}

func runBackupView(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.BackupDetail(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, backupDetailMarkdown(view), backupDetailPretty(view))
}

func runBackupRestorePlan(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.CreateRestorePlan(cmd.Context(), args[0], contracts.Actor("human:owner"))
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, restorePlanMarkdown(view), restorePlanPretty(view))
}

func runBackupRestoreApply(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	yes, _ := cmd.Flags().GetBool("yes")
	view, err := w.actions.ApplyRestorePlan(cmd.Context(), args[0], actor, reason, yes)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, restoreApplyMarkdown(view), restoreApplyPretty(view))
}

func runBackupDrill(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.RecoveryDrill(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, recoveryDrillMarkdown(view), recoveryDrillPretty(view))
}

func runAdminSecurityStatus(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.AdminSecurityStatus(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, adminSecurityMarkdown(view), adminSecurityPretty(view))
}

func runAdminTrustStore(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.TrustStoreStatus(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, trustStoreStatusMarkdown(view), trustStoreStatusPretty(view))
}

func runAdminRecoveryStatus(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.RecoveryStatus(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, recoveryStatusMarkdown(view), recoveryStatusPretty(view))
}

func runGoalBrief(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.GoalBrief(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, goalBriefMarkdown(view), goalBriefPretty(view))
}

func runGoalManifest(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.CreateGoalManifest(cmd.Context(), args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, goalManifestMarkdown(view), goalManifestPretty(view))
}

func runAuditReport(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	view, err := w.actions.CreateAuditReport(cmd.Context(), scope, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, auditReportMarkdown(view), auditReportPretty(view))
}

func runAuditList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ListAuditReports(cmd.Context())
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, auditReportListMarkdown(view), auditReportListPretty(view))
}

func runAuditView(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.AuditReportDetail(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, auditReportMarkdown(view), auditReportPretty(view))
}

func runAuditExport(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.ExportAuditPacket(cmd.Context(), args[0], actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, auditExportMarkdown(view), auditExportPretty(view))
}

func runAuditExplainPolicy(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ExplainAuditPolicy(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, auditPolicyMarkdown(view), auditPolicyPretty(view))
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

func runClassifyList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	project, _ := cmd.Flags().GetString("project")
	view, err := w.actions.ListClassifications(cmd.Context(), strings.TrimSpace(project))
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, classificationListMarkdown(view), classificationListPretty(view))
}

func runClassifyGet(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.ClassificationDetail(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, classificationDetailMarkdown(view), classificationDetailPretty(view))
}

func runClassifySet(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	view, err := w.actions.SetClassification(cmd.Context(), args[0], contracts.ClassificationLevel(strings.TrimSpace(args[1])), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, classificationDetailMarkdown(view), classificationDetailPretty(view))
}

func runRedactPreview(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	targetRaw, _ := cmd.Flags().GetString("target")
	if strings.TrimSpace(targetRaw) == "" {
		targetRaw = string(contracts.RedactionTargetExport)
	}
	view, err := w.actions.CreateRedactionPreview(cmd.Context(), scope, contracts.RedactionTarget(strings.TrimSpace(targetRaw)), actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, redactionPreviewMarkdown(view), redactionPreviewPretty(view))
}

func runRedactExport(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actor, reason := mutationActorReason(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	previewID, _ := cmd.Flags().GetString("preview-id")
	view, err := w.actions.CreateRedactedExport(cmd.Context(), scope, previewID, actor, reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, redactionExportMarkdown(view), redactionExportPretty(view))
}

func runRedactVerify(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	view, err := w.actions.VerifyRedactedArtifact(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view, redactionVerifyMarkdown(view), redactionVerifyPretty(view))
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
	case service.AuditIntegrityView:
		return true, view.Verified, append([]string{}, view.Errors...), append([]string{}, view.Warnings...)
	case service.BackupIntegrityView:
		return true, view.Verified, append([]string{}, view.Errors...), append([]string{}, view.Warnings...)
	default:
		return false, false, nil, nil
	}
}

func auditReportPretty(view service.AuditReportDetailView) string {
	return fmt.Sprintf("%s %s:%s events=%d-%d artifacts=%d findings=%d", view.Report.AuditReportID, view.Report.ScopeKind, view.Report.ScopeID, view.Report.EventRange.FromEventID, view.Report.EventRange.ToEventID, len(view.Report.IncludedArtifactHashes), len(view.Report.Findings))
}

func auditReportMarkdown(view service.AuditReportDetailView) string {
	report := view.Report
	lines := []string{
		"# Audit Report",
		"",
		fmt.Sprintf("- Report: `%s`", report.AuditReportID),
		fmt.Sprintf("- Scope: `%s` `%s`", report.ScopeKind, report.ScopeID),
		fmt.Sprintf("- Events: `%d..%d`", report.EventRange.FromEventID, report.EventRange.ToEventID),
		fmt.Sprintf("- Policy snapshot: `%s`", report.PolicySnapshotHash),
		fmt.Sprintf("- Trust snapshot: `%s`", report.TrustSnapshotHash),
		fmt.Sprintf("- Artifact hashes: `%d`", len(report.IncludedArtifactHashes)),
	}
	if len(report.Findings) > 0 {
		lines = append(lines, "", "## Findings", "")
		for _, finding := range report.Findings {
			lines = append(lines, fmt.Sprintf("- `%s` %s: %s", finding.Severity, finding.Code, finding.Message))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func auditReportListPretty(view service.AuditReportListView) string {
	if len(view.Items) == 0 {
		return "no audit reports"
	}
	lines := make([]string, 0, len(view.Items))
	for _, report := range view.Items {
		lines = append(lines, fmt.Sprintf("%s %s:%s", report.AuditReportID, report.ScopeKind, report.ScopeID))
	}
	return strings.Join(lines, "\n")
}

func auditReportListMarkdown(view service.AuditReportListView) string {
	if len(view.Items) == 0 {
		return "# Audit Reports\n\nNo audit reports.\n"
	}
	lines := []string{"# Audit Reports", ""}
	for _, report := range view.Items {
		lines = append(lines, fmt.Sprintf("- `%s` `%s` `%s`", report.AuditReportID, report.ScopeKind, report.ScopeID))
	}
	return strings.Join(lines, "\n") + "\n"
}

func auditExportPretty(view service.AuditReportExportResultView) string {
	return fmt.Sprintf("%s exported from %s", view.Packet.PacketID, view.Packet.Report.AuditReportID)
}

func auditExportMarkdown(view service.AuditReportExportResultView) string {
	return fmt.Sprintf("# Audit Packet\n\n- Packet: `%s`\n- Report: `%s`\n- Packet hash: `%s`\n- Path: `%s`\n", view.Packet.PacketID, view.Packet.Report.AuditReportID, view.Packet.PacketHash, view.Path)
}

func auditPolicyPretty(view service.AuditPolicyExplanationView) string {
	return fmt.Sprintf("%s %s", view.Target, strings.Join(view.ReasonCodes, ","))
}

func auditPolicyMarkdown(view service.AuditPolicyExplanationView) string {
	lines := []string{"# Policy Provenance", "", fmt.Sprintf("- Target: `%s`", view.Target)}
	if view.Event != nil {
		lines = append(lines, fmt.Sprintf("- Event: `%d` `%s`", view.Event.EventID, view.Event.Type))
	}
	for _, reason := range view.ReasonCodes {
		lines = append(lines, fmt.Sprintf("- Reason: `%s`", reason))
	}
	if strings.TrimSpace(view.SnapshotGuidance) != "" {
		lines = append(lines, "", view.SnapshotGuidance)
	}
	return strings.Join(lines, "\n") + "\n"
}

func backupDetailPretty(view service.BackupDetailView) string {
	return fmt.Sprintf("%s %s:%s files=%d", view.Snapshot.BackupID, view.Snapshot.ScopeKind, view.Snapshot.ScopeID, view.FileCount)
}

func backupDetailMarkdown(view service.BackupDetailView) string {
	return fmt.Sprintf("# Backup\n\n- Backup: `%s`\n- Scope: `%s` `%s`\n- Files: `%d`\n- Manifest hash: `%s`\n- Archive: `%s`\n", view.Snapshot.BackupID, view.Snapshot.ScopeKind, view.Snapshot.ScopeID, view.FileCount, view.Snapshot.ManifestHash, view.ArchivePath)
}

func backupListPretty(view service.BackupListView) string {
	if len(view.Items) == 0 {
		return "no backups"
	}
	lines := make([]string, 0, len(view.Items))
	for _, item := range view.Items {
		lines = append(lines, fmt.Sprintf("%s %s:%s %s", item.BackupID, item.ScopeKind, item.ScopeID, item.CreatedAt.Format(time.RFC3339)))
	}
	return strings.Join(lines, "\n")
}

func backupListMarkdown(view service.BackupListView) string {
	if len(view.Items) == 0 {
		return "# Backups\n\nNo backups.\n"
	}
	lines := []string{"# Backups", ""}
	for _, item := range view.Items {
		lines = append(lines, fmt.Sprintf("- `%s` `%s` `%s` `%s`", item.BackupID, item.ScopeKind, item.ScopeID, item.CreatedAt.Format(time.RFC3339)))
	}
	return strings.Join(lines, "\n") + "\n"
}

func restorePlanPretty(view service.RestorePlanDetailView) string {
	return fmt.Sprintf("%s backup=%s items=%d warnings=%d", view.Plan.RestorePlanID, view.Plan.BackupID, len(view.Plan.Items), len(view.Plan.Warnings))
}

func restorePlanMarkdown(view service.RestorePlanDetailView) string {
	lines := []string{"# Restore Plan", "", fmt.Sprintf("- Plan: `%s`", view.Plan.RestorePlanID), fmt.Sprintf("- Backup: `%s`", view.Plan.BackupID), fmt.Sprintf("- Items: `%d`", len(view.Plan.Items))}
	for _, item := range view.Plan.Items {
		lines = append(lines, fmt.Sprintf("- `%s` `%s`", item.Action, item.Path))
	}
	for _, warning := range view.Plan.Warnings {
		lines = append(lines, fmt.Sprintf("- Warning: `%s`", warning))
	}
	return strings.Join(lines, "\n") + "\n"
}

func restoreApplyPretty(view service.RestoreApplyResultView) string {
	return fmt.Sprintf("%s applied=%d skipped=%d", view.Plan.RestorePlanID, view.Applied, view.Skipped)
}

func restoreApplyMarkdown(view service.RestoreApplyResultView) string {
	return fmt.Sprintf("# Restore Applied\n\n- Plan: `%s`\n- Backup: `%s`\n- Applied: `%d`\n- Skipped: `%d`\n", view.Plan.RestorePlanID, view.Plan.BackupID, view.Applied, view.Skipped)
}

func recoveryDrillPretty(view service.RecoveryDrillView) string {
	return fmt.Sprintf("backups=%d verified=%d side_effect_free=%t", view.BackupCount, view.VerifiedBackups, view.SideEffectFree)
}

func recoveryDrillMarkdown(view service.RecoveryDrillView) string {
	lines := []string{"# Recovery Drill", "", fmt.Sprintf("- Backups: `%d`", view.BackupCount), fmt.Sprintf("- Verified backups: `%d`", view.VerifiedBackups), fmt.Sprintf("- Side-effect free: `%t`", view.SideEffectFree)}
	for _, warning := range view.Warnings {
		lines = append(lines, fmt.Sprintf("- Warning: `%s`", warning))
	}
	return strings.Join(lines, "\n") + "\n"
}

func adminSecurityPretty(view service.AdminSecurityStatusView) string {
	return fmt.Sprintf("keys=%d trust=%d governance=%d audits=%d backups=%d", view.PublicKeys, view.TrustBindings, view.GovernancePolicies, view.AuditReports, view.Backups)
}

func adminSecurityMarkdown(view service.AdminSecurityStatusView) string {
	lines := []string{"# Security Status", "", fmt.Sprintf("- Public keys: `%d`", view.PublicKeys), fmt.Sprintf("- Trust bindings: `%d`", view.TrustBindings), fmt.Sprintf("- Governance policies: `%d`", view.GovernancePolicies), fmt.Sprintf("- Audit reports: `%d`", view.AuditReports), fmt.Sprintf("- Backups: `%d`", view.Backups)}
	for _, warning := range view.Warnings {
		lines = append(lines, fmt.Sprintf("- Warning: `%s`", warning))
	}
	return strings.Join(lines, "\n") + "\n"
}

func trustStoreStatusPretty(view service.TrustStoreStatusView) string {
	return fmt.Sprintf("public_keys=%d local=%d trusted=%d revoked=%d expired=%d", view.PublicKeys, view.LocalKeys, view.TrustedKeys, view.RevokedKeys, view.ExpiredKeys)
}

func trustStoreStatusMarkdown(view service.TrustStoreStatusView) string {
	lines := []string{"# Trust Store", "", fmt.Sprintf("- Public keys: `%d`", view.PublicKeys), fmt.Sprintf("- Local keys: `%d`", view.LocalKeys), fmt.Sprintf("- Trusted keys: `%d`", view.TrustedKeys), fmt.Sprintf("- Revoked keys: `%d`", view.RevokedKeys), fmt.Sprintf("- Expired keys: `%d`", view.ExpiredKeys)}
	for _, warning := range view.Warnings {
		lines = append(lines, fmt.Sprintf("- Warning: `%s`", warning))
	}
	return strings.Join(lines, "\n") + "\n"
}

func recoveryStatusPretty(view service.RecoveryStatusView) string {
	return fmt.Sprintf("backups=%d latest=%s plans=%d", view.BackupCount, view.LatestBackupID, view.RestorePlanCount)
}

func recoveryStatusMarkdown(view service.RecoveryStatusView) string {
	lines := []string{"# Recovery Status", "", fmt.Sprintf("- Backups: `%d`", view.BackupCount), fmt.Sprintf("- Latest backup: `%s`", view.LatestBackupID), fmt.Sprintf("- Restore plans: `%d`", view.RestorePlanCount)}
	for _, warning := range view.Warnings {
		lines = append(lines, fmt.Sprintf("- Warning: `%s`", warning))
	}
	return strings.Join(lines, "\n") + "\n"
}

func goalBriefPretty(view service.GoalBriefView) string {
	return fmt.Sprintf("%s %s sections=%d", view.Brief.TargetKind, view.Brief.TargetID, len(view.Brief.Sections))
}

func goalBriefMarkdown(view service.GoalBriefView) string {
	return goalSectionsMarkdown(view.Brief.Objective, view.Brief.Sections)
}

func goalManifestPretty(view service.GoalManifestDetailView) string {
	return fmt.Sprintf("%s %s:%s sections=%d", view.Manifest.ManifestID, view.Manifest.TargetKind, view.Manifest.TargetID, len(view.Manifest.Sections))
}

func goalManifestMarkdown(view service.GoalManifestDetailView) string {
	return goalSectionsMarkdown(view.Manifest.Objective, view.Manifest.Sections)
}

func goalSectionsMarkdown(objective string, sections []contracts.GoalSection) string {
	lines := []string{"# Agent Goal", "", strings.TrimSpace(objective), ""}
	for _, section := range sections {
		lines = append(lines, "## "+section.Heading, "")
		if len(section.Items) > 0 {
			for _, item := range section.Items {
				lines = append(lines, "- "+item)
			}
		} else {
			lines = append(lines, strings.TrimSpace(section.Body))
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
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

func classificationDetailPretty(view service.ClassificationDetailView) string {
	return fmt.Sprintf("%s:%s %s", view.EntityKind, view.EntityID, view.Level)
}

func classificationDetailMarkdown(view service.ClassificationDetailView) string {
	lines := []string{
		"# Classification",
		"",
		fmt.Sprintf("- Entity: `%s:%s`", view.EntityKind, view.EntityID),
		fmt.Sprintf("- Effective level: `%s`", view.Level),
	}
	for _, step := range view.Inheritance {
		source := "inherited"
		if step.Explicit {
			source = "explicit"
		}
		lines = append(lines, fmt.Sprintf("- %s `%s:%s` -> `%s`", source, step.EntityKind, step.EntityID, step.Level))
	}
	return strings.Join(lines, "\n") + "\n"
}

func classificationListPretty(view service.ClassificationListView) string {
	if len(view.Items) == 0 {
		return "no classification labels"
	}
	lines := make([]string, 0, len(view.Items))
	for _, item := range view.Items {
		lines = append(lines, fmt.Sprintf("%s:%s %s", item.EntityKind, item.EntityID, item.Level))
	}
	return strings.Join(lines, "\n")
}

func classificationListMarkdown(view service.ClassificationListView) string {
	if len(view.Items) == 0 {
		return "# Classification Labels\n\nNo classification labels.\n"
	}
	lines := []string{"# Classification Labels", ""}
	for _, item := range view.Items {
		lines = append(lines, fmt.Sprintf("- `%s:%s` -> `%s`", item.EntityKind, item.EntityID, item.Level))
	}
	return strings.Join(lines, "\n") + "\n"
}

func redactionPreviewPretty(view service.RedactionPreviewDetailView) string {
	return fmt.Sprintf("%s %s items=%d expires=%s", view.Preview.PreviewID, view.Preview.Target, len(view.Preview.Items), view.Preview.ExpiresAt.Format(time.RFC3339))
}

func redactionPreviewMarkdown(view service.RedactionPreviewDetailView) string {
	lines := []string{
		"# Redaction Preview",
		"",
		fmt.Sprintf("- Preview: `%s`", view.Preview.PreviewID),
		fmt.Sprintf("- Target: `%s`", view.Preview.Target),
		fmt.Sprintf("- Scope: `%s`", view.Preview.Scope),
		fmt.Sprintf("- Expires: `%s`", view.Preview.ExpiresAt.Format(time.RFC3339)),
		fmt.Sprintf("- Items: `%d`", len(view.Preview.Items)),
	}
	for _, item := range view.Preview.Items {
		lines = append(lines, fmt.Sprintf("- `%s` `%s:%s` `%s`", item.Action, item.EntityKind, item.EntityID, item.FieldPath))
	}
	return strings.Join(lines, "\n") + "\n"
}

func redactionExportPretty(view service.RedactionExportResultView) string {
	return fmt.Sprintf("redacted export %s preview=%s included=%d omitted=%d", view.Bundle.BundleID, view.Preview.PreviewID, view.Included, view.Omitted)
}

func redactionExportMarkdown(view service.RedactionExportResultView) string {
	return fmt.Sprintf("# Redacted Export\n\n- Bundle: `%s`\n- Preview: `%s`\n- Included: `%d`\n- Omitted: `%d`\n", view.Bundle.BundleID, view.Preview.PreviewID, view.Included, view.Omitted)
}

func redactionVerifyPretty(view service.RedactionVerifyView) string {
	if view.Verified {
		return fmt.Sprintf("%s verified preview=%s", view.Artifact, view.RedactionPreviewID)
	}
	return fmt.Sprintf("%s redaction verification failed: %s", view.Artifact, strings.Join(view.Errors, ","))
}

func redactionVerifyMarkdown(view service.RedactionVerifyView) string {
	lines := []string{"# Redaction Verification", "", fmt.Sprintf("- Verified: `%t`", view.Verified)}
	if view.RedactionPreviewID != "" {
		lines = append(lines, fmt.Sprintf("- Preview: `%s`", view.RedactionPreviewID))
	}
	for _, item := range view.Errors {
		lines = append(lines, fmt.Sprintf("- Error: `%s`", item))
	}
	return strings.Join(lines, "\n") + "\n"
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

func readCommand(use string, short string, args cobra.PositionalArgs, run func(*cobra.Command, []string) error) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args, RunE: run}
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
