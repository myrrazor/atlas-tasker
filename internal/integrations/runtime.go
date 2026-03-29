package integrations

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type RunManifestInput struct {
	WorkspaceRoot    string                    `json:"workspace_root"`
	Run              contracts.RunSnapshot     `json:"run"`
	Ticket           contracts.TicketSnapshot  `json:"ticket"`
	Agent            contracts.AgentProfile    `json:"agent"`
	Gates            []contracts.GateSnapshot  `json:"gates,omitempty"`
	Evidence         []contracts.EvidenceItem  `json:"evidence,omitempty"`
	Handoffs         []contracts.HandoffPacket `json:"handoffs,omitempty"`
	RuntimeDir       string                    `json:"runtime_dir"`
	BriefPath        string                    `json:"brief_path"`
	ContextPath      string                    `json:"context_path"`
	CodexLaunchPath  string                    `json:"codex_launch_path"`
	ClaudeLaunchPath string                    `json:"claude_launch_path"`
	EvidenceDir      string                    `json:"evidence_dir"`
	GeneratedAt      time.Time                 `json:"generated_at"`
}

type RunManifest struct {
	BriefMarkdown string
	ContextJSON   []byte
	CodexLaunch   string
	ClaudeLaunch  string
}

func BuildRunManifest(input RunManifestInput) (RunManifest, error) {
	brief := buildBriefMarkdown(input)
	contextJSON, err := json.MarshalIndent(map[string]any{
		"format_version": "v1",
		"kind":           "run_launch_manifest",
		"generated_at":   input.GeneratedAt.UTC(),
		"payload":        input,
	}, "", "  ")
	if err != nil {
		return RunManifest{}, fmt.Errorf("marshal runtime context: %w", err)
	}
	return RunManifest{
		BriefMarkdown: brief,
		ContextJSON:   contextJSON,
		CodexLaunch:   buildProviderLaunch("codex", input),
		ClaudeLaunch:  buildProviderLaunch("claude", input),
	}, nil
}

func buildBriefMarkdown(input RunManifestInput) string {
	lines := []string{
		fmt.Sprintf("# Run %s", input.Run.RunID),
		"",
		fmt.Sprintf("- Ticket: %s", input.Ticket.ID),
		fmt.Sprintf("- Title: %s", input.Ticket.Title),
		fmt.Sprintf("- Run Status: %s", input.Run.Status),
		fmt.Sprintf("- Run Kind: %s", input.Run.Kind),
		fmt.Sprintf("- Agent: %s", input.Run.AgentID),
	}
	if input.Run.BlueprintStage != "" {
		lines = append(lines, "- Stage: "+input.Run.BlueprintStage)
	}
	if input.Run.WorktreePath != "" {
		lines = append(lines, "- Worktree: "+input.Run.WorktreePath)
	}
	if input.Run.BranchName != "" {
		lines = append(lines, "- Branch: "+input.Run.BranchName)
	}
	if input.Run.Summary != "" {
		lines = append(lines, "", "## Summary", "", input.Run.Summary)
	}
	if input.Ticket.Description != "" {
		lines = append(lines, "", "## Ticket Description", "", input.Ticket.Description)
	}
	if len(input.Ticket.AcceptanceCriteria) > 0 {
		lines = append(lines, "", "## Acceptance Criteria", "")
		for _, item := range input.Ticket.AcceptanceCriteria {
			lines = append(lines, "- "+item)
		}
	}
	if len(input.Gates) > 0 {
		lines = append(lines, "", "## Open Gates", "")
		for _, gate := range input.Gates {
			if gate.State != contracts.GateStateOpen {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s [%s]", gate.GateID, gate.Kind))
		}
	}
	if len(input.Evidence) > 0 {
		lines = append(lines, "", "## Evidence", "")
		for _, item := range input.Evidence {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s", item.EvidenceID, item.Type, item.Title))
		}
	}
	if len(input.Handoffs) > 0 {
		lines = append(lines, "", "## Prior Handoffs", "")
		for _, item := range input.Handoffs {
			lines = append(lines, fmt.Sprintf("- %s", item.HandoffID))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func buildProviderLaunch(provider string, input RunManifestInput) string {
	lines := []string{
		fmt.Sprintf("Atlas Tasker %s launch for %s", providerLabel(provider), input.Run.RunID),
		"",
		fmt.Sprintf("1. cd %s", input.WorkspaceRoot),
		fmt.Sprintf("2. open %s for the run brief", input.BriefPath),
		fmt.Sprintf("3. open %s for structured context", input.ContextPath),
	}
	if provider == "codex" {
		lines = append(lines,
			"4. read AGENTS.md for project-level rules",
			fmt.Sprintf("5. work in %s if you need isolated changes", input.Run.WorktreePath),
			fmt.Sprintf("6. when attached, record it with: tracker run attach %s --provider codex --session-ref <session>", input.Run.RunID),
		)
	} else {
		lines = append(lines,
			"4. read CLAUDE.md for project-level rules",
			fmt.Sprintf("5. work in %s if you need isolated changes", input.Run.WorktreePath),
			fmt.Sprintf("6. when attached, record it with: tracker run attach %s --provider claude --session-ref <session>", input.Run.RunID),
		)
	}
	lines = append(lines,
		fmt.Sprintf("7. evidence lives under %s", input.EvidenceDir),
		"8. do not treat runtime files as source of truth; Atlas snapshots and events stay canonical",
	)
	return strings.Join(lines, "\n") + "\n"
}

func providerLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude"
	default:
		return strings.ToUpper(strings.TrimSpace(provider))
	}
}
