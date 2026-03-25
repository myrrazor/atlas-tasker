package service

import (
	"context"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
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
