package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestCollaboratorStoreRejectsPathTraversalID(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	store := CollaboratorStore{Root: root}
	err := store.SaveCollaborator(context.Background(), contracts.CollaboratorProfile{
		CollaboratorID: "../EVIL",
		DisplayName:    "Evil",
		Status:         contracts.CollaboratorStatusActive,
		TrustState:     contracts.CollaboratorTrustStateUntrusted,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		SchemaVersion:  contracts.CurrentSchemaVersion,
	})
	if err == nil {
		t.Fatal("expected collaborator traversal id to be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(root, ".tracker", "EVIL.md")); !os.IsNotExist(statErr) {
		t.Fatalf("collaborator traversal wrote outside collaborators dir, stat err=%v", statErr)
	}
	if _, err := store.LoadCollaborator(context.Background(), "../EVIL"); err == nil {
		t.Fatal("expected load collaborator traversal to be rejected")
	}
}
