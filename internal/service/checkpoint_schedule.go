package service

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

const (
	scheduleMarker      = "Atlas-owned backup scheduler. Do not edit by hand."
	linuxServicePrefix  = "atlas-backup-"
	macPlistPrefix      = "com.atlas-tasker.backup."
)

type BackupSchedulePlan struct {
	Kind          string             `json:"kind"`
	GeneratedAt   any                `json:"generated_at"`
	Platform      string             `json:"platform"`
	BinaryPath    string             `json:"binary_path"`
	WorkspaceID   string             `json:"workspace_id"`
	WorkspaceRoot string             `json:"workspace_root"`
	UnitName      string             `json:"unit_name"`
	Files         []BackupScheduleFile `json:"files"`
	Consent       string             `json:"consent"`
	Notes         []string           `json:"notes,omitempty"`
}

type BackupScheduleFile struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Content string `json:"content"`
}

type BackupScheduleStatus struct {
	Kind        string   `json:"kind"`
	GeneratedAt any      `json:"generated_at"`
	State       string   `json:"state"`
	Platform    string   `json:"platform"`
	Installed   bool     `json:"installed"`
	Repair      bool     `json:"repair_required"`
	Notes       []string `json:"notes,omitempty"`
}

func (s *ActionService) scheduleRoot() (string, string, error) {
	if strings.TrimSpace(s.ScheduleHome) != "" {
		if runtime.GOOS == "darwin" {
			return s.ScheduleHome, "macos-launchd", nil
		}
		return s.ScheduleHome, "linux-systemd", nil
	}
	home := strings.TrimSpace(s.Home)
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", "watch-fallback", err
		}
		home = h
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents"), "macos-launchd", nil
	default:
		return filepath.Join(home, ".config", "systemd", "user"), "linux-systemd", nil
	}
}

func (s *ActionService) BackupSchedulePlan(ctx context.Context) (BackupSchedulePlan, error) {
	_ = ctx
	paths, workspaceID, err := s.backupPaths()
	if err != nil {
		return BackupSchedulePlan{}, err
	}
	_ = paths
	root, platform, err := s.scheduleRoot()
	if err != nil {
		return BackupSchedulePlan{}, err
	}
	binary, err := os.Executable()
	if err != nil {
		binary = ""
	}
	if strings.TrimSpace(s.TrackerBinary) != "" {
		binary = s.TrackerBinary
	}
	if !filepath.IsAbs(binary) {
		return BackupSchedulePlan{}, apperr.New(apperr.CodeInvalidInput, "scheduler requires an absolute tracker binary path")
	}
	workspaceRoot, err := filepath.Abs(s.Root)
	if err != nil {
		return BackupSchedulePlan{}, err
	}
	plan := BackupSchedulePlan{
		Kind:          "backup_schedule_plan",
		GeneratedAt:   s.now(),
		Platform:      platform,
		BinaryPath:    binary,
		WorkspaceID:   workspaceID,
		WorkspaceRoot: workspaceRoot,
		Consent:       "explicit --yes required; tracker init never installs a scheduler",
	}
	switch platform {
	case "macos-launchd":
		name := macPlistPrefix + workspaceID + ".plist"
		plan.UnitName = name
		plan.Files = []BackupScheduleFile{{
			Path:    filepath.Join(root, name),
			Mode:    "0600",
			Content: launchdPlist(binary, workspaceRoot, workspaceID),
		}}
	default:
		base := linuxServicePrefix + workspaceID
		plan.UnitName = base + ".timer"
		plan.Files = []BackupScheduleFile{
			{
				Path:    filepath.Join(root, base+".service"),
				Mode:    "0600",
				Content: systemdService(binary, workspaceRoot, workspaceID),
			},
			{
				Path:    filepath.Join(root, base+".timer"),
				Mode:    "0600",
				Content: systemdTimer(workspaceID),
			},
		}
	}
	plan.Notes = []string{"no root required", "exact binary and workspace identity are pinned", "uninstall does not delete backups"}
	return plan, nil
}

func (s *ActionService) BackupScheduleInstall(ctx context.Context, yes bool) (BackupScheduleStatus, error) {
	if !yes {
		return BackupScheduleStatus{}, apperr.New(apperr.CodeInvalidInput, "scheduler install requires explicit --yes consent")
	}
	plan, err := s.BackupSchedulePlan(ctx)
	if err != nil {
		return BackupScheduleStatus{}, err
	}
	for _, file := range plan.Files {
		if err := writeScheduleFile(file); err != nil {
			return BackupScheduleStatus{}, err
		}
	}
	paths, _, err := s.backupPaths()
	if err == nil {
		_ = atomicWriteJSON(paths.Schedule, map[string]any{
			"format": "atlas_backup_schedule_v1", "platform": plan.Platform, "unit": plan.UnitName,
			"binary": plan.BinaryPath, "workspace_root": plan.WorkspaceRoot, "installed_at": s.now(),
		})
		ledger, _ := loadLedger(paths.Ledger)
		ledger.SchedulerState = "installed"
		_ = atomicWriteJSON(paths.Ledger, ledger)
	}
	return s.BackupScheduleStatus(ctx)
}

func (s *ActionService) BackupScheduleRemove(ctx context.Context, yes bool) (BackupScheduleStatus, error) {
	if !yes {
		return BackupScheduleStatus{}, apperr.New(apperr.CodeInvalidInput, "scheduler remove requires --yes")
	}
	plan, err := s.BackupSchedulePlan(ctx)
	if err != nil {
		return BackupScheduleStatus{}, err
	}
	for _, file := range plan.Files {
		if err := removeAtlasScheduleFile(file.Path); err != nil {
			return BackupScheduleStatus{}, err
		}
	}
	paths, _, err := s.backupPaths()
	if err == nil {
		_ = os.Remove(paths.Schedule)
		ledger, _ := loadLedger(paths.Ledger)
		ledger.SchedulerState = "removed"
		_ = atomicWriteJSON(paths.Ledger, ledger)
	}
	status, err := s.BackupScheduleStatus(ctx)
	if err != nil {
		return BackupScheduleStatus{}, err
	}
	status.Notes = append(status.Notes, "backups_retained")
	return status, nil
}

func (s *ActionService) BackupScheduleStatus(ctx context.Context) (BackupScheduleStatus, error) {
	_ = ctx
	plan, err := s.BackupSchedulePlan(ctx)
	if err != nil {
		return BackupScheduleStatus{}, err
	}
	status := BackupScheduleStatus{Kind: "backup_schedule_status", GeneratedAt: s.now(), Platform: plan.Platform}
	installed := true
	repair := false
	for _, file := range plan.Files {
		info, err := os.Lstat(file.Path)
		if err != nil {
			installed = false
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return status, apperr.New(apperr.CodeConflict, "unexpected scheduler symlink: "+filepath.Base(file.Path))
		}
		if err := checkScheduleOwnership(file.Path); err != nil {
			return status, err
		}
		raw, err := os.ReadFile(file.Path)
		if err != nil {
			repair = true
			continue
		}
		body := string(raw)
		if !strings.Contains(body, scheduleMarker) {
			return status, apperr.New(apperr.CodeConflict, "scheduler file is not Atlas-owned")
		}
		if !strings.Contains(body, plan.WorkspaceID) || !strings.Contains(body, plan.BinaryPath) || !strings.Contains(body, plan.WorkspaceRoot) {
			repair = true
		}
	}
	status.Installed = installed && !repair
	status.Repair = repair
	switch {
	case repair:
		status.State = "repair_required"
	case installed:
		status.State = "installed"
	default:
		status.State = "not_installed"
	}
	return status, nil
}

func (s *ActionService) BackupScheduleRepair(ctx context.Context, yes bool) (BackupScheduleStatus, error) {
	return s.BackupScheduleInstall(ctx, yes)
}

func writeScheduleFile(file BackupScheduleFile) error {
	if _, err := os.Lstat(file.Path); err == nil {
		info, _ := os.Lstat(file.Path)
		if info != nil && info.Mode()&os.ModeSymlink != 0 {
			return apperr.New(apperr.CodeConflict, "unexpected scheduler symlink")
		}
		if err := checkScheduleOwnership(file.Path); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(file.Path), 0o700); err != nil {
		return err
	}
	tmp := file.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(file.Content), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, file.Path)
}

func removeAtlasScheduleFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeConflict, "refusing to remove unexpected scheduler symlink")
	}
	if err := checkScheduleOwnership(path); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(raw), scheduleMarker) {
		return apperr.New(apperr.CodeConflict, "refusing to remove a file Atlas does not own")
	}
	return os.Remove(path)
}

func checkScheduleOwnership(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil {
		return nil
	}
	uid, err := strconv.Atoi(current.Uid)
	if err != nil {
		return nil
	}
	if stat, ok := fileUID(info); ok && stat != uid {
		return apperr.New(apperr.CodeConflict, "scheduler file ownership is unexpected")
	}
	return nil
}

func systemdService(binary, workspace, workspaceID string) string {
	return fmt.Sprintf("[Unit]\nDescription=Atlas Tasker backup tick (%s)\n# %s\n\n[Service]\nType=oneshot\nExecStart=%s backup tick\nWorkingDirectory=%s\n", workspaceID, scheduleMarker, binary, workspace)
}

func systemdTimer(workspaceID string) string {
	return fmt.Sprintf("[Unit]\nDescription=Atlas Tasker backup timer (%s)\n# %s\n\n[Timer]\nOnUnitActiveSec=30s\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n", workspaceID, scheduleMarker)
}

func launchdPlist(binary, workspace, workspaceID string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s%s</string>
  <key>Comment</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>backup</string><string>tick</string></array>
  <key>WorkingDirectory</key><string>%s</string>
  <key>StartInterval</key><integer>30</integer>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`, macPlistPrefix, workspaceID, scheduleMarker, binary, workspace)
}
