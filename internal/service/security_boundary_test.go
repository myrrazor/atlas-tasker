package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestV17SyncBoundarySecurityPaths(t *testing.T) {
	syncable := []string{
		".tracker/security/keys/public/key_1.md",
		".tracker/security/revocations/rev_1.md",
		".tracker/security/signatures/sig_1.json",
		".tracker/governance/policies/default.toml",
		".tracker/governance/packs/security.toml",
		".tracker/classification/labels/APP.md",
		".tracker/classification/policies/default.toml",
		".tracker/redaction/rules/default.toml",
	}
	for _, path := range syncable {
		if !isSyncableRelativePath(path) {
			t.Fatalf("expected %s to be syncable", path)
		}
	}
	localOnly := []string{
		".tracker/security/keys/private/key_1.json",
		".tracker/security/trust/trust_1.md",
		".tracker/redaction/previews/preview_1.json",
		".tracker/backups/snapshots/backup_1.tar.gz",
		".tracker/goal/manifests/goal_1.json",
	}
	for _, path := range localOnly {
		if isSyncableRelativePath(path) {
			t.Fatalf("expected %s to stay local-only", path)
		}
	}
}

func TestSyncPublicationCarriesRedactionPreviewBinding(t *testing.T) {
	publication := SyncPublication{
		WorkspaceID:        "workspace_1",
		BundleID:           "syncbundle_1",
		Format:             "sync_bundle_v1",
		ArtifactName:       "syncbundle_1.tar.gz",
		ManifestName:       "syncbundle_1.manifest.json",
		ChecksumName:       "syncbundle_1.sha256",
		FileCount:          1,
		RedactionPreviewID: "redact_1",
	}
	raw, err := json.Marshal(publication)
	if err != nil {
		t.Fatalf("marshal sync publication: %v", err)
	}
	if !strings.Contains(string(raw), `"redaction_preview_id":"redact_1"`) {
		t.Fatalf("redaction preview binding missing from sync publication JSON: %s", raw)
	}
}

func TestCollectSyncableFilesSkipsSymlinkedSecurityRecords(t *testing.T) {
	root := t.TempDir()
	privateDir := storage.PrivateKeysDir(root)
	publicDir := storage.PublicKeysDir(root)
	if err := os.MkdirAll(privateDir, 0o700); err != nil {
		t.Fatalf("create private dir: %v", err)
	}
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		t.Fatalf("create public dir: %v", err)
	}
	privateKey := filepath.Join(privateDir, "key_1.json")
	if err := os.WriteFile(privateKey, []byte("secret-key-bytes"), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	if err := os.Symlink(privateKey, filepath.Join(publicDir, "leak.md")); err != nil {
		t.Skipf("symlinks unavailable in test environment: %v", err)
	}

	files, err := collectSyncableFiles(root)
	if err != nil {
		t.Fatalf("collect sync files: %v", err)
	}
	for _, file := range files {
		if file == ".tracker/security/keys/public/leak.md" {
			t.Fatalf("symlinked security record must not be syncable: %v", files)
		}
	}
}
