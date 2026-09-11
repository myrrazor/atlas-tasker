package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

type setupLockMeta struct {
	PID        int    `json:"pid"`
	Hostname   string `json:"hostname"`
	Purpose    string `json:"purpose"`
	AcquiredAt string `json:"acquired_at"`
}

// AcquireSetupLock takes the machine-local setup lock. It does not wait: a
// held lock is busy so the caller can fail the provider and roll back without
// sitting on the lock.
func AcquireSetupLock(stateDir string, purpose string) (func() error, error) {
	if err := ensurePrivateDir(stateDir); err != nil {
		return nil, err
	}
	path := setupLockPath(stateDir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open setup lock: %w", err)
	}
	if err := os.Chmod(path, filePerm); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("chmod setup lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, apperr.Wrap(apperr.CodeBusy, err, "setup lock is held by another process")
	}
	host, _ := os.Hostname()
	meta := setupLockMeta{PID: os.Getpid(), Hostname: host, Purpose: purpose, AcquiredAt: time.Now().UTC().Format(time.RFC3339)}
	raw, _ := json.Marshal(meta)
	_ = file.Truncate(0)
	_, _ = file.WriteAt(append(raw, '\n'), 0)
	_ = file.Sync()
	released := false
	return func() error {
		if released {
			return nil
		}
		released = true
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		return file.Close()
	}, nil
}
