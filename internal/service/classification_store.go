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

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type ClassificationLabelStore struct {
	Root string
}

type RedactionRuleStore struct {
	Root string
}

type RedactionPreviewStore struct {
	Root string
}

type classificationLabelFrontmatter struct {
	contracts.ClassificationLabel `yaml:",inline"`
}

func (s ClassificationLabelStore) SaveClassificationLabel(_ context.Context, label contracts.ClassificationLabel) error {
	label = normalizeClassificationLabel(label)
	if err := label.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ClassificationLabelsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create classification labels dir: %w", err)
	}
	raw, err := yaml.Marshal(classificationLabelFrontmatter{ClassificationLabel: label})
	if err != nil {
		return fmt.Errorf("encode classification label %s: %w", label.ClassificationID, err)
	}
	body := fmt.Sprintf("Classification `%s` for `%s:%s`.", label.Level, label.EntityKind, label.EntityID)
	path := classificationLabelPath(s.Root, label.EntityKind, label.EntityID)
	if err := os.WriteFile(path, []byte(fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)), 0o644); err != nil {
		return err
	}
	legacyPath := legacyClassificationLabelPath(s.Root, label.EntityKind, label.EntityID)
	if legacyPath != path {
		if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove legacy classification label %s: %w", label.ClassificationID, err)
		}
	}
	return nil
}

func (s ClassificationLabelStore) LoadClassificationLabel(_ context.Context, kind contracts.ClassifiedEntityKind, entityID string) (contracts.ClassificationLabel, error) {
	path := classificationLabelPath(s.Root, kind, entityID)
	raw, err := os.ReadFile(path)
	if err != nil {
		legacyPath := legacyClassificationLabelPath(s.Root, kind, entityID)
		if !os.IsNotExist(err) || legacyPath == path {
			return contracts.ClassificationLabel{}, fmt.Errorf("read classification label %s:%s: %w", kind, entityID, err)
		}
		raw, err = os.ReadFile(legacyPath)
		if err != nil {
			return contracts.ClassificationLabel{}, fmt.Errorf("read classification label %s:%s: %w", kind, entityID, err)
		}
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.ClassificationLabel{}, err
	}
	var fm classificationLabelFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.ClassificationLabel{}, fmt.Errorf("parse classification label %s:%s: %w", kind, entityID, err)
	}
	label := normalizeClassificationLabel(fm.ClassificationLabel)
	if label.EntityKind != kind || label.EntityID != strings.TrimSpace(entityID) {
		return contracts.ClassificationLabel{}, fmt.Errorf("classification label identity mismatch %s:%s: %w", kind, entityID, os.ErrNotExist)
	}
	return label, label.Validate()
}

func (s ClassificationLabelStore) ListClassificationLabels(_ context.Context) ([]contracts.ClassificationLabel, error) {
	entries, err := os.ReadDir(storage.ClassificationLabelsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ClassificationLabel{}, nil
		}
		return nil, fmt.Errorf("read classification labels dir: %w", err)
	}
	items := make([]contracts.ClassificationLabel, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(storage.ClassificationLabelsDir(s.Root), entry.Name()))
		if err != nil {
			return nil, err
		}
		fmRaw, _, err := splitDocument(string(raw))
		if err != nil {
			return nil, fmt.Errorf("parse classification label %s: %w", entry.Name(), err)
		}
		var fm classificationLabelFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			return nil, fmt.Errorf("parse classification label %s: %w", entry.Name(), err)
		}
		label := normalizeClassificationLabel(fm.ClassificationLabel)
		if err := label.Validate(); err != nil {
			return nil, fmt.Errorf("classification label %s: %w", entry.Name(), err)
		}
		items = append(items, label)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EntityKind != items[j].EntityKind {
			return items[i].EntityKind < items[j].EntityKind
		}
		return items[i].EntityID < items[j].EntityID
	})
	return items, nil
}

func (s RedactionRuleStore) SaveRedactionRule(_ context.Context, rule contracts.RedactionRule) error {
	rule = normalizeRedactionRule(rule)
	if err := rule.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.RedactionRulesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create redaction rules dir: %w", err)
	}
	raw, err := toml.Marshal(rule)
	if err != nil {
		return fmt.Errorf("encode redaction rule %s: %w", rule.RuleID, err)
	}
	return os.WriteFile(redactionRulePath(s.Root, rule.RuleID), raw, 0o644)
}

func (s RedactionRuleStore) ListRedactionRules(_ context.Context) ([]contracts.RedactionRule, error) {
	entries, err := os.ReadDir(storage.RedactionRulesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return defaultRedactionRules(), nil
		}
		return nil, fmt.Errorf("read redaction rules dir: %w", err)
	}
	items := make([]contracts.RedactionRule, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(storage.RedactionRulesDir(s.Root), entry.Name()))
		if err != nil {
			return nil, err
		}
		var rule contracts.RedactionRule
		if err := toml.Unmarshal(raw, &rule); err != nil {
			return nil, fmt.Errorf("parse redaction rule %s: %w", entry.Name(), err)
		}
		rule = normalizeRedactionRule(rule)
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("redaction rule %s: %w", entry.Name(), err)
		}
		items = append(items, rule)
	}
	if len(items) == 0 {
		items = defaultRedactionRules()
	} else {
		items = withMissingDefaultRedactionRules(items)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RuleID < items[j].RuleID })
	return items, nil
}

func (s RedactionPreviewStore) SaveRedactionPreview(_ context.Context, preview contracts.RedactionPreview) error {
	preview = normalizeRedactionPreview(preview)
	if err := preview.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.RedactionPreviewsDir(s.Root), 0o700); err != nil {
		return fmt.Errorf("create redaction previews dir: %w", err)
	}
	raw, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return fmt.Errorf("encode redaction preview %s: %w", preview.PreviewID, err)
	}
	return os.WriteFile(redactionPreviewPath(s.Root, preview.PreviewID), append(raw, '\n'), 0o600)
}

func (s RedactionPreviewStore) LoadRedactionPreview(_ context.Context, previewID string) (contracts.RedactionPreview, error) {
	previewID = sanitizeSecurityID(previewID)
	raw, err := os.ReadFile(redactionPreviewPath(s.Root, previewID))
	if err != nil {
		return contracts.RedactionPreview{}, fmt.Errorf("read redaction preview %s: %w", previewID, err)
	}
	var preview contracts.RedactionPreview
	if err := json.Unmarshal(raw, &preview); err != nil {
		return contracts.RedactionPreview{}, fmt.Errorf("parse redaction preview %s: %w", previewID, err)
	}
	preview = normalizeRedactionPreview(preview)
	return preview, preview.Validate()
}

func normalizeClassificationLabel(label contracts.ClassificationLabel) contracts.ClassificationLabel {
	label.EntityID = strings.TrimSpace(label.EntityID)
	if label.EntityKind == "" {
		label.EntityKind = contracts.ClassifiedEntityWorkspace
	}
	if label.EntityID == "" && label.EntityKind == contracts.ClassifiedEntityWorkspace {
		label.EntityID = "workspace"
	}
	label.ClassificationID = classificationLabelID(label.EntityKind, label.EntityID)
	label.Reason = strings.TrimSpace(label.Reason)
	if label.CreatedAt.IsZero() {
		label.CreatedAt = timeNowUTC()
	}
	if label.UpdatedAt.IsZero() {
		label.UpdatedAt = label.CreatedAt
	}
	if label.SchemaVersion == 0 {
		label.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return label
}

func normalizeRedactionRule(rule contracts.RedactionRule) contracts.RedactionRule {
	rule.RuleID = sanitizeGovernanceID(firstNonEmpty(rule.RuleID, string(rule.Target)+"-"+string(rule.MinLevel)+"-"+string(rule.Action)), "redaction-rule")
	rule.FieldPath = strings.TrimSpace(rule.FieldPath)
	if rule.FieldPath == "" {
		rule.FieldPath = "*"
	}
	rule.Marker = strings.TrimSpace(rule.Marker)
	rule.Reason = strings.TrimSpace(rule.Reason)
	if rule.SchemaVersion == 0 {
		rule.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return rule
}

func normalizeRedactionPreview(preview contracts.RedactionPreview) contracts.RedactionPreview {
	preview.PreviewID = sanitizeGovernanceID(preview.PreviewID, "redact")
	preview.Scope = strings.TrimSpace(preview.Scope)
	preview.CommandTarget = strings.TrimSpace(preview.CommandTarget)
	if preview.SchemaVersion == 0 {
		preview.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return preview
}

func defaultRedactionRules() []contracts.RedactionRule {
	return []contracts.RedactionRule{{
		RuleID:        "default-omit-restricted-export",
		Target:        contracts.RedactionTargetExport,
		FieldPath:     "*",
		MinLevel:      contracts.ClassificationRestricted,
		Action:        contracts.RedactionOmit,
		Reason:        "restricted records are excluded from redacted exports by default",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, {
		RuleID:        "default-omit-restricted-sync",
		Target:        contracts.RedactionTargetSync,
		FieldPath:     "*",
		MinLevel:      contracts.ClassificationRestricted,
		Action:        contracts.RedactionOmit,
		Reason:        "restricted records are excluded from redacted sync by default",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, {
		RuleID:        "default-marker-restricted-goal",
		Target:        contracts.RedactionTargetGoal,
		FieldPath:     "*",
		MinLevel:      contracts.ClassificationRestricted,
		Action:        contracts.RedactionReplaceWithMarker,
		Marker:        "[redacted: restricted]",
		Reason:        "restricted records are marked in goal output by default",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}}
}

func withMissingDefaultRedactionRules(items []contracts.RedactionRule) []contracts.RedactionRule {
	coveredTargets := map[contracts.RedactionTarget]struct{}{}
	for _, item := range items {
		coveredTargets[item.Target] = struct{}{}
	}
	out := append([]contracts.RedactionRule{}, items...)
	for _, rule := range defaultRedactionRules() {
		if _, ok := coveredTargets[rule.Target]; !ok {
			out = append(out, rule)
		}
	}
	return out
}

func classificationLabelID(kind contracts.ClassifiedEntityKind, entityID string) string {
	entityID = strings.TrimSpace(entityID)
	sum := sha256.Sum256([]byte(string(kind) + "\x00" + entityID))
	slug := sanitizeGovernanceID(string(kind)+"-"+entityID, "workspace")
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-.")
	}
	return "class-" + slug + "-" + hex.EncodeToString(sum[:])[:12]
}

func classificationLabelPath(root string, kind contracts.ClassifiedEntityKind, entityID string) string {
	return filepath.Join(storage.ClassificationLabelsDir(root), classificationLabelID(kind, entityID)+".md")
}

func legacyClassificationLabelPath(root string, kind contracts.ClassifiedEntityKind, entityID string) string {
	id := "class-" + sanitizeGovernanceID(string(kind)+"-"+strings.TrimSpace(entityID), "workspace")
	return filepath.Join(storage.ClassificationLabelsDir(root), id+".md")
}

func redactionRulePath(root string, ruleID string) string {
	return filepath.Join(storage.RedactionRulesDir(root), sanitizeGovernanceID(ruleID, "redaction-rule")+".toml")
}

func redactionPreviewPath(root string, previewID string) string {
	return filepath.Join(storage.RedactionPreviewsDir(root), sanitizeGovernanceID(previewID, "redact")+".json")
}
