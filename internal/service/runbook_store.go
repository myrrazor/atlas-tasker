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

type RunbookStore struct {
	Root string
}

func (s RunbookStore) SaveRunbook(_ context.Context, runbook contracts.Runbook) error {
	runbook.Name = sanitizeRunbookName(runbook.Name)
	if err := runbook.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.RunbooksDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create runbooks dir: %w", err)
	}
	raw, err := toml.Marshal(runbook)
	if err != nil {
		return fmt.Errorf("encode runbook %s: %w", runbook.Name, err)
	}
	if err := os.WriteFile(s.path(runbook.Name), raw, 0o644); err != nil {
		return fmt.Errorf("write runbook %s: %w", runbook.Name, err)
	}
	return nil
}

func (s RunbookStore) LoadRunbook(_ context.Context, name string) (contracts.Runbook, error) {
	name = sanitizeRunbookName(name)
	if builtIn, ok := builtInRunbooks()[name]; ok {
		return builtIn, nil
	}
	raw, err := os.ReadFile(s.path(name))
	if err != nil {
		return contracts.Runbook{}, fmt.Errorf("read runbook %s: %w", name, err)
	}
	var runbook contracts.Runbook
	if err := toml.Unmarshal(raw, &runbook); err != nil {
		return contracts.Runbook{}, fmt.Errorf("parse runbook %s: %w", name, err)
	}
	if strings.TrimSpace(runbook.Name) == "" {
		runbook.Name = name
	}
	return runbook, runbook.Validate()
}

func (s RunbookStore) ListRunbooks(_ context.Context) ([]contracts.Runbook, error) {
	catalog := builtInRunbooks()
	items := make([]contracts.Runbook, 0, len(catalog))
	for _, runbook := range catalog {
		items = append(items, runbook)
	}
	entries, err := os.ReadDir(storage.RunbooksDir(s.Root))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read runbooks dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		runbook, err := s.LoadRunbook(context.Background(), strings.TrimSuffix(entry.Name(), ".toml"))
		if err != nil {
			return nil, err
		}
		catalog[runbook.Name] = runbook
	}
	items = items[:0]
	for _, runbook := range catalog {
		items = append(items, runbook)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (s RunbookStore) path(name string) string {
	return filepath.Join(storage.RunbooksDir(s.Root), sanitizeRunbookName(name)+".toml")
}

func sanitizeRunbookName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	if name == "" {
		return "runbook"
	}
	return name
}

func builtInRunbooks() map[string]contracts.Runbook {
	return map[string]contracts.Runbook{
		"plan": {
			Name:                 "plan",
			DisplayName:          "Plan",
			AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeEpic, contracts.TicketTypeTask},
			DefaultInitialStage:  "plan",
			Stages:               []contracts.RunbookStage{{Key: "plan", DisplayName: "Plan", ExpectedRole: contracts.AgentRoleWorker, CompletionCriteria: []string{"proposed approach", "risks called out"}}},
		},
		"design": {
			Name:                 "design",
			DisplayName:          "Design",
			AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeEpic, contracts.TicketTypeTask},
			DefaultInitialStage:  "design",
			Stages:               []contracts.RunbookStage{{Key: "design", DisplayName: "Design", ExpectedRole: contracts.AgentRoleWorker, RequiredEvidenceTypes: []contracts.EvidenceType{contracts.EvidenceTypeManualAssertion}, CompletionCriteria: []string{"design notes recorded"}}},
		},
		"implement": {
			Name:                 "implement",
			DisplayName:          "Implement",
			AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeTask, contracts.TicketTypeBug, contracts.TicketTypeSubtask},
			DefaultInitialStage:  "implement",
			HandoffTemplate:      "default-handoff",
			Stages:               []contracts.RunbookStage{{Key: "implement", DisplayName: "Implement", ExpectedRole: contracts.AgentRoleWorker, CompletionCriteria: []string{"code written", "tests run"}}},
		},
		"review": {
			Name:                 "review",
			DisplayName:          "Review",
			AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeTask, contracts.TicketTypeBug, contracts.TicketTypeSubtask},
			DefaultInitialStage:  "review",
			Stages:               []contracts.RunbookStage{{Key: "review", DisplayName: "Review", ExpectedRole: contracts.AgentRoleReviewer, RequiredGates: []contracts.GateKind{contracts.GateKindReview}, CompletionCriteria: []string{"review complete"}}},
		},
		"qa": {
			Name:                 "qa",
			DisplayName:          "QA",
			AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeTask, contracts.TicketTypeBug, contracts.TicketTypeSubtask},
			DefaultInitialStage:  "qa",
			Stages:               []contracts.RunbookStage{{Key: "qa", DisplayName: "QA", ExpectedRole: contracts.AgentRoleQA, RequiredGates: []contracts.GateKind{contracts.GateKindQA}, CompletionCriteria: []string{"qa pass recorded"}}},
		},
		"release-ready": {
			Name:                 "release-ready",
			DisplayName:          "Release Ready",
			AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeTask, contracts.TicketTypeBug, contracts.TicketTypeSubtask},
			DefaultInitialStage:  "release",
			Stages:               []contracts.RunbookStage{{Key: "release", DisplayName: "Release", ExpectedRole: contracts.AgentRoleOwnerDelegate, RequiredGates: []contracts.GateKind{contracts.GateKindRelease}, CompletionCriteria: []string{"release gate resolved"}}},
		},
	}
}
