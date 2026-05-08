package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEvidenceAndHandoffCommands(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Collect proof", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner", "--summary", "implementing now")

	writeGitFile(t, "proof.log", "all green\n")

	checkpointOut := must("run", "checkpoint", dispatch.Payload.RunID, "--title", "checkpoint", "--body", "mid-flight note", "--actor", "human:owner", "--json")
	var checkpoint struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			EvidenceID string `json:"evidence_id"`
			Type       string `json:"type"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(checkpointOut), &checkpoint); err != nil {
		t.Fatalf("parse checkpoint output: %v\nraw=%s", err, checkpointOut)
	}
	if checkpoint.Kind != "evidence_detail" || checkpoint.Payload.EvidenceID == "" || checkpoint.Payload.Type != "note" {
		t.Fatalf("unexpected checkpoint payload: %#v", checkpoint)
	}

	evidenceOut := must(
		"run", "evidence", "add", dispatch.Payload.RunID,
		"--type", "test_result",
		"--title", "unit tests",
		"--body", "go test ./...",
		"--artifact", "proof.log",
		"--supersedes", checkpoint.Payload.EvidenceID,
		"--actor", "human:owner",
		"--json",
	)
	var evidence struct {
		Kind    string `json:"kind"`
		Payload struct {
			EvidenceID           string `json:"evidence_id"`
			SupersedesEvidenceID string `json:"supersedes_evidence_id"`
			ArtifactPath         string `json:"artifact_path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(evidenceOut), &evidence); err != nil {
		t.Fatalf("parse evidence output: %v\nraw=%s", err, evidenceOut)
	}
	if evidence.Kind != "evidence_detail" || evidence.Payload.SupersedesEvidenceID != checkpoint.Payload.EvidenceID || evidence.Payload.ArtifactPath == "" {
		t.Fatalf("unexpected evidence payload: %#v", evidence)
	}
	if _, err := os.Stat(evidence.Payload.ArtifactPath); err != nil {
		t.Fatalf("expected copied artifact to exist: %v", err)
	}

	handoffOut := must(
		"run", "handoff", dispatch.Payload.RunID,
		"--open-question", "Need review coverage",
		"--risk", "runtime drift",
		"--next-actor", "agent:reviewer-1",
		"--next-gate", "review",
		"--next-status", "in_review",
		"--actor", "agent:builder-1",
		"--json",
	)
	var handoff struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			HandoffID          string   `json:"handoff_id"`
			EvidenceLinks      []string `json:"evidence_links"`
			SuggestedNextActor string   `json:"suggested_next_actor"`
			SuggestedNextGate  string   `json:"suggested_next_gate"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(handoffOut), &handoff); err != nil {
		t.Fatalf("parse handoff output: %v\nraw=%s", err, handoffOut)
	}
	if handoff.Kind != "handoff_detail" || handoff.Payload.HandoffID == "" || len(handoff.Payload.EvidenceLinks) != 2 || handoff.Payload.SuggestedNextGate != "review" {
		t.Fatalf("unexpected handoff payload: %#v", handoff)
	}

	runViewOut := must("run", "view", dispatch.Payload.RunID, "--json")
	var runView struct {
		Kind    string `json:"kind"`
		Payload struct {
			Evidence []struct {
				EvidenceID string `json:"evidence_id"`
			} `json:"evidence"`
			Handoffs []struct {
				HandoffID string `json:"handoff_id"`
			} `json:"handoffs"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runViewOut), &runView); err != nil {
		t.Fatalf("parse run view: %v\nraw=%s", err, runViewOut)
	}
	if runView.Kind != "run_detail" || len(runView.Payload.Evidence) != 2 || len(runView.Payload.Handoffs) != 1 {
		t.Fatalf("unexpected run detail payload: %#v", runView)
	}

	evidenceListOut := must("evidence", "list", dispatch.Payload.RunID, "--json")
	var evidenceList struct {
		Kind  string `json:"kind"`
		Items []struct {
			EvidenceID string `json:"evidence_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(evidenceListOut), &evidenceList); err != nil {
		t.Fatalf("parse evidence list: %v\nraw=%s", err, evidenceListOut)
	}
	if evidenceList.Kind != "evidence_list" || len(evidenceList.Items) != 2 {
		t.Fatalf("unexpected evidence list payload: %#v", evidenceList)
	}
	runEvidenceListOut := must("run", "evidence", "list", dispatch.Payload.RunID, "--json")
	var runEvidenceList struct {
		Kind  string `json:"kind"`
		Items []struct {
			EvidenceID string `json:"evidence_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(runEvidenceListOut), &runEvidenceList); err != nil {
		t.Fatalf("parse run evidence list: %v\nraw=%s", err, runEvidenceListOut)
	}
	if runEvidenceList.Kind != "evidence_list" || len(runEvidenceList.Items) != len(evidenceList.Items) {
		t.Fatalf("unexpected run evidence list payload: %#v", runEvidenceList)
	}

	evidenceViewOut := must("evidence", "view", evidence.Payload.EvidenceID, "--json")
	var evidenceView struct {
		Kind    string `json:"kind"`
		Payload struct {
			EvidenceID string `json:"evidence_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(evidenceViewOut), &evidenceView); err != nil {
		t.Fatalf("parse evidence view: %v\nraw=%s", err, evidenceViewOut)
	}
	if evidenceView.Kind != "evidence_detail" || evidenceView.Payload.EvidenceID != evidence.Payload.EvidenceID {
		t.Fatalf("unexpected evidence view payload: %#v", evidenceView)
	}

	handoffViewOut := must("handoff", "view", handoff.Payload.HandoffID, "--json")
	var handoffView struct {
		Kind    string `json:"kind"`
		Payload struct {
			HandoffID string `json:"handoff_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(handoffViewOut), &handoffView); err != nil {
		t.Fatalf("parse handoff view: %v\nraw=%s", err, handoffViewOut)
	}
	if handoffView.Kind != "handoff_detail" || handoffView.Payload.HandoffID != handoff.Payload.HandoffID {
		t.Fatalf("unexpected handoff view payload: %#v", handoffView)
	}

	exportOut := must("handoff", "export", handoff.Payload.HandoffID, "--md")
	if !strings.Contains(exportOut, "## Evidence") || !strings.Contains(exportOut, "## Next") {
		t.Fatalf("unexpected handoff export: %s", exportOut)
	}
	if _, err := os.Stat(filepath.Join(".tracker", "handoffs", handoff.Payload.HandoffID+".md")); err != nil {
		t.Fatalf("expected stored handoff markdown to exist: %v", err)
	}
}

func TestRunEvidenceArtifactsKeepDistinctCopiesPerEvidence(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Collect proof", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner")

	writeGitFile(t, "proof.log", "first artifact\n")
	firstOut := must("run", "evidence", "add", dispatch.Payload.RunID, "--type", "artifact_ref", "--title", "first", "--artifact", "proof.log", "--actor", "human:owner", "--json")
	var first struct {
		Payload struct {
			EvidenceID   string `json:"evidence_id"`
			ArtifactPath string `json:"artifact_path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(firstOut), &first); err != nil {
		t.Fatalf("parse first evidence output: %v\nraw=%s", err, firstOut)
	}

	writeGitFile(t, "proof.log", "second artifact\n")
	secondOut := must("run", "evidence", "add", dispatch.Payload.RunID, "--type", "artifact_ref", "--title", "second", "--artifact", "proof.log", "--actor", "human:owner", "--json")
	var second struct {
		Payload struct {
			EvidenceID   string `json:"evidence_id"`
			ArtifactPath string `json:"artifact_path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(secondOut), &second); err != nil {
		t.Fatalf("parse second evidence output: %v\nraw=%s", err, secondOut)
	}
	if first.Payload.ArtifactPath == second.Payload.ArtifactPath {
		t.Fatalf("expected unique artifact paths, got %q", first.Payload.ArtifactPath)
	}
	firstBytes, err := os.ReadFile(first.Payload.ArtifactPath)
	if err != nil {
		t.Fatalf("read first artifact: %v", err)
	}
	secondBytes, err := os.ReadFile(second.Payload.ArtifactPath)
	if err != nil {
		t.Fatalf("read second artifact: %v", err)
	}
	if string(firstBytes) != "first artifact\n" {
		t.Fatalf("expected first artifact bytes to remain intact, got %q", string(firstBytes))
	}
	if string(secondBytes) != "second artifact\n" {
		t.Fatalf("expected second artifact bytes to remain intact, got %q", string(secondBytes))
	}
}
