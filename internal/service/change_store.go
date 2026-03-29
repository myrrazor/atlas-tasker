package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type ChangeStore struct {
	Root string
}

type CheckStore struct {
	Root string
}

type changeFrontmatter struct {
	contracts.ChangeRef `yaml:",inline"`
}

type checkFrontmatter struct {
	contracts.CheckResult `yaml:",inline"`
}

func (s ChangeStore) SaveChange(_ context.Context, change contracts.ChangeRef) error {
	change = normalizeChangeRef(change)
	if err := change.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ChangesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create changes dir: %w", err)
	}
	raw, err := yaml.Marshal(changeFrontmatter{ChangeRef: change})
	if err != nil {
		return fmt.Errorf("marshal change %s: %w", change.ChangeID, err)
	}
	body := strings.TrimSpace(change.ReviewSummary)
	if body == "" {
		body = fmt.Sprintf("Change `%s` for ticket `%s`.", change.ChangeID, change.TicketID)
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.ChangeFile(s.Root, change.ChangeID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write change %s: %w", change.ChangeID, err)
	}
	return nil
}

func (s ChangeStore) LoadChange(_ context.Context, changeID string) (contracts.ChangeRef, error) {
	raw, err := os.ReadFile(storage.ChangeFile(s.Root, changeID))
	if err != nil {
		return contracts.ChangeRef{}, fmt.Errorf("read change %s: %w", changeID, err)
	}
	fmRaw, body, err := splitDocument(string(raw))
	if err != nil {
		return contracts.ChangeRef{}, err
	}
	var fm changeFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.ChangeRef{}, fmt.Errorf("parse change %s: %w", changeID, err)
	}
	change := normalizeChangeRef(fm.ChangeRef)
	if strings.TrimSpace(change.ReviewSummary) == "" {
		change.ReviewSummary = strings.TrimSpace(body)
	}
	return change, nil
}

func (s ChangeStore) ListChanges(_ context.Context, ticketID string) ([]contracts.ChangeRef, error) {
	entries, err := os.ReadDir(storage.ChangesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ChangeRef{}, nil
		}
		return nil, fmt.Errorf("read changes dir: %w", err)
	}
	items := make([]contracts.ChangeRef, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		changeID := strings.TrimSuffix(entry.Name(), ".md")
		change, err := s.LoadChange(context.Background(), changeID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(ticketID) != "" && change.TicketID != ticketID {
			continue
		}
		items = append(items, change)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ChangeID < items[j].ChangeID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s CheckStore) SaveCheck(_ context.Context, check contracts.CheckResult) error {
	check = normalizeCheckResult(check)
	if err := check.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ChecksDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create checks dir: %w", err)
	}
	raw, err := yaml.Marshal(checkFrontmatter{CheckResult: check})
	if err != nil {
		return fmt.Errorf("marshal check %s: %w", check.CheckID, err)
	}
	body := strings.TrimSpace(check.Summary)
	if body == "" {
		body = fmt.Sprintf("Check `%s` for %s `%s`.", check.Name, check.Scope, check.ScopeID)
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.CheckFile(s.Root, check.CheckID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write check %s: %w", check.CheckID, err)
	}
	return nil
}

func (s CheckStore) LoadCheck(_ context.Context, checkID string) (contracts.CheckResult, error) {
	raw, err := os.ReadFile(storage.CheckFile(s.Root, checkID))
	if err != nil {
		return contracts.CheckResult{}, fmt.Errorf("read check %s: %w", checkID, err)
	}
	fmRaw, body, err := splitDocument(string(raw))
	if err != nil {
		return contracts.CheckResult{}, err
	}
	var fm checkFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.CheckResult{}, fmt.Errorf("parse check %s: %w", checkID, err)
	}
	check := normalizeCheckResult(fm.CheckResult)
	if strings.TrimSpace(check.Summary) == "" {
		check.Summary = strings.TrimSpace(body)
	}
	return check, nil
}

func (s CheckStore) ListChecks(_ context.Context, scope contracts.CheckScope, scopeID string) ([]contracts.CheckResult, error) {
	entries, err := os.ReadDir(storage.ChecksDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.CheckResult{}, nil
		}
		return nil, fmt.Errorf("read checks dir: %w", err)
	}
	items := make([]contracts.CheckResult, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		checkID := strings.TrimSuffix(entry.Name(), ".md")
		check, err := s.LoadCheck(context.Background(), checkID)
		if err != nil {
			return nil, err
		}
		if scope != "" && check.Scope != scope {
			continue
		}
		if strings.TrimSpace(scopeID) != "" && check.ScopeID != strings.TrimSpace(scopeID) {
			continue
		}
		items = append(items, check)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].CheckID < items[j].CheckID
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	return items, nil
}

func normalizeChangeRef(change contracts.ChangeRef) contracts.ChangeRef {
	if change.SchemaVersion == 0 {
		change.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if change.ChangeUID == "" {
		change.ChangeUID = contracts.ChangeUID(change.ChangeID)
	}
	if change.Provider == "" {
		change.Provider = contracts.ChangeProviderLocal
	}
	if change.Status == "" {
		change.Status = contracts.ChangeStatusLocalOnly
	}
	if change.ChecksStatus == "" {
		change.ChecksStatus = contracts.CheckAggregateUnknown
	}
	if change.CreatedAt.IsZero() {
		change.CreatedAt = timeNowUTC()
	}
	if change.UpdatedAt.IsZero() {
		change.UpdatedAt = change.CreatedAt
	}
	return change
}

func normalizeCheckResult(check contracts.CheckResult) contracts.CheckResult {
	if check.SchemaVersion == 0 {
		check.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if check.CheckUID == "" {
		check.CheckUID = contracts.CheckUID(check.CheckID)
	}
	if check.Source == "" {
		check.Source = contracts.CheckSourceManual
	}
	if check.Status == "" {
		check.Status = contracts.CheckStatusQueued
	}
	if check.Conclusion == "" {
		check.Conclusion = contracts.CheckConclusionUnknown
	}
	if check.UpdatedAt.IsZero() {
		check.UpdatedAt = timeNowUTC()
	}
	return check
}
