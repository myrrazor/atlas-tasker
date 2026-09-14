package integrations

import (
	"strings"
	"testing"
)

func TestSharedSkillPresentsStatusBoardAsCompactMarkdownTable(t *testing.T) {
	needles := []string{
		"compact Markdown ticket TABLE",
		"blockers and next steps",
		"shown/total",
		"unbounded catalog",
		"atlas.board",
		"atlas.status",
		"Never invent tickets",
	}
	core := lifecycleSection(atlasWorkerSkill("claude"))
	for _, needle := range needles {
		if !strings.Contains(core, needle) {
			t.Fatalf("shared lifecycle missing %q:\n%s", needle, core)
		}
	}
	for _, provider := range []string{"codex", "claude", "openclaw", "generic", "cursor", "grok"} {
		section := lifecycleSection(atlasWorkerSkill(provider))
		if section != core {
			t.Fatalf("%s lifecycle drifted from shared status-table guidance", provider)
		}
	}
	grok := atlasWorkerSkill("grok")
	if !strings.Contains(grok, "atlas_status") || !strings.Contains(grok, "atlas_board") {
		t.Fatal("grok skill lost portable underscore names")
	}
}
