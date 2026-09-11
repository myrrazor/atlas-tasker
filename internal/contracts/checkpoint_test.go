package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckpointManifestRoundTripAndGoldenHash(t *testing.T) {
	created := time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC)
	manifest, err := CheckpointManifest{
		Format:       CheckpointManifestFormat,
		WorkspaceID:  "ws-checkpoint-test",
		ReplicaID:    "replica-1",
		AtlasVersion: "dev",
		CreatedAt:    created,
		EventWatermarks: map[string]int64{
			"APP":       4,
			"workspace": 2,
		},
		SchemaVersion: CurrentSchemaVersion,
		Files: []CheckpointFile{
			{Path: "projects/APP/APP.md", SHA256: strings.Repeat("a", 64), Size: 12},
			{Path: ".tracker/events/2026-09.jsonl", SHA256: strings.Repeat("b", 64), Size: 40},
		},
	}.Canonicalize()
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if manifest.ManifestSHA256 == "" || manifest.CanonicalTreeSHA256 == "" {
		t.Fatal("hashes must be populated")
	}
	if manifest.FileCount != 2 {
		t.Fatalf("file_count=%d", manifest.FileCount)
	}
	raw, err := EncodeCheckpointManifest(manifest)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	parsed, err := ParseCheckpointManifest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.ManifestSHA256 != manifest.ManifestSHA256 || parsed.CanonicalTreeSHA256 != manifest.CanonicalTreeSHA256 {
		t.Fatalf("round trip hashes diverged")
	}
	goldenPath := filepath.Join("testdata", "checkpoint_manifest_v1.golden")
	if os.Getenv("ATLAS_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (set ATLAS_UPDATE_GOLDEN=1 to create): %v", err)
	}
	if string(raw) != string(want) {
		t.Fatalf("manifest encoding drifted\n got %s\nwant %s", raw, want)
	}
	tampered := strings.Replace(string(raw), `"atlas_version":"dev"`, `"atlas_version":"other"`, 1)
	if _, err := ParseCheckpointManifest([]byte(tampered)); err == nil {
		t.Fatal("edited manifest must fail hash verification")
	}
}

func TestCanonicalTreeHashIgnoresManifestEntry(t *testing.T) {
	files := []CheckpointFile{
		{Path: "projects/APP/APP.md", SHA256: strings.Repeat("c", 64), Size: 1},
		{Path: CheckpointManifestName, SHA256: strings.Repeat("d", 64), Size: 9},
	}
	if CanonicalTreeHash(files) != CanonicalTreeHash(files[:1]) {
		t.Fatal("manifest blob must not participate in the canonical tree hash")
	}
}

func TestLogicalCheckpointIDStableAndSensitive(t *testing.T) {
	marks := map[string]int64{"workspace": 1, "APP": 2}
	a := LogicalCheckpointID("ws", "r1", "tree", "local", marks)
	b := LogicalCheckpointID("ws", "r1", "tree", "local", map[string]int64{"APP": 2, "workspace": 1})
	if a != b || a == "" {
		t.Fatalf("watermark key order must not change the id: %s %s", a, b)
	}
	if LogicalCheckpointID("ws", "r1", "other", "local", marks) == a {
		t.Fatal("tree hash must change the logical id")
	}
}

func TestTriggersAutomaticCheckpointExcludesBookkeeping(t *testing.T) {
	for _, eventType := range []EventType{
		EventBackupCreated, EventBackupVerified, EventBackupRestorePlanned,
		EventSyncStarted, EventSyncCompleted, EventSyncFailed, EventBundleVerified,
		EventRemoteAdded, EventAgentWorkAvailable,
	} {
		if TriggersAutomaticCheckpoint(eventType) {
			t.Fatalf("%s must not mark the backup outbox", eventType)
		}
	}
	if !TriggersAutomaticCheckpoint(EventTicketMoved) || !TriggersAutomaticCheckpoint(EventBackupRestored) {
		t.Fatal("ticket moves and restore apply must mark the outbox")
	}
}
