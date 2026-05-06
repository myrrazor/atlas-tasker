package contracts

import (
	"fmt"
	"strings"
	"time"
)

type KeyAlgorithm string

const KeyAlgorithmEd25519 KeyAlgorithm = "ed25519"

func (a KeyAlgorithm) IsValid() bool {
	return a == KeyAlgorithmEd25519
}

type KeyScope string

const (
	KeyScopeWorkspace    KeyScope = "workspace"
	KeyScopeCollaborator KeyScope = "collaborator"
	KeyScopeRelease      KeyScope = "release"
	KeyScopeAdmin        KeyScope = "admin"
)

var validKeyScopes = map[KeyScope]struct{}{
	KeyScopeWorkspace: {}, KeyScopeCollaborator: {}, KeyScopeRelease: {}, KeyScopeAdmin: {},
}

func (s KeyScope) IsValid() bool {
	_, ok := validKeyScopes[s]
	return ok
}

type KeyState string

const (
	KeyStateGenerated KeyState = "generated"
	KeyStateActive    KeyState = "active"
	KeyStateRotated   KeyState = "rotated"
	KeyStateRevoked   KeyState = "revoked"
	KeyStateExpired   KeyState = "expired"
	KeyStateLost      KeyState = "lost"
	KeyStateImported  KeyState = "imported"
	KeyStateDisabled  KeyState = "disabled"
)

var validKeyStates = map[KeyState]struct{}{
	KeyStateGenerated: {}, KeyStateActive: {}, KeyStateRotated: {}, KeyStateRevoked: {},
	KeyStateExpired: {}, KeyStateLost: {}, KeyStateImported: {}, KeyStateDisabled: {},
}

func (s KeyState) IsValid() bool {
	_, ok := validKeyStates[s]
	return ok
}

func CanTransitionKeyState(from KeyState, to KeyState) bool {
	if !from.IsValid() || !to.IsValid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case KeyStateGenerated:
		return to == KeyStateActive || to == KeyStateDisabled || to == KeyStateLost || to == KeyStateRevoked
	case KeyStateImported:
		return to == KeyStateActive || to == KeyStateDisabled || to == KeyStateRevoked
	case KeyStateActive:
		return to == KeyStateRotated || to == KeyStateRevoked || to == KeyStateExpired || to == KeyStateDisabled || to == KeyStateLost
	case KeyStateDisabled:
		return to == KeyStateActive || to == KeyStateRevoked || to == KeyStateLost
	case KeyStateRotated:
		return to == KeyStateRevoked || to == KeyStateExpired
	case KeyStateExpired, KeyStateLost:
		return to == KeyStateRevoked
	case KeyStateRevoked:
		return false
	default:
		return false
	}
}

type PublicKeyOwnerKind string

const (
	PublicKeyOwnerWorkspace    PublicKeyOwnerKind = "workspace"
	PublicKeyOwnerCollaborator PublicKeyOwnerKind = "collaborator"
	PublicKeyOwnerRelease      PublicKeyOwnerKind = "release"
)

var validPublicKeyOwnerKinds = map[PublicKeyOwnerKind]struct{}{
	PublicKeyOwnerWorkspace: {}, PublicKeyOwnerCollaborator: {}, PublicKeyOwnerRelease: {},
}

func (k PublicKeyOwnerKind) IsValid() bool {
	_, ok := validPublicKeyOwnerKinds[k]
	return ok
}

type PublicKeySource string

const (
	PublicKeySourceLocal          PublicKeySource = "local"
	PublicKeySourceSynced         PublicKeySource = "synced"
	PublicKeySourceImportedBundle PublicKeySource = "imported_bundle"
	PublicKeySourceManualImport   PublicKeySource = "manual_import"
)

var validPublicKeySources = map[PublicKeySource]struct{}{
	PublicKeySourceLocal: {}, PublicKeySourceSynced: {}, PublicKeySourceImportedBundle: {}, PublicKeySourceManualImport: {},
}

func (s PublicKeySource) IsValid() bool {
	_, ok := validPublicKeySources[s]
	return ok
}

type TrustLevel string

const (
	TrustLevelUnknown    TrustLevel = "unknown"
	TrustLevelTrusted    TrustLevel = "trusted"
	TrustLevelRestricted TrustLevel = "restricted"
	TrustLevelRevoked    TrustLevel = "revoked"
)

var validTrustLevels = map[TrustLevel]struct{}{
	TrustLevelUnknown: {}, TrustLevelTrusted: {}, TrustLevelRestricted: {}, TrustLevelRevoked: {},
}

func (l TrustLevel) IsValid() bool {
	_, ok := validTrustLevels[l]
	return ok
}

type TrustScope string

const (
	TrustScopeBundle      TrustScope = "bundle"
	TrustScopeSync        TrustScope = "sync"
	TrustScopeApproval    TrustScope = "approval"
	TrustScopeHandoff     TrustScope = "handoff"
	TrustScopeEvidence    TrustScope = "evidence"
	TrustScopeAuditReport TrustScope = "audit_report"
	TrustScopeAuditPacket TrustScope = "audit_packet"
	TrustScopeBackup      TrustScope = "backup"
	TrustScopeGoal        TrustScope = "goal"
	TrustScopeRelease     TrustScope = "release"
)

var validTrustScopes = map[TrustScope]struct{}{
	TrustScopeBundle: {}, TrustScopeSync: {}, TrustScopeApproval: {}, TrustScopeHandoff: {},
	TrustScopeEvidence: {}, TrustScopeAuditReport: {}, TrustScopeAuditPacket: {}, TrustScopeBackup: {},
	TrustScopeGoal: {}, TrustScopeRelease: {},
}

func (s TrustScope) IsValid() bool {
	_, ok := validTrustScopes[s]
	return ok
}

type ArtifactKind string

const (
	ArtifactKindBundle          ArtifactKind = "bundle"
	ArtifactKindSyncPublication ArtifactKind = "sync_publication"
	ArtifactKindApproval        ArtifactKind = "approval"
	ArtifactKindHandoff         ArtifactKind = "handoff"
	ArtifactKindEvidencePacket  ArtifactKind = "evidence_packet"
	ArtifactKindAuditReport     ArtifactKind = "audit_report"
	ArtifactKindAuditPacket     ArtifactKind = "audit_packet"
	ArtifactKindBackupSnapshot  ArtifactKind = "backup_snapshot"
	ArtifactKindGoalManifest    ArtifactKind = "goal_manifest"
)

var validArtifactKinds = map[ArtifactKind]struct{}{
	ArtifactKindBundle: {}, ArtifactKindSyncPublication: {}, ArtifactKindApproval: {}, ArtifactKindHandoff: {},
	ArtifactKindEvidencePacket: {}, ArtifactKindAuditReport: {}, ArtifactKindAuditPacket: {},
	ArtifactKindBackupSnapshot: {}, ArtifactKindGoalManifest: {},
}

func (k ArtifactKind) IsValid() bool {
	_, ok := validArtifactKinds[k]
	return ok
}

func TrustScopeForArtifactKind(kind ArtifactKind) (TrustScope, bool) {
	switch kind {
	case ArtifactKindBundle:
		return TrustScopeBundle, true
	case ArtifactKindSyncPublication:
		return TrustScopeSync, true
	case ArtifactKindApproval:
		return TrustScopeApproval, true
	case ArtifactKindHandoff:
		return TrustScopeHandoff, true
	case ArtifactKindEvidencePacket:
		return TrustScopeEvidence, true
	case ArtifactKindAuditReport:
		return TrustScopeAuditReport, true
	case ArtifactKindAuditPacket:
		return TrustScopeAuditPacket, true
	case ArtifactKindBackupSnapshot:
		return TrustScopeBackup, true
	case ArtifactKindGoalManifest:
		return TrustScopeGoal, true
	default:
		return "", false
	}
}

type CanonicalizationVersion string

const CanonicalizationAtlasV1 CanonicalizationVersion = "atlas-c14n-v1"

func (v CanonicalizationVersion) IsValid() bool {
	return v == CanonicalizationAtlasV1
}

type PayloadHashAlgorithm string

const PayloadHashSHA256 PayloadHashAlgorithm = "sha256"

func (a PayloadHashAlgorithm) IsValid() bool {
	return a == PayloadHashSHA256
}

type SignatureVerificationState string

const (
	VerificationTrustedValid                SignatureVerificationState = "trusted_valid"
	VerificationValidUntrusted              SignatureVerificationState = "valid_untrusted"
	VerificationValidUnknownKey             SignatureVerificationState = "valid_unknown_key"
	VerificationValidRevokedKey             SignatureVerificationState = "valid_revoked_key"
	VerificationValidExpiredKey             SignatureVerificationState = "valid_expired_key"
	VerificationValidRotatedKey             SignatureVerificationState = "valid_rotated_key"
	VerificationInvalidSignature            SignatureVerificationState = "invalid_signature"
	VerificationMissingSignature            SignatureVerificationState = "missing_signature"
	VerificationMalformedSignature          SignatureVerificationState = "malformed_signature"
	VerificationPayloadHashMismatch         SignatureVerificationState = "payload_hash_mismatch"
	VerificationCanonicalizationMismatch    SignatureVerificationState = "canonicalization_mismatch"
	VerificationUnsupportedSignatureVersion SignatureVerificationState = "unsupported_signature_version"
)

var validVerificationStates = map[SignatureVerificationState]struct{}{
	VerificationTrustedValid: {}, VerificationValidUntrusted: {}, VerificationValidUnknownKey: {},
	VerificationValidRevokedKey: {}, VerificationValidExpiredKey: {}, VerificationValidRotatedKey: {},
	VerificationInvalidSignature: {}, VerificationMissingSignature: {}, VerificationMalformedSignature: {},
	VerificationPayloadHashMismatch:      {},
	VerificationCanonicalizationMismatch: {}, VerificationUnsupportedSignatureVersion: {},
}

func (s SignatureVerificationState) IsValid() bool {
	_, ok := validVerificationStates[s]
	return ok
}

type SigningIdentity struct {
	IdentityID           string       `json:"identity_id" yaml:"identity_id"`
	Scope                KeyScope     `json:"scope" yaml:"scope"`
	DisplayName          string       `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Algorithm            KeyAlgorithm `json:"algorithm" yaml:"algorithm"`
	PublicKeyID          string       `json:"public_key_id" yaml:"public_key_id"`
	PublicKeyFingerprint string       `json:"public_key_fingerprint" yaml:"public_key_fingerprint"`
	PublicKeyMaterial    string       `json:"public_key_material,omitempty" yaml:"public_key_material,omitempty"`
	PrivateKeyRef        string       `json:"private_key_ref,omitempty" yaml:"private_key_ref,omitempty"`
	Status               KeyState     `json:"status" yaml:"status"`
	CreatedAt            time.Time    `json:"created_at" yaml:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at" yaml:"updated_at"`
	RotatedAt            time.Time    `json:"rotated_at,omitempty" yaml:"rotated_at,omitempty"`
	RevokedAt            time.Time    `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
	ExpiresAt            time.Time    `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	RevocationReason     string       `json:"revocation_reason,omitempty" yaml:"revocation_reason,omitempty"`
	SchemaVersion        int          `json:"schema_version" yaml:"schema_version"`
}

func (i SigningIdentity) Validate() error {
	if strings.TrimSpace(i.IdentityID) == "" {
		return fmt.Errorf("identity_id is required")
	}
	if !i.Scope.IsValid() {
		return fmt.Errorf("invalid key scope: %s", i.Scope)
	}
	if !i.Algorithm.IsValid() {
		return fmt.Errorf("invalid key algorithm: %s", i.Algorithm)
	}
	if strings.TrimSpace(i.PublicKeyID) == "" || strings.TrimSpace(i.PublicKeyFingerprint) == "" {
		return fmt.Errorf("public key id and fingerprint are required")
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() {
		return fmt.Errorf("created_at and updated_at are required")
	}
	if !i.Status.IsValid() {
		return fmt.Errorf("invalid key status: %s", i.Status)
	}
	return nil
}

type PublicKeyRecord struct {
	PublicKeyID       string             `json:"public_key_id" yaml:"public_key_id"`
	Fingerprint       string             `json:"fingerprint" yaml:"fingerprint"`
	Algorithm         KeyAlgorithm       `json:"algorithm" yaml:"algorithm"`
	PublicKeyMaterial string             `json:"public_key_material" yaml:"public_key_material"`
	OwnerKind         PublicKeyOwnerKind `json:"owner_kind" yaml:"owner_kind"`
	OwnerID           string             `json:"owner_id" yaml:"owner_id"`
	CreatedAt         time.Time          `json:"created_at" yaml:"created_at"`
	ExpiresAt         time.Time          `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Status            KeyState           `json:"status" yaml:"status"`
	Source            PublicKeySource    `json:"source" yaml:"source"`
	SchemaVersion     int                `json:"schema_version" yaml:"schema_version"`
}

func (r PublicKeyRecord) Validate() error {
	if strings.TrimSpace(r.PublicKeyID) == "" || strings.TrimSpace(r.Fingerprint) == "" {
		return fmt.Errorf("public key id and fingerprint are required")
	}
	if !r.Algorithm.IsValid() {
		return fmt.Errorf("invalid key algorithm: %s", r.Algorithm)
	}
	if strings.TrimSpace(r.PublicKeyMaterial) == "" {
		return fmt.Errorf("public_key_material is required")
	}
	if !r.OwnerKind.IsValid() || strings.TrimSpace(r.OwnerID) == "" {
		return fmt.Errorf("valid owner kind and owner id are required")
	}
	if r.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if !r.Status.IsValid() {
		return fmt.Errorf("invalid key status: %s", r.Status)
	}
	if !r.Source.IsValid() {
		return fmt.Errorf("invalid public key source: %s", r.Source)
	}
	return nil
}

type TrustBinding struct {
	TrustBindingID   string             `json:"trust_binding_id" yaml:"trust_binding_id"`
	PublicKeyID      string             `json:"public_key_id" yaml:"public_key_id"`
	Fingerprint      string             `json:"fingerprint" yaml:"fingerprint"`
	TrustedOwnerKind PublicKeyOwnerKind `json:"trusted_owner_kind" yaml:"trusted_owner_kind"`
	TrustedOwnerID   string             `json:"trusted_owner_id" yaml:"trusted_owner_id"`
	TrustedBy        Actor              `json:"trusted_by" yaml:"trusted_by"`
	TrustLevel       TrustLevel         `json:"trust_level" yaml:"trust_level"`
	AllowedScopes    []TrustScope       `json:"allowed_scopes,omitempty" yaml:"allowed_scopes,omitempty"`
	CreatedAt        time.Time          `json:"created_at" yaml:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" yaml:"updated_at"`
	RevokedAt        time.Time          `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
	Reason           string             `json:"reason" yaml:"reason"`
	LocalOnly        bool               `json:"local_only" yaml:"local_only"`
	SchemaVersion    int                `json:"schema_version" yaml:"schema_version"`
}

func (b TrustBinding) Validate() error {
	if strings.TrimSpace(b.TrustBindingID) == "" || strings.TrimSpace(b.PublicKeyID) == "" || strings.TrimSpace(b.Fingerprint) == "" {
		return fmt.Errorf("trust binding id, public key id, and fingerprint are required")
	}
	if !b.TrustedOwnerKind.IsValid() || strings.TrimSpace(b.TrustedOwnerID) == "" {
		return fmt.Errorf("valid trusted owner kind and id are required")
	}
	if !b.TrustedBy.IsValid() {
		return fmt.Errorf("invalid trusted_by actor: %s", b.TrustedBy)
	}
	if !b.TrustLevel.IsValid() {
		return fmt.Errorf("invalid trust level: %s", b.TrustLevel)
	}
	if b.TrustLevel == TrustLevelRevoked && b.RevokedAt.IsZero() {
		return fmt.Errorf("revoked trust bindings require revoked_at")
	}
	if b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
		return fmt.Errorf("created_at and updated_at are required")
	}
	if (b.TrustLevel == TrustLevelTrusted || b.TrustLevel == TrustLevelRestricted) && len(b.AllowedScopes) == 0 {
		return fmt.Errorf("trusted bindings require allowed_scopes")
	}
	for _, scope := range b.AllowedScopes {
		if !scope.IsValid() {
			return fmt.Errorf("invalid trust scope: %s", scope)
		}
	}
	if strings.TrimSpace(b.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

type RevocationRecord struct {
	RevocationID  string    `json:"revocation_id" yaml:"revocation_id"`
	PublicKeyID   string    `json:"public_key_id" yaml:"public_key_id"`
	Fingerprint   string    `json:"fingerprint" yaml:"fingerprint"`
	RevokedBy     Actor     `json:"revoked_by" yaml:"revoked_by"`
	RevokedAt     time.Time `json:"revoked_at" yaml:"revoked_at"`
	Reason        string    `json:"reason" yaml:"reason"`
	SchemaVersion int       `json:"schema_version" yaml:"schema_version"`
}

func (r RevocationRecord) Validate() error {
	if strings.TrimSpace(r.RevocationID) == "" || strings.TrimSpace(r.PublicKeyID) == "" || strings.TrimSpace(r.Fingerprint) == "" {
		return fmt.Errorf("revocation id, public key id, and fingerprint are required")
	}
	if !r.RevokedBy.IsValid() {
		return fmt.Errorf("invalid revoked_by actor: %s", r.RevokedBy)
	}
	if r.RevokedAt.IsZero() {
		return fmt.Errorf("revoked_at is required")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

type SignatureEnvelope struct {
	SignatureID             string                     `json:"signature_id" yaml:"signature_id"`
	ArtifactKind            ArtifactKind               `json:"artifact_kind" yaml:"artifact_kind"`
	ArtifactUID             string                     `json:"artifact_uid" yaml:"artifact_uid"`
	CanonicalizationVersion CanonicalizationVersion    `json:"canonicalization_version" yaml:"canonicalization_version"`
	PayloadHashAlgorithm    PayloadHashAlgorithm       `json:"payload_hash_algorithm" yaml:"payload_hash_algorithm"`
	PayloadHash             string                     `json:"payload_hash" yaml:"payload_hash"`
	SignedAt                time.Time                  `json:"signed_at" yaml:"signed_at"`
	SignerKind              PublicKeyOwnerKind         `json:"signer_kind" yaml:"signer_kind"`
	SignerID                string                     `json:"signer_id" yaml:"signer_id"`
	PublicKeyID             string                     `json:"public_key_id" yaml:"public_key_id"`
	PublicKeyFingerprint    string                     `json:"public_key_fingerprint" yaml:"public_key_fingerprint"`
	Algorithm               KeyAlgorithm               `json:"algorithm" yaml:"algorithm"`
	Signature               string                     `json:"signature" yaml:"signature"`
	VerificationState       SignatureVerificationState `json:"verification_state,omitempty" yaml:"verification_state,omitempty" atlasc14n:"-"`
	SchemaVersion           int                        `json:"schema_version" yaml:"schema_version"`
}

func (e SignatureEnvelope) Validate() error {
	if strings.TrimSpace(e.SignatureID) == "" || strings.TrimSpace(e.ArtifactUID) == "" {
		return fmt.Errorf("signature_id and artifact_uid are required")
	}
	if !e.ArtifactKind.IsValid() {
		return fmt.Errorf("invalid artifact kind: %s", e.ArtifactKind)
	}
	if !e.CanonicalizationVersion.IsValid() {
		return fmt.Errorf("invalid canonicalization version: %s", e.CanonicalizationVersion)
	}
	if !e.PayloadHashAlgorithm.IsValid() || strings.TrimSpace(e.PayloadHash) == "" {
		return fmt.Errorf("payload hash algorithm and payload hash are required")
	}
	if e.SignedAt.IsZero() {
		return fmt.Errorf("signed_at is required")
	}
	if !e.SignerKind.IsValid() || strings.TrimSpace(e.SignerID) == "" {
		return fmt.Errorf("valid signer kind and signer id are required")
	}
	if strings.TrimSpace(e.PublicKeyID) == "" || strings.TrimSpace(e.PublicKeyFingerprint) == "" {
		return fmt.Errorf("public key id and fingerprint are required")
	}
	if !e.Algorithm.IsValid() {
		return fmt.Errorf("invalid key algorithm: %s", e.Algorithm)
	}
	if strings.TrimSpace(e.Signature) == "" {
		return fmt.Errorf("signature is required")
	}
	if e.VerificationState != "" && !e.VerificationState.IsValid() {
		return fmt.Errorf("invalid verification state: %s", e.VerificationState)
	}
	return nil
}

type SignatureVerificationResult struct {
	ArtifactKind     ArtifactKind               `json:"artifact_kind"`
	ArtifactUID      string                     `json:"artifact_uid"`
	State            SignatureVerificationState `json:"state"`
	Signature        *SignatureEnvelope         `json:"signature,omitempty"`
	ReasonCodes      []string                   `json:"reason_codes,omitempty"`
	TrustedOwnerKind PublicKeyOwnerKind         `json:"trusted_owner_kind,omitempty"`
	TrustedOwnerID   string                     `json:"trusted_owner_id,omitempty"`
	VerifiedAt       time.Time                  `json:"verified_at"`
	SchemaVersion    int                        `json:"schema_version"`
}

func (r SignatureVerificationResult) Validate() error {
	if !r.ArtifactKind.IsValid() {
		return fmt.Errorf("invalid artifact kind: %s", r.ArtifactKind)
	}
	if strings.TrimSpace(r.ArtifactUID) == "" {
		return fmt.Errorf("artifact_uid is required")
	}
	if !r.State.IsValid() {
		return fmt.Errorf("invalid verification state: %s", r.State)
	}
	if r.Signature != nil {
		if err := validateSignatureEnvelopeForArtifact(*r.Signature, r.ArtifactKind, r.ArtifactUID); err != nil {
			return err
		}
	}
	hasTrustedOwner := r.TrustedOwnerKind != "" || strings.TrimSpace(r.TrustedOwnerID) != ""
	if r.State == VerificationTrustedValid || hasTrustedOwner {
		if !r.TrustedOwnerKind.IsValid() {
			return fmt.Errorf("invalid trusted_owner_kind: %s", r.TrustedOwnerKind)
		}
		if strings.TrimSpace(r.TrustedOwnerID) == "" {
			return fmt.Errorf("trusted_owner_id is required when trusted owner metadata is present")
		}
	}
	if r.VerifiedAt.IsZero() {
		return fmt.Errorf("verified_at is required")
	}
	return nil
}

func validateSignatureEnvelopeForArtifact(sig SignatureEnvelope, kind ArtifactKind, uid string) error {
	if err := sig.Validate(); err != nil {
		return err
	}
	if sig.ArtifactKind != kind || sig.ArtifactUID != uid {
		return fmt.Errorf("signature envelope does not match %s %s", kind, uid)
	}
	return nil
}
