package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type SignatureDetailView struct {
	Kind         string                      `json:"kind"`
	ArtifactKind contracts.ArtifactKind      `json:"artifact_kind"`
	ArtifactUID  string                      `json:"artifact_uid"`
	Signature    contracts.SignatureEnvelope `json:"signature"`
	GeneratedAt  time.Time                   `json:"generated_at"`
}

type ArtifactSignatureVerifyView struct {
	Kind        string                                `json:"kind"`
	Integrity   any                                   `json:"integrity,omitempty"`
	Signature   contracts.SignatureVerificationResult `json:"signature"`
	GeneratedAt time.Time                             `json:"generated_at"`
}

type signedBundlePayload struct {
	BundleID       string `json:"bundle_id"`
	Format         string `json:"format"`
	Scope          string `json:"scope,omitempty"`
	ArchiveSHA256  string `json:"archive_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
	FileCount      int    `json:"file_count"`
}

type signedSyncPublicationPayload struct {
	WorkspaceID        string    `json:"workspace_id"`
	BundleID           string    `json:"bundle_id"`
	Format             string    `json:"format"`
	CreatedAt          time.Time `json:"created_at"`
	ArtifactName       string    `json:"artifact_name"`
	ManifestName       string    `json:"manifest_name"`
	ChecksumName       string    `json:"checksum_name"`
	FileCount          int       `json:"file_count"`
	ArchiveSHA256      string    `json:"archive_sha256,omitempty"`
	ManifestSHA256     string    `json:"manifest_sha256,omitempty"`
	RedactionPreviewID string    `json:"redaction_preview_id,omitempty"`
}

type exportSignatureSidecar struct {
	FormatVersion      string                        `json:"format_version"`
	Kind               string                        `json:"kind"`
	BundleID           string                        `json:"bundle_id"`
	SignatureEnvelopes []contracts.SignatureEnvelope `json:"signature_envelopes,omitempty"`
}

func (s *ActionService) SignExportBundle(ctx context.Context, bundleRef string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sign export bundle", func(ctx context.Context) (SignatureDetailView, error) {
		if !actor.IsValid() {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		bundle, err := s.resolveExportBundle(ctx, bundleRef)
		if err != nil {
			return SignatureDetailView{}, err
		}
		payload, err := s.exportBundleSignaturePayload(bundle)
		if err != nil {
			return SignatureDetailView{}, err
		}
		envelope, err := s.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: bundle.BundleID, PublicKeyID: publicKeyID, Payload: payload})
		if err != nil {
			return SignatureDetailView{}, err
		}
		bundle.SignatureEnvelopes = upsertSignatureEnvelope(bundle.SignatureEnvelopes, envelope)
		pathRef := exportBundleRefIsPath(bundleRef)
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventSignatureCreated, "", envelope)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if err := s.commitMutation(ctx, "sign export bundle", "signature", event, func(ctx context.Context) error {
			if !pathRef {
				if err := s.ExportBundles.SaveExportBundle(ctx, bundle); err != nil {
					return err
				}
			}
			if err := writeExportSignatureSidecar(bundle); err != nil {
				return err
			}
			return s.Signatures.SaveSignature(ctx, envelope)
		}); err != nil {
			return SignatureDetailView{}, err
		}
		return SignatureDetailView{Kind: "signature_detail", ArtifactKind: envelope.ArtifactKind, ArtifactUID: envelope.ArtifactUID, Signature: envelope, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) VerifyExportBundleSignature(ctx context.Context, bundleRef string) (ArtifactSignatureVerifyView, error) {
	bundle, err := s.resolveExportBundle(ctx, bundleRef)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	payload, integrity, err := s.exportBundleSignaturePayloadWithIntegrity(bundle)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	result, err := s.VerifyPayloadSignatures(ctx, payload, bundle.SignatureEnvelopes, contracts.ArtifactKindBundle, bundle.BundleID)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	return ArtifactSignatureVerifyView{Kind: "signature_verify_result", Integrity: integrity, Signature: result, GeneratedAt: s.now()}, nil
}

func (s *ActionService) SignSyncPublication(ctx context.Context, bundleRef string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sign sync publication", func(ctx context.Context) (SignatureDetailView, error) {
		if !actor.IsValid() {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		artifactPath := resolveSyncBundlePath(s.Root, bundleRef)
		publication, err := inspectSyncBundle(artifactPath)
		if err != nil {
			return SignatureDetailView{}, err
		}
		integrity, err := verifySyncBundle(artifactPath)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if !integrity.Verified {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "sync bundle integrity must verify before signing")
		}
		payload, publication, err := currentSyncPublicationSignaturePayload(artifactPath, publication)
		if err != nil {
			return SignatureDetailView{}, err
		}
		envelope, err := s.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindSyncPublication, ArtifactUID: publication.BundleID, PublicKeyID: publicKeyID, Payload: payload})
		if err != nil {
			return SignatureDetailView{}, err
		}
		publication.SignatureEnvelopes = upsertSignatureEnvelope(publication.SignatureEnvelopes, envelope)
		publicationPath := syncPublicationPathForArtifact(artifactPath)
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventSignatureCreated, "", envelope)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if err := s.commitMutation(ctx, "sign sync publication", "signature", event, func(ctx context.Context) error {
			if err := writeSyncPublication(publicationPath, publication); err != nil {
				return err
			}
			return s.Signatures.SaveSignature(ctx, envelope)
		}); err != nil {
			return SignatureDetailView{}, err
		}
		return SignatureDetailView{Kind: "signature_detail", ArtifactKind: envelope.ArtifactKind, ArtifactUID: envelope.ArtifactUID, Signature: envelope, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) VerifySyncPublicationSignature(ctx context.Context, bundleRef string) (ArtifactSignatureVerifyView, error) {
	artifactPath := resolveSyncBundlePath(s.Root, bundleRef)
	publication, err := inspectSyncBundle(artifactPath)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	integrity, err := verifySyncBundle(artifactPath)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	payload, publication, err := currentSyncPublicationSignaturePayload(artifactPath, publication)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	result, err := s.VerifyPayloadSignatures(ctx, payload, publication.SignatureEnvelopes, contracts.ArtifactKindSyncPublication, publication.BundleID)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	return ArtifactSignatureVerifyView{Kind: "signature_verify_result", Integrity: integrity, Signature: result, GeneratedAt: s.now()}, nil
}

func (s *ActionService) exportBundleSignaturePayload(bundle contracts.ExportBundle) (signedBundlePayload, error) {
	payload, integrity, err := s.exportBundleSignaturePayloadWithIntegrity(bundle)
	if err != nil {
		return signedBundlePayload{}, err
	}
	if !integrity.Verified {
		return signedBundlePayload{}, apperr.New(apperr.CodeInvalidInput, "export bundle integrity must verify before signing")
	}
	return payload, err
}

func (s *ActionService) exportBundleSignaturePayloadWithIntegrity(bundle contracts.ExportBundle) (signedBundlePayload, ExportVerifyView, error) {
	integrity, err := verifyBundle(bundle)
	if err != nil {
		return signedBundlePayload{}, integrity, err
	}
	manifest, err := loadBundleManifest(bundle.ManifestPath)
	if err != nil {
		return signedBundlePayload{}, integrity, err
	}
	if strings.TrimSpace(manifest.BundleID) != "" && manifest.BundleID != bundle.BundleID {
		return signedBundlePayload{}, integrity, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("export bundle manifest %s does not match bundle %s", manifest.BundleID, bundle.BundleID))
	}
	return signedBundlePayload{
		BundleID:       bundle.BundleID,
		Format:         bundle.Format,
		Scope:          bundle.Scope,
		ArchiveSHA256:  integrity.ArchiveSHA256,
		ManifestSHA256: integrity.ManifestSHA256,
		FileCount:      len(manifest.Files),
	}, integrity, nil
}

func syncPublicationSignaturePayload(publication SyncPublication) signedSyncPublicationPayload {
	return signedSyncPublicationPayload{
		WorkspaceID:        publication.WorkspaceID,
		BundleID:           publication.BundleID,
		Format:             publication.Format,
		CreatedAt:          publication.CreatedAt,
		ArtifactName:       filepath.Base(publication.ArtifactName),
		ManifestName:       filepath.Base(publication.ManifestName),
		ChecksumName:       filepath.Base(publication.ChecksumName),
		FileCount:          publication.FileCount,
		ArchiveSHA256:      publication.ArchiveSHA256,
		ManifestSHA256:     publication.ManifestSHA256,
		RedactionPreviewID: publication.RedactionPreviewID,
	}
}

func currentSyncPublicationSignaturePayload(artifactPath string, publication SyncPublication) (signedSyncPublicationPayload, SyncPublication, error) {
	manifestPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".manifest.json"
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return signedSyncPublicationPayload{}, publication, fmt.Errorf("read sync manifest: %w", err)
	}
	var manifest bundleManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return signedSyncPublicationPayload{}, publication, fmt.Errorf("decode sync manifest: %w", err)
	}
	archiveSHA, err := fileSHA256(artifactPath)
	if err != nil {
		return signedSyncPublicationPayload{}, publication, err
	}
	if strings.TrimSpace(publication.BundleID) != "" && publication.BundleID != manifest.BundleID {
		return signedSyncPublicationPayload{}, publication, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("sync publication %s does not match manifest %s", publication.BundleID, manifest.BundleID))
	}
	manifestHash := sha256.Sum256(manifestRaw)
	publication.FileCount = len(manifest.Files)
	publication.ArchiveSHA256 = archiveSHA
	publication.ManifestSHA256 = hex.EncodeToString(manifestHash[:])
	if publication.BundleID == "" {
		publication.BundleID = manifest.BundleID
	}
	if publication.Format == "" {
		publication.Format = manifest.Scope
	}
	if publication.CreatedAt.IsZero() {
		publication.CreatedAt = manifest.CreatedAt
	}
	return syncPublicationSignaturePayload(publication), publication, nil
}

func upsertSignatureEnvelope(items []contracts.SignatureEnvelope, envelope contracts.SignatureEnvelope) []contracts.SignatureEnvelope {
	out := make([]contracts.SignatureEnvelope, 0, len(items)+1)
	for _, item := range items {
		if item.SignatureID != envelope.SignatureID {
			out = append(out, item)
		}
	}
	return append(out, envelope)
}

func (s *ActionService) VerifyPayloadSignatures(ctx context.Context, payload any, envelopes []contracts.SignatureEnvelope, kind contracts.ArtifactKind, uid string) (contracts.SignatureVerificationResult, error) {
	if len(envelopes) == 0 {
		result, err := s.VerifyPayloadSignature(ctx, payload, nil)
		if err != nil {
			return result, err
		}
		result.ArtifactKind = kind
		result.ArtifactUID = uid
		return result, nil
	}
	var best contracts.SignatureVerificationResult
	bestRank := -1
	for idx := range envelopes {
		envelope := envelopes[idx]
		if envelope.ArtifactKind != kind || envelope.ArtifactUID != uid {
			return contracts.SignatureVerificationResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("signature envelope does not match %s %s", kind, uid))
		}
		result, err := s.VerifyPayloadSignature(ctx, payload, &envelope)
		if err != nil {
			return contracts.SignatureVerificationResult{}, err
		}
		rank := signatureVerificationRank(result.State)
		if rank > bestRank {
			best = result
			bestRank = rank
		}
	}
	return best, nil
}

func signatureVerificationRank(state contracts.SignatureVerificationState) int {
	switch state {
	case contracts.VerificationTrustedValid:
		return 100
	case contracts.VerificationValidRotatedKey:
		return 90
	case contracts.VerificationValidExpiredKey:
		return 80
	case contracts.VerificationValidUntrusted:
		return 70
	case contracts.VerificationValidRevokedKey:
		return 60
	case contracts.VerificationValidUnknownKey:
		return 50
	case contracts.VerificationPayloadHashMismatch:
		return 40
	case contracts.VerificationInvalidSignature:
		return 30
	case contracts.VerificationUnsupportedSignatureVersion:
		return 20
	case contracts.VerificationCanonicalizationMismatch:
		return 10
	case contracts.VerificationMalformedSignature:
		return 5
	case contracts.VerificationMissingSignature:
		return 0
	default:
		return 0
	}
}

func mergeSignatureEnvelopes(items []contracts.SignatureEnvelope, extra []contracts.SignatureEnvelope) []contracts.SignatureEnvelope {
	out := append([]contracts.SignatureEnvelope{}, items...)
	for _, envelope := range extra {
		out = upsertSignatureEnvelope(out, envelope)
	}
	return out
}

func exportSignatureSidecarPath(artifactPath string) string {
	return bundleSidecarBase(artifactPath) + ".signatures.json"
}

func exportBundleRefIsPath(ref string) bool {
	ref = strings.TrimSpace(ref)
	return strings.HasSuffix(ref, ".tar.gz") || strings.HasSuffix(ref, ".tgz")
}

func writeExportSignatureSidecar(bundle contracts.ExportBundle) error {
	if strings.TrimSpace(bundle.ArtifactPath) == "" || len(bundle.SignatureEnvelopes) == 0 {
		return nil
	}
	sidecar := exportSignatureSidecar{
		FormatVersion:      "v1",
		Kind:               "export_bundle_signatures",
		BundleID:           bundle.BundleID,
		SignatureEnvelopes: bundle.SignatureEnvelopes,
	}
	raw, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return fmt.Errorf("encode export signature sidecar: %w", err)
	}
	path := exportSignatureSidecarPath(bundle.ArtifactPath)
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write export signature sidecar: %w", err)
	}
	return nil
}

func readExportSignatureSidecar(artifactPath string, bundleID string) ([]contracts.SignatureEnvelope, error) {
	raw, err := os.ReadFile(exportSignatureSidecarPath(artifactPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read export signature sidecar: %w", err)
	}
	var sidecar exportSignatureSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return nil, fmt.Errorf("decode export signature sidecar: %w", err)
	}
	if sidecar.FormatVersion != "v1" || sidecar.Kind != "export_bundle_signatures" {
		return nil, apperr.New(apperr.CodeInvalidInput, "invalid export signature sidecar")
	}
	if strings.TrimSpace(bundleID) != "" && sidecar.BundleID != bundleID {
		return nil, apperr.New(apperr.CodeInvalidInput, "export signature sidecar does not match bundle")
	}
	for _, envelope := range sidecar.SignatureEnvelopes {
		if err := validateExportSidecarSignature(envelope, sidecar.BundleID); err != nil {
			return nil, err
		}
	}
	return sidecar.SignatureEnvelopes, nil
}

func validateExportSidecarSignature(envelope contracts.SignatureEnvelope, bundleID string) error {
	if envelope.ArtifactKind != contracts.ArtifactKindBundle || envelope.ArtifactUID != bundleID {
		return apperr.New(apperr.CodeInvalidInput, "export signature sidecar contains a mismatched signature")
	}
	return envelope.Validate()
}
