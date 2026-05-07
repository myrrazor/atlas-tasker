package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type BackupDetailView struct {
	Kind         string                   `json:"kind"`
	GeneratedAt  time.Time                `json:"generated_at"`
	Snapshot     contracts.BackupSnapshot `json:"snapshot"`
	ManifestPath string                   `json:"manifest_path"`
	ArchivePath  string                   `json:"archive_path"`
	FileCount    int                      `json:"file_count"`
}

type BackupListView struct {
	Kind        string                     `json:"kind"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Items       []contracts.BackupSnapshot `json:"items"`
}

type BackupIntegrityView struct {
	BackupID             string    `json:"backup_id"`
	ArchivePath          string    `json:"archive_path,omitempty"`
	ManifestPath         string    `json:"manifest_path,omitempty"`
	ManifestSHA256       string    `json:"manifest_sha256,omitempty"`
	ExpectedManifestHash string    `json:"expected_manifest_hash,omitempty"`
	FileCount            int       `json:"file_count"`
	Verified             bool      `json:"verified"`
	Errors               []string  `json:"errors,omitempty"`
	Warnings             []string  `json:"warnings,omitempty"`
	GeneratedAt          time.Time `json:"generated_at"`
}

type RestorePlanDetailView struct {
	Kind        string                `json:"kind"`
	GeneratedAt time.Time             `json:"generated_at"`
	Plan        contracts.RestorePlan `json:"plan"`
}

type RestoreApplyResultView struct {
	Kind        string                `json:"kind"`
	GeneratedAt time.Time             `json:"generated_at"`
	Plan        contracts.RestorePlan `json:"plan"`
	Applied     int                   `json:"applied"`
	Skipped     int                   `json:"skipped"`
}

type RecoveryDrillView struct {
	Kind             string    `json:"kind"`
	GeneratedAt      time.Time `json:"generated_at"`
	BackupCount      int       `json:"backup_count"`
	VerifiedBackups  int       `json:"verified_backups"`
	RestorePlanCount int       `json:"restore_plan_count"`
	SideEffectFree   bool      `json:"side_effect_free"`
	Warnings         []string  `json:"warnings,omitempty"`
	SchemaVersion    int       `json:"schema_version"`
}

type AdminSecurityStatusView struct {
	Kind               string    `json:"kind"`
	GeneratedAt        time.Time `json:"generated_at"`
	PublicKeys         int       `json:"public_keys"`
	TrustBindings      int       `json:"trust_bindings"`
	Revocations        int       `json:"revocations"`
	GovernancePolicies int       `json:"governance_policies"`
	AuditReports       int       `json:"audit_reports"`
	Backups            int       `json:"backups"`
	GoalManifests      int       `json:"goal_manifests"`
	Warnings           []string  `json:"warnings,omitempty"`
	SchemaVersion      int       `json:"schema_version"`
}

type TrustStoreStatusView struct {
	Kind          string    `json:"kind"`
	GeneratedAt   time.Time `json:"generated_at"`
	PublicKeys    int       `json:"public_keys"`
	LocalKeys     int       `json:"local_keys"`
	TrustedKeys   int       `json:"trusted_keys"`
	RevokedKeys   int       `json:"revoked_keys"`
	ExpiredKeys   int       `json:"expired_keys"`
	Warnings      []string  `json:"warnings,omitempty"`
	SchemaVersion int       `json:"schema_version"`
}

type RecoveryStatusView struct {
	Kind             string    `json:"kind"`
	GeneratedAt      time.Time `json:"generated_at"`
	BackupCount      int       `json:"backup_count"`
	LatestBackupID   string    `json:"latest_backup_id,omitempty"`
	LatestBackupAt   time.Time `json:"latest_backup_at,omitempty"`
	RestorePlanCount int       `json:"restore_plan_count"`
	Warnings         []string  `json:"warnings,omitempty"`
	SchemaVersion    int       `json:"schema_version"`
}

type GoalBriefView struct {
	Kind        string              `json:"kind"`
	GeneratedAt time.Time           `json:"generated_at"`
	Brief       contracts.GoalBrief `json:"brief"`
}

type GoalManifestDetailView struct {
	Kind        string                 `json:"kind"`
	GeneratedAt time.Time              `json:"generated_at"`
	Manifest    contracts.GoalManifest `json:"manifest"`
	Path        string                 `json:"path,omitempty"`
}

func (s *ActionService) CreateBackup(ctx context.Context, scopeRaw string, actor contracts.Actor, reason string) (BackupDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create backup", func(ctx context.Context) (BackupDetailView, error) {
		if !actor.IsValid() {
			return BackupDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return BackupDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		scopeKind, scopeID, err := s.resolveBackupScope(ctx, scopeRaw)
		if err != nil {
			return BackupDetailView{}, err
		}
		files, err := s.backupFilesForScope(ctx, scopeKind, scopeID)
		if err != nil {
			return BackupDetailView{}, err
		}
		if len(files) == 0 {
			return BackupDetailView{}, apperr.New(apperr.CodeInvalidInput, "backup scope has no Atlas-owned files")
		}
		backupID := "backup-" + NewOpaqueID()
		now := s.now()
		manifest, err := buildBundleManifest(s.Root, backupID, backupScopeString(scopeKind, scopeID), now, files)
		if err != nil {
			return BackupDetailView{}, err
		}
		manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return BackupDetailView{}, fmt.Errorf("encode backup manifest: %w", err)
		}
		sum := sha256.Sum256(manifestRaw)
		snapshot := contracts.BackupSnapshot{
			BackupID:         backupID,
			CreatedAt:        now,
			CreatedBy:        actor,
			ScopeKind:        scopeKind,
			ScopeID:          scopeID,
			ManifestHash:     hex.EncodeToString(sum[:]),
			IncludedSections: backupSectionsForFiles(files),
			SchemaVersion:    contracts.CurrentSchemaVersion,
		}
		snapshot = normalizeBackupSnapshot(snapshot)
		event, err := s.newEvent(ctx, workspaceProjectKey, now, actor, reason, contracts.EventBackupCreated, "", snapshot)
		if err != nil {
			return BackupDetailView{}, err
		}
		if err := s.commitMutation(ctx, "create backup", "backup_snapshot", event, func(ctx context.Context) error {
			if err := os.MkdirAll(storage.BackupManifestsDir(s.Root), 0o755); err != nil {
				return fmt.Errorf("create backup manifests dir: %w", err)
			}
			if err := os.MkdirAll(storage.BackupSnapshotsDir(s.Root), 0o755); err != nil {
				return fmt.Errorf("create backup snapshots dir: %w", err)
			}
			if err := os.WriteFile(backupManifestPath(s.Root, backupID), manifestRaw, 0o644); err != nil {
				return fmt.Errorf("write backup manifest: %w", err)
			}
			if err := writeBundleArchive(s.Root, backupArchivePath(s.Root, backupID), manifestRaw, files); err != nil {
				return err
			}
			return s.Backups.SaveBackupSnapshot(ctx, snapshot)
		}); err != nil {
			return BackupDetailView{}, err
		}
		return BackupDetailView{Kind: "backup_detail", GeneratedAt: s.now(), Snapshot: snapshot, ManifestPath: backupManifestPath(s.Root, backupID), ArchivePath: backupArchivePath(s.Root, backupID), FileCount: len(files)}, nil
	})
}

func (s *ActionService) ListBackups(ctx context.Context) (BackupListView, error) {
	items, err := s.Backups.ListBackupSnapshots(ctx)
	if err != nil {
		return BackupListView{}, err
	}
	return BackupListView{Kind: "backup_list", GeneratedAt: s.now(), Items: items}, nil
}

func (s *ActionService) BackupDetail(ctx context.Context, backupID string) (BackupDetailView, error) {
	snapshot, err := s.Backups.LoadBackupSnapshot(ctx, backupID)
	if err != nil {
		return BackupDetailView{}, err
	}
	manifest, _, err := loadBundleManifestRaw(backupManifestPath(s.Root, snapshot.BackupID))
	if err != nil {
		return BackupDetailView{}, err
	}
	return BackupDetailView{Kind: "backup_detail", GeneratedAt: s.now(), Snapshot: snapshot, ManifestPath: backupManifestPath(s.Root, snapshot.BackupID), ArchivePath: backupArchivePath(s.Root, snapshot.BackupID), FileCount: len(manifest.Files)}, nil
}

func (s *ActionService) VerifyBackupSnapshot(ctx context.Context, ref string) (ArtifactSignatureVerifyView, error) {
	snapshot, integrity, err := s.verifyBackupIntegrity(ctx, ref)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	payload, uid := backupSignaturePayloadAndUID(snapshot, integrity.BackupID)
	envelopes := []contracts.SignatureEnvelope{}
	if snapshot != nil {
		stored, err := s.signaturesForArtifact(ctx, contracts.ArtifactKindBackupSnapshot, snapshot.BackupID)
		if err != nil {
			return ArtifactSignatureVerifyView{}, err
		}
		envelopes = mergeSignatureEnvelopes(snapshot.SignatureEnvelopes, stored)
	}
	result, err := s.VerifyPayloadSignatures(ctx, payload, envelopes, contracts.ArtifactKindBackupSnapshot, uid)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	return ArtifactSignatureVerifyView{Kind: "backup_verify_result", Integrity: integrity, Signature: result, GeneratedAt: s.now()}, nil
}

func (s *ActionService) SignBackupSnapshot(ctx context.Context, backupID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sign backup", func(ctx context.Context) (SignatureDetailView, error) {
		if !actor.IsValid() {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		snapshot, integrity, err := s.verifyBackupIntegrity(ctx, backupID)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if snapshot == nil {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "backup signing requires a stored snapshot id")
		}
		if !integrity.Verified {
			return SignatureDetailView{}, apperr.New(apperr.CodeConflict, "backup integrity must verify before signing")
		}
		payload, _ := backupSignaturePayloadAndUID(snapshot, snapshot.BackupID)
		envelope, err := s.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBackupSnapshot, ArtifactUID: snapshot.BackupID, PublicKeyID: publicKeyID, Payload: payload})
		if err != nil {
			return SignatureDetailView{}, err
		}
		snapshot.SignatureEnvelopes = upsertSignatureEnvelope(snapshot.SignatureEnvelopes, envelope)
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventSignatureCreated, "", envelope)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if err := s.commitMutation(ctx, "sign backup", "backup_signature", event, func(ctx context.Context) error {
			if err := s.Backups.SaveBackupSnapshot(ctx, *snapshot); err != nil {
				return err
			}
			return s.Signatures.SaveSignature(ctx, envelope)
		}); err != nil {
			return SignatureDetailView{}, err
		}
		return SignatureDetailView{Kind: "signature_detail", ArtifactKind: envelope.ArtifactKind, ArtifactUID: envelope.ArtifactUID, Signature: envelope, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) CreateRestorePlan(ctx context.Context, ref string, actor contracts.Actor) (RestorePlanDetailView, error) {
	if !actor.IsValid() {
		return RestorePlanDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
	}
	snapshot, manifest, _, _, err := s.resolveBackupArtifact(ctx, ref)
	if err != nil {
		return RestorePlanDetailView{}, err
	}
	backupID := manifest.BundleID
	if snapshot != nil {
		backupID = snapshot.BackupID
	}
	plan, err := s.buildRestorePlan(ctx, backupID, manifest, actor)
	if err != nil {
		return RestorePlanDetailView{}, err
	}
	return RestorePlanDetailView{Kind: "backup_restore_plan", GeneratedAt: s.now(), Plan: plan}, nil
}

func (s *ActionService) ApplyRestorePlan(ctx context.Context, ref string, actor contracts.Actor, reason string, yes bool) (RestoreApplyResultView, error) {
	return withWriteLock(ctx, s.LockManager, "apply backup restore", func(ctx context.Context) (RestoreApplyResultView, error) {
		if !yes {
			return RestoreApplyResultView{}, apperr.New(apperr.CodeInvalidInput, "restore apply requires --yes")
		}
		if !actor.IsValid() {
			return RestoreApplyResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return RestoreApplyResultView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		if _, integrity, err := s.verifyBackupIntegrity(ctx, ref); err != nil {
			return RestoreApplyResultView{}, err
		} else if !integrity.Verified {
			return RestoreApplyResultView{}, apperr.New(apperr.CodeConflict, "backup integrity must verify before restore")
		}
		snapshot, manifest, _, archivePath, err := s.resolveBackupArtifact(ctx, ref)
		if err != nil {
			return RestoreApplyResultView{}, err
		}
		backupID := manifest.BundleID
		if snapshot != nil {
			backupID = snapshot.BackupID
		}
		plan, err := s.buildRestorePlan(ctx, backupID, manifest, actor)
		if err != nil {
			return RestoreApplyResultView{}, err
		}
		for _, item := range plan.Items {
			if item.Action == contracts.RestorePlanBlock {
				return RestoreApplyResultView{}, apperr.New(apperr.CodeConflict, "restore plan has blocked items")
			}
		}
		files, err := readBundleArchive(archivePath)
		if err != nil {
			return RestoreApplyResultView{}, err
		}
		applied := 0
		skipped := 0
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventBackupRestored, "", plan)
		if err != nil {
			return RestoreApplyResultView{}, err
		}
		if err := s.commitMutation(ctx, "apply backup restore", "backup_restore", event, func(ctx context.Context) error {
			for _, item := range plan.Items {
				if item.Action == contracts.RestorePlanSkip {
					skipped++
					continue
				}
				raw, ok := files[item.Path]
				if !ok {
					return apperr.New(apperr.CodeConflict, fmt.Sprintf("backup archive missing %s", item.Path))
				}
				target := filepath.Join(s.Root, filepath.FromSlash(item.Path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(target, raw, 0o644); err != nil {
					return err
				}
				applied++
			}
			return nil
		}); err != nil {
			return RestoreApplyResultView{}, err
		}
		return RestoreApplyResultView{Kind: "backup_restore_result", GeneratedAt: s.now(), Plan: plan, Applied: applied, Skipped: skipped}, nil
	})
}

func (s *ActionService) RecoveryDrill(ctx context.Context) (RecoveryDrillView, error) {
	backups, err := s.Backups.ListBackupSnapshots(ctx)
	if err != nil {
		return RecoveryDrillView{}, err
	}
	plans, err := s.RestorePlans.ListRestorePlans(ctx)
	if err != nil {
		return RecoveryDrillView{}, err
	}
	view := RecoveryDrillView{Kind: "recovery_drill_result", GeneratedAt: s.now(), BackupCount: len(backups), RestorePlanCount: len(plans), SideEffectFree: true, SchemaVersion: contracts.CurrentSchemaVersion}
	for _, backup := range backups {
		_, integrity, err := s.verifyBackupIntegrity(ctx, backup.BackupID)
		if err != nil {
			view.Warnings = append(view.Warnings, "backup_verify_error:"+backup.BackupID)
			continue
		}
		if integrity.Verified {
			view.VerifiedBackups++
		} else {
			view.Warnings = append(view.Warnings, "backup_not_verified:"+backup.BackupID)
		}
	}
	if len(backups) == 0 {
		view.Warnings = append(view.Warnings, "no_backups")
	}
	sort.Strings(view.Warnings)
	return view, nil
}

func (s *ActionService) AdminSecurityStatus(ctx context.Context) (AdminSecurityStatusView, error) {
	keys, err := s.SecurityKeys.ListPublicKeys(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	bindings, err := s.TrustBindings.ListTrustBindings(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	revocations, err := s.SecurityKeys.ListRevocations(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	policies, err := s.GovernancePolicies.ListGovernancePolicies(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	reports, err := s.AuditReports.ListAuditReports(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	backups, err := s.Backups.ListBackupSnapshots(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	goals, err := s.GoalManifests.ListGoalManifests(ctx)
	if err != nil {
		return AdminSecurityStatusView{}, err
	}
	view := AdminSecurityStatusView{Kind: "admin_security_status", GeneratedAt: s.now(), PublicKeys: len(keys), TrustBindings: len(bindings), Revocations: len(revocations), GovernancePolicies: len(policies), AuditReports: len(reports), Backups: len(backups), GoalManifests: len(goals), SchemaVersion: contracts.CurrentSchemaVersion}
	if len(keys) == 0 {
		view.Warnings = append(view.Warnings, "no_public_keys")
	}
	if len(bindings) == 0 {
		view.Warnings = append(view.Warnings, "no_trust_bindings")
	}
	if len(backups) == 0 {
		view.Warnings = append(view.Warnings, "no_backups")
	}
	return view, nil
}

func (s *ActionService) TrustStoreStatus(ctx context.Context) (TrustStoreStatusView, error) {
	keys, err := s.SecurityKeys.ListPublicKeys(ctx)
	if err != nil {
		return TrustStoreStatusView{}, err
	}
	bindings, err := s.TrustBindings.ListTrustBindings(ctx)
	if err != nil {
		return TrustStoreStatusView{}, err
	}
	view := TrustStoreStatusView{Kind: "trust_store_status", GeneratedAt: s.now(), PublicKeys: len(keys), SchemaVersion: contracts.CurrentSchemaVersion}
	for _, key := range keys {
		if key.Source == contracts.PublicKeySourceLocal {
			view.LocalKeys++
		}
		if key.Status == contracts.KeyStateRevoked {
			view.RevokedKeys++
		}
		if keyExpired(key, s.now()) {
			view.ExpiredKeys++
		}
	}
	for _, binding := range bindings {
		if binding.TrustLevel == contracts.TrustLevelTrusted || binding.TrustLevel == contracts.TrustLevelRestricted {
			view.TrustedKeys++
		}
	}
	if view.LocalKeys == 0 {
		view.Warnings = append(view.Warnings, "no_local_signing_keys")
	}
	if view.TrustedKeys == 0 {
		view.Warnings = append(view.Warnings, "no_trusted_keys")
	}
	return view, nil
}

func (s *ActionService) RecoveryStatus(ctx context.Context) (RecoveryStatusView, error) {
	backups, err := s.Backups.ListBackupSnapshots(ctx)
	if err != nil {
		return RecoveryStatusView{}, err
	}
	plans, err := s.RestorePlans.ListRestorePlans(ctx)
	if err != nil {
		return RecoveryStatusView{}, err
	}
	view := RecoveryStatusView{Kind: "recovery_status", GeneratedAt: s.now(), BackupCount: len(backups), RestorePlanCount: len(plans), SchemaVersion: contracts.CurrentSchemaVersion}
	if len(backups) == 0 {
		view.Warnings = append(view.Warnings, "no_backups")
		return view, nil
	}
	latest := backups[len(backups)-1]
	view.LatestBackupID = latest.BackupID
	view.LatestBackupAt = latest.CreatedAt
	if _, integrity, err := s.verifyBackupIntegrity(ctx, latest.BackupID); err != nil || !integrity.Verified {
		view.Warnings = append(view.Warnings, "latest_backup_not_verified")
	}
	return view, nil
}

func (s *ActionService) GoalBrief(ctx context.Context, target string) (GoalBriefView, error) {
	brief, err := s.buildGoalBrief(ctx, target)
	if err != nil {
		return GoalBriefView{}, err
	}
	return GoalBriefView{Kind: "goal_brief", GeneratedAt: s.now(), Brief: brief}, nil
}

func (s *ActionService) CreateGoalManifest(ctx context.Context, target string, actor contracts.Actor, reason string) (GoalManifestDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create goal manifest", func(ctx context.Context) (GoalManifestDetailView, error) {
		if !actor.IsValid() {
			return GoalManifestDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return GoalManifestDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		brief, err := s.buildGoalBrief(ctx, target)
		if err != nil {
			return GoalManifestDetailView{}, err
		}
		policyHash, err := s.auditPolicySnapshotHash(ctx)
		if err != nil {
			return GoalManifestDetailView{}, err
		}
		trustHash, err := s.auditTrustSnapshotHash(ctx)
		if err != nil {
			return GoalManifestDetailView{}, err
		}
		sourceHash, err := goalSourceHash(brief)
		if err != nil {
			return GoalManifestDetailView{}, err
		}
		manifest := contracts.GoalManifest{
			ManifestID:         "goal-" + NewOpaqueID(),
			TargetKind:         brief.TargetKind,
			TargetID:           brief.TargetID,
			Objective:          brief.Objective,
			Sections:           completeGoalSections(brief.Sections),
			PolicySnapshotHash: policyHash,
			TrustSnapshotHash:  trustHash,
			SourceHash:         sourceHash,
			GeneratedAt:        s.now(),
			GeneratedBy:        actor,
			Reason:             reason,
			SchemaVersion:      contracts.CurrentSchemaVersion,
		}
		manifest = normalizeGoalManifest(manifest)
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventGoalManifestGenerated, goalTicketID(manifest), manifest)
		if err != nil {
			return GoalManifestDetailView{}, err
		}
		if err := s.commitMutation(ctx, "create goal manifest", "goal_manifest", event, func(ctx context.Context) error {
			return s.GoalManifests.SaveGoalManifest(ctx, manifest)
		}); err != nil {
			return GoalManifestDetailView{}, err
		}
		return GoalManifestDetailView{Kind: "goal_manifest", GeneratedAt: s.now(), Manifest: manifest, Path: goalManifestPath(s.Root, manifest.ManifestID)}, nil
	})
}

func (s *ActionService) GoalManifestDetail(ctx context.Context, manifestID string) (GoalManifestDetailView, error) {
	manifest, err := s.GoalManifests.LoadGoalManifest(ctx, manifestID)
	if err != nil {
		return GoalManifestDetailView{}, err
	}
	return GoalManifestDetailView{Kind: "goal_manifest", GeneratedAt: s.now(), Manifest: manifest, Path: goalManifestPath(s.Root, manifest.ManifestID)}, nil
}

func (s *ActionService) SignGoalManifest(ctx context.Context, manifestID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sign goal manifest", func(ctx context.Context) (SignatureDetailView, error) {
		if !actor.IsValid() {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		manifest, err := s.GoalManifests.LoadGoalManifest(ctx, manifestID)
		if err != nil {
			return SignatureDetailView{}, err
		}
		payload := goalSignaturePayload(manifest)
		envelope, err := s.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindGoalManifest, ArtifactUID: manifest.ManifestID, PublicKeyID: publicKeyID, Payload: payload})
		if err != nil {
			return SignatureDetailView{}, err
		}
		manifest.SignatureEnvelopes = upsertSignatureEnvelope(manifest.SignatureEnvelopes, envelope)
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventSignatureCreated, goalTicketID(manifest), envelope)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if err := s.commitMutation(ctx, "sign goal manifest", "goal_signature", event, func(ctx context.Context) error {
			if err := s.GoalManifests.SaveGoalManifest(ctx, manifest); err != nil {
				return err
			}
			return s.Signatures.SaveSignature(ctx, envelope)
		}); err != nil {
			return SignatureDetailView{}, err
		}
		return SignatureDetailView{Kind: "signature_detail", ArtifactKind: envelope.ArtifactKind, ArtifactUID: envelope.ArtifactUID, Signature: envelope, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) VerifyGoalManifest(ctx context.Context, ref string) (ArtifactSignatureVerifyView, error) {
	manifest, err := s.resolveGoalManifest(ctx, ref)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	stored, err := s.signaturesForArtifact(ctx, contracts.ArtifactKindGoalManifest, manifest.ManifestID)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	envelopes := mergeSignatureEnvelopes(manifest.SignatureEnvelopes, stored)
	result, err := s.VerifyPayloadSignatures(ctx, goalSignaturePayload(manifest), envelopes, contracts.ArtifactKindGoalManifest, manifest.ManifestID)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	return ArtifactSignatureVerifyView{Kind: "goal_manifest_verify_result", Signature: result, GeneratedAt: s.now()}, nil
}

func (s *ActionService) verifyBackupIntegrity(ctx context.Context, ref string) (*contracts.BackupSnapshot, BackupIntegrityView, error) {
	snapshot, manifest, manifestRaw, archivePath, err := s.resolveBackupArtifact(ctx, ref)
	if err != nil {
		return nil, BackupIntegrityView{}, err
	}
	manifestHash := sha256.Sum256(manifestRaw)
	manifestSHA := hex.EncodeToString(manifestHash[:])
	expected := manifestSHA
	if snapshot != nil {
		expected = snapshot.ManifestHash
	}
	view := BackupIntegrityView{BackupID: manifest.BundleID, ArchivePath: archivePath, ManifestSHA256: manifestSHA, ExpectedManifestHash: expected, FileCount: len(manifest.Files), Verified: true, GeneratedAt: s.now()}
	if snapshot != nil {
		view.BackupID = snapshot.BackupID
		view.ManifestPath = backupManifestPath(s.Root, snapshot.BackupID)
	}
	if expected != "" && expected != manifestSHA {
		view.Verified = false
		view.Errors = append(view.Errors, "manifest_hash_mismatch")
	}
	files, err := readBundleArchive(archivePath)
	if err != nil {
		return snapshot, view, err
	}
	archiveManifest, ok := files["manifest.json"]
	if !ok || hashBytes(archiveManifest) != manifestSHA {
		view.Verified = false
		view.Errors = append(view.Errors, "archive_manifest_mismatch")
	}
	for _, item := range manifest.Files {
		if err := (contracts.RestorePlanItem{Path: item.Path, Action: contracts.RestorePlanCreate}).Validate(); err != nil {
			view.Verified = false
			view.Errors = append(view.Errors, "unsafe_path:"+item.Path)
			continue
		}
		raw, ok := files[item.Path]
		if !ok {
			view.Verified = false
			view.Errors = append(view.Errors, "archive_missing:"+item.Path)
			continue
		}
		if hashBytes(raw) != item.SHA256 {
			view.Verified = false
			view.Errors = append(view.Errors, "file_hash_mismatch:"+item.Path)
		}
	}
	if snapshot != nil && len(snapshot.SignatureEnvelopes) == 0 {
		view.Warnings = append(view.Warnings, "backup_unsigned")
	}
	return snapshot, view, nil
}

func (s *ActionService) resolveBackupArtifact(ctx context.Context, ref string) (*contracts.BackupSnapshot, bundleManifest, []byte, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, bundleManifest{}, nil, "", apperr.New(apperr.CodeInvalidInput, "backup reference is required")
	}
	if backupRefLooksPath(ref) {
		if strings.HasSuffix(ref, ".tar.gz") || strings.HasSuffix(ref, ".tgz") {
			manifest, raw, err := loadManifestFromArchive(ref)
			if err != nil {
				return nil, bundleManifest{}, nil, "", err
			}
			var snapshot *contracts.BackupSnapshot
			if stored, err := s.Backups.LoadBackupSnapshot(ctx, manifest.BundleID); err == nil {
				snapshot = &stored
			}
			return snapshot, manifest, raw, ref, nil
		}
		raw, err := os.ReadFile(ref)
		if err != nil {
			return nil, bundleManifest{}, nil, "", err
		}
		var snapshot contracts.BackupSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return nil, bundleManifest{}, nil, "", err
		}
		snapshot = normalizeBackupSnapshot(snapshot)
		manifest, manifestRaw, err := loadBundleManifestRaw(backupManifestPath(s.Root, snapshot.BackupID))
		if err != nil {
			return nil, bundleManifest{}, nil, "", err
		}
		return &snapshot, manifest, manifestRaw, backupArchivePath(s.Root, snapshot.BackupID), nil
	}
	snapshot, err := s.Backups.LoadBackupSnapshot(ctx, ref)
	if err != nil {
		return nil, bundleManifest{}, nil, "", err
	}
	manifest, manifestRaw, err := loadBundleManifestRaw(backupManifestPath(s.Root, snapshot.BackupID))
	if err != nil {
		return nil, bundleManifest{}, nil, "", err
	}
	return &snapshot, manifest, manifestRaw, backupArchivePath(s.Root, snapshot.BackupID), nil
}

func (s *ActionService) buildRestorePlan(ctx context.Context, backupID string, manifest bundleManifest, actor contracts.Actor) (contracts.RestorePlan, error) {
	items := make([]contracts.RestorePlanItem, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		item := contracts.RestorePlanItem{Path: file.Path, Action: contracts.RestorePlanCreate}
		if err := item.Validate(); err != nil {
			item.Action = contracts.RestorePlanBlock
			item.ReasonCodes = []string{"unsafe_restore_path"}
		} else if _, err := os.Stat(filepath.Join(s.Root, filepath.FromSlash(file.Path))); err == nil {
			current, hashErr := fileSHA256(filepath.Join(s.Root, filepath.FromSlash(file.Path)))
			if hashErr != nil {
				return contracts.RestorePlan{}, hashErr
			}
			if current == file.SHA256 {
				item.Action = contracts.RestorePlanSkip
				item.ReasonCodes = []string{"already_current"}
			} else {
				item.Action = contracts.RestorePlanUpdate
				item.ReasonCodes = []string{"content_differs"}
			}
		} else if !os.IsNotExist(err) {
			return contracts.RestorePlan{}, err
		}
		items = append(items, item)
	}
	rootHash, err := s.backupTargetRootHash(ctx)
	if err != nil {
		return contracts.RestorePlan{}, err
	}
	plan := contracts.RestorePlan{
		RestorePlanID:  "restore-" + NewOpaqueID(),
		BackupID:       backupID,
		GeneratedAt:    s.now(),
		GeneratedBy:    actor,
		TargetRootHash: rootHash,
		Items:          items,
		Warnings:       []string{"restore_does_not_recreate_provider_state", "restore_does_not_recreate_worktrees_runtime_notifiers_or_mcp_approvals"},
		SchemaVersion:  contracts.CurrentSchemaVersion,
	}
	plan = normalizeRestorePlan(plan)
	return plan, plan.Validate()
}

func (s *ActionService) resolveBackupScope(ctx context.Context, raw string) (contracts.BackupScopeKind, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "workspace" {
		return contracts.BackupScopeWorkspace, "", nil
	}
	kind, id, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(kind) != "project" || strings.TrimSpace(id) == "" {
		return "", "", apperr.New(apperr.CodeInvalidInput, "backup scope must be workspace or project:<KEY>")
	}
	projectID := strings.TrimSpace(id)
	if _, err := s.Projects.GetProject(ctx, projectID); err != nil {
		return "", "", err
	}
	return contracts.BackupScopeProject, projectID, nil
}

func (s *ActionService) backupFilesForScope(ctx context.Context, kind contracts.BackupScopeKind, id string) ([]string, error) {
	files, err := collectExportFiles(s.Root)
	if err != nil {
		return nil, err
	}
	if kind == contracts.BackupScopeWorkspace {
		return backupRestoreSafeFiles(files), nil
	}
	ids, err := s.auditScopeArtifactIDs(ctx, auditScope{Kind: contracts.AuditScopeProject, ID: id, Project: id})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, rel := range files {
		if auditFileRelevantToScope(rel, auditScope{Kind: contracts.AuditScopeProject, ID: id, Project: id}, ids) || strings.HasPrefix(rel, ".tracker/audit/") {
			out = append(out, rel)
		}
	}
	return backupRestoreSafeFiles(out), nil
}

func backupRestoreSafeFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, rel := range files {
		item := contracts.RestorePlanItem{Path: rel, Action: contracts.RestorePlanCreate}
		if item.Validate() == nil {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return out
}

func backupSectionsForFiles(files []string) []contracts.BackupSection {
	sections := []contracts.BackupSection{}
	add := func(section contracts.BackupSection) { sections = append(sections, section) }
	for _, rel := range files {
		switch {
		case strings.HasPrefix(rel, "projects/"):
			add(contracts.BackupSectionProjects)
		case strings.HasPrefix(rel, ".tracker/events/"):
			add(contracts.BackupSectionEvents)
		case strings.HasPrefix(rel, ".tracker/runs/"):
			add(contracts.BackupSectionRuns)
		case strings.HasPrefix(rel, ".tracker/gates/"):
			add(contracts.BackupSectionGates)
		case strings.HasPrefix(rel, ".tracker/evidence/"):
			add(contracts.BackupSectionEvidence)
		case strings.HasPrefix(rel, ".tracker/collaborators/") || strings.HasPrefix(rel, ".tracker/memberships/") || strings.HasPrefix(rel, ".tracker/mentions/"):
			add(contracts.BackupSectionCollaboration)
		case strings.HasPrefix(rel, ".tracker/security/"):
			add(contracts.BackupSectionPublicSecurity)
		case strings.HasPrefix(rel, ".tracker/governance/"):
			add(contracts.BackupSectionGovernance)
		case strings.HasPrefix(rel, ".tracker/classification/") || strings.HasPrefix(rel, ".tracker/redaction/rules/"):
			add(contracts.BackupSectionClassification)
		case strings.HasPrefix(rel, ".tracker/audit/"):
			add(contracts.BackupSectionAudit)
		}
	}
	return normalizeBackupSections(sections)
}

func backupScopeString(kind contracts.BackupScopeKind, id string) string {
	if kind == contracts.BackupScopeProject {
		return "project:" + id
	}
	return "workspace"
}

func backupRefLooksPath(ref string) bool {
	return strings.Contains(ref, string(os.PathSeparator)) || strings.HasSuffix(ref, ".json") || strings.HasSuffix(ref, ".tar.gz") || strings.HasSuffix(ref, ".tgz")
}

func backupSignaturePayloadAndUID(snapshot *contracts.BackupSnapshot, fallbackUID string) (any, string) {
	if snapshot == nil {
		return map[string]string{"backup_id": fallbackUID}, fallbackUID
	}
	payload := *snapshot
	payload.SignatureEnvelopes = nil
	return normalizeBackupSnapshot(payload), snapshot.BackupID
}

func (s *ActionService) backupTargetRootHash(ctx context.Context) (string, error) {
	files, err := collectExportFiles(s.Root)
	if err != nil {
		return "", err
	}
	items := make([]bundleFileRecord, 0, len(files))
	for _, rel := range backupRestoreSafeFiles(files) {
		sum, err := fileSHA256(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		items = append(items, bundleFileRecord{Path: rel, SHA256: sum})
	}
	return hashJSON(items), nil
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *ActionService) buildGoalBrief(ctx context.Context, target string) (contracts.GoalBrief, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return contracts.GoalBrief{}, apperr.New(apperr.CodeInvalidInput, "goal target is required")
	}
	if ticket, err := s.Tickets.GetTicket(ctx, target); err == nil {
		return s.goalBriefForTicket(ctx, ticket, nil)
	} else if apperr.CodeOf(err) != apperr.CodeNotFound {
		return contracts.GoalBrief{}, err
	}
	run, err := s.Runs.LoadRun(ctx, target)
	if err != nil {
		return contracts.GoalBrief{}, err
	}
	ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
	if err != nil {
		return contracts.GoalBrief{}, err
	}
	return s.goalBriefForTicket(ctx, ticket, &run)
}

func (s *ActionService) goalBriefForTicket(ctx context.Context, ticket contracts.TicketSnapshot, run *contracts.RunSnapshot) (contracts.GoalBrief, error) {
	targetKind := contracts.GoalTargetTicket
	targetID := ticket.ID
	objective := strings.TrimSpace(firstNonEmpty(ticket.Summary, ticket.Description, ticket.Title))
	if run != nil {
		targetKind = contracts.GoalTargetRun
		targetID = run.RunID
		objective = strings.TrimSpace(firstNonEmpty(run.Summary, run.Result, ticket.Summary, ticket.Description, ticket.Title))
	}
	if objective == "" {
		objective = "Complete " + ticket.ID
	}
	sections, err := s.goalSections(ctx, ticket, run)
	if err != nil {
		return contracts.GoalBrief{}, err
	}
	brief := contracts.GoalBrief{TargetKind: targetKind, TargetID: targetID, Objective: objective, Sections: sections, GeneratedAt: s.now(), SchemaVersion: contracts.CurrentSchemaVersion}
	return brief, brief.Validate()
}

func (s *ActionService) goalSections(ctx context.Context, ticket contracts.TicketSnapshot, run *contracts.RunSnapshot) ([]contracts.GoalSection, error) {
	runs, err := s.Runs.ListRuns(ctx, ticket.ID)
	if err != nil {
		return nil, err
	}
	gates, err := s.Gates.ListGates(ctx, ticket.ID)
	if err != nil {
		return nil, err
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, ticket.ID)
	if err != nil {
		return nil, err
	}
	changes, err := s.Changes.ListChanges(ctx, ticket.ID)
	if err != nil {
		return nil, err
	}
	evidenceItems := []contracts.EvidenceItem{}
	if run != nil {
		evidenceItems, err = s.Evidence.ListEvidence(ctx, run.RunID)
		if err != nil {
			return nil, err
		}
	} else {
		for _, item := range runs {
			items, err := s.Evidence.ListEvidence(ctx, item.RunID)
			if err != nil {
				return nil, err
			}
			evidenceItems = append(evidenceItems, items...)
		}
	}
	current := ticket.ID + " " + ticket.Title
	if run != nil {
		current = run.RunID + " for " + ticket.ID
	}
	return completeGoalSections([]contracts.GoalSection{
		{Heading: "Goal", Body: ticket.Title},
		{Heading: "Objective", Body: firstNonEmpty(ticket.Summary, ticket.Description, ticket.Title)},
		{Heading: "Current State", Items: currentStateLines(ticket, run, gates)},
		{Heading: "Ticket / Run", Items: goalCompactStrings(current, latestRunLine(run, ticket))},
		{Heading: "Acceptance Criteria", Items: fallbackItems(ticket.AcceptanceCriteria, "Satisfy the ticket acceptance criteria and record evidence.")},
		{Heading: "Constraints", Items: goalCompactStrings("preserve existing user work", "do not bypass governance gates", strings.Join(ticket.RequiredCapabilities, ", "))},
		{Heading: "Required Evidence", Items: evidenceNeedLines(ticket, gates)},
		{Heading: "Required Gates", Items: gateLines(gates)},
		{Heading: "Allowed Actions", Items: goalCompactStrings("read and update Atlas-owned files for this ticket", "run local tests and attach evidence", "request review when done")},
		{Heading: "Do Not Do", Items: goalCompactStrings("do not alter private keys or trust decisions", "do not recreate provider state from local manifests", "do not skip required approvals")},
		{Heading: "Context", Items: contextLines(ticket, runs, changes, handoffs, evidenceItems)},
		{Heading: "Suggested Commands", Items: suggestedCommandLines(ticket, run)},
		{Heading: "Done When", Items: doneWhenLines(ticket, gates)},
		{Heading: "Verification", Items: verificationLines(ticket, run)},
	}), nil
}

func completeGoalSections(input []contracts.GoalSection) []contracts.GoalSection {
	byHeading := map[string]contracts.GoalSection{}
	for _, section := range input {
		byHeading[section.Heading] = section
	}
	out := make([]contracts.GoalSection, 0, len(contracts.GoalManifestSectionOrder))
	for _, heading := range contracts.GoalManifestSectionOrder {
		section, ok := byHeading[heading]
		if !ok {
			section = contracts.GoalSection{Heading: heading, Body: "None"}
		}
		if len(section.Items) == 0 && strings.TrimSpace(section.Body) == "" {
			section.Body = "None"
		}
		out = append(out, section)
	}
	return out
}

func (s *ActionService) resolveGoalManifest(ctx context.Context, ref string) (contracts.GoalManifest, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return contracts.GoalManifest{}, apperr.New(apperr.CodeInvalidInput, "goal manifest reference is required")
	}
	if strings.Contains(ref, string(os.PathSeparator)) || strings.HasSuffix(ref, ".json") {
		raw, err := os.ReadFile(ref)
		if err != nil {
			return contracts.GoalManifest{}, err
		}
		return decodeGoalManifest(raw, ref)
	}
	return s.GoalManifests.LoadGoalManifest(ctx, ref)
}

func goalSignaturePayload(manifest contracts.GoalManifest) contracts.GoalManifest {
	manifest = normalizeGoalManifest(manifest)
	manifest.SignatureEnvelopes = nil
	return manifest
}

func goalSourceHash(brief contracts.GoalBrief) (string, error) {
	raw, err := contracts.CanonicalizeAtlasV1(brief)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func goalTicketID(manifest contracts.GoalManifest) string {
	if manifest.TargetKind == contracts.GoalTargetTicket {
		return manifest.TargetID
	}
	return ""
}

func goalCompactStrings(values ...string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return []string{"None"}
	}
	return out
}

func fallbackItems(items []string, fallback string) []string {
	out := goalCompactStrings(items...)
	if len(out) == 1 && out[0] == "None" {
		return []string{fallback}
	}
	return out
}

func latestRunLine(run *contracts.RunSnapshot, ticket contracts.TicketSnapshot) string {
	if run != nil {
		return "run: " + run.RunID + " " + string(run.Status)
	}
	if strings.TrimSpace(ticket.LatestRunID) != "" {
		return "latest run: " + ticket.LatestRunID
	}
	return ""
}

func currentStateLines(ticket contracts.TicketSnapshot, run *contracts.RunSnapshot, gates []contracts.GateSnapshot) []string {
	lines := []string{
		"ticket status: " + string(ticket.Status),
		"priority: " + string(ticket.Priority),
	}
	if run != nil {
		lines = append(lines, "run status: "+string(run.Status))
	}
	for _, blocker := range blockerLines(ticket, gates) {
		if blocker != "None" {
			lines = append(lines, blocker)
		}
	}
	return goalCompactStrings(lines...)
}

func gateLines(gates []contracts.GateSnapshot) []string {
	lines := []string{}
	for _, gate := range gates {
		lines = append(lines, fmt.Sprintf("%s %s %s", gate.GateID, gate.Kind, gate.State))
	}
	return goalCompactStrings(lines...)
}

func evidenceNeedLines(ticket contracts.TicketSnapshot, gates []contracts.GateSnapshot) []string {
	lines := append([]string{}, ticket.AcceptanceCriteria...)
	for _, gate := range gates {
		for _, kind := range gate.EvidenceRequirements {
			lines = append(lines, "gate "+gate.GateID+" requires "+string(kind))
		}
	}
	return fallbackItems(lines, "Record test output, review notes, and handoff evidence before completion.")
}

func blockerLines(ticket contracts.TicketSnapshot, gates []contracts.GateSnapshot) []string {
	lines := append([]string{}, ticket.BlockedBy...)
	for _, gate := range gates {
		if gate.State == contracts.GateStateOpen {
			lines = append(lines, "open gate: "+gate.GateID)
		}
	}
	return goalCompactStrings(lines...)
}

func contextLines(ticket contracts.TicketSnapshot, runs []contracts.RunSnapshot, changes []contracts.ChangeRef, handoffs []contracts.HandoffPacket, evidence []contracts.EvidenceItem) []string {
	lines := []string{"ticket:" + ticket.ID}
	for _, run := range runs {
		lines = append(lines, "run:"+run.RunID+" "+string(run.Status))
	}
	for _, change := range changes {
		lines = append(lines, "change:"+change.ChangeID+" "+string(change.Status))
	}
	for _, handoff := range handoffs {
		lines = append(lines, "handoff:"+handoff.HandoffID)
	}
	for _, item := range evidence {
		lines = append(lines, "evidence:"+item.EvidenceID+" "+item.Title)
	}
	return goalCompactStrings(lines...)
}

func suggestedCommandLines(ticket contracts.TicketSnapshot, run *contracts.RunSnapshot) []string {
	target := ticket.ID
	lines := []string{
		"tracker inspect " + ticket.ID + " --actor <actor> --json",
		"tracker ticket claim " + ticket.ID + " --actor <actor>",
		"tracker ticket move " + ticket.ID + " in_progress --actor <actor> --reason \"start work\"",
	}
	if run != nil {
		target = run.RunID
		lines = append(lines,
			"tracker run checkpoint "+run.RunID+" --title \"progress\" --body \"what changed\" --actor <actor> --reason \"record progress\"",
			"tracker run evidence add "+run.RunID+" --type test_result --title \"verification\" --body \"test output\" --actor <actor> --reason \"record verification\"",
		)
	} else if strings.TrimSpace(ticket.LatestRunID) != "" {
		target = ticket.LatestRunID
		lines = append(lines, "tracker run open "+ticket.LatestRunID+" --json")
	}
	lines = append(lines, "tracker goal brief "+target+" --md")
	return goalCompactStrings(lines...)
}

func doneWhenLines(ticket contracts.TicketSnapshot, gates []contracts.GateSnapshot) []string {
	lines := append([]string{}, ticket.AcceptanceCriteria...)
	for _, gate := range gates {
		if gate.State == contracts.GateStateOpen {
			lines = append(lines, "gate "+gate.GateID+" is approved or resolved")
		}
	}
	lines = append(lines, "tests and handoff evidence are recorded")
	return goalCompactStrings(lines...)
}

func verificationLines(ticket contracts.TicketSnapshot, run *contracts.RunSnapshot) []string {
	lines := []string{
		"run the relevant local tests before requesting review",
		"attach command output or artifact evidence to Atlas",
		"confirm required gates are approved before completion",
	}
	if run != nil {
		lines = append(lines, "verify run "+run.RunID+" evidence before handoff")
	} else if strings.TrimSpace(ticket.LatestRunID) != "" {
		lines = append(lines, "review latest run "+ticket.LatestRunID+" before completion")
	}
	return goalCompactStrings(lines...)
}
