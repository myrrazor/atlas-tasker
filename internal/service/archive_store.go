package service

import (
	"context"
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

type RetentionPolicyStore struct {
	Root string
}

type ArchiveRecordStore struct {
	Root string
}

type archiveRecordFrontmatter struct {
	contracts.ArchiveRecord `yaml:",inline"`
}

var builtinRetentionPolicies = map[string]contracts.RetentionPolicy{
	"runtime-default": {
		PolicyID:               "runtime-default",
		Target:                 contracts.RetentionTargetRuntime,
		MaxAgeDays:             7,
		ArchiveInsteadOfDelete: true,
		RequiresConfirmation:   true,
		SchemaVersion:          contracts.CurrentSchemaVersion,
	},
	"evidence-artifacts-default": {
		PolicyID:               "evidence-artifacts-default",
		Target:                 contracts.RetentionTargetEvidenceArtifacts,
		MaxAgeDays:             21,
		ArchiveInsteadOfDelete: true,
		RequiresConfirmation:   true,
		SchemaVersion:          contracts.CurrentSchemaVersion,
	},
	"export-bundles-default": {
		PolicyID:               "export-bundles-default",
		Target:                 contracts.RetentionTargetExportBundles,
		MaxAgeDays:             14,
		ArchiveInsteadOfDelete: true,
		RequiresConfirmation:   true,
		SchemaVersion:          contracts.CurrentSchemaVersion,
	},
	"logs-default": {
		PolicyID:               "logs-default",
		Target:                 contracts.RetentionTargetLogs,
		MaxAgeDays:             14,
		ArchiveInsteadOfDelete: true,
		RequiresConfirmation:   true,
		SchemaVersion:          contracts.CurrentSchemaVersion,
	},
}

func (s RetentionPolicyStore) SaveRetentionPolicy(_ context.Context, policy contracts.RetentionPolicy) error {
	policy.PolicyID = sanitizeRetentionPolicyID(policy.PolicyID)
	if policy.SchemaVersion == 0 {
		policy.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.RetentionPoliciesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create retention dir: %w", err)
	}
	raw, err := toml.Marshal(policy)
	if err != nil {
		return fmt.Errorf("encode retention policy %s: %w", policy.PolicyID, err)
	}
	if err := os.WriteFile(storage.RetentionPolicyFile(s.Root, policy.PolicyID), raw, 0o644); err != nil {
		return fmt.Errorf("write retention policy %s: %w", policy.PolicyID, err)
	}
	return nil
}

func (s RetentionPolicyStore) LoadRetentionPolicy(_ context.Context, policyID string) (contracts.RetentionPolicy, error) {
	policyID = sanitizeRetentionPolicyID(policyID)
	if policy, ok := builtinRetentionPolicies[policyID]; ok {
		return policy, nil
	}
	raw, err := os.ReadFile(storage.RetentionPolicyFile(s.Root, policyID))
	if err != nil {
		return contracts.RetentionPolicy{}, fmt.Errorf("read retention policy %s: %w", policyID, err)
	}
	var policy contracts.RetentionPolicy
	if err := toml.Unmarshal(raw, &policy); err != nil {
		return contracts.RetentionPolicy{}, fmt.Errorf("parse retention policy %s: %w", policyID, err)
	}
	if strings.TrimSpace(policy.PolicyID) == "" {
		policy.PolicyID = policyID
	}
	if policy.SchemaVersion == 0 {
		policy.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return policy, policy.Validate()
}

func (s RetentionPolicyStore) ListRetentionPolicies(_ context.Context) ([]contracts.RetentionPolicy, error) {
	items := make(map[string]contracts.RetentionPolicy, len(builtinRetentionPolicies))
	for id, policy := range builtinRetentionPolicies {
		items[id] = policy
	}
	entries, err := os.ReadDir(storage.RetentionPoliciesDir(s.Root))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read retention dir: %w", err)
		}
	} else {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
				continue
			}
			policy, err := s.LoadRetentionPolicy(context.Background(), strings.TrimSuffix(entry.Name(), ".toml"))
			if err != nil {
				return nil, err
			}
			items[policy.PolicyID] = policy
		}
	}
	list := make([]contracts.RetentionPolicy, 0, len(items))
	for _, policy := range items {
		list = append(list, policy)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Target != list[j].Target {
			return list[i].Target < list[j].Target
		}
		return list[i].PolicyID < list[j].PolicyID
	})
	return list, nil
}

func (s ArchiveRecordStore) SaveArchiveRecord(_ context.Context, record contracts.ArchiveRecord) error {
	record = normalizeArchiveRecord(record)
	if err := record.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ArchivesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create archives dir: %w", err)
	}
	raw, err := yaml.Marshal(archiveRecordFrontmatter{ArchiveRecord: record})
	if err != nil {
		return fmt.Errorf("marshal archive record %s: %w", record.ArchiveID, err)
	}
	body := fmt.Sprintf("Archive `%s` for `%s`.", record.ArchiveID, record.Target)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.ArchiveRecordFile(s.Root, record.ArchiveID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write archive record %s: %w", record.ArchiveID, err)
	}
	return nil
}

func (s ArchiveRecordStore) LoadArchiveRecord(_ context.Context, archiveID string) (contracts.ArchiveRecord, error) {
	raw, err := os.ReadFile(storage.ArchiveRecordFile(s.Root, archiveID))
	if err != nil {
		return contracts.ArchiveRecord{}, fmt.Errorf("read archive record %s: %w", archiveID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.ArchiveRecord{}, err
	}
	var fm archiveRecordFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.ArchiveRecord{}, fmt.Errorf("parse archive record %s: %w", archiveID, err)
	}
	record := normalizeArchiveRecord(fm.ArchiveRecord)
	return record, record.Validate()
}

func (s ArchiveRecordStore) ListArchiveRecords(_ context.Context) ([]contracts.ArchiveRecord, error) {
	entries, err := os.ReadDir(storage.ArchivesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ArchiveRecord{}, nil
		}
		return nil, fmt.Errorf("read archives dir: %w", err)
	}
	items := make([]contracts.ArchiveRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		record, err := s.LoadArchiveRecord(context.Background(), strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ArchiveID < items[j].ArchiveID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func sanitizeRetentionPolicyID(policyID string) string {
	policyID = strings.TrimSpace(strings.ToLower(policyID))
	policyID = strings.ReplaceAll(policyID, " ", "-")
	policyID = strings.ReplaceAll(policyID, "/", "-")
	policyID = strings.ReplaceAll(policyID, "_", "-")
	if policyID == "" {
		return "retention-policy"
	}
	return policyID
}

func archivePayloadPath(root string, archiveID string, sourcePath string) string {
	return filepath.Join(storage.ArchivePayloadDir(root, archiveID), filepath.Clean(sourcePath))
}

func normalizeArchiveRecord(record contracts.ArchiveRecord) contracts.ArchiveRecord {
	if record.SchemaVersion == 0 {
		record.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if record.ArchiveRecordUID == "" {
		record.ArchiveRecordUID = contracts.ArchiveRecordUID(record.ArchiveID)
	}
	return record
}
