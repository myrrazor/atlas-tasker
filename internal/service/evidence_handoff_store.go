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
	"gopkg.in/yaml.v3"
)

type EvidenceStore struct {
	Root string
}

type HandoffStore struct {
	Root string
}

type evidenceFrontmatter struct {
	contracts.EvidenceItem `yaml:",inline"`
}

type handoffFrontmatter struct {
	contracts.HandoffPacket `yaml:",inline"`
}

func (s EvidenceStore) SaveEvidence(_ context.Context, evidence contracts.EvidenceItem) error {
	evidence = normalizeEvidenceItem(evidence)
	if err := evidence.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.EvidenceDir(s.Root, evidence.RunID), 0o755); err != nil {
		return fmt.Errorf("create evidence dir: %w", err)
	}
	raw, err := yaml.Marshal(evidenceFrontmatter{EvidenceItem: evidence})
	if err != nil {
		return fmt.Errorf("marshal evidence %s: %w", evidence.EvidenceID, err)
	}
	body := strings.TrimSpace(evidence.Body)
	if body == "" {
		body = evidence.Title
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.EvidenceFile(s.Root, evidence.RunID, evidence.EvidenceID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write evidence %s: %w", evidence.EvidenceID, err)
	}
	return nil
}

func (s EvidenceStore) LoadEvidence(_ context.Context, evidenceID string) (contracts.EvidenceItem, error) {
	matches, err := filepath.Glob(filepath.Join(storage.TrackerDir(s.Root), "evidence", "*", evidenceID+".md"))
	if err != nil || len(matches) == 0 {
		return contracts.EvidenceItem{}, fmt.Errorf("read evidence %s: %w", evidenceID, os.ErrNotExist)
	}
	return loadEvidenceFile(matches[0], evidenceID)
}

func (s EvidenceStore) LoadEvidenceForRun(_ context.Context, runID string, evidenceID string) (contracts.EvidenceItem, error) {
	return loadEvidenceFile(storage.EvidenceFile(s.Root, runID, evidenceID), evidenceID)
}

func (s EvidenceStore) ListEvidence(_ context.Context, runID string) ([]contracts.EvidenceItem, error) {
	entries, err := os.ReadDir(storage.EvidenceDir(s.Root, runID))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.EvidenceItem{}, nil
		}
		return nil, fmt.Errorf("read evidence dir: %w", err)
	}
	items := make([]contracts.EvidenceItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		evidenceID := strings.TrimSuffix(entry.Name(), ".md")
		evidence, err := s.LoadEvidenceForRun(context.Background(), runID, evidenceID)
		if err != nil {
			return nil, err
		}
		items = append(items, evidence)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].EvidenceID < items[j].EvidenceID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func loadEvidenceFile(path string, evidenceID string) (contracts.EvidenceItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contracts.EvidenceItem{}, fmt.Errorf("read evidence %s: %w", evidenceID, err)
	}
	fmRaw, body, err := splitDocument(string(raw))
	if err != nil {
		return contracts.EvidenceItem{}, err
	}
	var fm evidenceFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.EvidenceItem{}, fmt.Errorf("parse evidence %s: %w", evidenceID, err)
	}
	evidence := fm.EvidenceItem
	if strings.TrimSpace(evidence.Body) == "" {
		evidence.Body = strings.TrimSpace(body)
	}
	return normalizeEvidenceItem(evidence), nil
}

func (s HandoffStore) SaveHandoff(_ context.Context, handoff contracts.HandoffPacket) error {
	handoff = normalizeHandoffPacket(handoff)
	if err := handoff.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.HandoffsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create handoffs dir: %w", err)
	}
	raw, err := yaml.Marshal(handoffFrontmatter{HandoffPacket: handoff})
	if err != nil {
		return fmt.Errorf("marshal handoff %s: %w", handoff.HandoffID, err)
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), RenderHandoffMarkdown(handoff))
	if err := os.WriteFile(storage.HandoffFile(s.Root, handoff.HandoffID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write handoff %s: %w", handoff.HandoffID, err)
	}
	return nil
}

func (s HandoffStore) LoadHandoff(_ context.Context, handoffID string) (contracts.HandoffPacket, error) {
	raw, err := os.ReadFile(storage.HandoffFile(s.Root, handoffID))
	if err != nil {
		return contracts.HandoffPacket{}, fmt.Errorf("read handoff %s: %w", handoffID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.HandoffPacket{}, err
	}
	var fm handoffFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.HandoffPacket{}, fmt.Errorf("parse handoff %s: %w", handoffID, err)
	}
	return normalizeHandoffPacket(fm.HandoffPacket), nil
}

func (s HandoffStore) ListHandoffs(_ context.Context, ticketID string) ([]contracts.HandoffPacket, error) {
	entries, err := os.ReadDir(storage.HandoffsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.HandoffPacket{}, nil
		}
		return nil, fmt.Errorf("read handoffs dir: %w", err)
	}
	items := make([]contracts.HandoffPacket, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		handoffID := strings.TrimSuffix(entry.Name(), ".md")
		handoff, err := s.LoadHandoff(context.Background(), handoffID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(ticketID) != "" && handoff.TicketID != ticketID {
			continue
		}
		items = append(items, handoff)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].HandoffID < items[j].HandoffID
		}
		return items[i].GeneratedAt.Before(items[j].GeneratedAt)
	})
	return items, nil
}

// RenderHandoffMarkdown returns the stable markdown body used for handoff exports.
func RenderHandoffMarkdown(handoff contracts.HandoffPacket) string {
	lines := []string{
		fmt.Sprintf("# Handoff %s", handoff.HandoffID),
		"",
		fmt.Sprintf("- Ticket: %s", handoff.TicketID),
		fmt.Sprintf("- Run: %s", handoff.SourceRunID),
		fmt.Sprintf("- Actor: %s", handoff.Actor),
	}
	if strings.TrimSpace(handoff.StatusSummary) != "" {
		lines = append(lines, "", "## Status", "", handoff.StatusSummary)
	}
	appendList := func(title string, values []string) {
		if len(values) == 0 {
			return
		}
		lines = append(lines, "", "## "+title, "")
		for _, value := range values {
			lines = append(lines, "- "+value)
		}
	}
	appendList("Changed Files", handoff.ChangedFiles)
	appendList("Commit Refs", handoff.CommitRefs)
	appendList("Tests", handoff.Tests)
	appendList("Evidence", handoff.EvidenceLinks)
	appendList("Open Questions", handoff.OpenQuestions)
	appendList("Risks", handoff.Risks)
	if strings.TrimSpace(handoff.SuggestedNextActor) != "" || handoff.SuggestedNextGate != "" || handoff.SuggestedNextTicketStatus != "" {
		lines = append(lines, "", "## Next", "")
		if handoff.SuggestedNextActor != "" {
			lines = append(lines, "- Actor: "+handoff.SuggestedNextActor)
		}
		if handoff.SuggestedNextGate != "" {
			lines = append(lines, "- Gate: "+string(handoff.SuggestedNextGate))
		}
		if handoff.SuggestedNextTicketStatus != "" {
			lines = append(lines, "- Ticket Status: "+string(handoff.SuggestedNextTicketStatus))
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeEvidenceItem(evidence contracts.EvidenceItem) contracts.EvidenceItem {
	if evidence.SchemaVersion == 0 {
		evidence.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if evidence.EvidenceUID == "" {
		evidence.EvidenceUID = contracts.EvidenceUID(evidence.RunID, evidence.EvidenceID)
	}
	return evidence
}

func normalizeHandoffPacket(handoff contracts.HandoffPacket) contracts.HandoffPacket {
	if handoff.SchemaVersion == 0 {
		handoff.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if handoff.HandoffUID == "" {
		handoff.HandoffUID = contracts.HandoffUID(handoff.HandoffID)
	}
	return handoff
}
