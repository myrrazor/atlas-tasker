package service

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestViewStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := ViewStore{Root: root}
	view := contracts.SavedView{
		Name:  "ready-work",
		Title: "Ready Work",
		Kind:  contracts.SavedViewKindSearch,
		Query: "status=ready",
	}
	if err := store.SaveView(view); err != nil {
		t.Fatalf("save view: %v", err)
	}
	loaded, err := store.LoadView("ready-work")
	if err != nil {
		t.Fatalf("load view: %v", err)
	}
	if loaded.Name != "ready-work" || loaded.Query != "status=ready" || loaded.Title != "Ready Work" {
		t.Fatalf("unexpected loaded view: %#v", loaded)
	}
	views, err := store.ListViews()
	if err != nil {
		t.Fatalf("list views: %v", err)
	}
	if len(views) != 1 || views[0].Name != "ready-work" {
		t.Fatalf("unexpected listed views: %#v", views)
	}
	if err := store.DeleteView("ready-work"); err != nil {
		t.Fatalf("delete view: %v", err)
	}
	views, err = store.ListViews()
	if err != nil {
		t.Fatalf("list views after delete: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("expected empty views after delete, got %#v", views)
	}
}
