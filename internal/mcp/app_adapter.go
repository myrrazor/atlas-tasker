package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type appMachine struct {
	app *app.App
	cwd string
}

func AdaptApp(a *app.App, cwd string) Machine {
	return &appMachine{app: a, cwd: cwd}
}

func (m *appMachine) Close() error {
	if m == nil || m.app == nil {
		return nil
	}
	return m.app.Close()
}

func (m *appMachine) StateDir() string { return m.app.StateDir() }
func (m *appMachine) CWD() string      { return m.cwd }

func (m *appMachine) Settings() app.MachineSettings { return m.app.Settings() }

func (m *appMachine) UpdateSettings(ctx context.Context, patch app.MachineSettingsPatch) (app.MachineSettings, error) {
	if patch.Discovery != nil {
		current := m.app.Settings()
		if discoveryWidens(current.Discovery, *patch.Discovery) {
			return app.MachineSettings{}, apperr.New(apperr.CodePermissionDenied, "widening discovery roots requires atlas.settings.grant_discovery")
		}
	}
	if patch.Service != nil {
		bind := strings.TrimSpace(patch.Service.Bind)
		if bind != "" && bind != "127.0.0.1" && bind != "localhost" && bind != app.DefaultHomeBind {
			return app.MachineSettings{}, apperr.New(apperr.CodePermissionDenied, "service bind cannot leave loopback")
		}
	}
	return m.app.UpdateSettings(ctx, patch)
}

func (m *appMachine) GrantDiscovery(ctx context.Context, discovery app.DiscoverySettings) (app.MachineSettings, error) {
	return m.app.UpdateSettings(ctx, app.MachineSettingsPatch{Discovery: &discovery})
}

func discoveryWidens(current, next app.DiscoverySettings) bool {
	if next.Enabled && !current.Enabled {
		return true
	}
	if next.MaxDepth > 0 && current.MaxDepth > 0 && next.MaxDepth > current.MaxDepth {
		return true
	}
	have := map[string]struct{}{}
	for _, root := range current.Roots {
		have[filepath.Clean(strings.TrimSpace(root))] = struct{}{}
	}
	for _, root := range next.Roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || root == "." {
			continue
		}
		if _, ok := have[root]; !ok {
			return true
		}
	}
	return false
}

func (m *appMachine) Init(ctx context.Context, opts InitCall) (app.InitResult, error) {
	root := strings.TrimSpace(opts.Root)
	if opts.GrantID != "" {
		grant, err := m.app.ConsumeGrantFor(ctx, opts.GrantID, app.PathGrantInit)
		if err != nil {
			return app.InitResult{}, err
		}
		if root == "" {
			root = grant.Path
		} else if !samePathIdentity(root, grant.Path) {
			return app.InitResult{}, apperr.New(apperr.CodePermissionDenied, "init path does not match the path grant")
		}
	} else if err := m.authorizeApprovedPath(root); err != nil {
		return app.InitResult{}, err
	}
	writeClients := opts.WriteClientCfg
	if !opts.Agents {
		writeClients = false
	}
	// PartialError is a real failure (service/backup/etc after the workspace
	// exists). Keep the InitResult for repair/retry, but do not swallow it.
	return m.app.Init(ctx, app.InitOptions{
		Root:           root,
		GitMode:        app.GitMode(opts.GitMode),
		Register:       opts.Register,
		Agents:         opts.Agents,
		Backup:         opts.Backup,
		DefaultProject: opts.DefaultProject,
		Actor:          opts.Actor,
		WriteClientCfg: writeClients,
	})
}

func (m *appMachine) authorizeInitPath(raw string) error {
	return m.authorizeApprovedPath(raw)
}

func (m *appMachine) authorizeApprovedPath(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return apperr.New(apperr.CodeInvalidInput, "path is required")
	}
	if m.cwd != "" && samePathIdentity(raw, m.cwd) {
		return nil
	}
	settings := m.app.Settings()
	if settings.Discovery.Enabled {
		for _, root := range settings.Discovery.Roots {
			if pathContainedInApprovedRoot(raw, root) {
				return nil
			}
		}
	}
	return apperr.New(apperr.CodePermissionDenied, "path is not the current directory, a consumed path grant, or an approved discovery root")
}

func (m *appMachine) Register(ctx context.Context, opts app.RegisterOptions) (app.WorkspaceRecord, error) {
	if err := m.authorizeApprovedPath(opts.Root); err != nil {
		return app.WorkspaceRecord{}, err
	}
	return m.app.Register(ctx, opts)
}

func (m *appMachine) ListWorkspaces(ctx context.Context, opts app.ListOptions) ([]app.WorkspaceRecord, error) {
	return m.app.ListWorkspaces(ctx, opts)
}

func (m *appMachine) GetWorkspace(ctx context.Context, id string) (app.WorkspaceRecord, error) {
	id = strings.TrimSpace(id)
	list, err := m.app.ListWorkspaces(ctx, app.ListOptions{IncludeHidden: true})
	if err != nil {
		return app.WorkspaceRecord{}, err
	}
	for _, rec := range list {
		if rec.WorkspaceID == id {
			if rec.Health != app.HealthAvailable && rec.Health != app.HealthDisabled {
				return rec, healthError(rec)
			}
			if rec.Health == app.HealthDisabled {
				return rec, apperr.New(apperr.CodePermissionDenied, "workspace is hidden")
			}
			return rec, nil
		}
	}
	return app.WorkspaceRecord{}, apperr.New(apperr.CodeNotFound, "workspace is not registered")
}

func (m *appMachine) Repair(ctx context.Context, opts app.RepairOptions) (app.RepairResult, error) {
	if strings.TrimSpace(opts.NewPath) != "" {
		if err := m.authorizeApprovedPath(opts.NewPath); err != nil {
			return app.RepairResult{}, err
		}
	}
	return m.app.Repair(ctx, opts)
}

func (m *appMachine) Attention(ctx context.Context, opts app.AttentionOptions) (app.AttentionReport, error) {
	report, err := m.app.Attention(ctx, opts)
	if err != nil {
		return report, err
	}
	return report, nil
}

func (m *appMachine) Search(ctx context.Context, opts app.SearchOptions) (app.SearchReport, error) {
	return m.app.Search(ctx, opts)
}

func (m *appMachine) Bind(ctx context.Context, id string) (*Workspace, error) {
	w, err := m.app.Hub().Bind(ctx, id)
	if err != nil {
		return nil, err
	}
	return adaptWorkspace(w), nil
}

func (m *appMachine) BindRoot(ctx context.Context, root string) (*Workspace, error) {
	w, err := m.app.Hub().BindRoot(ctx, root)
	if err != nil {
		return nil, err
	}
	list, listErr := m.app.ListWorkspaces(ctx, app.ListOptions{IncludeHidden: true})
	if listErr == nil {
		for _, rec := range list {
			if rec.WorkspaceID == w.ID && filepath.Clean(rec.Path) != filepath.Clean(w.Root) {
				if rec.Health == app.HealthAvailable {
					return nil, apperr.New(apperr.CodeConflict, "copied workspace with the same identity fails until forked or repaired")
				}
				return nil, healthError(rec)
			}
			if rec.WorkspaceID == w.ID && rec.Health != app.HealthAvailable && rec.Health != app.HealthDisabled {
				return nil, healthError(rec)
			}
		}
	}
	return adaptWorkspace(w), nil
}

func (m *appMachine) InferCWD(ctx context.Context) (InferResult, error) {
	cwd := strings.TrimSpace(m.cwd)
	if cwd == "" {
		return InferResult{}, nil
	}
	root, err := uniqueAtlasRoot(cwd)
	if err != nil {
		return InferResult{}, err
	}
	if root == "" {
		return InferResult{}, nil
	}
	ws, err := m.BindRoot(ctx, root)
	if err != nil {
		return InferResult{}, err
	}
	return InferResult{WorkspaceID: ws.ID, Root: ws.Root, OK: true}, nil
}

func (m *appMachine) BoardURL(workspaceID, projectKey string) (string, error) {
	id := strings.TrimSpace(workspaceID)
	if id == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "workspace_id is required")
	}
	port := m.app.Settings().Service.Port
	if port <= 0 {
		port = app.DefaultHomePort
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/w/%s", port, id)
	if key := strings.TrimSpace(projectKey); key != "" {
		return url + "/projects/" + key, nil
	}
	return url, nil
}

func (m *appMachine) GrantPath(ctx context.Context, absPath, purpose string) (app.PathGrant, error) {
	return m.app.GrantPath(ctx, absPath, purpose)
}

func (m *appMachine) ConsumeGrant(ctx context.Context, id string) (app.PathGrant, error) {
	return m.app.ConsumeGrant(ctx, id)
}

func adaptWorkspace(w *app.Workspace) *Workspace {
	if w == nil {
		return nil
	}
	return &Workspace{
		Root:    w.Root,
		ID:      w.ID,
		Actions: w.Actions,
		Queries: w.Queries,
		Locks:   w.Locks,
		closeFn: w.Close,
	}
}

func healthError(rec app.WorkspaceRecord) error {
	switch rec.Health {
	case app.HealthMoved:
		return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
	case app.HealthCopiedIdentityConflict:
		return apperr.New(apperr.CodeConflict, "copied workspace with the same identity fails until forked or repaired")
	case app.HealthReplacedPath:
		return apperr.New(apperr.CodeInvalidInput, "replaced workspace: the registered path no longer refers to the same directory")
	case app.HealthPermissionDenied:
		return apperr.New(apperr.CodePermissionDenied, "workspace is not readable")
	case app.HealthCorruptIdentity:
		return apperr.New(apperr.CodeRepairNeeded, "corrupt workspace identity")
	case app.HealthDisabled:
		return apperr.New(apperr.CodePermissionDenied, "workspace is disabled")
	case app.HealthUnavailable, app.HealthSchemaUpgradeRequired:
		return apperr.New(apperr.CodeNotFound, "workspace is unavailable")
	default:
		if rec.Health != app.HealthAvailable {
			return apperr.New(apperr.CodeInvalidInput, "workspace is not available")
		}
	}
	return nil
}

func absClean(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "path is required")
	}
	if !filepath.IsAbs(raw) {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", err
		}
		raw = abs
	}
	return filepath.Clean(raw), nil
}

func resolvePathIdentity(raw string) (string, error) {
	abs, err := absClean(raw)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	return resolveExistingPrefix(abs)
}

func resolveExistingPrefix(abs string) (string, error) {
	dir := abs
	var missing []string
	for {
		info, err := os.Lstat(dir)
		if err == nil {
			resolved := dir
			if info.Mode()&os.ModeSymlink != 0 {
				resolved, err = filepath.EvalSymlinks(dir)
				if err != nil {
					return "", err
				}
			} else if resolvedEval, evalErr := filepath.EvalSymlinks(dir); evalErr == nil {
				resolved = resolvedEval
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		missing = append(missing, filepath.Base(dir))
		dir = parent
	}
}

func samePathIdentity(a, b string) bool {
	ra, errA := resolvePathIdentity(a)
	rb, errB := resolvePathIdentity(b)
	if errA == nil && errB == nil {
		return ra == rb
	}
	aa, errA := absClean(a)
	bb, errB := absClean(b)
	return errA == nil && errB == nil && aa == bb
}

func containedInResolved(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func symlinkEscapes(raw, approved string) bool {
	approvedID, err := resolvePathIdentity(approved)
	if err != nil {
		abs, absErr := absClean(approved)
		if absErr != nil {
			return true
		}
		approvedID = abs
	}
	abs, err := absClean(raw)
	if err != nil {
		return true
	}
	acc := abs
	if !filepath.IsAbs(acc) {
		return true
	}
	var parts []string
	dir := abs
	for {
		parts = append([]string{filepath.Base(dir)}, parts...)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	cur := string(os.PathSeparator)
	if filepath.VolumeName(abs) != "" {
		cur = filepath.VolumeName(abs) + string(os.PathSeparator)
		if len(parts) > 0 {
			parts = parts[1:]
		}
	}
	for _, part := range parts {
		if part == "" || part == string(os.PathSeparator) {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return !containedInResolved(approvedID, cur) && !containedInResolved(approvedID, abs)
			}
			return true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return true
			}
			if !containedInResolved(approvedID, resolved) && !containedInResolved(resolved, approvedID) {
				return true
			}
			cur = resolved
		}
	}
	final, err := resolvePathIdentity(abs)
	if err != nil {
		return true
	}
	return !containedInResolved(approvedID, final)
}

func pathContainedInApprovedRoot(raw, root string) bool {
	rootID, err := resolvePathIdentity(root)
	if err != nil {
		return false
	}
	pathID, err := resolvePathIdentity(raw)
	if err != nil {
		return false
	}
	if !containedInResolved(rootID, pathID) {
		return false
	}
	return !symlinkEscapes(raw, root)
}

func uniqueAtlasRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	var found []string
	dir := filepath.Clean(abs)
	for {
		marker := filepath.Join(dir, storage.TrackerDirName)
		info, err := os.Lstat(marker)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", apperr.New(apperr.CodeInvalidInput, "refusing a symlinked .tracker directory")
			}
			if info.IsDir() {
				found = append(found, dir)
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if len(found) == 0 {
		return "", nil
	}
	if len(found) > 1 {
		return "", apperr.New(apperr.CodeInvalidInput, "ambiguous workspace scope: nested Atlas workspaces")
	}
	if _, err := service.InitializedWorkspaceRoot(found[0]); err != nil {
		return "", err
	}
	return found[0], nil
}
