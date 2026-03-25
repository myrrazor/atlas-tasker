package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestFileLockManagerBlocksCompetingWriters(t *testing.T) {
	root := t.TempDir()
	locks := FileLockManager{Root: root, Wait: 100 * time.Millisecond, RetryEvery: 10 * time.Millisecond}

	unlock, err := locks.Acquire(context.Background(), "first")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer func() { _ = unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := locks.Acquire(ctx, "second"); err == nil {
		t.Fatal("expected competing lock acquisition to fail")
	} else if apperr.CodeOf(err) != apperr.CodeBusy {
		t.Fatalf("expected busy error, got %v (%s)", err, apperr.CodeOf(err))
	}
}

func TestWithWriteLockAllowsNestedMutationScopes(t *testing.T) {
	root := t.TempDir()
	locks := FileLockManager{Root: root, Wait: time.Second, RetryEvery: 10 * time.Millisecond}
	called := 0

	_, err := withWriteLock(context.Background(), locks, "outer", func(ctx context.Context) (struct{}, error) {
		called++
		_, err := withWriteLock(ctx, locks, "inner", func(context.Context) (struct{}, error) {
			called++
			return struct{}{}, nil
		})
		return struct{}{}, err
	})
	if err != nil {
		t.Fatalf("nested lock scopes: %v", err)
	}
	if called != 2 {
		t.Fatalf("expected both scopes to run, got %d", called)
	}
}

func TestFileLockManagerCanonicalizesPathAliases(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	primary := FileLockManager{Root: root, Wait: time.Second, RetryEvery: 10 * time.Millisecond}
	aliasLock := FileLockManager{Root: alias, Wait: 60 * time.Millisecond, RetryEvery: 10 * time.Millisecond}

	unlock, err := primary.Acquire(context.Background(), "primary")
	if err != nil {
		t.Fatalf("acquire primary lock: %v", err)
	}
	defer func() { _ = unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := aliasLock.Acquire(ctx, "alias"); err == nil {
		t.Fatal("expected alias path to contend on the same lock")
	} else if apperr.CodeOf(err) != apperr.CodeBusy {
		t.Fatalf("expected busy error for alias path, got %v (%s)", err, apperr.CodeOf(err))
	}
}

func TestFileLockManagerRecoversFromStaleMetadata(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := CanonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("canonical root: %v", err)
	}
	lockPath := filepath.Join(storage.TrackerDir(canonicalRoot), "write.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir tracker dir: %v", err)
	}
	hostname, _ := os.Hostname()
	stale := lockMetadata{
		PID:        999999,
		Hostname:   hostname,
		Root:       canonicalRoot,
		Purpose:    "stale writer",
		AcquiredAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal stale metadata: %v", err)
	}
	if err := os.WriteFile(lockPath, raw, 0o644); err != nil {
		t.Fatalf("seed stale metadata: %v", err)
	}

	locks := FileLockManager{Root: root, Wait: time.Second, RetryEvery: 10 * time.Millisecond}
	unlock, err := locks.Acquire(context.Background(), "fresh writer")
	if err != nil {
		t.Fatalf("acquire lock with stale metadata: %v", err)
	}
	defer func() { _ = unlock() }()

	updatedRaw, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read updated lock file: %v", err)
	}
	var updated lockMetadata
	if err := json.Unmarshal(updatedRaw, &updated); err != nil {
		t.Fatalf("decode updated metadata: %v", err)
	}
	if updated.PID != os.Getpid() {
		t.Fatalf("expected current pid in lock metadata, got %#v", updated)
	}
	if updated.Root != canonicalRoot {
		t.Fatalf("expected canonical root in lock metadata, got %#v", updated)
	}
}

func TestCanonicalWorkspaceRootFallsBackWhenNoSymlinkResolutionIsNeeded(t *testing.T) {
	root := t.TempDir()
	got, err := CanonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("canonical root: %v", err)
	}
	if got == "" {
		t.Fatal("expected canonical root to be non-empty")
	}
	if runtime.GOOS != "windows" && got[0] != '/' {
		t.Fatalf("expected absolute canonical root, got %q", got)
	}
}
