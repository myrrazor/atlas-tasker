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

type GateStore struct {
	Root string
}

type gateFrontmatter struct {
	contracts.GateSnapshot `yaml:",inline"`
}

func (s GateStore) SaveGate(_ context.Context, gate contracts.GateSnapshot) error {
	if err := gate.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.GatesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create gates dir: %w", err)
	}
	raw, err := yaml.Marshal(gateFrontmatter{GateSnapshot: gate})
	if err != nil {
		return fmt.Errorf("marshal gate %s: %w", gate.GateID, err)
	}
	body := RenderGateMarkdown(gate)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.GateFile(s.Root, gate.GateID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write gate %s: %w", gate.GateID, err)
	}
	return nil
}

func (s GateStore) LoadGate(_ context.Context, gateID string) (contracts.GateSnapshot, error) {
	raw, err := os.ReadFile(storage.GateFile(s.Root, gateID))
	if err != nil {
		return contracts.GateSnapshot{}, fmt.Errorf("read gate %s: %w", gateID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.GateSnapshot{}, err
	}
	var fm gateFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.GateSnapshot{}, fmt.Errorf("parse gate %s: %w", gateID, err)
	}
	return fm.GateSnapshot, nil
}

func (s GateStore) ListGates(_ context.Context, ticketID string) ([]contracts.GateSnapshot, error) {
	entries, err := os.ReadDir(storage.GatesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.GateSnapshot{}, nil
		}
		return nil, fmt.Errorf("read gates dir: %w", err)
	}
	items := make([]contracts.GateSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		gateID := strings.TrimSuffix(entry.Name(), ".md")
		gate, err := s.LoadGate(context.Background(), gateID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(ticketID) != "" && gate.TicketID != ticketID {
			continue
		}
		items = append(items, gate)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].GateID < items[j].GateID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

// RenderGateMarkdown returns the stable markdown body used for gate views.
func RenderGateMarkdown(gate contracts.GateSnapshot) string {
	lines := []string{
		fmt.Sprintf("# Gate %s", gate.GateID),
		"",
		fmt.Sprintf("- Ticket: %s", gate.TicketID),
		fmt.Sprintf("- Kind: %s", gate.Kind),
		fmt.Sprintf("- State: %s", gate.State),
		fmt.Sprintf("- Created By: %s", gate.CreatedBy),
	}
	if gate.RunID != "" {
		lines = append(lines, "- Run: "+gate.RunID)
	}
	if gate.RequiredAgentID != "" {
		lines = append(lines, "- Required Agent: "+gate.RequiredAgentID)
	}
	if gate.RequiredRole != "" {
		lines = append(lines, "- Required Role: "+string(gate.RequiredRole))
	}
	if gate.DecidedBy != "" || gate.DecisionReason != "" {
		lines = append(lines, "", "## Decision", "")
		if gate.DecidedBy != "" {
			lines = append(lines, "- By: "+string(gate.DecidedBy))
		}
		if gate.DecisionReason != "" {
			lines = append(lines, "- Reason: "+gate.DecisionReason)
		}
	}
	if len(gate.RelatedRunIDs) > 0 {
		lines = append(lines, "", "## Related Runs", "")
		for _, runID := range gate.RelatedRunIDs {
			lines = append(lines, "- "+runID)
		}
	}
	return strings.Join(lines, "\n")
}
