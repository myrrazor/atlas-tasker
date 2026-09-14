package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

const (
	userStateDirName             = "atlas-tasker"
	macOSAppSupportName          = "Atlas Tasker"
	BackupStateConflictIssueCode = "backup_state_conflict"

	backupStateSourceCanonical = "canonical"
	backupStateSourceLegacy    = "legacy"
	backupStateSourceExplicit  = "explicit"
	backupStateSourceXDG       = "xdg"
)

// BackupStateDirOptions selects the machine-local backup state root for one
// workspace. Getenv/GOOS are injectable so tests never touch the owner Home.
type BackupStateDirOptions struct {
	Home        string
	StateDir    string
	WorkspaceID string
	Getenv      func(string) string
	GOOS        string
}

// BackupStateResolution is the chosen backup state root plus the unused
// candidate, for doctor/status. Source is canonical, legacy, explicit, or xdg.
type BackupStateResolution struct {
	StateDir         string
	Source           string
	CanonicalDir     string
	LegacyDir        string
	ReplicaID        string
	Conflict         bool
	CanonicalReplica string
	LegacyReplica    string
}

// BackupStateConflictError is returned when canonical App Support and the
// v1.14 ~/.local/state/atlas-tasker tree both have backup lineage for the
// same workspace. Atlas will not pick, merge, or move either copy.
type BackupStateConflictError struct {
	WorkspaceID      string
	CanonicalDir     string
	LegacyDir        string
	CanonicalReplica string
	LegacyReplica    string
}

func (e *BackupStateConflictError) Error() string {
	if e == nil {
		return "duplicate backup state"
	}
	return fmt.Sprintf(
		"duplicate backup state for workspace %s; canonical %s replica %s; legacy %s replica %s; run tracker doctor (Atlas will not merge or move backup data)",
		e.WorkspaceID,
		e.CanonicalDir,
		displayReplica(e.CanonicalReplica),
		e.LegacyDir,
		displayReplica(e.LegacyReplica),
	)
}

func displayReplica(id string) string {
	if strings.TrimSpace(id) == "" {
		return "(none)"
	}
	return id
}

func IsBackupStateConflict(err error) bool {
	var conflict *BackupStateConflictError
	return errors.As(err, &conflict)
}

// CanonicalUserStateDir is the machine default, matching setup.DefaultStateDir
// (duplicated so service does not import setup).
func CanonicalUserStateDir(home string, getenv func(string) string) (string, error) {
	return canonicalUserStateDir(home, getenv, runtime.GOOS)
}

func canonicalUserStateDir(home string, getenv func(string) string, goos string) (string, error) {
	getenv = getenvOr(getenv)
	if goos == "" {
		goos = runtime.GOOS
	}
	if xdg := strings.TrimSpace(getenv("XDG_STATE_HOME")); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
		}
		return filepath.Clean(filepath.Join(xdg, userStateDirName)), nil
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home is required to resolve the Atlas state directory")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("home must be an absolute path")
	}
	home = filepath.Clean(home)
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", macOSAppSupportName), nil
	}
	return filepath.Join(home, ".local", "state", userStateDirName), nil
}

func legacyMacOSUserStateDir(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home is required to resolve the Atlas state directory")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("home must be an absolute path")
	}
	return filepath.Join(filepath.Clean(home), ".local", "state", userStateDirName), nil
}

func getenvOr(fn func(string) string) func(string) string {
	if fn == nil {
		return os.Getenv
	}
	return fn
}

// ResolveBackupStateDir is the hook Core/CLI/TUI must use for backup auto.json,
// replica, ledger, targets, outbox, and checkpoint paths.
func ResolveBackupStateDir(opts BackupStateDirOptions) (string, error) {
	res, err := ResolveBackupState(opts)
	if err != nil {
		return "", err
	}
	return res.StateDir, nil
}

func ResolveBackupState(opts BackupStateDirOptions) (BackupStateResolution, error) {
	getenv := getenvOr(opts.Getenv)
	goos := strings.TrimSpace(opts.GOOS)
	if goos == "" {
		goos = runtime.GOOS
	}
	xdgSet := strings.TrimSpace(getenv("XDG_STATE_HOME")) != ""
	canonical, err := canonicalUserStateDir(opts.Home, getenv, goos)
	if err != nil && strings.TrimSpace(opts.StateDir) == "" {
		return BackupStateResolution{}, err
	}

	explicit := strings.TrimSpace(opts.StateDir)
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			return BackupStateResolution{}, fmt.Errorf("state dir must be absolute")
		}
		explicit = filepath.Clean(explicit)
	}

	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	if workspaceID != "" && !validBackupWorkspaceID(workspaceID) {
		return BackupStateResolution{}, apperr.New(apperr.CodeInvalidInput, "workspace identity is not a portable backup id")
	}

	res := BackupStateResolution{CanonicalDir: canonical}
	if goos == "darwin" && !xdgSet && strings.TrimSpace(opts.Home) != "" {
		if legacy, err := legacyMacOSUserStateDir(opts.Home); err == nil && !sameStatePath(legacy, canonical) {
			res.LegacyDir = legacy
		}
	}

	if explicit != "" && (canonical == "" || !sameStatePath(explicit, canonical)) {
		res.StateDir = explicit
		res.Source = backupStateSourceExplicit
		if workspaceID != "" {
			res.ReplicaID = lineageReplicaID(explicit, workspaceID)
		}
		return res, nil
	}

	if canonical == "" {
		return BackupStateResolution{}, fmt.Errorf("home is required to resolve the Atlas state directory")
	}

	if xdgSet {
		res.StateDir = canonical
		res.Source = backupStateSourceXDG
		if workspaceID != "" {
			res.ReplicaID = lineageReplicaID(canonical, workspaceID)
		}
		return res, nil
	}

	if res.LegacyDir == "" || workspaceID == "" {
		res.StateDir = canonical
		res.Source = backupStateSourceCanonical
		if workspaceID != "" {
			res.ReplicaID = lineageReplicaID(canonical, workspaceID)
		}
		return res, nil
	}

	canonicalHas, err := backupLineagePresent(canonical, workspaceID)
	if err != nil {
		return BackupStateResolution{}, err
	}
	legacyHas, err := backupLineagePresent(res.LegacyDir, workspaceID)
	if err != nil {
		return BackupStateResolution{}, err
	}
	res.CanonicalReplica = lineageReplicaID(canonical, workspaceID)
	res.LegacyReplica = lineageReplicaID(res.LegacyDir, workspaceID)

	switch {
	case canonicalHas && legacyHas:
		conflict := &BackupStateConflictError{
			WorkspaceID:      workspaceID,
			CanonicalDir:     canonical,
			LegacyDir:        res.LegacyDir,
			CanonicalReplica: res.CanonicalReplica,
			LegacyReplica:    res.LegacyReplica,
		}
		res.Conflict = true
		res.StateDir = ""
		return res, apperr.Wrap(apperr.CodeRepairNeeded, conflict, "%s", conflict.Error())
	case legacyHas:
		res.StateDir = res.LegacyDir
		res.Source = backupStateSourceLegacy
		res.ReplicaID = res.LegacyReplica
		return res, nil
	default:
		res.StateDir = canonical
		res.Source = backupStateSourceCanonical
		res.ReplicaID = res.CanonicalReplica
		return res, nil
	}
}

func sameStatePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func backupLineagePresent(stateDir, workspaceID string) (bool, error) {
	if strings.TrimSpace(stateDir) == "" || !validBackupWorkspaceID(workspaceID) {
		return false, nil
	}
	paths := backupStatePaths(stateDir, workspaceID)
	for _, p := range []string{paths.Replica, paths.Ledger, paths.Targets, paths.Auto, paths.Outbox, paths.Schedule, paths.Repo} {
		if _, err := os.Lstat(p); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

func lineageReplicaID(stateDir, workspaceID string) string {
	if strings.TrimSpace(stateDir) == "" || !validBackupWorkspaceID(workspaceID) {
		return ""
	}
	paths := backupStatePaths(stateDir, workspaceID)
	if ident, err := loadReplicaIdentity(paths.Replica); err == nil {
		if id := strings.TrimSpace(ident.ReplicaID); id != "" {
			return id
		}
	}
	if ledger, err := loadLedger(paths.Ledger); err == nil {
		return strings.TrimSpace(ledger.ReplicaID)
	}
	return ""
}

func (s *ActionService) backupStateOpts(workspaceID string) BackupStateDirOptions {
	if s == nil {
		return BackupStateDirOptions{WorkspaceID: workspaceID}
	}
	return BackupStateDirOptions{
		Home:        s.Home,
		StateDir:    s.StateDir,
		WorkspaceID: workspaceID,
		Getenv:      s.Getenv,
		GOOS:        s.GOOS,
	}
}

func (s *QueryService) backupStateOpts(workspaceID string) BackupStateDirOptions {
	if s == nil {
		return BackupStateDirOptions{WorkspaceID: workspaceID}
	}
	return BackupStateDirOptions{
		Home:        s.Home,
		StateDir:    s.StateDir,
		WorkspaceID: workspaceID,
		Getenv:      s.Getenv,
		GOOS:        s.GOOS,
	}
}

func (s *ActionService) resolveBackupStateDir(workspaceID string) (string, error) {
	return ResolveBackupStateDir(s.backupStateOpts(workspaceID))
}

// AttachUserStateEnv copies test/production Getenv and GOOS onto attached
// services. Call after AttachUserState. Empty goos means runtime.GOOS.
func AttachUserStateEnv(actions *ActionService, queries *QueryService, getenv func(string) string, goos string) {
	if actions != nil {
		actions.Getenv = getenv
		actions.GOOS = goos
	}
	if queries != nil {
		queries.Getenv = getenv
		queries.GOOS = goos
	}
}
