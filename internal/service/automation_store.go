package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

type AutomationStore struct {
	Root string
}

type automationRuleFile struct {
	Name       string                        `toml:"name"`
	Enabled    *bool                         `toml:"enabled"`
	Trigger    contracts.AutomationTrigger   `toml:"trigger"`
	Conditions contracts.AutomationCondition `toml:"conditions"`
	Actions    []contracts.AutomationAction  `toml:"actions"`
}

func (s AutomationStore) ListRules() ([]contracts.AutomationRule, error) {
	entries, err := os.ReadDir(storage.AutomationsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.AutomationRule{}, nil
		}
		return nil, fmt.Errorf("read automations dir: %w", err)
	}
	rules := make([]contracts.AutomationRule, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		rule, err := s.LoadRule(strings.TrimSuffix(entry.Name(), ".toml"))
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Name < rules[j].Name
	})
	return rules, nil
}

func (s AutomationStore) LoadRule(name string) (contracts.AutomationRule, error) {
	raw, err := os.ReadFile(s.rulePath(name))
	if err != nil {
		return contracts.AutomationRule{}, fmt.Errorf("read automation %s: %w", name, err)
	}
	var rule contracts.AutomationRule
	if err := toml.Unmarshal(raw, &rule); err != nil {
		return contracts.AutomationRule{}, fmt.Errorf("parse automation %s: %w", name, err)
	}
	var file automationRuleFile
	if err := toml.Unmarshal(raw, &file); err != nil {
		return contracts.AutomationRule{}, fmt.Errorf("parse automation %s defaults: %w", name, err)
	}
	if file.Enabled == nil {
		rule.Enabled = true
	} else {
		rule.Enabled = *file.Enabled
	}
	if strings.TrimSpace(rule.Name) == "" {
		rule.Name = sanitizeAutomationName(name)
	}
	if err := rule.Validate(); err != nil {
		return contracts.AutomationRule{}, err
	}
	return rule, nil
}

func (s AutomationStore) SaveRule(rule contracts.AutomationRule) error {
	rule.Name = sanitizeAutomationName(rule.Name)
	if !rule.Enabled {
		// preserve explicit false
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.AutomationsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create automations dir: %w", err)
	}
	raw, err := toml.Marshal(rule)
	if err != nil {
		return fmt.Errorf("encode automation %s: %w", rule.Name, err)
	}
	if err := os.WriteFile(s.rulePath(rule.Name), raw, 0o644); err != nil {
		return fmt.Errorf("write automation %s: %w", rule.Name, err)
	}
	return nil
}

func (s AutomationStore) DeleteRule(name string) error {
	if err := os.Remove(s.rulePath(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete automation %s: %w", name, err)
	}
	return nil
}

func (s AutomationStore) rulePath(name string) string {
	return filepath.Join(storage.AutomationsDir(s.Root), sanitizeAutomationName(name)+".toml")
}

func sanitizeAutomationName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	if name == "" {
		return "automation"
	}
	return name
}
