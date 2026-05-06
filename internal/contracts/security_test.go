package contracts

import (
	"strings"
	"testing"
	"time"
)

func TestV17KeyStateTransitions(t *testing.T) {
	if !CanTransitionKeyState(KeyStateGenerated, KeyStateActive) {
		t.Fatalf("generated key should transition to active")
	}
	if !CanTransitionKeyState(KeyStateActive, KeyStateRotated) {
		t.Fatalf("active key should transition to rotated")
	}
	if CanTransitionKeyState(KeyStateRevoked, KeyStateActive) {
		t.Fatalf("revoked key must not reactivate")
	}
	if CanTransitionKeyState(KeyStateExpired, KeyStateActive) {
		t.Fatalf("expired key must not become active again")
	}
}

func TestV17SecurityContractsValidate(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	key := PublicKeyRecord{
		PublicKeyID:       "key_1",
		Fingerprint:       "ed25519:abc",
		Algorithm:         KeyAlgorithmEd25519,
		PublicKeyMaterial: "AAA",
		OwnerKind:         PublicKeyOwnerCollaborator,
		OwnerID:           "rev-1",
		CreatedAt:         now,
		Status:            KeyStateImported,
		Source:            PublicKeySourceManualImport,
		SchemaVersion:     CurrentSchemaVersion,
	}
	if err := key.Validate(); err != nil {
		t.Fatalf("valid public key record rejected: %v", err)
	}
	key.Status = KeyState("bogus")
	if err := key.Validate(); err == nil {
		t.Fatalf("invalid key status should fail validation")
	}

	binding := TrustBinding{
		TrustBindingID:   "trust_1",
		PublicKeyID:      "key_1",
		Fingerprint:      "ed25519:abc",
		TrustedOwnerKind: PublicKeyOwnerCollaborator,
		TrustedOwnerID:   "rev-1",
		TrustedBy:        Actor("human:owner"),
		TrustLevel:       TrustLevelTrusted,
		AllowedScopes:    []TrustScope{TrustScopeBundle, TrustScopeApproval},
		CreatedAt:        now,
		UpdatedAt:        now,
		Reason:           "manual trust ceremony",
		LocalOnly:        true,
		SchemaVersion:    CurrentSchemaVersion,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("valid trust binding rejected: %v", err)
	}
	binding.Reason = ""
	if err := binding.Validate(); err == nil {
		t.Fatalf("trust binding without reason should fail")
	}
	binding.Reason = "manual trust ceremony"
	binding.AllowedScopes = nil
	if err := binding.Validate(); err == nil {
		t.Fatalf("trusted binding without allowed scopes should fail")
	}
	binding.TrustLevel = TrustLevelRevoked
	if err := binding.Validate(); err == nil {
		t.Fatalf("revoked trust binding without revoked_at should fail")
	}
	binding.RevokedAt = now
	if err := binding.Validate(); err != nil {
		t.Fatalf("revoked trust binding with revoked_at rejected: %v", err)
	}

	revocation := RevocationRecord{
		RevocationID:  "rev_1",
		PublicKeyID:   "key_1",
		Fingerprint:   "ed25519:abc",
		RevokedBy:     Actor("human:owner"),
		RevokedAt:     now,
		Reason:        "compromised",
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := revocation.Validate(); err != nil {
		t.Fatalf("valid revocation rejected: %v", err)
	}
	revocation.RevokedAt = time.Time{}
	if err := revocation.Validate(); err == nil {
		t.Fatalf("revocation without timestamp should fail")
	}
}

func TestV17VerificationStatesAreFrozen(t *testing.T) {
	states := []SignatureVerificationState{
		VerificationTrustedValid,
		VerificationValidUntrusted,
		VerificationValidUnknownKey,
		VerificationValidRevokedKey,
		VerificationValidExpiredKey,
		VerificationValidRotatedKey,
		VerificationInvalidSignature,
		VerificationMissingSignature,
		VerificationMalformedSignature,
		VerificationPayloadHashMismatch,
		VerificationCanonicalizationMismatch,
		VerificationUnsupportedSignatureVersion,
	}
	for _, state := range states {
		if !state.IsValid() {
			t.Fatalf("expected verification state %q to be valid", state)
		}
	}
	if SignatureVerificationState("verified_trusted").IsValid() {
		t.Fatalf("old v1.6/v1.6.1 style verification wording should not be accepted")
	}
}

func TestV17ArtifactKindsHaveTrustScopes(t *testing.T) {
	kinds := []ArtifactKind{
		ArtifactKindBundle,
		ArtifactKindSyncPublication,
		ArtifactKindApproval,
		ArtifactKindHandoff,
		ArtifactKindEvidencePacket,
		ArtifactKindAuditReport,
		ArtifactKindAuditPacket,
		ArtifactKindBackupSnapshot,
		ArtifactKindGoalManifest,
	}
	for _, kind := range kinds {
		scope, ok := TrustScopeForArtifactKind(kind)
		if !ok {
			t.Fatalf("missing trust scope for artifact kind %q", kind)
		}
		if !scope.IsValid() {
			t.Fatalf("artifact kind %q maps to invalid trust scope %q", kind, scope)
		}
	}
	if _, ok := TrustScopeForArtifactKind(ArtifactKind("release")); ok {
		t.Fatalf("release trust scope should not imply a signed artifact kind")
	}
}

func TestV17SignatureEnvelopeValidation(t *testing.T) {
	envelope := SignatureEnvelope{
		SignatureID:             "sig_1",
		ArtifactKind:            ArtifactKindBundle,
		ArtifactUID:             "bundle_1",
		CanonicalizationVersion: CanonicalizationAtlasV1,
		PayloadHashAlgorithm:    PayloadHashSHA256,
		PayloadHash:             strings.Repeat("a", 64),
		SignedAt:                time.Now().UTC(),
		SignerKind:              PublicKeyOwnerCollaborator,
		SignerID:                "rev-1",
		PublicKeyID:             "key_1",
		PublicKeyFingerprint:    "ed25519:abc",
		Algorithm:               KeyAlgorithmEd25519,
		Signature:               "base64-signature",
		VerificationState:       VerificationTrustedValid,
		SchemaVersion:           CurrentSchemaVersion,
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("valid signature envelope rejected: %v", err)
	}
	envelope.CanonicalizationVersion = CanonicalizationVersion("atlas-c14n-v2")
	if err := envelope.Validate(); err == nil {
		t.Fatalf("unsupported canonicalization should fail")
	}
}

func TestV17SignatureVerificationResultValidation(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	result := SignatureVerificationResult{
		ArtifactKind:     ArtifactKindBundle,
		ArtifactUID:      "bundle_1",
		State:            VerificationTrustedValid,
		TrustedOwnerKind: PublicKeyOwnerCollaborator,
		TrustedOwnerID:   "rev-1",
		VerifiedAt:       now,
		SchemaVersion:    CurrentSchemaVersion,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid trusted verification result rejected: %v", err)
	}
	envelope := SignatureEnvelope{
		SignatureID:             "sig_1",
		ArtifactKind:            ArtifactKindBundle,
		ArtifactUID:             "bundle_1",
		CanonicalizationVersion: CanonicalizationAtlasV1,
		PayloadHashAlgorithm:    PayloadHashSHA256,
		PayloadHash:             strings.Repeat("a", 64),
		SignedAt:                now,
		SignerKind:              PublicKeyOwnerCollaborator,
		SignerID:                "rev-1",
		PublicKeyID:             "key_1",
		PublicKeyFingerprint:    "ed25519:abc",
		Algorithm:               KeyAlgorithmEd25519,
		Signature:               "base64-signature",
		SchemaVersion:           CurrentSchemaVersion,
	}
	result.Signature = &envelope
	if err := result.Validate(); err != nil {
		t.Fatalf("matching signature envelope should validate: %v", err)
	}
	result.Signature.ArtifactUID = "goal_1"
	if err := result.Validate(); err == nil {
		t.Fatalf("verification result with mismatched signature artifact should fail")
	}
	result.Signature = nil
	result.TrustedOwnerID = ""
	if err := result.Validate(); err == nil {
		t.Fatalf("trusted verification result without owner id should fail")
	}
	result.TrustedOwnerID = "rev-1"
	result.TrustedOwnerKind = PublicKeyOwnerKind("team")
	if err := result.Validate(); err == nil {
		t.Fatalf("trusted verification result with malformed owner kind should fail")
	}
	result.TrustedOwnerKind = ""
	result.State = VerificationValidUntrusted
	if err := result.Validate(); err == nil {
		t.Fatalf("owner id without owner kind should fail")
	}
	result.TrustedOwnerID = ""
	if err := result.Validate(); err != nil {
		t.Fatalf("untrusted verification result should not require owner metadata: %v", err)
	}
	result.VerifiedAt = time.Time{}
	if err := result.Validate(); err == nil {
		t.Fatalf("verification result without verified_at should fail")
	}
}
