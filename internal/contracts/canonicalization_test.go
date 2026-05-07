package contracts

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalizeAtlasV1DeterministicWeirdCases(t *testing.T) {
	left := map[string]any{
		"unicode": "snowman ☃",
		"paths":   []string{"b\\c", "a/b"},
		"text":    "line 1\r\nline 2\rline 3",
		"empty":   []string{},
		"nested": map[string]any{
			"z": int64(2),
			"a": "first",
		},
		"when": time.Date(2026, 5, 6, 8, 30, 0, 123, time.FixedZone("EDT", -4*60*60)),
	}
	right := map[string]any{
		"when":    time.Date(2026, 5, 6, 12, 30, 0, 123, time.UTC),
		"nested":  map[string]any{"a": "first", "z": int64(2)},
		"empty":   []string{},
		"text":    "line 1\nline 2\nline 3",
		"paths":   []string{"b\\c", "a/b"},
		"unicode": "snowman ☃",
	}
	a, err := CanonicalizeAtlasV1(left)
	if err != nil {
		t.Fatalf("canonicalize left: %v", err)
	}
	b, err := CanonicalizeAtlasV1(right)
	if err != nil {
		t.Fatalf("canonicalize right: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("expected equivalent semantic payloads to match\nleft:  %s\nright: %s", a, b)
	}
	want := `{"empty":[],"nested":{"a":"first","z":2},"paths":["b\\c","a/b"],"text":"line 1\nline 2\nline 3","unicode":"snowman ☃","when":"2026-05-06T12:30:00.000000123Z"}`
	if string(a) != want {
		t.Fatalf("unexpected canonical bytes\nwant: %s\n got: %s", want, a)
	}
}

func TestCanonicalizeAtlasV1RejectsFloatsAndInvalidUTF8(t *testing.T) {
	if _, err := CanonicalizeAtlasV1(map[string]any{"bad": 1.25}); err == nil {
		t.Fatalf("floats must not be accepted in signed manifests")
	}
	bad := string([]byte{0xff, 0xfe})
	if _, err := CanonicalizeAtlasV1(map[string]any{"bad": bad}); err == nil {
		t.Fatalf("invalid utf-8 must not be accepted")
	}
}

func TestCanonicalizeAtlasV1RejectsExcessiveDepth(t *testing.T) {
	payload := any("leaf")
	for i := 0; i < maxCanonicalDepth+2; i++ {
		payload = map[string]any{"next": payload}
	}
	if _, err := CanonicalizeAtlasV1(payload); err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("excessive nesting should fail closed, got %v", err)
	}
}

func TestCanonicalizeAtlasV1TamperChangesBytes(t *testing.T) {
	base, err := CanonicalizeAtlasV1(map[string]any{"payload_hash": strings.Repeat("a", 64)})
	if err != nil {
		t.Fatalf("canonicalize base: %v", err)
	}
	tampered, err := CanonicalizeAtlasV1(map[string]any{"payload_hash": strings.Repeat("b", 64)})
	if err != nil {
		t.Fatalf("canonicalize tampered: %v", err)
	}
	if string(base) == string(tampered) {
		t.Fatalf("one-byte semantic tamper should change canonical bytes")
	}
}

func TestCanonicalizeAtlasV1OmitsEmptySlicesAndMaps(t *testing.T) {
	type payload struct {
		Heading string         `json:"heading"`
		Items   []string       `json:"items,omitempty"`
		Meta    map[string]int `json:"meta,omitempty"`
		Until   time.Time      `json:"until,omitempty"`
		Body    string         `json:"body,omitempty"`
	}

	nilFields, err := CanonicalizeAtlasV1(payload{Heading: "Goal", Body: "ship it"})
	if err != nil {
		t.Fatalf("canonicalize nil fields: %v", err)
	}
	emptyFields, err := CanonicalizeAtlasV1(payload{
		Heading: "Goal",
		Items:   []string{},
		Meta:    map[string]int{},
		Body:    "ship it",
	})
	if err != nil {
		t.Fatalf("canonicalize empty fields: %v", err)
	}
	if string(nilFields) != string(emptyFields) {
		t.Fatalf("empty omitempty slices/maps should match nil round-trip form\nnil:   %s\nempty: %s", nilFields, emptyFields)
	}
	if strings.Contains(string(emptyFields), "until") {
		t.Fatalf("zero omitempty timestamp should be absent from canonical bytes: %s", emptyFields)
	}
}

func TestCanonicalizeAtlasV1SignatureEnvelopeIgnoresVerificationState(t *testing.T) {
	envelope := SignatureEnvelope{
		SignatureID:             "sig_1",
		ArtifactKind:            ArtifactKindBundle,
		ArtifactUID:             "bundle_1",
		CanonicalizationVersion: CanonicalizationAtlasV1,
		PayloadHashAlgorithm:    PayloadHashSHA256,
		PayloadHash:             strings.Repeat("a", 64),
		SignedAt:                time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
		SignerKind:              PublicKeyOwnerCollaborator,
		SignerID:                "rev-1",
		PublicKeyID:             "key_1",
		PublicKeyFingerprint:    "ed25519:abc",
		Algorithm:               KeyAlgorithmEd25519,
		Signature:               "base64-signature",
		SchemaVersion:           CurrentSchemaVersion,
	}
	unsignedView, err := CanonicalizeAtlasV1(envelope)
	if err != nil {
		t.Fatalf("canonicalize unsigned view: %v", err)
	}
	envelope.VerificationState = VerificationTrustedValid
	verifiedView, err := CanonicalizeAtlasV1(envelope)
	if err != nil {
		t.Fatalf("canonicalize verified view: %v", err)
	}
	if string(unsignedView) != string(verifiedView) {
		t.Fatalf("verification state must not change signed canonical bytes\nbase: %s\nverified: %s", unsignedView, verifiedView)
	}
}

func TestCanonicalizeAtlasV1SignedArtifactsIgnoreEmbeddedSignatures(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	report := AuditReport{
		AuditReportID:      "audit_1",
		ScopeKind:          AuditScopeWorkspace,
		GeneratedAt:        now,
		GeneratedBy:        Actor("human:owner"),
		EventRange:         EventRange{FromEventID: 1, ToEventID: 3},
		PolicySnapshotHash: "policyhash",
		TrustSnapshotHash:  "trusthash",
		SchemaVersion:      CurrentSchemaVersion,
	}
	baseReport, err := CanonicalizeAtlasV1(report)
	if err != nil {
		t.Fatalf("canonicalize unsigned audit report: %v", err)
	}
	report.SignatureEnvelopes = []SignatureEnvelope{sampleSignatureEnvelope()}
	signedReport, err := CanonicalizeAtlasV1(report)
	if err != nil {
		t.Fatalf("canonicalize signed audit report: %v", err)
	}
	if string(baseReport) != string(signedReport) {
		t.Fatalf("embedded audit signatures must not change signed bytes")
	}
	packet := AuditPacket{
		PacketID:         "packet_1",
		Report:           report,
		Canonicalization: CanonicalizationAtlasV1,
		PacketHash:       "packethash",
		SchemaVersion:    CurrentSchemaVersion,
	}
	basePacket, err := CanonicalizeAtlasV1(packet)
	if err != nil {
		t.Fatalf("canonicalize unsigned audit packet: %v", err)
	}
	packet.SignatureEnvelopes = []SignatureEnvelope{sampleSignatureEnvelope()}
	signedPacket, err := CanonicalizeAtlasV1(packet)
	if err != nil {
		t.Fatalf("canonicalize signed audit packet: %v", err)
	}
	if string(basePacket) != string(signedPacket) {
		t.Fatalf("embedded audit packet signatures must not change signed bytes")
	}

	backup := BackupSnapshot{
		BackupID:         "backup_1",
		CreatedAt:        now,
		CreatedBy:        Actor("human:owner"),
		ScopeKind:        BackupScopeWorkspace,
		ManifestHash:     "manifesthash",
		IncludedSections: []BackupSection{BackupSectionProjects},
		SchemaVersion:    CurrentSchemaVersion,
	}
	baseBackup, err := CanonicalizeAtlasV1(backup)
	if err != nil {
		t.Fatalf("canonicalize unsigned backup: %v", err)
	}
	backup.SignatureEnvelopes = []SignatureEnvelope{sampleSignatureEnvelope()}
	signedBackup, err := CanonicalizeAtlasV1(backup)
	if err != nil {
		t.Fatalf("canonicalize signed backup: %v", err)
	}
	if string(baseBackup) != string(signedBackup) {
		t.Fatalf("embedded backup signatures must not change signed bytes")
	}

	goal := GoalManifest{
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
	baseGoal, err := CanonicalizeAtlasV1(goal)
	if err != nil {
		t.Fatalf("canonicalize unsigned goal: %v", err)
	}
	goal.SignatureEnvelopes = []SignatureEnvelope{sampleSignatureEnvelope()}
	signedGoal, err := CanonicalizeAtlasV1(goal)
	if err != nil {
		t.Fatalf("canonicalize signed goal: %v", err)
	}
	if string(baseGoal) != string(signedGoal) {
		t.Fatalf("embedded goal signatures must not change signed bytes")
	}
}

func sampleSignatureEnvelope() SignatureEnvelope {
	return SignatureEnvelope{
		SignatureID:             "sig_1",
		ArtifactKind:            ArtifactKindAuditReport,
		ArtifactUID:             "audit_1",
		CanonicalizationVersion: CanonicalizationAtlasV1,
		PayloadHashAlgorithm:    PayloadHashSHA256,
		PayloadHash:             strings.Repeat("a", 64),
		SignedAt:                time.Date(2026, 5, 6, 12, 0, 1, 0, time.UTC),
		SignerKind:              PublicKeyOwnerCollaborator,
		SignerID:                "rev-1",
		PublicKeyID:             "key_1",
		PublicKeyFingerprint:    "ed25519:abc",
		Algorithm:               KeyAlgorithmEd25519,
		Signature:               "base64-signature",
		SchemaVersion:           CurrentSchemaVersion,
	}
}
