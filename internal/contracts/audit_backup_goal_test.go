package contracts

import (
	"strings"
	"testing"
	"time"
)

func TestV17AuditBackupGoalContractsValidate(t *testing.T) {
	now := time.Now().UTC()
	report := AuditReport{
		AuditReportID:      "audit_1",
		ScopeKind:          AuditScopeTicket,
		ScopeID:            "APP-1",
		GeneratedAt:        now,
		GeneratedBy:        Actor("human:owner"),
		EventRange:         EventRange{FromEventID: 1, ToEventID: 5},
		PolicySnapshotHash: "policyhash",
		TrustSnapshotHash:  "trusthash",
		Findings: []AuditFinding{{
			FindingID: "finding_1",
			Severity:  AuditFindingInfo,
			Code:      "ok",
			Message:   "all good",
		}},
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("valid audit report rejected: %v", err)
	}
	report.ScopeKind = AuditScopeWorkspace
	report.ScopeID = ""
	if err := report.Validate(); err != nil {
		t.Fatalf("workspace audit report should not require scope id: %v", err)
	}
	report.ScopeID = "APP-1"
	if err := report.Validate(); err == nil {
		t.Fatalf("workspace audit report with scope id should fail")
	}
	report.ScopeKind = AuditScopeTicket
	report.ScopeID = ""
	if err := report.Validate(); err == nil {
		t.Fatalf("ticket audit report without scope id should fail")
	}
	report.ScopeID = "APP-1"
	report.PolicySnapshotHash = ""
	if err := report.Validate(); err == nil {
		t.Fatalf("audit report without policy snapshot hash should fail")
	}
	report.PolicySnapshotHash = "policyhash"
	report.EventRange = EventRange{FromEventID: 9, ToEventID: 2}
	if err := report.Validate(); err == nil {
		t.Fatalf("audit report with inverted event range should fail")
	}
	report.EventRange = EventRange{FromEventID: 1, ToEventID: 5}
	report.IncludedArtifactHashes = []ArtifactHash{{Kind: ArtifactKindBundle, UID: "bundle_1"}}
	if err := report.Validate(); err == nil {
		t.Fatalf("audit report with malformed artifact hash should fail")
	}
	report.IncludedArtifactHashes = []ArtifactHash{{Kind: ArtifactKindBundle, UID: "bundle_1", SHA256: "abc123"}}
	if err := report.Validate(); err != nil {
		t.Fatalf("valid audit artifact hash rejected: %v", err)
	}
	report.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindGoalManifest, "goal_1", now)}
	if err := report.Validate(); err == nil {
		t.Fatalf("audit report with mismatched signature envelope should fail")
	}
	report.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindAuditReport, "audit_1", now)}
	if err := report.Validate(); err != nil {
		t.Fatalf("audit report with matching signature envelope rejected: %v", err)
	}
	report.SignatureEnvelopes = nil
	packet := AuditPacket{
		PacketID:         "packet_1",
		Report:           report,
		Canonicalization: CanonicalizationAtlasV1,
		PacketHash:       "packethash",
		SchemaVersion:    CurrentSchemaVersion,
	}
	if err := packet.Validate(); err != nil {
		t.Fatalf("valid audit packet rejected: %v", err)
	}
	packet.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindAuditReport, "audit_1", now)}
	if err := packet.Validate(); err == nil {
		t.Fatalf("audit packet with mismatched signature envelope should fail")
	}
	packet.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindAuditPacket, "packet_1", now)}
	if err := packet.Validate(); err != nil {
		t.Fatalf("audit packet with matching signature envelope rejected: %v", err)
	}

	backup := BackupSnapshot{
		BackupID:         "backup_1",
		CreatedAt:        now,
		CreatedBy:        Actor("human:owner"),
		ScopeKind:        BackupScopeWorkspace,
		ManifestHash:     "manifesthash",
		IncludedSections: []BackupSection{BackupSectionProjects, BackupSectionPublicSecurity},
		SchemaVersion:    CurrentSchemaVersion,
	}
	if err := backup.Validate(); err != nil {
		t.Fatalf("valid backup rejected: %v", err)
	}
	backup.ScopeID = "APP"
	if err := backup.Validate(); err == nil {
		t.Fatalf("workspace backup with scope id should fail")
	}
	backup.ScopeID = ""
	backup.ScopeKind = BackupScopeProject
	backup.ScopeID = ""
	if err := backup.Validate(); err == nil {
		t.Fatalf("project backup without scope id should fail")
	}
	backup.ScopeID = "APP"
	if err := backup.Validate(); err != nil {
		t.Fatalf("valid project backup rejected: %v", err)
	}
	backup.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindAuditReport, "audit_1", now)}
	if err := backup.Validate(); err == nil {
		t.Fatalf("backup with mismatched signature envelope should fail")
	}
	backup.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindBackupSnapshot, "backup_1", now)}
	if err := backup.Validate(); err != nil {
		t.Fatalf("backup with matching signature envelope rejected: %v", err)
	}
	backup.SignatureEnvelopes = nil
	restoreItem := RestorePlanItem{Path: ".tracker/events/events.jsonl", Action: RestorePlanBlock}
	if err := restoreItem.Validate(); err == nil {
		t.Fatalf("blocked restore item without reason code should fail")
	}
	for _, path := range []string{
		"/tmp/outside",
		"../outside",
		".tracker/../secret",
		"README.md",
		".tracker/runtime/run_1/launch.codex.txt",
		".tracker/security/keys/private/key.json",
		".tracker/backups/snapshots/backup.tar.gz",
		".tracker/goal/manifests/goal.json",
		" projects/APP/tickets/APP-1.md ",
	} {
		restoreItem.Path = path
		restoreItem.ReasonCodes = []string{"unsafe_path"}
		if err := restoreItem.Validate(); err == nil {
			t.Fatalf("unsafe restore path %q should fail", path)
		}
	}
	for _, path := range []string{
		"projects/APP/tickets/APP-1.md",
		".tracker/security/keys/public/key.md",
		".tracker/audit/packets/audit_1.json",
		".tracker/governance/policies/default.toml",
	} {
		restoreItem.Path = path
		restoreItem.ReasonCodes = []string{"untrusted_signer"}
		if err := restoreItem.Validate(); err != nil {
			t.Fatalf("canonical restore path %q rejected: %v", path, err)
		}
	}
	restoreItem.Path = ".tracker/events/events.jsonl"
	restoreItem.ReasonCodes = []string{"untrusted_signer"}
	if err := restoreItem.Validate(); err != nil {
		t.Fatalf("valid blocked restore item rejected: %v", err)
	}

	manifest := GoalManifest{
		ManifestID:    "goal_1",
		TargetKind:    GoalTargetTicket,
		TargetID:      "APP-1",
		Objective:     "Fix the thing",
		Sections:      completeGoalSections(),
		SourceHash:    "abc123",
		GeneratedAt:   now,
		GeneratedBy:   Actor("human:owner"),
		Reason:        "prepare agent goal",
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid goal manifest rejected: %v", err)
	}
	manifest.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindBackupSnapshot, "backup_1", now)}
	if err := manifest.Validate(); err == nil {
		t.Fatalf("goal manifest with mismatched signature envelope should fail")
	}
	manifest.SignatureEnvelopes = []SignatureEnvelope{signatureForArtifact(ArtifactKindGoalManifest, "goal_1", now)}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("goal manifest with matching signature envelope rejected: %v", err)
	}
	manifest.SignatureEnvelopes = nil
	manifest.Sections[0].Body = ""
	if err := manifest.Validate(); err == nil {
		t.Fatalf("empty goal section should fail")
	}
	manifest.Sections = completeGoalSections()
	manifest.Sections[0], manifest.Sections[1] = manifest.Sections[1], manifest.Sections[0]
	if err := manifest.Validate(); err == nil {
		t.Fatalf("out-of-order goal manifest sections should fail")
	}
}

func signatureForArtifact(kind ArtifactKind, uid string, signedAt time.Time) SignatureEnvelope {
	return SignatureEnvelope{
		SignatureID:             "sig_1",
		ArtifactKind:            kind,
		ArtifactUID:             uid,
		CanonicalizationVersion: CanonicalizationAtlasV1,
		PayloadHashAlgorithm:    PayloadHashSHA256,
		PayloadHash:             strings.Repeat("a", 64),
		SignedAt:                signedAt,
		SignerKind:              PublicKeyOwnerCollaborator,
		SignerID:                "rev-1",
		PublicKeyID:             "key_1",
		PublicKeyFingerprint:    "ed25519:abc",
		Algorithm:               KeyAlgorithmEd25519,
		Signature:               "base64-signature",
		SchemaVersion:           CurrentSchemaVersion,
	}
}

func completeGoalSections() []GoalSection {
	sections := make([]GoalSection, 0, len(GoalManifestSectionOrder))
	for _, heading := range GoalManifestSectionOrder {
		sections = append(sections, GoalSection{Heading: heading, Body: "None"})
	}
	return sections
}
