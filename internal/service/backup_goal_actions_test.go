package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestBackupCreateVerifySignPlanAndApply(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "owner"}, contracts.Actor("human:owner"), "backup signer")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "owner", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust backup signer"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	view, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "create release backup")
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if view.FileCount == 0 || view.Snapshot.ManifestHash == "" {
		t.Fatalf("backup should include files and manifest hash: %#v", view)
	}
	sections := make([]string, 0, len(view.Snapshot.IncludedSections))
	for _, section := range view.Snapshot.IncludedSections {
		sections = append(sections, string(section))
	}
	if strings.Contains(strings.Join(sections, ","), "private") {
		t.Fatalf("backup sections should never mention private key material: %#v", view.Snapshot.IncludedSections)
	}
	verified, err := actions.VerifyBackupSnapshot(ctx, view.Snapshot.BackupID)
	if err != nil {
		t.Fatalf("verify backup: %v", err)
	}
	integrity, ok := verified.Integrity.(BackupIntegrityView)
	if !ok || !integrity.Verified {
		t.Fatalf("backup integrity should verify: %#v", verified.Integrity)
	}
	if _, err := actions.SignBackupSnapshot(ctx, view.Snapshot.BackupID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign backup"); err != nil {
		t.Fatalf("sign backup: %v", err)
	}
	signed, err := actions.VerifyBackupSnapshot(ctx, view.Snapshot.BackupID)
	if err != nil {
		t.Fatalf("verify signed backup: %v", err)
	}
	if signed.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("signed backup should verify trusted, got %#v", signed.Signature)
	}
	corruptView, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "create corruptible backup")
	if err != nil {
		t.Fatalf("create corruptible backup: %v", err)
	}

	projectPath := storage.ProjectFile(actions.Root, "APP")
	if err := os.WriteFile(projectPath, []byte("tampered\n"), 0o644); err != nil {
		t.Fatalf("tamper project file: %v", err)
	}
	corruptManifest, corruptRaw, err := loadBundleManifestRaw(backupManifestPath(actions.Root, corruptView.Snapshot.BackupID))
	if err != nil {
		t.Fatalf("load corruptible manifest: %v", err)
	}
	corruptPaths := make([]string, 0, len(corruptManifest.Files))
	for _, item := range corruptManifest.Files {
		corruptPaths = append(corruptPaths, item.Path)
	}
	if err := writeBundleArchive(actions.Root, backupArchivePath(actions.Root, corruptView.Snapshot.BackupID), corruptRaw, corruptPaths); err != nil {
		t.Fatalf("rewrite corruptible archive: %v", err)
	}
	if _, err := actions.ApplyRestorePlan(ctx, corruptView.Snapshot.BackupID, contracts.Actor("human:owner"), "restore corrupt backup", true); err == nil || !strings.Contains(err.Error(), "backup integrity") {
		t.Fatalf("restore apply should reject corrupt backup integrity, got %v", err)
	}
	planView, err := actions.CreateRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("restore plan: %v", err)
	}
	hasUpdate := false
	for _, item := range planView.Plan.Items {
		if item.Path == "projects/APP/project.md" && item.Action == contracts.RestorePlanUpdate {
			hasUpdate = true
		}
	}
	if !hasUpdate {
		t.Fatalf("restore plan should notice changed project file: %#v", planView.Plan.Items)
	}
	if _, err := actions.ApplyRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"), "restore backup", false); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("restore apply should require --yes, got %v", err)
	}
	applied, err := actions.ApplyRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"), "restore backup", true)
	if err != nil {
		t.Fatalf("apply restore: %v", err)
	}
	if applied.Applied == 0 {
		t.Fatalf("restore should apply at least one changed file: %#v", applied)
	}
	after, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read restored project: %v", err)
	}
	if strings.Contains(string(after), "tampered") {
		t.Fatalf("restore apply did not replace tampered content")
	}
}

func TestGoalManifestSignVerifyAndAdminStatus(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "owner"}, contracts.Actor("human:owner"), "goal signer")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "owner", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust goal signer"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	brief, err := actions.GoalBrief(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("goal brief: %v", err)
	}
	if brief.Brief.TargetID != ticket.ID || len(brief.Brief.Sections) != len(contracts.GoalManifestSectionOrder) {
		t.Fatalf("brief should use stable goal sections: %#v", brief.Brief)
	}
	manifest, err := actions.CreateGoalManifest(ctx, ticket.ID, contracts.Actor("human:owner"), "create goal manifest")
	if err != nil {
		t.Fatalf("goal manifest: %v", err)
	}
	if manifest.Manifest.PolicySnapshotHash == "" || manifest.Manifest.TrustSnapshotHash == "" || manifest.Manifest.SourceHash == "" || manifest.Manifest.GeneratedBy != contracts.Actor("human:owner") || manifest.Manifest.Reason == "" {
		t.Fatalf("goal manifest should bind source, policy, trust, and creation metadata: %#v", manifest.Manifest)
	}
	if _, err := actions.SignGoalManifest(ctx, manifest.Manifest.ManifestID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign goal"); err != nil {
		t.Fatalf("sign goal: %v", err)
	}
	verified, err := actions.VerifyGoalManifest(ctx, manifest.Manifest.ManifestID)
	if err != nil {
		t.Fatalf("verify goal: %v", err)
	}
	if verified.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("goal signature should verify trusted: %#v", verified.Signature)
	}
	admin, err := actions.AdminSecurityStatus(context.Background())
	if err != nil {
		t.Fatalf("admin status: %v", err)
	}
	if admin.PublicKeys == 0 || admin.TrustBindings == 0 || admin.GoalManifests == 0 {
		t.Fatalf("admin security status should count v1.7 artifacts: %#v", admin)
	}
}

func TestGoalBriefFailsClosedOnCorruptContext(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if err := os.MkdirAll(storage.GatesDir(actions.Root), 0o755); err != nil {
		t.Fatalf("create gates dir: %v", err)
	}
	if err := os.WriteFile(storage.GateFile(actions.Root, "gate_corrupt"), []byte("---\ngate_id: [\n"), 0o644); err != nil {
		t.Fatalf("write corrupt gate: %v", err)
	}
	if _, err := actions.GoalBrief(ctx, ticket.ID); err == nil {
		t.Fatalf("goal brief should fail closed when related context cannot load")
	}
}

func TestGoalBriefFailsClosedOnCorruptTicketTarget(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if err := os.WriteFile(storage.TicketFile(actions.Root, ticket.Project, ticket.ID), []byte("---\nid: [\n"), 0o644); err != nil {
		t.Fatalf("corrupt ticket: %v", err)
	}
	if _, err := actions.GoalBrief(ctx, ticket.ID); err == nil {
		t.Fatalf("goal brief should return the ticket load error instead of falling through to run lookup")
	}
}

func TestRecoveryDrillIsSideEffectFree(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	before, err := collectExportFiles(actions.Root)
	if err != nil {
		t.Fatalf("collect before: %v", err)
	}
	drill, err := actions.RecoveryDrill(ctx)
	if err != nil {
		t.Fatalf("recovery drill: %v", err)
	}
	after, err := collectExportFiles(actions.Root)
	if err != nil {
		t.Fatalf("collect after: %v", err)
	}
	if !drill.SideEffectFree || strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("recovery drill should be side-effect free: %#v", drill)
	}
}
