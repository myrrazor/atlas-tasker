package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

func TestRestoreApplyRequiresBoundPlanAndRefusesStaleDigest(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	view, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "bind restore")
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if _, err := actions.ApplyRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"), "no plan", true); err == nil || !strings.Contains(err.Error(), "bound restore plan") {
		t.Fatalf("apply without plan must fail, got %v", err)
	}
	planned, err := actions.CreateRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("restore plan: %v", err)
	}
	if planned.PlanID == "" || planned.PlanDigest == "" || planned.PlanDigest != RestorePlanBindingDigest(planned.Plan) {
		t.Fatalf("plan digest missing: %#v", planned)
	}
	projectPath := storage.ProjectFile(actions.Root, "APP")
	if err := os.WriteFile(projectPath, []byte("changed after plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.ApplyRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"), "stale", true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale plan must fail, got %v", err)
	}
	if _, err := actions.CreateRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner")); err != nil {
		t.Fatal(err)
	}
	applied, err := actions.ApplyRestorePlan(ctx, planned.PlanID, contracts.Actor("human:owner"), "fresh bind", true)
	if err == nil {
		t.Fatal("apply by old plan id must still fail after the live tree moved")
	}
	_ = applied
	fresh, err := actions.CreateRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actions.ApplyRestorePlan(ctx, fresh.PlanID, contracts.Actor("human:owner"), "apply bound plan", true); err != nil {
		t.Fatalf("apply by plan id: %v", err)
	}
}

func TestRestoreApplyRefusesSwappedBackupContent(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	first, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "source a")
	if err != nil {
		t.Fatal(err)
	}
	planned, err := actions.CreateRestorePlan(ctx, first.Snapshot.BackupID, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatal(err)
	}
	if planned.Plan.SourceManifestHash == "" || planned.Plan.SourceArchiveHash == "" {
		t.Fatalf("plan must bind source content hashes: %#v", planned.Plan)
	}
	if err := os.WriteFile(storage.ProjectFile(actions.Root, "APP"), []byte("mutated source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "source b")
	if err != nil {
		t.Fatal(err)
	}
	copyFile := func(src, dst string) {
		t.Helper()
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	copyFile(backupArchivePath(actions.Root, second.Snapshot.BackupID), backupArchivePath(actions.Root, first.Snapshot.BackupID))
	copyFile(backupManifestPath(actions.Root, second.Snapshot.BackupID), backupManifestPath(actions.Root, first.Snapshot.BackupID))
	replaced := first.Snapshot
	replaced.ManifestHash = second.Snapshot.ManifestHash
	if err := actions.Backups.SaveBackupSnapshot(ctx, replaced); err != nil {
		t.Fatal(err)
	}
	_, err = actions.ApplyRestorePlan(ctx, first.Snapshot.BackupID, contracts.Actor("human:owner"), "swapped", true)
	if err == nil || (!strings.Contains(err.Error(), "content") && !strings.Contains(err.Error(), "archive") && !strings.Contains(err.Error(), "stale")) {
		t.Fatalf("swapped backup content must fail binding, got %v", err)
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
	if got := goalHeadings(brief.Brief.Sections); strings.Join(got, "\n") != strings.Join(contracts.GoalManifestSectionOrder, "\n") {
		t.Fatalf("brief headings mismatch:\ngot  %v\nwant %v", got, contracts.GoalManifestSectionOrder)
	}
	if goalSectionContains(brief.Brief.Sections, "Current State", "None") {
		t.Fatalf("current state should not mix status lines with a None blocker fallback: %#v", brief.Brief.Sections)
	}
	if !goalSectionContains(brief.Brief.Sections, "Suggested Commands", "tracker goal brief "+ticket.ID+" --md") {
		t.Fatalf("suggested commands should include a paste-ready markdown brief command: %#v", brief.Brief.Sections)
	}
	manifest, err := actions.CreateGoalManifest(ctx, ticket.ID, contracts.Actor("human:owner"), "create goal manifest")
	if err != nil {
		t.Fatalf("goal manifest: %v", err)
	}
	if manifest.Manifest.PolicySnapshotHash == "" || manifest.Manifest.TrustSnapshotHash == "" || manifest.Manifest.SourceHash == "" || manifest.Manifest.GeneratedBy != contracts.Actor("human:owner") || manifest.Manifest.Reason == "" {
		t.Fatalf("goal manifest should bind source, policy, trust, and creation metadata: %#v", manifest.Manifest)
	}
	reloaded, err := actions.GoalManifestDetail(ctx, manifest.Manifest.ManifestID)
	if err != nil {
		t.Fatalf("reload goal manifest: %v", err)
	}
	if got := goalHeadings(reloaded.Manifest.Sections); strings.Join(got, "\n") != strings.Join(contracts.GoalManifestSectionOrder, "\n") {
		t.Fatalf("reloaded manifest headings mismatch:\ngot  %v\nwant %v", got, contracts.GoalManifestSectionOrder)
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

func TestGoalBriefAndManifestSupportRunTargets(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	run := contracts.RunSnapshot{
		RunID:         "RUN-42",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		AgentID:       "builder-1",
		Provider:      contracts.AgentProviderCodex,
		Status:        contracts.RunStatusActive,
		Summary:       "finish the run target work",
		CreatedAt:     defaultTestTime(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	brief, err := actions.GoalBrief(ctx, run.RunID)
	if err != nil {
		t.Fatalf("run goal brief: %v", err)
	}
	if brief.Brief.TargetKind != contracts.GoalTargetRun || brief.Brief.TargetID != run.RunID {
		t.Fatalf("brief should target run: %#v", brief.Brief)
	}
	if got := goalHeadings(brief.Brief.Sections); strings.Join(got, "\n") != strings.Join(contracts.GoalManifestSectionOrder, "\n") {
		t.Fatalf("run brief headings mismatch:\ngot  %v\nwant %v", got, contracts.GoalManifestSectionOrder)
	}
	if !goalSectionContains(brief.Brief.Sections, "Ticket / Run", run.RunID+" for "+ticket.ID) {
		t.Fatalf("run brief should include run/ticket line: %#v", brief.Brief.Sections)
	}
	if !goalSectionContains(brief.Brief.Sections, "Current State", "run status: "+string(run.Status)) {
		t.Fatalf("run brief should include run status: %#v", brief.Brief.Sections)
	}
	if !goalSectionContains(brief.Brief.Sections, "Suggested Commands", "tracker goal brief "+run.RunID+" --md") {
		t.Fatalf("run suggested commands should include a paste-ready markdown brief command: %#v", brief.Brief.Sections)
	}
	if !goalSectionContains(brief.Brief.Sections, "Suggested Commands", "tracker run evidence add "+run.RunID+" --type test_result") {
		t.Fatalf("run suggested commands should use a valid evidence type: %#v", brief.Brief.Sections)
	}
	manifest, err := actions.CreateGoalManifest(ctx, run.RunID, contracts.Actor("human:owner"), "create run goal")
	if err != nil {
		t.Fatalf("run goal manifest: %v", err)
	}
	if manifest.Manifest.TargetKind != contracts.GoalTargetRun || manifest.Manifest.TargetID != run.RunID {
		t.Fatalf("manifest should target run: %#v", manifest.Manifest)
	}
}

func TestGoalBriefUsesRunWorkerAndEffectiveReviewerIdentities(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	ticket.Assignee = contracts.Actor("agent:assigned-builder")
	ticket.Reviewer = contracts.Actor("agent:reviewer-1")
	ticket.Policy.CompletionMode = contracts.CompletionModeReviewGate
	if err := actions.Tickets.UpdateTicket(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	run := contracts.RunSnapshot{
		RunID:         "RUN-IDENTITY",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		AgentID:       "run-builder",
		Provider:      contracts.AgentProviderCodex,
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     defaultTestTime(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	brief, err := actions.GoalBrief(ctx, run.RunID)
	if err != nil {
		t.Fatalf("goal brief: %v", err)
	}
	commands := goalSectionText(brief.Brief.Sections, "Suggested Commands")
	for _, want := range []string{
		"tracker ticket claim " + ticket.ID + " --actor 'agent:run-builder' --reason \"start work\"",
		"--next-actor 'agent:reviewer-1'",
		"the worker requests review; the reviewer approves separately",
		"tracker ticket approve " + ticket.ID + " --actor 'agent:reviewer-1'",
		"reviewer approval completes the ticket in review_gate mode",
		"tracker ticket view " + ticket.ID + " --json",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("suggested commands missing %q:\n%s", want, commands)
		}
	}
	if strings.Contains(commands, "agent:assigned-builder") || strings.Contains(commands, "<actor>") || strings.Contains(commands, "<reviewer>") || strings.Contains(commands, "tracker ticket complete") {
		t.Fatalf("run identity should override the assignee without literal placeholders:\n%s", commands)
	}
}

func TestGoalBriefRequiresExplicitIdentityWhenTicketIsUnassigned(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	brief, err := actions.GoalBrief(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("goal brief: %v", err)
	}
	commands := goalSectionText(brief.Brief.Sections, "Suggested Commands")
	for _, want := range []string{
		"set TRACKER_ACTOR to the valid",
		`--actor "$TRACKER_ACTOR"`,
		"set TRACKER_REVIEWER to the valid reviewer identity",
		`--reviewer "$TRACKER_REVIEWER"`,
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("suggested commands missing %q:\n%s", want, commands)
		}
	}
	if strings.Contains(commands, "<actor>") || strings.Contains(commands, "<reviewer>") || strings.Contains(commands, "human:owner") {
		t.Fatalf("an unassigned open-mode brief must not invent or impersonate an identity:\n%s", commands)
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

func goalHeadings(sections []contracts.GoalSection) []string {
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		out = append(out, section.Heading)
	}
	return out
}

func goalSectionContains(sections []contracts.GoalSection, heading string, text string) bool {
	for _, section := range sections {
		if section.Heading != heading {
			continue
		}
		if strings.Contains(section.Body, text) {
			return true
		}
		for _, item := range section.Items {
			if strings.Contains(item, text) {
				return true
			}
		}
	}
	return false
}

func goalSectionText(sections []contracts.GoalSection, heading string) string {
	for _, section := range sections {
		if section.Heading == heading {
			return section.Body + "\n" + strings.Join(section.Items, "\n")
		}
	}
	return ""
}

func defaultTestTime() time.Time {
	return time.Date(2026, 5, 6, 18, 30, 0, 0, time.UTC)
}
