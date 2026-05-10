package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestTemplateRejectsPathDerivedNames(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(filepath.Join(storage.TrackerDir(root), "templates"), 0o755); err != nil {
		t.Fatalf("create templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.md"), []byte("do not read me"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}

	queries := NewQueryService(root, nil, nil, nil, nil, nil)
	if _, err := queries.Template(context.Background(), "../secret"); err == nil || !strings.Contains(err.Error(), "template name must match") {
		t.Fatalf("expected traversal template name to be rejected, got %v", err)
	}
	if _, err := queries.Template(context.Background(), "bad/name"); err == nil || !strings.Contains(err.Error(), "template name must match") {
		t.Fatalf("expected slash template name to be rejected, got %v", err)
	}
}
