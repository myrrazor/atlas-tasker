package markdown

import (
	"context"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestProjectStoreCreateListGet(t *testing.T) {
	root := t.TempDir()
	store := ProjectStore{RootDir: root}

	project := contracts.Project{Key: "APP", Name: "App Project", CreatedAt: time.Now().UTC()}
	if err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	if err := store.CreateProject(context.Background(), project); err == nil {
		t.Fatal("expected duplicate project create to fail")
	}

	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list projects failed: %v", err)
	}
	if len(projects) != 1 || projects[0].Key != "APP" {
		t.Fatalf("unexpected projects list: %#v", projects)
	}

	loaded, err := store.GetProject(context.Background(), "APP")
	if err != nil {
		t.Fatalf("get project failed: %v", err)
	}
	if loaded.Name != project.Name {
		t.Fatalf("unexpected project name: %s", loaded.Name)
	}
}

func TestProjectStoreUpdatePersistsDefaults(t *testing.T) {
	root := t.TempDir()
	store := ProjectStore{RootDir: root}
	project := contracts.Project{Key: "APP", Name: "App Project", CreatedAt: time.Now().UTC()}
	if err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	project.Defaults = contracts.ProjectDefaults{
		CompletionMode:   contracts.CompletionModeReviewGate,
		LeaseTTLMinutes:  90,
		AllowedWorkers:   []contracts.Actor{"agent:builder-1"},
		RequiredReviewer: contracts.Actor("human:owner"),
	}
	if err := store.UpdateProject(context.Background(), project); err != nil {
		t.Fatalf("update project failed: %v", err)
	}
	loaded, err := store.GetProject(context.Background(), "APP")
	if err != nil {
		t.Fatalf("get project failed: %v", err)
	}
	if loaded.Defaults.CompletionMode != contracts.CompletionModeReviewGate {
		t.Fatalf("unexpected completion mode: %s", loaded.Defaults.CompletionMode)
	}
	if loaded.Defaults.LeaseTTLMinutes != 90 {
		t.Fatalf("unexpected lease ttl: %d", loaded.Defaults.LeaseTTLMinutes)
	}
	if len(loaded.Defaults.AllowedWorkers) != 1 || loaded.Defaults.AllowedWorkers[0] != contracts.Actor("agent:builder-1") {
		t.Fatalf("unexpected allowed workers: %#v", loaded.Defaults.AllowedWorkers)
	}
}
