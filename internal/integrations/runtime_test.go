package integrations

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestBuildRunManifestIncludesVersionedContextAndProviderHints(t *testing.T) {
	generatedAt := time.Date(2026, 3, 25, 12, 30, 0, 0, time.UTC)
	manifest, err := BuildRunManifest(RunManifestInput{
		WorkspaceRoot: "/tmp/atlas",
		Run: contracts.RunSnapshot{
			RunID:          "run_123",
			TicketID:       "APP-1",
			AgentID:        "builder-1",
			Status:         contracts.RunStatusActive,
			Kind:           contracts.RunKindWork,
			BlueprintStage: "implement",
			WorktreePath:   "/tmp/worktree",
			BranchName:     "run/app-1-run_123",
			Summary:        "Finish the runtime slice.",
		},
		Ticket: contracts.TicketSnapshot{
			ID:                 "APP-1",
			Title:              "Build runtime launch flow",
			Description:        "Need a durable run context.",
			AcceptanceCriteria: []string{"write brief", "write provider launch files"},
		},
		RuntimeDir:       "/tmp/atlas/.tracker/runtime/run_123",
		BriefPath:        "/tmp/atlas/.tracker/runtime/run_123/brief.md",
		ContextPath:      "/tmp/atlas/.tracker/runtime/run_123/context.json",
		CodexLaunchPath:  "/tmp/atlas/.tracker/runtime/run_123/launch.codex.txt",
		ClaudeLaunchPath: "/tmp/atlas/.tracker/runtime/run_123/launch.claude.txt",
		EvidenceDir:      "/tmp/atlas/.tracker/evidence/run_123",
		GeneratedAt:      generatedAt,
	})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}

	if !strings.Contains(manifest.BriefMarkdown, "# Run run_123") || !strings.Contains(manifest.BriefMarkdown, "## Acceptance Criteria") {
		t.Fatalf("unexpected brief markdown: %s", manifest.BriefMarkdown)
	}
	if !strings.Contains(manifest.CodexLaunch, "tracker run attach run_123 --provider codex --session-ref <session>") {
		t.Fatalf("missing codex attach hint: %s", manifest.CodexLaunch)
	}
	if !strings.Contains(manifest.ClaudeLaunch, "tracker run attach run_123 --provider claude --session-ref <session>") {
		t.Fatalf("missing claude attach hint: %s", manifest.ClaudeLaunch)
	}

	var envelope struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		GeneratedAt   string `json:"generated_at"`
		Payload       struct {
			Run struct {
				RunID string `json:"run_id"`
			} `json:"run"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(manifest.ContextJSON, &envelope); err != nil {
		t.Fatalf("unmarshal context json: %v\nraw=%s", err, string(manifest.ContextJSON))
	}
	if envelope.FormatVersion != "v1" || envelope.Kind != "run_launch_manifest" || envelope.Payload.Run.RunID != "run_123" {
		t.Fatalf("unexpected context envelope: %#v", envelope)
	}
}
