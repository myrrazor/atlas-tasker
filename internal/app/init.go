package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/uninstall"
)

type PartialError struct {
	Summary string
}

func (e *PartialError) Error() string {
	if e == nil || e.Summary == "" {
		return "init completed with failures"
	}
	return e.Summary
}

func IsPartial(err error) bool {
	_, ok := err.(*PartialError)
	return ok
}

func (a *App) Init(ctx context.Context, opts InitOptions) (InitResult, error) {
	opts.applyDefaults()
	if strings.TrimSpace(opts.Root) == "" {
		return InitResult{}, apperr.New(apperr.CodeInvalidInput, "init root is required")
	}
	root, err := service.CanonicalWorkspaceRoot(opts.Root)
	if err != nil {
		return InitResult{}, err
	}
	release, err := a.lockMachine("tracker init")
	if err != nil {
		return InitResult{}, err
	}
	defer func() {
		if release != nil {
			_ = release()
		}
	}()
	settings := a.snapshotSettings()

	scaffold, err := ScaffoldWorkspace(root, ScaffoldOptions{Now: a.opts.Now, GitMode: opts.GitMode})
	if err != nil {
		return InitResult{}, err
	}
	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil {
		return InitResult{}, err
	}
	result := InitResult{
		Kind:        "workspace_init",
		Workspace:   root,
		WorkspaceID: id,
		Created:     scaffold.Created,
		GitMode:     string(opts.GitMode.Normalized()),
		Already:     len(scaffold.Created) == 0,
		Steps: []InitStep{
			{Name: "scaffold", Status: InitStepDone},
			{Name: "identity", Status: InitStepDone, Detail: id},
		},
	}

	ws, err := OpenWorkspace(root, OpenOptions{
		Home:     a.home,
		StateDir: a.stateDir,
		Now:      a.opts.Now,
		Notice:   a.opts.Notice,
		Getenv:   a.getenv(),
		GOOS:     a.opts.GOOS,
	})
	if err != nil {
		return InitResult{}, err
	}
	defer func() { _ = ws.Close() }()

	if strings.TrimSpace(opts.ProjectKey) != "" || (opts.DefaultProject && settings.DefaultProject) {
		key, err := ensureInitProject(ctx, ws, root, opts.ProjectKey, opts.ProjectName)
		if err != nil {
			result.Steps = append(result.Steps, InitStep{Name: "default_project", Status: InitStepFailed, Detail: err.Error()})
		} else {
			result.DefaultProject = key
			detail := key
			if key == "" {
				detail = "already present"
			}
			result.Steps = append(result.Steps, InitStep{Name: "default_project", Status: InitStepDone, Detail: detail})
		}
	} else {
		result.Steps = append(result.Steps, InitStep{Name: "default_project", Status: InitStepSkipped})
	}

	if opts.Register && settings.AutoRegister {
		rec, err := a.register(ctx, RegisterOptions{Root: root, DisplayName: filepath.Base(root)}, true)
		if err != nil {
			result.Steps = append(result.Steps, InitStep{Name: "register", Status: InitStepFailed, Detail: err.Error()})
		} else {
			result.Registered = rec.WorkspaceID != ""
			result.Steps = append(result.Steps, InitStep{Name: "register", Status: InitStepDone})
		}
	} else {
		result.Steps = append(result.Steps, InitStep{Name: "register", Status: InitStepSkipped})
	}

	if opts.Backup && settings.LocalCheckpoints {
		result.Backup = a.initLocalBackup(ctx, ws)
		result.Steps = append(result.Steps, backupStep(result.Backup))
	} else {
		result.Backup = BackupInitReport{Attempted: false, Detail: "local checkpoints skipped"}
		result.Steps = append(result.Steps, InitStep{Name: "backup", Status: InitStepSkipped, Detail: result.Backup.Detail})
	}

	if opts.Agents && settings.Agents.AutoInstall && opts.WriteClientCfg {
		result.Agents = a.setupAgents(ctx, ws)
		result.Steps = append(result.Steps, agentStep(result.Agents))
	} else if opts.Agents && settings.Agents.AutoInstall && !opts.WriteClientCfg {
		result.Agents = AgentSetupReport{Attempted: false, Notes: []string{"client config writes disabled by caller"}}
		result.Steps = append(result.Steps, InitStep{Name: "agents", Status: InitStepSkipped, Detail: "client config writes disabled by caller"})
	} else {
		result.Agents = AgentSetupReport{Attempted: false, Notes: []string{"agent setup skipped"}}
		result.Steps = append(result.Steps, InitStep{Name: "agents", Status: InitStepSkipped})
	}

	_ = release()
	release = nil

	result.Service, result.Steps = a.initHomeService(ctx, opts, result.Steps)
	if err := a.writeInitJournal(result); err != nil {
		result.Steps = append(result.Steps, InitStep{Name: "journal", Status: InitStepFailed, Detail: err.Error()})
	}
	result.Summary = summarizeInit(result)
	if hasFailedStep(result.Steps) {
		return result, &PartialError{Summary: result.Summary}
	}
	got, recErr := writeInstallReceipt(a.stateDir, a.now())
	if step := installReceiptStep(got, recErr); step != nil {
		result.Steps = append(result.Steps, *step)
		result.Summary = summarizeInit(result)
	}
	return result, nil
}

var writeInstallReceipt = uninstall.MaybeWriteRunningReceipt

func installReceiptStep(got uninstall.EnsureReceiptResult, err error) *InitStep {
	if err != nil {
		return &InitStep{
			Name:   "install_receipt",
			Status: InitStepUnverified,
			Detail: "install receipt not written: " + err.Error() + "; uninstall will refuse until a verifiable receipt exists",
		}
	}
	if got.Wrote {
		return &InitStep{Name: "install_receipt", Status: InitStepDone, Detail: got.Receipt.InstallMethod}
	}
	return nil
}

// EnsureDefaultProjectAndRegister fills a hollow initialized workspace: it
// creates the default project only when none exist, then registers the tree
// with Home. It does not scaffold, rewrite git mode, start Home, or install
// agents.
func (a *App) EnsureDefaultProjectAndRegister(ctx context.Context, root string) (string, WorkspaceRecord, error) {
	root, err := service.InitializedWorkspaceRoot(root)
	if err != nil {
		return "", WorkspaceRecord{}, err
	}
	release, err := a.lockMachine("tracker workspace bootstrap")
	if err != nil {
		return "", WorkspaceRecord{}, err
	}
	defer func() { _ = release() }()
	ws, err := OpenWorkspace(root, OpenOptions{
		Home:     a.home,
		StateDir: a.stateDir,
		Now:      a.opts.Now,
		Notice:   a.opts.Notice,
		Getenv:   a.getenv(),
		GOOS:     a.opts.GOOS,
	})
	if err != nil {
		return "", WorkspaceRecord{}, err
	}
	defer func() { _ = ws.Close() }()
	settings := a.snapshotSettings()
	var key string
	if settings.DefaultProject {
		key, err = ensureInitProject(ctx, ws, root, "", "")
		if err != nil {
			return "", WorkspaceRecord{}, err
		}
	}
	if !settings.AutoRegister {
		return key, WorkspaceRecord{}, nil
	}
	rec, err := a.register(ctx, RegisterOptions{Root: root, DisplayName: filepath.Base(root)}, true)
	if err != nil {
		return key, WorkspaceRecord{}, err
	}
	return key, rec, nil
}

func (a *App) initHomeService(ctx context.Context, opts InitOptions, steps []InitStep) (*ServiceStatus, []InitStep) {
	if opts.SkipHomeService {
		status, _ := a.ServiceStatus(ctx)
		status.Detail = "Home service skipped during non-interactive workspace bootstrap"
		return &status, append(steps, InitStep{Name: "service", Status: InitStepSkipped, Detail: status.Detail})
	}
	settings := a.snapshotSettings()
	if !settings.Service.Enabled {
		status, _ := a.ServiceStatus(ctx)
		status.Detail = "Home service is disabled in settings"
		return &status, append(steps, InitStep{Name: "service", Status: InitStepSkipped, Detail: status.Detail})
	}
	status, err := a.EnsureService(ctx, ServiceOptions{OpenBrowser: opts.OpenHome})
	if err != nil {
		if status.URL == "" {
			status = ServiceStatus{
				Kind:       "atlas_home_status",
				Host:       settings.Service.Bind,
				Port:       settings.Service.Port,
				URL:        homeURL(settings.Service.Bind, settings.Service.Port),
				InstanceID: settings.InstanceID,
				Detail:     err.Error(),
			}
		} else if status.Detail == "" {
			status.Detail = err.Error()
		}
		return &status, append(steps, InitStep{Name: "service", Status: InitStepFailed, Detail: err.Error()})
	}
	return &status, append(steps, InitStep{Name: "service", Status: InitStepDone, Detail: status.URL})
}

func ensureInitProject(ctx context.Context, ws *Workspace, root, key, name string) (string, error) {
	projects, err := ws.Actions.Projects.ListProjects(ctx)
	if err != nil {
		return "", err
	}
	if len(projects) > 0 {
		return "", nil
	}
	key = strings.TrimSpace(key)
	name = strings.TrimSpace(name)
	fallback := filepath.Base(root)
	if key == "" {
		key = DefaultProjectKey(fallback)
	}
	if name == "" {
		name = fallback
	}
	if strings.TrimSpace(name) == "" {
		name = key
	}
	now := time.Now().UTC()
	if ws.Actions != nil && ws.Actions.Clock != nil {
		now = ws.Actions.Clock().UTC()
	}
	project := contracts.NormalizeProject(contracts.Project{
		Key:           key,
		Name:          name,
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := project.Validate(); err != nil {
		return "", apperr.New(apperr.CodeInvalidInput, err.Error())
	}
	if err := ws.Actions.CreateProject(ctx, project); err != nil {
		return "", err
	}
	return project.Key, nil
}

func (a *App) initLocalBackup(ctx context.Context, ws *Workspace) BackupInitReport {
	report := BackupInitReport{Attempted: true}
	if _, err := ws.Actions.ReplicaStatus(ctx); err != nil {
		report.Detail = err.Error()
		return report
	}
	enabled, err := a.enableLocalCheckpoints(ws.ID)
	if err != nil {
		report.Detail = err.Error()
		return report
	}
	if !enabled {
		report.Detail = "local checkpoints remain opted out"
		return report
	}
	tick, err := ws.Actions.BackupTick(ctx, true)
	if err != nil {
		report.Detail = err.Error()
		return report
	}
	report.CheckpointID = tick.CheckpointID
	if tick.SkipReason != "" {
		report.Detail = tick.SkipReason
	}
	if tick.Created && tick.CheckpointID != "" {
		report.ReplicaReady = true
		return report
	}
	if unchangedBackup(tick.SkipReason) && tick.CheckpointID != "" {
		report.ReplicaReady = true
		return report
	}
	if !report.ReplicaReady && report.Detail == "" {
		report.Detail = "local checkpoint was not created"
	}
	return report
}

func unchangedBackup(reason string) bool {
	switch reason {
	case "canonical_state_unchanged", "duplicate_logical_checkpoint":
		return true
	default:
		return false
	}
}

func backupStep(report BackupInitReport) InitStep {
	step := InitStep{Name: "backup", Status: InitStepDone, Detail: report.Detail}
	if report.Detail == "local checkpoints remain opted out" {
		step.Status = InitStepSkipped
		return step
	}
	if report.ReplicaReady {
		return step
	}
	step.Status = InitStepFailed
	return step
}

func agentStep(report AgentSetupReport) InitStep {
	if !report.Attempted {
		return InitStep{Name: "agents", Status: InitStepSkipped, Detail: strings.Join(report.Notes, "; ")}
	}
	var written, unverified int
	for _, client := range report.Clients {
		switch client.Status {
		case AgentWritten, AgentPendingClientRestart:
			written++
		case AgentUnverified:
			unverified++
		}
	}
	detail := strings.Join(report.Notes, "; ")
	if unverified > 0 && written == 0 {
		if detail == "" {
			detail = "agent registration is unverified; not a live connection"
		}
		return InitStep{Name: "agents", Status: InitStepUnverified, Detail: detail}
	}
	if written > 0 {
		if detail == "" {
			detail = "configured; client restart may be required"
		}
		return InitStep{Name: "agents", Status: InitStepConfigured, Detail: detail}
	}
	if detail == "" {
		detail = "no supported clients detected"
	}
	return InitStep{Name: "agents", Status: InitStepDone, Detail: detail}
}

func (a *App) enableLocalCheckpoints(workspaceID string) (bool, error) {
	stateDir, err := service.ResolveBackupStateDir(service.BackupStateDirOptions{
		Home:        a.home,
		StateDir:    a.stateDir,
		WorkspaceID: workspaceID,
		Getenv:      a.getenv(),
		GOOS:        a.opts.GOOS,
	})
	if err != nil {
		return false, err
	}
	path := filepath.Join(stateDir, "backups", workspaceID, "auto.json")
	var existing struct {
		Format          string    `json:"format"`
		Enabled         bool      `json:"enabled"`
		DefaultTargetID string    `json:"default_target_id,omitempty"`
		EnabledAt       time.Time `json:"enabled_at,omitempty"`
		DisabledAt      time.Time `json:"disabled_at,omitempty"`
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(raw, &existing) == nil {
			if !existing.Enabled {
				return false, nil
			}
			return true, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := atomicJSON(path, map[string]any{
		"format":     "atlas_backup_auto_v1",
		"enabled":    true,
		"enabled_at": a.now(),
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) writeInitJournal(result InitResult) error {
	if result.WorkspaceID == "" {
		return nil
	}
	path := filepath.Join(a.stateDir, "init-journal", result.WorkspaceID+".json")
	return atomicJSON(path, map[string]any{
		"workspace_id": result.WorkspaceID,
		"root":         result.Workspace,
		"steps":        result.Steps,
		"updated_at":   a.now(),
	})
}

func hasFailedStep(steps []InitStep) bool {
	for _, step := range steps {
		if step.Status == InitStepFailed {
			return true
		}
	}
	return false
}

func summarizeInit(result InitResult) string {
	var parts []string
	if result.Already {
		parts = append(parts, fmt.Sprintf("already bootstrapped %s", result.Workspace))
	} else {
		parts = append(parts, fmt.Sprintf("initialized %s", result.Workspace))
	}
	if result.DefaultProject != "" {
		parts = append(parts, "project "+result.DefaultProject)
	}
	if result.Registered {
		parts = append(parts, "registered")
	}
	if result.Agents.Attempted {
		parts = append(parts, fmt.Sprintf("agents=%d", len(result.Agents.Clients)))
		if info, err := os.Stat(filepath.Join(result.Workspace, "AGENTS.md")); err == nil && info.Mode().IsRegular() {
			parts = append(parts, "read AGENTS.md for board display in chat")
		}
	}
	for _, step := range result.Steps {
		switch step.Status {
		case InitStepFailed:
			parts = append(parts, step.Name+" failed")
		case InitStepUnverified:
			parts = append(parts, step.Name+" unverified")
		case InitStepConfigured:
			parts = append(parts, step.Name+" configured")
		}
	}
	if result.Service != nil && result.Service.URL != "" {
		parts = append(parts, result.Service.URL)
	}
	return strings.Join(parts, "; ")
}
