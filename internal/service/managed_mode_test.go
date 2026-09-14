package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestManagedModeFileHelper(t *testing.T) {
	root := t.TempDir()
	got := storage.ManagedModeFile(root)
	want := filepath.Join(root, ".tracker", "managed-mode.json")
	if got != want {
		t.Fatalf("ManagedModeFile = %q, want %q", got, want)
	}
}

func TestLoadManagedModeMissingUsesRecommendedDefault(t *testing.T) {
	root := t.TempDir()
	policy, present, err := LoadManagedModePolicy(root)
	if err != nil {
		t.Fatalf("load missing policy: %v", err)
	}
	if present {
		t.Fatal("missing file should not be present")
	}
	if policy != contracts.DefaultManagedModePolicy() {
		t.Fatalf("missing file should yield recommended default, got %#v", policy)
	}
}

func TestSaveAndLoadManagedModeRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeReviewGate}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	actions := NewActionService(root, mdstore.ProjectStore{RootDir: root}, mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}, &eventstore.Log{RootDir: root}, nil, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	policy, err := contracts.ManagedModePolicyForMode(contracts.ManagedModeGuidance)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if err := actions.SaveManagedModePolicy(context.Background(), policy); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, present, err := LoadManagedModePolicy(root)
	if err != nil || !present {
		t.Fatalf("reload: present=%v err=%v", present, err)
	}
	if loaded != policy {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", loaded, policy)
	}
}

func TestCaptureDecisionNeverCreatesTicketForStatusOrReadOnly(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	queries := NewQueryService(root, mdstore.ProjectStore{RootDir: root}, mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}, &eventstore.Log{RootDir: root}, nil, func() time.Time { return now })
	for _, intent := range []contracts.WorkIntent{
		contracts.WorkIntentStatusQuery,
		contracts.WorkIntentReadOnlyExplanation,
		contracts.WorkIntentCasualDiscussion,
		contracts.WorkIntentSimpleQuestion,
		contracts.WorkIntentTrackingExcluded,
	} {
		decision, err := queries.CaptureDecisionFor(intent)
		if err != nil {
			t.Fatalf("decision %s: %v", intent, err)
		}
		if decision != contracts.CaptureNoTicket {
			t.Fatalf("intent %s should be no_ticket, got %s", intent, decision)
		}
	}
	material, err := queries.CaptureDecisionFor(contracts.WorkIntentMaterialWork)
	if err != nil {
		t.Fatalf("material: %v", err)
	}
	if material != contracts.CaptureAttachOrCreate {
		t.Fatalf("recommended material work should attach_or_create, got %s", material)
	}
}

func TestManagedModeViewHonorsWorkspaceCompletionAndLocalDelivery(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeDualGate, RequiredReviewer: "agent:reviewer-1"}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	actions := NewActionService(root, mdstore.ProjectStore{RootDir: root}, mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}, &eventstore.Log{RootDir: root}, nil, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	policy, err := contracts.ManagedModePolicyForMode(contracts.ManagedModeDelivery)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if err := actions.SaveManagedModePolicy(context.Background(), policy); err != nil {
		t.Fatalf("save: %v", err)
	}
	queries := NewQueryService(root, mdstore.ProjectStore{RootDir: root}, mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}, &eventstore.Log{RootDir: root}, nil, func() time.Time { return now })
	cloned, err := queries.ManagedModeView(false)
	if err != nil {
		t.Fatalf("view without local delivery: %v", err)
	}
	if cloned.DeclaredMode != contracts.ManagedModeDelivery || cloned.EffectiveMode != contracts.ManagedModeManaged {
		t.Fatalf("clone must follow delivery as managed, declared=%s effective=%s", cloned.DeclaredMode, cloned.EffectiveMode)
	}
	if cloned.CompletionMode != contracts.CompletionModeDualGate {
		t.Fatalf("completion must follow workspace, got %s", cloned.CompletionMode)
	}
	if cloned.RequiredReviewer != "agent:reviewer-1" {
		t.Fatalf("required reviewer = %s", cloned.RequiredReviewer)
	}
	enabled, err := queries.ManagedModeView(true)
	if err != nil {
		t.Fatalf("view with local delivery: %v", err)
	}
	if enabled.EffectiveMode != contracts.ManagedModeDelivery {
		t.Fatalf("local delivery should take effect, got %s", enabled.EffectiveMode)
	}
}

func TestCollectExportFilesIncludesManagedMode(t *testing.T) {
	root := t.TempDir()
	path := storage.ManagedModeFile(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, err := contracts.DefaultManagedModePolicy().Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	files, err := collectExportFiles(root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	joined := strings.Join(files, "\n")
	if !strings.Contains(joined, ".tracker/managed-mode.json") {
		t.Fatalf("export candidate list omitted managed-mode.json:\n%s", joined)
	}
	safe := backupRestoreSafeFiles(files)
	found := false
	for _, rel := range safe {
		if rel == ".tracker/managed-mode.json" {
			found = true
		}
	}
	if !found {
		t.Fatalf("restore-safe allowlist dropped managed-mode.json: %v", safe)
	}
}
