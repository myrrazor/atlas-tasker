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
