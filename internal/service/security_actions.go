package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type KeyGenerateOptions struct {
	Scope   contracts.KeyScope `json:"scope"`
	OwnerID string             `json:"owner_id,omitempty"`
}

type KeyDetailView struct {
	Kind             string                    `json:"kind"`
	GeneratedAt      time.Time                 `json:"generated_at"`
	PublicKey        contracts.PublicKeyRecord `json:"public_key"`
	PrivateKeyHealth privateKeyHealth          `json:"private_key_health,omitempty"`
	CanSign          bool                      `json:"can_sign"`
	ReasonCodes      []string                  `json:"reason_codes,omitempty"`
}

type KeyListView struct {
	Kind        string          `json:"kind"`
	GeneratedAt time.Time       `json:"generated_at"`
	Items       []KeyDetailView `json:"items"`
}

type TrustStatusView struct {
	Kind              string    `json:"kind"`
	GeneratedAt       time.Time `json:"generated_at"`
	PublicKeys        int       `json:"public_keys"`
	LocalPrivateKeys  int       `json:"local_private_keys"`
	TrustedBindings   int       `json:"trusted_bindings"`
	RevokedBindings   int       `json:"revoked_bindings"`
	ImportedUntrusted int       `json:"imported_untrusted"`
}

type TrustListView struct {
	Kind        string                   `json:"kind"`
	GeneratedAt time.Time                `json:"generated_at"`
	Items       []contracts.TrustBinding `json:"items"`
}

type TrustExplanationView struct {
	Kind          string                     `json:"kind"`
	GeneratedAt   time.Time                  `json:"generated_at"`
	Target        string                     `json:"target"`
	PublicKey     *contracts.PublicKeyRecord `json:"public_key,omitempty"`
	Bindings      []contracts.TrustBinding   `json:"bindings,omitempty"`
	TrustedScopes []contracts.TrustScope     `json:"trusted_scopes,omitempty"`
	ReasonCodes   []string                   `json:"reason_codes,omitempty"`
}

type SignatureRequest struct {
	ArtifactKind contracts.ArtifactKind `json:"artifact_kind"`
	ArtifactUID  string                 `json:"artifact_uid"`
	PublicKeyID  string                 `json:"public_key_id,omitempty"`
	Payload      any                    `json:"-"`
}

type signaturePreimage struct {
	ArtifactKind contracts.ArtifactKind `json:"artifact_kind"`
	ArtifactUID  string                 `json:"artifact_uid"`
	Payload      any                    `json:"payload"`
}

func (s *ActionService) GenerateKey(ctx context.Context, opts KeyGenerateOptions, actor contracts.Actor, reason string) (KeyDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "generate signing key", func(ctx context.Context) (KeyDetailView, error) {
		if !actor.IsValid() {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		scope := opts.Scope
		if scope == "" {
			scope = contracts.KeyScopeWorkspace
		}
		if !scope.IsValid() {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid key scope: %s", scope))
		}
		now := s.now()
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return KeyDetailView{}, fmt.Errorf("generate ed25519 key: %w", err)
		}
		fingerprint := fingerprintPublicKey(public)
		publicKeyID := "key-" + fingerprintShort(fingerprint)
		ownerKind, ownerID, err := s.keyOwner(ctx, scope, opts.OwnerID, actor)
		if err != nil {
			return KeyDetailView{}, err
		}
		record := contracts.PublicKeyRecord{
			PublicKeyID:       publicKeyID,
			Fingerprint:       fingerprint,
			Algorithm:         contracts.KeyAlgorithmEd25519,
			PublicKeyMaterial: base64.StdEncoding.EncodeToString(public),
			OwnerKind:         ownerKind,
			OwnerID:           ownerID,
			CreatedAt:         now,
			Status:            contracts.KeyStateActive,
			Source:            contracts.PublicKeySourceLocal,
			SchemaVersion:     contracts.CurrentSchemaVersion,
		}
		privateRecord := privateKeyFile{
			PublicKeyID:        publicKeyID,
			Fingerprint:        fingerprint,
			Algorithm:          contracts.KeyAlgorithmEd25519,
			PrivateKeyMaterial: base64.StdEncoding.EncodeToString(private),
			CreatedAt:          now.Format(time.RFC3339Nano),
			SchemaVersion:      contracts.CurrentSchemaVersion,
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, now, actor, reason, contracts.EventKeyGenerated, "", record)
		if err != nil {
			return KeyDetailView{}, err
		}
		if err := s.commitMutation(ctx, "generate signing key", "public_key_record", event, func(ctx context.Context) error {
			if err := s.SecurityKeys.SavePublicKey(ctx, record); err != nil {
				return err
			}
			return s.SecurityKeys.SavePrivateKey(ctx, privateRecord)
		}); err != nil {
			return KeyDetailView{}, err
		}
		return s.KeyDetail(ctx, publicKeyID)
	})
}

func (s *ActionService) ImportPublicKey(ctx context.Context, path string, actor contracts.Actor, reason string) (KeyDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "import public key", func(ctx context.Context) (KeyDetailView, error) {
		if !actor.IsValid() {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		record, err := s.SecurityKeys.ImportPublicKeyFile(path)
		if err != nil {
			return KeyDetailView{}, err
		}
		if existing, err := s.SecurityKeys.LoadPublicKey(ctx, record.PublicKeyID); err == nil && existing.Source == contracts.PublicKeySourceLocal {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, "refusing to overwrite local signing key with imported public key")
		}
		now := s.now()
		event, err := s.newEvent(ctx, workspaceProjectKey, now, actor, reason, contracts.EventKeyImported, "", record)
		if err != nil {
			return KeyDetailView{}, err
		}
		if err := s.commitMutation(ctx, "import public key", "public_key_record", event, func(ctx context.Context) error {
			return s.SecurityKeys.SavePublicKey(ctx, record)
		}); err != nil {
			return KeyDetailView{}, err
		}
		return s.KeyDetail(ctx, record.PublicKeyID)
	})
}

func (s *ActionService) RotateKey(ctx context.Context, publicKeyID string, actor contracts.Actor, reason string) (KeyDetailView, error) {
	return s.transitionPublicKey(ctx, publicKeyID, contracts.KeyStateRotated, contracts.EventKeyRotated, actor, reason)
}

func (s *ActionService) RevokeKey(ctx context.Context, publicKeyID string, actor contracts.Actor, reason string) (KeyDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "revoke signing key", func(ctx context.Context) (KeyDetailView, error) {
		if !actor.IsValid() {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		record, err := s.SecurityKeys.LoadPublicKey(ctx, publicKeyID)
		if err != nil {
			return KeyDetailView{}, apperr.Wrap(apperr.CodeNotFound, err, "public key not found")
		}
		if !contracts.CanTransitionKeyState(record.Status, contracts.KeyStateRevoked) {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("cannot revoke key in state %s", record.Status))
		}
		now := s.now()
		record.Status = contracts.KeyStateRevoked
		revocation := contracts.RevocationRecord{
			RevocationID:  "revocation-" + fingerprintShort(record.Fingerprint),
			PublicKeyID:   record.PublicKeyID,
			Fingerprint:   record.Fingerprint,
			RevokedBy:     actor,
			RevokedAt:     now,
			Reason:        strings.TrimSpace(reason),
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, now, actor, reason, contracts.EventKeyRevoked, "", revocation)
		if err != nil {
			return KeyDetailView{}, err
		}
		if err := s.commitMutation(ctx, "revoke signing key", "public_key_record", event, func(ctx context.Context) error {
			if err := s.SecurityKeys.SavePublicKey(ctx, record); err != nil {
				return err
			}
			return s.SecurityKeys.SaveRevocation(ctx, revocation)
		}); err != nil {
			return KeyDetailView{}, err
		}
		return s.KeyDetail(ctx, publicKeyID)
	})
}

func (s *ActionService) transitionPublicKey(ctx context.Context, publicKeyID string, state contracts.KeyState, eventType contracts.EventType, actor contracts.Actor, reason string) (KeyDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "transition signing key", func(ctx context.Context) (KeyDetailView, error) {
		if !actor.IsValid() {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		record, err := s.SecurityKeys.LoadPublicKey(ctx, publicKeyID)
		if err != nil {
			return KeyDetailView{}, apperr.Wrap(apperr.CodeNotFound, err, "public key not found")
		}
		if !contracts.CanTransitionKeyState(record.Status, state) {
			return KeyDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("cannot transition key from %s to %s", record.Status, state))
		}
		record.Status = state
		now := s.now()
		event, err := s.newEvent(ctx, workspaceProjectKey, now, actor, reason, eventType, "", record)
		if err != nil {
			return KeyDetailView{}, err
		}
		if err := s.commitMutation(ctx, "transition signing key", "public_key_record", event, func(ctx context.Context) error {
			return s.SecurityKeys.SavePublicKey(ctx, record)
		}); err != nil {
			return KeyDetailView{}, err
		}
		return s.KeyDetail(ctx, publicKeyID)
	})
}

func (s *ActionService) KeyDetail(ctx context.Context, publicKeyID string) (KeyDetailView, error) {
	record, err := s.SecurityKeys.LoadPublicKey(ctx, publicKeyID)
	if err != nil {
		return KeyDetailView{}, apperr.Wrap(apperr.CodeNotFound, err, "public key not found")
	}
	health := s.SecurityKeys.PrivateKeyHealth(record.PublicKeyID)
	canSign := false
	reasons := make([]string, 0)
	if record.Status != contracts.KeyStateActive {
		reasons = append(reasons, "key_not_active")
		reasons = append(reasons, health.Warnings...)
	} else if _, reason, err := s.loadSigningPrivateKey(ctx, record); err != nil {
		if reason != "" {
			reasons = append(reasons, reason)
		}
		reasons = append(reasons, health.Warnings...)
	} else {
		canSign = true
	}
	return KeyDetailView{Kind: "key_detail", GeneratedAt: s.now(), PublicKey: record, PrivateKeyHealth: health, CanSign: canSign, ReasonCodes: reasons}, nil
}

func (s *ActionService) ListKeys(ctx context.Context) (KeyListView, error) {
	keys, err := s.SecurityKeys.ListPublicKeys(ctx)
	if err != nil {
		return KeyListView{}, err
	}
	items := make([]KeyDetailView, 0, len(keys))
	for _, key := range keys {
		detail, err := s.KeyDetail(ctx, key.PublicKeyID)
		if err != nil {
			return KeyListView{}, err
		}
		items = append(items, detail)
	}
	return KeyListView{Kind: "key_list", GeneratedAt: s.now(), Items: items}, nil
}

func (s *ActionService) BindTrust(ctx context.Context, collaboratorID string, publicKeyID string, actor contracts.Actor, reason string) (contracts.TrustBinding, error) {
	return withWriteLock(ctx, s.LockManager, "bind trust", func(ctx context.Context) (contracts.TrustBinding, error) {
		if !actor.IsValid() {
			return contracts.TrustBinding{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return contracts.TrustBinding{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		record, err := s.SecurityKeys.LoadPublicKey(ctx, publicKeyID)
		if err != nil {
			return contracts.TrustBinding{}, apperr.Wrap(apperr.CodeNotFound, err, "public key not found")
		}
		collaboratorID = strings.TrimSpace(collaboratorID)
		if record.OwnerKind != contracts.PublicKeyOwnerCollaborator || record.OwnerID != collaboratorID {
			return contracts.TrustBinding{}, apperr.New(apperr.CodeInvalidInput, "public key owner does not match collaborator")
		}
		now := s.now()
		binding := contracts.TrustBinding{
			TrustBindingID:   "trust-" + sanitizeSecurityID(collaboratorID) + "-" + fingerprintShort(record.Fingerprint),
			PublicKeyID:      record.PublicKeyID,
			Fingerprint:      record.Fingerprint,
			TrustedOwnerKind: contracts.PublicKeyOwnerCollaborator,
			TrustedOwnerID:   strings.TrimSpace(collaboratorID),
			TrustedBy:        actor,
			TrustLevel:       contracts.TrustLevelTrusted,
			AllowedScopes:    defaultTrustScopes(),
			CreatedAt:        now,
			UpdatedAt:        now,
			Reason:           strings.TrimSpace(reason),
			LocalOnly:        true,
			SchemaVersion:    contracts.CurrentSchemaVersion,
		}
		if err := s.TrustBindings.SaveTrustBinding(ctx, binding); err != nil {
			return contracts.TrustBinding{}, err
		}
		return binding, nil
	})
}

func (s *ActionService) RevokeTrustForKey(ctx context.Context, publicKeyID string, actor contracts.Actor, reason string) (TrustListView, error) {
	return withWriteLock(ctx, s.LockManager, "revoke trust", func(ctx context.Context) (TrustListView, error) {
		if !actor.IsValid() {
			return TrustListView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return TrustListView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		bindings, err := s.TrustBindings.ListTrustBindingsForKey(ctx, publicKeyID)
		if err != nil {
			return TrustListView{}, err
		}
		if len(bindings) == 0 {
			return TrustListView{}, apperr.New(apperr.CodeNotFound, "no trust bindings for public key")
		}
		now := s.now()
		updated := make([]contracts.TrustBinding, 0, len(bindings))
		for _, binding := range bindings {
			binding.TrustLevel = contracts.TrustLevelRevoked
			binding.RevokedAt = now
			binding.UpdatedAt = now
			binding.Reason = strings.TrimSpace(reason)
			updated = append(updated, binding)
		}
		for _, binding := range updated {
			if err := s.TrustBindings.SaveTrustBinding(ctx, binding); err != nil {
				return TrustListView{}, err
			}
		}
		return TrustListView{Kind: "trust_list", GeneratedAt: s.now(), Items: updated}, nil
	})
}

func (s *ActionService) TrustStatus(ctx context.Context) (TrustStatusView, error) {
	keys, err := s.SecurityKeys.ListPublicKeys(ctx)
	if err != nil {
		return TrustStatusView{}, err
	}
	bindings, err := s.TrustBindings.ListTrustBindings(ctx)
	if err != nil {
		return TrustStatusView{}, err
	}
	view := TrustStatusView{Kind: "trust_status", GeneratedAt: s.now(), PublicKeys: len(keys)}
	trustedKeys := map[string]struct{}{}
	for _, binding := range bindings {
		if binding.TrustLevel == contracts.TrustLevelRevoked {
			view.RevokedBindings++
		}
		if binding.TrustLevel == contracts.TrustLevelTrusted || binding.TrustLevel == contracts.TrustLevelRestricted {
			view.TrustedBindings++
			trustedKeys[binding.PublicKeyID] = struct{}{}
		}
	}
	for _, key := range keys {
		health := s.SecurityKeys.PrivateKeyHealth(key.PublicKeyID)
		if health.Present {
			view.LocalPrivateKeys++
		}
		_, trusted := trustedKeys[key.PublicKeyID]
		if !trusted && (key.Source == contracts.PublicKeySourceManualImport || key.Status == contracts.KeyStateImported) {
			view.ImportedUntrusted++
		}
	}
	return view, nil
}

func (s *ActionService) ListTrust(ctx context.Context, collaboratorID string) (TrustListView, error) {
	items, err := s.TrustBindings.ListTrustBindings(ctx)
	if err != nil {
		return TrustListView{}, err
	}
	filtered := make([]contracts.TrustBinding, 0, len(items))
	for _, item := range items {
		if collaboratorID == "" || item.TrustedOwnerID == collaboratorID {
			filtered = append(filtered, item)
		}
	}
	return TrustListView{Kind: "trust_list", GeneratedAt: s.now(), Items: filtered}, nil
}

func (s *ActionService) ExplainTrust(ctx context.Context, target string) (TrustExplanationView, error) {
	target = strings.TrimSpace(target)
	view := TrustExplanationView{Kind: "trust_explanation", GeneratedAt: s.now(), Target: target}
	bindings, err := s.TrustBindings.ListTrustBindings(ctx)
	if err != nil {
		return view, err
	}
	for _, binding := range bindings {
		if binding.PublicKeyID == target || binding.TrustedOwnerID == target {
			view.Bindings = append(view.Bindings, binding)
			if binding.TrustLevel == contracts.TrustLevelTrusted || binding.TrustLevel == contracts.TrustLevelRestricted {
				view.TrustedScopes = append(view.TrustedScopes, binding.AllowedScopes...)
			}
		}
	}
	if key, err := s.SecurityKeys.LoadPublicKey(ctx, target); err == nil {
		view.PublicKey = &key
	}
	if view.PublicKey == nil && len(view.Bindings) == 0 {
		view.ReasonCodes = append(view.ReasonCodes, "no_trust_material")
	}
	return view, nil
}

func (s *ActionService) SignPayload(ctx context.Context, req SignatureRequest) (contracts.SignatureEnvelope, error) {
	if !req.ArtifactKind.IsValid() {
		return contracts.SignatureEnvelope{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid artifact kind: %s", req.ArtifactKind))
	}
	if strings.TrimSpace(req.ArtifactUID) == "" {
		return contracts.SignatureEnvelope{}, apperr.New(apperr.CodeInvalidInput, "artifact_uid is required")
	}
	publicKeyID := strings.TrimSpace(req.PublicKeyID)
	if publicKeyID == "" {
		keys, err := s.SecurityKeys.ListPublicKeys(ctx)
		if err != nil {
			return contracts.SignatureEnvelope{}, err
		}
		for _, key := range keys {
			if key.Source == contracts.PublicKeySourceLocal && key.Status == contracts.KeyStateActive {
				if _, _, err := s.loadSigningPrivateKey(ctx, key); err == nil {
					publicKeyID = key.PublicKeyID
					break
				}
			}
		}
	}
	if publicKeyID == "" {
		return contracts.SignatureEnvelope{}, apperr.New(apperr.CodeNotFound, "no active local signing key found")
	}
	key, err := s.SecurityKeys.LoadPublicKey(ctx, publicKeyID)
	if err != nil {
		return contracts.SignatureEnvelope{}, err
	}
	now := s.now()
	if key.Status != contracts.KeyStateActive {
		return contracts.SignatureEnvelope{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("key %s cannot sign in state %s", key.PublicKeyID, key.Status))
	}
	if keyExpired(key, now) {
		return contracts.SignatureEnvelope{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("key %s expired at %s", key.PublicKeyID, key.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	}
	privateKey, _, err := s.loadSigningPrivateKey(ctx, key)
	if err != nil {
		return contracts.SignatureEnvelope{}, err
	}
	artifactUID := strings.TrimSpace(req.ArtifactUID)
	payloadCanonical, err := contracts.CanonicalizeAtlasV1(req.Payload)
	if err != nil {
		return contracts.SignatureEnvelope{}, err
	}
	hash := sha256.Sum256(payloadCanonical)
	signingBytes, err := canonicalSignaturePreimage(req.ArtifactKind, artifactUID, req.Payload)
	if err != nil {
		return contracts.SignatureEnvelope{}, err
	}
	sig := ed25519.Sign(privateKey, signingBytes)
	envelope := contracts.SignatureEnvelope{
		SignatureID:             "sig-" + artifactUID + "-" + fingerprintShort(hex.EncodeToString(hash[:])) + "-" + fingerprintShort(key.PublicKeyID),
		ArtifactKind:            req.ArtifactKind,
		ArtifactUID:             artifactUID,
		CanonicalizationVersion: contracts.CanonicalizationAtlasV1,
		PayloadHashAlgorithm:    contracts.PayloadHashSHA256,
		PayloadHash:             hex.EncodeToString(hash[:]),
		SignedAt:                now,
		SignerKind:              key.OwnerKind,
		SignerID:                key.OwnerID,
		PublicKeyID:             key.PublicKeyID,
		PublicKeyFingerprint:    key.Fingerprint,
		Algorithm:               contracts.KeyAlgorithmEd25519,
		Signature:               base64.StdEncoding.EncodeToString(sig),
		SchemaVersion:           contracts.CurrentSchemaVersion,
	}
	return envelope, envelope.Validate()
}

func (s *ActionService) loadSigningPrivateKey(ctx context.Context, key contracts.PublicKeyRecord) (ed25519.PrivateKey, string, error) {
	health := s.SecurityKeys.PrivateKeyHealth(key.PublicKeyID)
	if !health.Present {
		return nil, "private_key_missing", apperr.New(apperr.CodeInvalidInput, "private key is missing or has unsafe permissions")
	}
	if !health.PermissionsOK {
		reason := "private_key_permissions_too_broad"
		return nil, reason, apperr.New(apperr.CodeInvalidInput, "private key is missing or has unsafe permissions")
	}
	privateFile, err := s.SecurityKeys.LoadPrivateKey(ctx, key.PublicKeyID)
	if err != nil {
		return nil, "private_key_decode_failed", err
	}
	if privateFile.PublicKeyID != key.PublicKeyID || privateFile.Fingerprint != key.Fingerprint {
		return nil, "private_key_metadata_mismatch", apperr.New(apperr.CodeInvalidInput, "private key metadata does not match public key record")
	}
	privateBytes, err := base64.StdEncoding.DecodeString(privateFile.PrivateKeyMaterial)
	if err != nil {
		return nil, "private_key_decode_failed", fmt.Errorf("decode private key: %w", err)
	}
	if len(privateBytes) != ed25519.PrivateKeySize {
		return nil, "private_key_invalid_length", fmt.Errorf("private key has invalid length")
	}
	publicBytes, err := base64.StdEncoding.DecodeString(key.PublicKeyMaterial)
	if err != nil || len(publicBytes) != ed25519.PublicKeySize {
		return nil, "public_key_malformed", apperr.New(apperr.CodeInvalidInput, "public key material is malformed")
	}
	if !bytes.Equal(ed25519.PrivateKey(privateBytes).Public().(ed25519.PublicKey), publicBytes) {
		return nil, "private_key_material_mismatch", apperr.New(apperr.CodeInvalidInput, "private key does not match public key record")
	}
	return ed25519.PrivateKey(privateBytes), "", nil
}

func (s *ActionService) VerifyPayloadSignature(ctx context.Context, payload any, envelope *contracts.SignatureEnvelope) (contracts.SignatureVerificationResult, error) {
	now := s.now()
	if envelope == nil {
		result := contracts.SignatureVerificationResult{State: contracts.VerificationMissingSignature, VerifiedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
		return result, nil
	}
	result := contracts.SignatureVerificationResult{
		ArtifactKind:  envelope.ArtifactKind,
		ArtifactUID:   envelope.ArtifactUID,
		State:         contracts.VerificationMalformedSignature,
		Signature:     envelope,
		VerifiedAt:    now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := envelope.Validate(); err != nil {
		result.ReasonCodes = []string{"malformed_signature"}
		return result, nil
	}
	if envelope.CanonicalizationVersion != contracts.CanonicalizationAtlasV1 {
		result.State = contracts.VerificationUnsupportedSignatureVersion
		return result, nil
	}
	payloadCanonical, err := contracts.CanonicalizeAtlasV1(payload)
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(payloadCanonical)
	if envelope.PayloadHash != hex.EncodeToString(hash[:]) {
		result.State = contracts.VerificationPayloadHashMismatch
		return result, nil
	}
	rawSig, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(rawSig) != ed25519.SignatureSize {
		result.State = contracts.VerificationMalformedSignature
		result.ReasonCodes = []string{"signature_malformed"}
		return result, nil
	}
	key, err := s.SecurityKeys.LoadPublicKey(ctx, envelope.PublicKeyID)
	if err != nil {
		result.State = contracts.VerificationValidUnknownKey
		result.ReasonCodes = []string{"public_key_unknown"}
		return result, nil
	}
	if envelope.PublicKeyFingerprint != key.Fingerprint || envelope.Algorithm != key.Algorithm || envelope.SignerKind != key.OwnerKind || envelope.SignerID != key.OwnerID {
		result.State = contracts.VerificationMalformedSignature
		result.ReasonCodes = []string{"signature_key_metadata_mismatch"}
		return result, nil
	}
	publicBytes, err := base64.StdEncoding.DecodeString(key.PublicKeyMaterial)
	if err != nil || len(publicBytes) != ed25519.PublicKeySize {
		result.State = contracts.VerificationMalformedSignature
		result.ReasonCodes = []string{"public_key_malformed"}
		return result, nil
	}
	signingBytes, err := canonicalSignaturePreimage(envelope.ArtifactKind, envelope.ArtifactUID, payload)
	if err != nil {
		return result, err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicBytes), signingBytes, rawSig) {
		result.State = contracts.VerificationInvalidSignature
		return result, nil
	}
	revoked, err := s.publicKeyRevoked(ctx, key)
	if err != nil {
		return result, err
	}
	if revoked {
		result.State = contracts.VerificationValidRevokedKey
		return result, nil
	}
	switch {
	case key.Status == contracts.KeyStateRevoked:
		result.State = contracts.VerificationValidRevokedKey
		return result, nil
	case key.Status == contracts.KeyStateExpired || keyExpired(key, now):
		result.State = contracts.VerificationValidExpiredKey
		return result, nil
	case key.Status == contracts.KeyStateRotated:
		result.State = contracts.VerificationValidRotatedKey
		return result, nil
	}
	if trusted, ownerKind, ownerID, err := s.signatureTrusted(ctx, key, envelope.ArtifactKind); err != nil {
		return result, err
	} else if trusted {
		result.State = contracts.VerificationTrustedValid
		result.TrustedOwnerKind = ownerKind
		result.TrustedOwnerID = ownerID
		return result, nil
	}
	result.State = contracts.VerificationValidUntrusted
	return result, nil
}

func canonicalSignaturePreimage(kind contracts.ArtifactKind, artifactUID string, payload any) ([]byte, error) {
	return contracts.CanonicalizeAtlasV1(signaturePreimage{
		ArtifactKind: kind,
		ArtifactUID:  strings.TrimSpace(artifactUID),
		Payload:      payload,
	})
}

func keyExpired(key contracts.PublicKeyRecord, now time.Time) bool {
	return !key.ExpiresAt.IsZero() && !now.UTC().Before(key.ExpiresAt.UTC())
}

func (s *ActionService) publicKeyRevoked(ctx context.Context, key contracts.PublicKeyRecord) (bool, error) {
	revocations, err := s.SecurityKeys.ListRevocations(ctx)
	if err != nil {
		return false, err
	}
	for _, revocation := range revocations {
		if revocation.PublicKeyID == key.PublicKeyID || revocation.Fingerprint == key.Fingerprint {
			return true, nil
		}
	}
	return false, nil
}

func (s *ActionService) signatureTrusted(ctx context.Context, key contracts.PublicKeyRecord, kind contracts.ArtifactKind) (bool, contracts.PublicKeyOwnerKind, string, error) {
	scope, ok := contracts.TrustScopeForArtifactKind(kind)
	if !ok {
		return false, "", "", nil
	}
	bindings, err := s.TrustBindings.ListTrustBindingsForKey(ctx, key.PublicKeyID)
	if err != nil {
		return false, "", "", err
	}
	for _, binding := range bindings {
		if binding.TrustLevel != contracts.TrustLevelTrusted && binding.TrustLevel != contracts.TrustLevelRestricted {
			continue
		}
		for _, allowed := range binding.AllowedScopes {
			if allowed == scope {
				return true, binding.TrustedOwnerKind, binding.TrustedOwnerID, nil
			}
		}
	}
	return false, "", "", nil
}

func (s *ActionService) keyOwner(ctx context.Context, scope contracts.KeyScope, ownerID string, actor contracts.Actor) (contracts.PublicKeyOwnerKind, string, error) {
	ownerID = strings.TrimSpace(ownerID)
	switch scope {
	case contracts.KeyScopeCollaborator:
		if ownerID == "" {
			ownerID = strings.TrimPrefix(string(actor), "human:")
		}
		return contracts.PublicKeyOwnerCollaborator, ownerID, nil
	case contracts.KeyScopeRelease:
		if ownerID == "" {
			ownerID = "release"
		}
		return contracts.PublicKeyOwnerRelease, ownerID, nil
	case contracts.KeyScopeAdmin:
		if ownerID == "" {
			ownerID = "admin"
		}
		return contracts.PublicKeyOwnerAdmin, ownerID, nil
	default:
		if ownerID == "" {
			workspaceID, err := ensureWorkspaceIdentity(s.Root)
			if err != nil {
				return "", "", err
			}
			ownerID = workspaceID
		}
		return contracts.PublicKeyOwnerWorkspace, ownerID, nil
	}
}

func defaultTrustScopes() []contracts.TrustScope {
	return []contracts.TrustScope{
		contracts.TrustScopeBundle,
		contracts.TrustScopeSync,
		contracts.TrustScopeApproval,
		contracts.TrustScopeHandoff,
		contracts.TrustScopeEvidence,
		contracts.TrustScopeAuditReport,
		contracts.TrustScopeAuditPacket,
		contracts.TrustScopeBackup,
		contracts.TrustScopeGoal,
	}
}

func fingerprintPublicKey(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return "ed25519:" + hex.EncodeToString(sum[:])
}

func fingerprintShort(fingerprint string) string {
	clean := strings.TrimPrefix(strings.TrimSpace(fingerprint), "ed25519:")
	clean = strings.ReplaceAll(clean, ":", "")
	if len(clean) > 16 {
		return clean[:16]
	}
	if clean == "" {
		return "unknown"
	}
	return clean
}
