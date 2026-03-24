package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type WriteLockManager interface {
	Acquire(ctx context.Context, purpose string) (func() error, error)
}

type FileLockManager struct {
	Root       string
	Wait       time.Duration
	RetryEvery time.Duration
}

type lockContextKey struct{}

type lockMetadata struct {
	PID        int    `json:"pid"`
	Purpose    string `json:"purpose"`
	AcquiredAt string `json:"acquired_at"`
}

func (m FileLockManager) Acquire(ctx context.Context, purpose string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wait := m.Wait
	if wait <= 0 {
		wait = 5 * time.Second
	}
	retryEvery := m.RetryEvery
	if retryEvery <= 0 {
		retryEvery = 50 * time.Millisecond
	}
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, wait)
	}

	if err := os.MkdirAll(storage.TrackerDir(m.Root), 0o755); err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, apperr.Wrap(apperr.CodeInternal, err, "create tracker dir for write lock")
	}
	path := filepath.Join(storage.TrackerDir(m.Root), "write.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, apperr.Wrap(apperr.CodeInternal, err, "open write lock")
	}

	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			if cancel != nil {
				cancel()
			}
			_ = file.Close()
			return nil, apperr.Wrap(apperr.CodeInternal, err, "acquire write lock")
		}
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
			}
			_ = file.Close()
			return nil, apperr.Wrap(apperr.CodeBusy, ctx.Err(), "workspace is busy: could not acquire write lock")
		case <-time.After(retryEvery):
		}
	}

	meta := lockMetadata{PID: os.Getpid(), Purpose: purpose, AcquiredAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if raw, err := json.Marshal(meta); err == nil {
		if err := file.Truncate(0); err == nil {
			_, _ = file.Seek(0, 0)
			_, _ = file.Write(raw)
		}
	}

	return func() error {
		if cancel != nil {
			cancel()
		}
		unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		closeErr := file.Close()
		if unlockErr != nil {
			return fmt.Errorf("unlock write lock: %w", unlockErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close write lock: %w", closeErr)
		}
		return nil
	}, nil
}

func withWriteLock[T any](ctx context.Context, manager WriteLockManager, purpose string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if manager == nil {
		return fn(ctx)
	}
	if _, ok := ctx.Value(lockContextKey{}).(bool); ok {
		return fn(ctx)
	}
	unlock, err := manager.Acquire(ctx, purpose)
	if err != nil {
		return zero, err
	}
	defer func() {
		_ = unlock()
	}()
	lockedCtx := context.WithValue(ctx, lockContextKey{}, true)
	return fn(lockedCtx)
}

func WithWriteLock(ctx context.Context, manager WriteLockManager, purpose string, fn func(context.Context) error) error {
	_, err := withWriteLock(ctx, manager, purpose, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}
