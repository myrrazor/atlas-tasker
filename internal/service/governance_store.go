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
)

type GovernancePolicyStore struct {
	Root string
}

type GovernancePackStore struct {
	Root string
}

func (s GovernancePolicyStore) SaveGovernancePolicy(_ context.Context, policy contracts.GovernancePolicy) error {
	policy = normalizeGovernancePolicy(policy)
	if err := policy.Validate(); err != nil {
		return err
	}
	if err := validateGovernancePolicyRuntime(policy); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.GovernancePoliciesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create governance policies dir: %w", err)
	}
	raw, err := toml.Marshal(policy)
	if err != nil {
		return fmt.Errorf("encode governance policy %s: %w", policy.PolicyID, err)
	}
	if err := os.WriteFile(governancePolicyPath(s.Root, policy.PolicyID), raw, 0o644); err != nil {
		return fmt.Errorf("write governance policy %s: %w", policy.PolicyID, err)
	}
	return nil
}

func (s GovernancePolicyStore) LoadGovernancePolicy(_ context.Context, policyID string) (contracts.GovernancePolicy, error) {
	policyID = sanitizeGovernanceID(policyID, "policy")
	raw, err := os.ReadFile(governancePolicyPath(s.Root, policyID))
	if err != nil {
		return contracts.GovernancePolicy{}, fmt.Errorf("read governance policy %s: %w", policyID, err)
	}
	var policy contracts.GovernancePolicy
	if err := toml.Unmarshal(raw, &policy); err != nil {
		return contracts.GovernancePolicy{}, fmt.Errorf("parse governance policy %s: %w", policyID, err)
	}
	if strings.TrimSpace(policy.PolicyID) == "" {
		policy.PolicyID = policyID
	}
	policy = normalizeGovernancePolicy(policy)
	return policy, policy.Validate()
}

func (s GovernancePolicyStore) ListGovernancePolicies(_ context.Context) ([]contracts.GovernancePolicy, error) {
	entries, err := os.ReadDir(storage.GovernancePoliciesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.GovernancePolicy{}, nil
		}
		return nil, fmt.Errorf("read governance policies dir: %w", err)
	}
	items := make([]contracts.GovernancePolicy, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		policyID := strings.TrimSuffix(entry.Name(), ".toml")
		policy, err := s.LoadGovernancePolicy(context.Background(), policyID)
		if err != nil {
			return nil, fmt.Errorf("load governance policy %s: %w", policyID, err)
		}
		items = append(items, policy)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ScopeKind != items[j].ScopeKind {
			return items[i].ScopeKind < items[j].ScopeKind
		}
		if items[i].ScopeID != items[j].ScopeID {
			return items[i].ScopeID < items[j].ScopeID
		}
		return items[i].PolicyID < items[j].PolicyID
	})
	return items, nil
}

func (s GovernancePackStore) SaveGovernancePack(_ context.Context, pack contracts.PolicyPack) error {
	pack = normalizeGovernancePack(pack)
	if err := pack.Validate(); err != nil {
		return err
	}
	for _, policy := range pack.Policies {
		if err := validateGovernancePolicyRuntime(policy); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(storage.GovernancePacksDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create governance packs dir: %w", err)
	}
	raw, err := toml.Marshal(pack)
	if err != nil {
		return fmt.Errorf("encode governance pack %s: %w", pack.PackID, err)
	}
	if err := os.WriteFile(governancePackPath(s.Root, pack.PackID), raw, 0o644); err != nil {
		return fmt.Errorf("write governance pack %s: %w", pack.PackID, err)
	}
	return nil
}

func (s GovernancePackStore) LoadGovernancePack(_ context.Context, packID string) (contracts.PolicyPack, error) {
	packID = sanitizeGovernanceID(packID, "pack")
	raw, err := os.ReadFile(governancePackPath(s.Root, packID))
	if err != nil {
		return contracts.PolicyPack{}, fmt.Errorf("read governance pack %s: %w", packID, err)
	}
	var pack contracts.PolicyPack
	if err := toml.Unmarshal(raw, &pack); err != nil {
		return contracts.PolicyPack{}, fmt.Errorf("parse governance pack %s: %w", packID, err)
	}
	if strings.TrimSpace(pack.PackID) == "" {
		pack.PackID = packID
	}
	pack = normalizeGovernancePack(pack)
	return pack, pack.Validate()
}

func (s GovernancePackStore) ListGovernancePacks(_ context.Context) ([]contracts.PolicyPack, error) {
	entries, err := os.ReadDir(storage.GovernancePacksDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.PolicyPack{}, nil
		}
		return nil, fmt.Errorf("read governance packs dir: %w", err)
	}
	items := make([]contracts.PolicyPack, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		packID := strings.TrimSuffix(entry.Name(), ".toml")
		pack, err := s.LoadGovernancePack(context.Background(), packID)
		if err != nil {
			return nil, fmt.Errorf("load governance pack %s: %w", packID, err)
		}
		items = append(items, pack)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].PackID < items[j].PackID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func normalizeGovernancePack(pack contracts.PolicyPack) contracts.PolicyPack {
	pack.PackID = sanitizeGovernanceID(firstNonEmpty(pack.PackID, pack.Name), "pack")
	pack.Name = strings.TrimSpace(pack.Name)
	if pack.Name == "" {
		pack.Name = pack.PackID
	}
	if pack.CreatedAt.IsZero() {
		pack.CreatedAt = timeNowUTC()
	}
	if pack.UpdatedAt.IsZero() {
		pack.UpdatedAt = pack.CreatedAt
	}
	if pack.SchemaVersion == 0 {
		pack.SchemaVersion = contracts.CurrentSchemaVersion
	}
	for i := range pack.Policies {
		pack.Policies[i] = normalizeGovernancePolicy(pack.Policies[i])
	}
	return pack
}

func normalizeGovernancePolicy(policy contracts.GovernancePolicy) contracts.GovernancePolicy {
	policy.PolicyID = sanitizeGovernanceID(firstNonEmpty(policy.PolicyID, policy.Name), "policy")
	policy.Name = strings.TrimSpace(policy.Name)
	if policy.Name == "" {
		policy.Name = policy.PolicyID
	}
	policy.ScopeID = strings.TrimSpace(policy.ScopeID)
	if policy.ScopeKind == "" {
		policy.ScopeKind = contracts.PolicyScopeWorkspace
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = timeNowUTC()
	}
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = policy.CreatedAt
	}
	if policy.SchemaVersion == 0 {
		policy.SchemaVersion = contracts.CurrentSchemaVersion
	}
	policy.ProtectedActions = uniqueProtectedActions(policy.ProtectedActions)
	return policy
}

func sanitizeGovernanceID(value string, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			return r
		}
		return '-'
	}, value)
	value = strings.Trim(value, "-.")
	if value == "" {
		return fallback
	}
	return value
}

func uniqueProtectedActions(values []contracts.ProtectedAction) []contracts.ProtectedAction {
	seen := map[contracts.ProtectedAction]struct{}{}
	out := make([]contracts.ProtectedAction, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validateGovernancePolicyRuntime(policy contracts.GovernancePolicy) error {
	if policy.RequiredSignatures > 0 {
		if len(policy.ProtectedActions) == 0 {
			return fmt.Errorf("required_signatures needs an explicit protected action with signature evidence")
		}
		for _, action := range policy.ProtectedActions {
			if !governanceActionHasSignatureEvidence(action) {
				return fmt.Errorf("required_signatures is not supported for %s yet; use quorum or separation rules", action)
			}
		}
	}
	for _, rule := range policy.QuorumRules {
		if rule.RequireTrustedSignatures && !governanceActionHasSignatureEvidence(rule.ActionKind) {
			return fmt.Errorf("trusted-signature quorum is not supported for %s yet", rule.ActionKind)
		}
	}
	for _, rule := range policy.OverrideRules {
		if rule.RequireTrustedSignature && !governanceActionHasSignatureEvidence(rule.ActionKind) {
			return fmt.Errorf("trusted-signature override is not supported for %s yet", rule.ActionKind)
		}
	}
	return nil
}

func governanceActionHasSignatureEvidence(action contracts.ProtectedAction) bool {
	switch action {
	case contracts.ProtectedActionBundleImportApply, contracts.ProtectedActionSyncImportApply:
		return true
	default:
		return false
	}
}

func scopedGovernancePolicyID(policyID string, scopeKind contracts.PolicyScopeKind, scopeID string) string {
	scope := string(scopeKind)
	if strings.TrimSpace(scopeID) != "" {
		scope += "-" + strings.TrimSpace(scopeID)
	}
	return sanitizeGovernanceID(policyID+"-"+scope, "policy")
}

func governancePolicyPath(root string, policyID string) string {
	return filepath.Join(storage.GovernancePoliciesDir(root), sanitizeGovernanceID(policyID, "policy")+".toml")
}

func governancePackPath(root string, packID string) string {
	return filepath.Join(storage.GovernancePacksDir(root), sanitizeGovernanceID(packID, "pack")+".toml")
}
