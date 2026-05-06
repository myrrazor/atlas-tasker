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
	"gopkg.in/yaml.v3"
)

const defaultRedactionPreviewTTL = 10 * time.Minute

type ClassificationStep struct {
	EntityKind  contracts.ClassifiedEntityKind `json:"entity_kind"`
	EntityID    string                         `json:"entity_id"`
	Level       contracts.ClassificationLevel  `json:"level"`
	Explicit    bool                           `json:"explicit"`
	ReasonCodes []string                       `json:"reason_codes,omitempty"`
}

type ClassificationDetailView struct {
	Kind          string                         `json:"kind"`
	GeneratedAt   time.Time                      `json:"generated_at"`
	EntityKind    contracts.ClassifiedEntityKind `json:"entity_kind"`
	EntityID      string                         `json:"entity_id"`
	Level         contracts.ClassificationLevel  `json:"level"`
	Label         *contracts.ClassificationLabel `json:"label,omitempty"`
	Inheritance   []ClassificationStep           `json:"inheritance,omitempty"`
	ReasonCodes   []string                       `json:"reason_codes,omitempty"`
	SchemaVersion int                            `json:"schema_version"`
}

type ClassificationListView struct {
	Kind        string                          `json:"kind"`
	GeneratedAt time.Time                       `json:"generated_at"`
	Items       []contracts.ClassificationLabel `json:"items"`
}

type RedactionPreviewDetailView struct {
	Kind        string                     `json:"kind"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Preview     contracts.RedactionPreview `json:"preview"`
}

type RedactionExportResultView struct {
	Kind          string                     `json:"kind"`
	GeneratedAt   time.Time                  `json:"generated_at"`
	Bundle        contracts.ExportBundle     `json:"bundle"`
	Preview       contracts.RedactionPreview `json:"preview"`
	Included      int                        `json:"included"`
	Omitted       int                        `json:"omitted"`
	ReasonCodes   []string                   `json:"reason_codes,omitempty"`
	SchemaVersion int                        `json:"schema_version"`
}

type RedactionVerifyView struct {
	Kind               string    `json:"kind"`
	GeneratedAt        time.Time `json:"generated_at"`
	Artifact           string    `json:"artifact"`
	RedactionPreviewID string    `json:"redaction_preview_id,omitempty"`
	Verified           bool      `json:"verified"`
	Errors             []string  `json:"errors,omitempty"`
	SchemaVersion      int       `json:"schema_version"`
}

func (s *ActionService) SetClassification(ctx context.Context, entity string, level contracts.ClassificationLevel, actor contracts.Actor, reason string) (ClassificationDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "set classification", func(ctx context.Context) (ClassificationDetailView, error) {
		if !actor.IsValid() {
			return ClassificationDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return ClassificationDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		if !level.IsValid() {
			return ClassificationDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid classification level: %s", level))
		}
		kind, entityID, err := s.resolveClassifiedEntity(ctx, entity)
		if err != nil {
			return ClassificationDetailView{}, err
		}
		now := s.now()
		label := contracts.ClassificationLabel{
			EntityKind:    kind,
			EntityID:      entityID,
			Level:         level,
			AppliedBy:     actor,
			Reason:        strings.TrimSpace(reason),
			CreatedAt:     now,
			UpdatedAt:     now,
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if existing, err := s.Classifications.LoadClassificationLabel(ctx, kind, entityID); err == nil {
			label.ClassificationID = existing.ClassificationID
			label.CreatedAt = existing.CreatedAt
		}
		label = normalizeClassificationLabel(label)
		eventProject, eventTicket, err := s.classificationEventRoute(ctx, kind, entityID)
		if err != nil {
			return ClassificationDetailView{}, err
		}
		event, err := s.newEvent(ctx, eventProject, now, actor, reason, contracts.EventClassificationSet, eventTicket, label)
		if err != nil {
			return ClassificationDetailView{}, err
		}
		if err := s.commitMutation(ctx, "set classification", "classification_label", event, func(ctx context.Context) error {
			return s.Classifications.SaveClassificationLabel(ctx, label)
		}); err != nil {
			return ClassificationDetailView{}, err
		}
		return s.ClassificationDetail(ctx, entity)
	})
}

func (s *ActionService) ClassificationDetail(ctx context.Context, entity string) (ClassificationDetailView, error) {
	kind, entityID, err := s.resolveClassifiedEntity(ctx, entity)
	if err != nil {
		return ClassificationDetailView{}, err
	}
	level, steps, label, err := s.effectiveClassification(ctx, kind, entityID)
	if err != nil {
		return ClassificationDetailView{}, err
	}
	return ClassificationDetailView{
		Kind:          "classification_detail",
		GeneratedAt:   s.now(),
		EntityKind:    kind,
		EntityID:      entityID,
		Level:         level,
		Label:         label,
		Inheritance:   steps,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, nil
}

func (s *ActionService) ListClassifications(ctx context.Context, project string) (ClassificationListView, error) {
	items, err := s.Classifications.ListClassificationLabels(ctx)
	if err != nil {
		return ClassificationListView{}, err
	}
	if strings.TrimSpace(project) != "" {
		filtered := make([]contracts.ClassificationLabel, 0, len(items))
		for _, item := range items {
			ok, err := s.classificationLabelBelongsToProject(ctx, item, project)
			if err != nil {
				return ClassificationListView{}, err
			}
			if ok {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return ClassificationListView{Kind: "classification_list", GeneratedAt: s.now(), Items: items}, nil
}

func (s *ActionService) CreateRedactionPreview(ctx context.Context, scope string, target contracts.RedactionTarget, actor contracts.Actor, reason string) (RedactionPreviewDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create redaction preview", func(ctx context.Context) (RedactionPreviewDetailView, error) {
		if !actor.IsValid() {
			return RedactionPreviewDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return RedactionPreviewDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		preview, err := s.buildRedactionPreview(ctx, scope, target, actor)
		if err != nil {
			return RedactionPreviewDetailView{}, err
		}
		if err := s.RedactionPreviews.SaveRedactionPreview(ctx, preview); err != nil {
			return RedactionPreviewDetailView{}, err
		}
		return RedactionPreviewDetailView{Kind: "redaction_preview", GeneratedAt: s.now(), Preview: preview}, nil
	})
}

func (s *ActionService) CreateRedactedExport(ctx context.Context, scope string, previewID string, actor contracts.Actor, reason string) (RedactionExportResultView, error) {
	return withWriteLock(ctx, s.LockManager, "create redacted export", func(ctx context.Context) (RedactionExportResultView, error) {
		if !actor.IsValid() {
			return RedactionExportResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return RedactionExportResultView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		preview, err := s.validateRedactionPreview(ctx, scope, contracts.RedactionTargetExport, actor, previewID)
		if err != nil {
			return RedactionExportResultView{}, err
		}
		if err := validateRedactedExportActions(preview.Items); err != nil {
			return RedactionExportResultView{}, err
		}
		governanceInput := GovernanceEvaluationInput{
			Action: contracts.ProtectedActionExportCreate,
			Target: "workspace",
			Actor:  actor,
			Reason: reason,
		}
		governanceExplanation, err := s.requireGovernance(ctx, governanceInput)
		if err != nil {
			return RedactionExportResultView{}, err
		}
		files, err := collectExportFiles(s.Root)
		if err != nil {
			return RedactionExportResultView{}, err
		}
		included := redactedFileSet(files, preview.Items)
		bundleID := "bundle_" + NewOpaqueID()
		manifestPath := filepath.Join(storage.ExportsDir(s.Root), bundleID+".manifest.json")
		artifactPath := filepath.Join(storage.ExportsDir(s.Root), bundleID+".tar.gz")
		checksumPath := filepath.Join(storage.ExportsDir(s.Root), bundleID+".sha256")
		manifest, err := buildBundleManifest(s.Root, bundleID, normalizeRedactionScope(scope), s.now(), included)
		if err != nil {
			return RedactionExportResultView{}, err
		}
		manifest.RedactionPreviewID = preview.PreviewID
		if err := os.MkdirAll(storage.ExportsDir(s.Root), 0o755); err != nil {
			return RedactionExportResultView{}, fmt.Errorf("create exports dir: %w", err)
		}
		manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return RedactionExportResultView{}, fmt.Errorf("marshal manifest: %w", err)
		}
		if err := os.WriteFile(manifestPath, append(manifestRaw, '\n'), 0o644); err != nil {
			return RedactionExportResultView{}, fmt.Errorf("write manifest: %w", err)
		}
		if err := writeBundleArchive(s.Root, artifactPath, manifestRaw, included); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath)
			return RedactionExportResultView{}, err
		}
		archiveSHA, err := fileSHA256(artifactPath)
		if err != nil {
			cleanupExportSidecars(manifestPath, artifactPath)
			return RedactionExportResultView{}, err
		}
		if err := os.WriteFile(checksumPath, []byte(archiveSHA+"  "+filepath.Base(artifactPath)+"\n"), 0o644); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath)
			return RedactionExportResultView{}, fmt.Errorf("write checksum: %w", err)
		}
		bundle := contracts.ExportBundle{
			BundleID:           bundleID,
			Scope:              normalizeRedactionScope(scope),
			Format:             exportBundleFormatV1,
			ArtifactPath:       artifactPath,
			ManifestPath:       manifestPath,
			ChecksumPath:       checksumPath,
			RedactionPreviewID: preview.PreviewID,
			Status:             contracts.ExportBundleCreated,
			CreatedAt:          s.now(),
			SchemaVersion:      contracts.CurrentSchemaVersion,
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventRedactionExported, "", bundle)
		if err != nil {
			cleanupExportSidecars(manifestPath, artifactPath, checksumPath)
			return RedactionExportResultView{}, err
		}
		if err := s.commitMutation(ctx, "create redacted export", "export_bundle", event, func(ctx context.Context) error {
			return s.ExportBundles.SaveExportBundle(ctx, bundle)
		}); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath, checksumPath)
			return RedactionExportResultView{}, err
		}
		if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
			return RedactionExportResultView{}, err
		}
		return RedactionExportResultView{Kind: "redaction_export_result", GeneratedAt: s.now(), Bundle: bundle, Preview: preview, Included: len(included), Omitted: len(files) - len(included), SchemaVersion: contracts.CurrentSchemaVersion}, nil
	})
}

func (s *ActionService) VerifyRedactedArtifact(ctx context.Context, artifact string) (RedactionVerifyView, error) {
	bundle, err := s.resolveExportBundle(ctx, artifact)
	if err != nil {
		return RedactionVerifyView{}, err
	}
	view := RedactionVerifyView{Kind: "redaction_verify_result", GeneratedAt: s.now(), Artifact: artifact, RedactionPreviewID: bundle.RedactionPreviewID, Verified: true, SchemaVersion: contracts.CurrentSchemaVersion}
	if strings.TrimSpace(bundle.RedactionPreviewID) == "" {
		view.Verified = false
		view.Errors = append(view.Errors, "missing_redaction_preview_id")
		return view, nil
	}
	preview, err := s.RedactionPreviews.LoadRedactionPreview(ctx, bundle.RedactionPreviewID)
	if err != nil {
		view.Verified = false
		view.Errors = append(view.Errors, "preview_not_found")
		return view, nil
	}
	integrity, err := verifyBundle(bundle)
	if err != nil {
		return view, err
	}
	if !integrity.Verified {
		view.Verified = false
		view.Errors = append(view.Errors, integrity.Errors...)
	}
	if redactionErrors := verifyRedactedBundleAgainstPreview(bundle, preview); len(redactionErrors) > 0 {
		view.Verified = false
		view.Errors = append(view.Errors, redactionErrors...)
	}
	return view, nil
}

func (s *ActionService) buildRedactionPreview(ctx context.Context, scope string, target contracts.RedactionTarget, actor contracts.Actor) (contracts.RedactionPreview, error) {
	if !target.IsValid() {
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid redaction target: %s", target))
	}
	scope = normalizeRedactionScope(scope)
	if scope != "workspace" {
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported redaction scope: %s", scope))
	}
	rules, err := s.RedactionRules.ListRedactionRules(ctx)
	if err != nil {
		return contracts.RedactionPreview{}, err
	}
	files, err := collectExportFiles(s.Root)
	if err != nil {
		return contracts.RedactionPreview{}, err
	}
	items, err := s.redactionResultsForFiles(ctx, target, files, rules)
	if err != nil {
		return contracts.RedactionPreview{}, err
	}
	classificationHash, err := s.classificationVersionHash(ctx)
	if err != nil {
		return contracts.RedactionPreview{}, err
	}
	now := s.now()
	preview := contracts.RedactionPreview{
		PreviewID:          "redact-" + NewOpaqueID(),
		Scope:              scope,
		Target:             target,
		Actor:              actor,
		PolicyVersionHash:  hashJSON(rules),
		ClassificationHash: classificationHash,
		SourceStateHash:    sourceStateHash(s.Root, files),
		CommandTarget:      string(target) + ":" + scope,
		CreatedAt:          now,
		ExpiresAt:          now.Add(defaultRedactionPreviewTTL),
		Items:              items,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}
	return normalizeRedactionPreview(preview), nil
}

func (s *ActionService) validateRedactionPreview(ctx context.Context, scope string, target contracts.RedactionTarget, actor contracts.Actor, previewID string) (contracts.RedactionPreview, error) {
	preview, err := s.RedactionPreviews.LoadRedactionPreview(ctx, previewID)
	if err != nil {
		return contracts.RedactionPreview{}, err
	}
	if preview.Actor != actor {
		return contracts.RedactionPreview{}, apperr.New(apperr.CodePermissionDenied, "redaction preview actor mismatch")
	}
	if preview.Target != target || preview.Scope != normalizeRedactionScope(scope) || preview.CommandTarget != string(target)+":"+normalizeRedactionScope(scope) {
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview target mismatch")
	}
	if !s.now().Before(preview.ExpiresAt) {
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview expired")
	}
	if used, err := s.redactionPreviewUsed(ctx, preview.PreviewID); err != nil {
		return contracts.RedactionPreview{}, err
	} else if used {
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview already used")
	}
	current, err := s.buildRedactionPreview(ctx, scope, target, actor)
	if err != nil {
		return contracts.RedactionPreview{}, err
	}
	switch {
	case current.PolicyVersionHash != preview.PolicyVersionHash:
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview policy hash mismatch")
	case current.ClassificationHash != preview.ClassificationHash:
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview classification hash mismatch")
	case current.SourceStateHash != preview.SourceStateHash:
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview source hash mismatch")
	case hashJSON(current.Items) != hashJSON(preview.Items):
		return contracts.RedactionPreview{}, apperr.New(apperr.CodeConflict, "redaction preview items mismatch")
	}
	preview.Items = current.Items
	return preview, nil
}

func (s *ActionService) redactionPreviewUsed(ctx context.Context, previewID string) (bool, error) {
	bundles, err := s.ExportBundles.ListExportBundles(ctx)
	if err != nil {
		return false, err
	}
	for _, bundle := range bundles {
		if bundle.RedactionPreviewID == previewID {
			return true, nil
		}
	}
	return false, nil
}

func verifyRedactedBundleAgainstPreview(bundle contracts.ExportBundle, preview contracts.RedactionPreview) []string {
	errors := []string{}
	if preview.Target != contracts.RedactionTargetExport {
		errors = append(errors, "redaction_preview_target_mismatch")
	}
	if preview.Scope != "" && bundle.Scope != "" && preview.Scope != bundle.Scope {
		errors = append(errors, "redaction_preview_scope_mismatch")
	}
	manifest, err := loadBundleManifest(bundle.ManifestPath)
	if err != nil {
		return append(errors, "redaction_manifest_unreadable")
	}
	if manifest.RedactionPreviewID == "" {
		errors = append(errors, "missing_manifest_redaction_preview_id")
	} else if manifest.RedactionPreviewID != preview.PreviewID {
		errors = append(errors, "redaction_preview_binding_mismatch")
	}
	manifestFiles := map[string]struct{}{}
	for _, file := range manifest.Files {
		manifestFiles[filepath.ToSlash(file.Path)] = struct{}{}
	}
	archiveFiles, err := readBundleArchive(bundle.ArtifactPath)
	if err != nil {
		return append(errors, "redaction_archive_unreadable")
	}
	for _, item := range preview.Items {
		if item.Action != contracts.RedactionOmit {
			continue
		}
		path := filepath.ToSlash(item.FieldPath)
		if _, ok := manifestFiles[path]; ok {
			errors = append(errors, "redaction_omitted_file_present:"+path)
			continue
		}
		if _, ok := archiveFiles[path]; ok {
			errors = append(errors, "redaction_omitted_file_present:"+path)
		}
	}
	return errors
}

func (s *ActionService) redactionResultsForFiles(ctx context.Context, target contracts.RedactionTarget, files []string, rules []contracts.RedactionRule) ([]contracts.RedactionResult, error) {
	out := make([]contracts.RedactionResult, 0)
	for _, rel := range files {
		if target == contracts.RedactionTargetExport && strings.HasPrefix(filepath.ToSlash(rel), ".tracker/events/") {
			out = append(out, contracts.RedactionResult{EntityKind: contracts.ClassifiedEntityWorkspace, EntityID: "workspace", FieldPath: rel, Level: contracts.ClassificationInternal, Action: contracts.RedactionOmit, ReasonCodes: []string{"event_history_payload_snapshots"}})
			continue
		}
		kind, entityID, err := s.classifiedEntityForExportPath(ctx, rel)
		if err != nil {
			return nil, err
		}
		level, _, _, err := s.effectiveClassification(ctx, kind, entityID)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			if rule.Target != target || (rule.EntityKind != "" && rule.EntityKind != kind) {
				continue
			}
			if !classificationAtLeast(level, rule.MinLevel) {
				continue
			}
			out = append(out, contracts.RedactionResult{EntityKind: kind, EntityID: entityID, FieldPath: rel, Level: level, Action: rule.Action, ReasonCodes: []string{rule.RuleID}})
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EntityKind != out[j].EntityKind {
			return out[i].EntityKind < out[j].EntityKind
		}
		if out[i].EntityID != out[j].EntityID {
			return out[i].EntityID < out[j].EntityID
		}
		return out[i].FieldPath < out[j].FieldPath
	})
	return out, nil
}

func validateRedactedExportActions(items []contracts.RedactionResult) error {
	for _, item := range items {
		if item.Action != contracts.RedactionOmit {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("redacted export only supports omit actions, got %s for %s", item.Action, item.FieldPath))
		}
	}
	return nil
}

func (s *ActionService) effectiveClassification(ctx context.Context, kind contracts.ClassifiedEntityKind, entityID string) (contracts.ClassificationLevel, []ClassificationStep, *contracts.ClassificationLabel, error) {
	level := contracts.ClassificationInternal
	steps := []ClassificationStep{{EntityKind: contracts.ClassifiedEntityWorkspace, EntityID: "workspace", Level: level, Explicit: false, ReasonCodes: []string{"default_internal"}}}
	addExplicit := func(k contracts.ClassifiedEntityKind, id string) (*contracts.ClassificationLabel, bool, error) {
		label, err := s.Classifications.LoadClassificationLabel(ctx, k, id)
		if err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
				return nil, false, nil
			}
			return nil, false, err
		}
		level = contracts.HigherClassification(level, label.Level)
		steps = append(steps, ClassificationStep{EntityKind: k, EntityID: id, Level: label.Level, Explicit: true, ReasonCodes: []string{"explicit_label"}})
		return &label, true, nil
	}
	label, _, err := addExplicit(contracts.ClassifiedEntityWorkspace, "workspace")
	if err != nil {
		return "", nil, nil, err
	}
	entityLabel := label
	switch kind {
	case contracts.ClassifiedEntityWorkspace:
		return level, steps, entityLabel, nil
	case contracts.ClassifiedEntityProject:
		if current, ok, err := addExplicit(kind, entityID); err != nil {
			return "", nil, nil, err
		} else if ok {
			entityLabel = current
		}
	case contracts.ClassifiedEntityTicket:
		ticket, err := s.Tickets.GetTicket(ctx, entityID)
		if err != nil {
			return "", nil, nil, err
		}
		if _, _, err := addExplicit(contracts.ClassifiedEntityProject, ticket.Project); err != nil {
			return "", nil, nil, err
		}
		if ticket.Protected || ticket.Sensitive {
			level = contracts.HigherClassification(level, contracts.ClassificationRestricted)
			steps = append(steps, ClassificationStep{EntityKind: contracts.ClassifiedEntityTicket, EntityID: ticket.ID, Level: contracts.ClassificationRestricted, Explicit: false, ReasonCodes: []string{"legacy_sensitive_or_protected"}})
		}
		if current, ok, err := addExplicit(kind, entityID); err != nil {
			return "", nil, nil, err
		} else if ok {
			entityLabel = current
		}
	case contracts.ClassifiedEntityRun:
		run, err := s.Runs.LoadRun(ctx, entityID)
		if err != nil {
			return "", nil, nil, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
		if err != nil {
			return "", nil, nil, err
		}
		if _, _, err := addExplicit(contracts.ClassifiedEntityProject, ticket.Project); err != nil {
			return "", nil, nil, err
		}
		ticketLevel, ticketSteps, _, err := s.effectiveClassification(ctx, contracts.ClassifiedEntityTicket, ticket.ID)
		if err != nil {
			return "", nil, nil, err
		}
		level = contracts.HigherClassification(level, ticketLevel)
		steps = append(steps, ticketSteps...)
		if current, ok, err := addExplicit(kind, entityID); err != nil {
			return "", nil, nil, err
		} else if ok {
			entityLabel = current
		}
	case contracts.ClassifiedEntityEvidence:
		evidence, err := s.Evidence.LoadEvidence(ctx, entityID)
		if err != nil {
			return "", nil, nil, err
		}
		runLevel, runSteps, _, err := s.effectiveClassification(ctx, contracts.ClassifiedEntityRun, evidence.RunID)
		if err != nil {
			return "", nil, nil, err
		}
		level = contracts.HigherClassification(level, runLevel)
		steps = append(steps, runSteps...)
		if current, ok, err := addExplicit(kind, entityID); err != nil {
			return "", nil, nil, err
		} else if ok {
			entityLabel = current
		}
	case contracts.ClassifiedEntityHandoff:
		handoff, err := s.Handoffs.LoadHandoff(ctx, entityID)
		if err != nil {
			return "", nil, nil, err
		}
		runLevel, runSteps, _, err := s.effectiveClassification(ctx, contracts.ClassifiedEntityRun, handoff.SourceRunID)
		if err != nil {
			return "", nil, nil, err
		}
		level = contracts.HigherClassification(level, runLevel)
		steps = append(steps, runSteps...)
		if current, ok, err := addExplicit(kind, entityID); err != nil {
			return "", nil, nil, err
		} else if ok {
			entityLabel = current
		}
	default:
		if current, ok, err := addExplicit(kind, entityID); err != nil {
			return "", nil, nil, err
		} else if ok {
			entityLabel = current
		}
	}
	return level, compactClassificationSteps(steps), entityLabel, nil
}

func (s *ActionService) classificationLabelBelongsToProject(ctx context.Context, label contracts.ClassificationLabel, project string) (bool, error) {
	project = strings.TrimSpace(project)
	switch label.EntityKind {
	case contracts.ClassifiedEntityProject:
		return label.EntityID == project, nil
	case contracts.ClassifiedEntityTicket:
		ticket, err := s.Tickets.GetTicket(ctx, label.EntityID)
		if err != nil {
			return false, nil
		}
		return ticket.Project == project, nil
	case contracts.ClassifiedEntityRun:
		run, err := s.Runs.LoadRun(ctx, label.EntityID)
		if err != nil {
			return false, nil
		}
		return run.Project == project, nil
	case contracts.ClassifiedEntityEvidence:
		evidence, err := s.Evidence.LoadEvidence(ctx, label.EntityID)
		if err != nil {
			return false, nil
		}
		return s.ticketBelongsToProject(ctx, evidence.TicketID, project), nil
	case contracts.ClassifiedEntityHandoff:
		handoff, err := s.Handoffs.LoadHandoff(ctx, label.EntityID)
		if err != nil {
			return false, nil
		}
		return s.ticketBelongsToProject(ctx, handoff.TicketID, project), nil
	default:
		return false, nil
	}
}

func (s *ActionService) ticketBelongsToProject(ctx context.Context, ticketID string, project string) bool {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	return err == nil && ticket.Project == project
}

func (s *ActionService) resolveClassifiedEntity(ctx context.Context, raw string) (contracts.ClassifiedEntityKind, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "workspace" {
		return contracts.ClassifiedEntityWorkspace, "workspace", nil
	}
	kindRaw, entityID, ok := strings.Cut(raw, ":")
	if !ok {
		return "", "", apperr.New(apperr.CodeInvalidInput, "classification entity must be workspace or kind:id")
	}
	kind := contracts.ClassifiedEntityKind(strings.TrimSpace(kindRaw))
	entityID = strings.TrimSpace(entityID)
	if !kind.IsValid() || entityID == "" {
		return "", "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid classification entity: %s", raw))
	}
	switch kind {
	case contracts.ClassifiedEntityProject:
		if _, err := s.Projects.GetProject(ctx, entityID); err != nil {
			return "", "", err
		}
	case contracts.ClassifiedEntityTicket:
		if _, err := s.Tickets.GetTicket(ctx, entityID); err != nil {
			return "", "", err
		}
	case contracts.ClassifiedEntityRun:
		if _, err := s.Runs.LoadRun(ctx, entityID); err != nil {
			return "", "", err
		}
	case contracts.ClassifiedEntityEvidence:
		if _, err := s.Evidence.LoadEvidence(ctx, entityID); err != nil {
			return "", "", err
		}
	case contracts.ClassifiedEntityHandoff:
		if _, err := s.Handoffs.LoadHandoff(ctx, entityID); err != nil {
			return "", "", err
		}
	}
	return kind, entityID, nil
}

func compactClassificationSteps(steps []ClassificationStep) []ClassificationStep {
	seen := map[string]struct{}{}
	out := make([]ClassificationStep, 0, len(steps))
	for _, step := range steps {
		key := string(step.EntityKind) + ":" + step.EntityID + ":" + string(step.Level) + ":" + fmt.Sprint(step.Explicit)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, step)
	}
	return out
}

func (s *ActionService) classifiedEntityForExportPath(ctx context.Context, rel string) (contracts.ClassifiedEntityKind, string, error) {
	kind, entityID, ok := classifiedEntityForPath(rel)
	if ok {
		return kind, entityID, nil
	}
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) == 4 && parts[0] == ".tracker" && parts[1] == "classification" && parts[2] == "labels" && strings.HasSuffix(parts[3], ".md"):
		raw, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil {
			return "", "", err
		}
		fmRaw, _, err := splitDocument(string(raw))
		if err != nil {
			return "", "", err
		}
		var fm classificationLabelFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			return "", "", fmt.Errorf("parse classification label %s: %w", rel, err)
		}
		label := normalizeClassificationLabel(fm.ClassificationLabel)
		return label.EntityKind, label.EntityID, nil
	case len(parts) == 3 && parts[0] == ".tracker" && parts[1] == "gates" && strings.HasSuffix(parts[2], ".md"):
		gate, err := s.Gates.LoadGate(ctx, strings.TrimSuffix(parts[2], ".md"))
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(gate.RunID) != "" {
			return contracts.ClassifiedEntityRun, gate.RunID, nil
		}
		return contracts.ClassifiedEntityTicket, gate.TicketID, nil
	case len(parts) == 3 && parts[0] == ".tracker" && parts[1] == "changes" && strings.HasSuffix(parts[2], ".md"):
		change, err := s.Changes.LoadChange(ctx, strings.TrimSuffix(parts[2], ".md"))
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(change.RunID) != "" {
			return contracts.ClassifiedEntityRun, change.RunID, nil
		}
		return contracts.ClassifiedEntityTicket, change.TicketID, nil
	case len(parts) == 3 && parts[0] == ".tracker" && parts[1] == "checks" && strings.HasSuffix(parts[2], ".md"):
		check, err := s.Checks.LoadCheck(ctx, strings.TrimSuffix(parts[2], ".md"))
		if err != nil {
			return "", "", err
		}
		return s.classifiedEntityForCheck(ctx, check)
	default:
		return contracts.ClassifiedEntityWorkspace, "workspace", nil
	}
}

func (s *ActionService) classifiedEntityForCheck(ctx context.Context, check contracts.CheckResult) (contracts.ClassifiedEntityKind, string, error) {
	switch check.Scope {
	case contracts.CheckScopeTicket:
		return contracts.ClassifiedEntityTicket, check.ScopeID, nil
	case contracts.CheckScopeRun:
		return contracts.ClassifiedEntityRun, check.ScopeID, nil
	case contracts.CheckScopeChange:
		change, err := s.Changes.LoadChange(ctx, check.ScopeID)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(change.RunID) != "" {
			return contracts.ClassifiedEntityRun, change.RunID, nil
		}
		return contracts.ClassifiedEntityTicket, change.TicketID, nil
	default:
		return contracts.ClassifiedEntityWorkspace, "workspace", nil
	}
}

func classifiedEntityForPath(rel string) (contracts.ClassifiedEntityKind, string, bool) {
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) == 3 && parts[0] == "projects" && parts[2] == "project.md":
		return contracts.ClassifiedEntityProject, parts[1], true
	case len(parts) == 4 && parts[0] == "projects" && parts[2] == "tickets" && strings.HasSuffix(parts[3], ".md"):
		return contracts.ClassifiedEntityTicket, strings.TrimSuffix(parts[3], ".md"), true
	case len(parts) >= 2 && parts[0] == "projects" && strings.TrimSpace(parts[1]) != "":
		return contracts.ClassifiedEntityProject, parts[1], true
	case len(parts) == 3 && parts[0] == ".tracker" && parts[1] == "runs" && strings.HasSuffix(parts[2], ".md"):
		return contracts.ClassifiedEntityRun, strings.TrimSuffix(parts[2], ".md"), true
	case len(parts) == 4 && parts[0] == ".tracker" && parts[1] == "evidence" && strings.HasSuffix(parts[3], ".md"):
		return contracts.ClassifiedEntityEvidence, strings.TrimSuffix(parts[3], ".md"), true
	case len(parts) == 3 && parts[0] == ".tracker" && parts[1] == "handoffs" && strings.HasSuffix(parts[2], ".md"):
		return contracts.ClassifiedEntityHandoff, strings.TrimSuffix(parts[2], ".md"), true
	default:
		return "", "", false
	}
}

func redactedFileSet(files []string, items []contracts.RedactionResult) []string {
	omitted := map[string]struct{}{}
	for _, item := range items {
		if item.Action == contracts.RedactionOmit && strings.TrimSpace(item.FieldPath) != "" {
			omitted[filepath.ToSlash(item.FieldPath)] = struct{}{}
		}
	}
	out := make([]string, 0, len(files))
	for _, file := range files {
		if _, ok := omitted[filepath.ToSlash(file)]; !ok {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

func normalizeRedactionScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "workspace"
	}
	return scope
}

func hashJSON(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *ActionService) classificationVersionHash(ctx context.Context) (string, error) {
	labels, err := s.Classifications.ListClassificationLabels(ctx)
	if err != nil {
		return "", err
	}
	return hashJSON(labels), nil
}

func sourceStateHash(root string, files []string) string {
	hash := sha256.New()
	files = append([]string{}, files...)
	sort.Strings(files)
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		fileSum := sha256.Sum256(raw)
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		hash.Write([]byte(hex.EncodeToString(fileSum[:])))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func classificationAtLeast(actual contracts.ClassificationLevel, minimum contracts.ClassificationLevel) bool {
	return contracts.HigherClassification(actual, minimum) == actual
}

func (s *ActionService) classificationEventRoute(ctx context.Context, kind contracts.ClassifiedEntityKind, entityID string) (string, string, error) {
	switch kind {
	case contracts.ClassifiedEntityProject:
		return entityID, "", nil
	case contracts.ClassifiedEntityTicket:
		ticket, err := s.Tickets.GetTicket(ctx, entityID)
		if err != nil {
			return "", "", err
		}
		return ticket.Project, ticket.ID, nil
	case contracts.ClassifiedEntityRun:
		run, err := s.Runs.LoadRun(ctx, entityID)
		if err != nil {
			return "", "", err
		}
		return run.Project, run.TicketID, nil
	case contracts.ClassifiedEntityEvidence:
		evidence, err := s.Evidence.LoadEvidence(ctx, entityID)
		if err != nil {
			return "", "", err
		}
		ticket, err := s.Tickets.GetTicket(ctx, evidence.TicketID)
		if err != nil {
			return "", "", err
		}
		return ticket.Project, evidence.TicketID, nil
	case contracts.ClassifiedEntityHandoff:
		handoff, err := s.Handoffs.LoadHandoff(ctx, entityID)
		if err != nil {
			return "", "", err
		}
		ticket, err := s.Tickets.GetTicket(ctx, handoff.TicketID)
		if err != nil {
			return "", "", err
		}
		return ticket.Project, handoff.TicketID, nil
	default:
		return workspaceEventProject, "", nil
	}
}
