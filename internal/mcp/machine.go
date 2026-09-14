package mcp

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/setup"
)

// GlobalServeArgs is the exact argv Core/installer must register for clients
// that accept canonical dotted tool names.
func GlobalServeArgs() []string {
	return []string{"mcp", "serve", "--global", "--tool-profile", "workflow"}
}

// GlobalServeArgsPortable is the Grok registration argv. It keeps the same
// profile and bounds and only changes advertised tool names.
func GlobalServeArgsPortable() []string {
	return append(GlobalServeArgs(), FlagToolNameStyle, string(ToolNameStylePortable))
}

func GlobalServeArgv(tracker string) []string {
	return append([]string{tracker}, GlobalServeArgs()...)
}

type Machine interface {
	Close() error
	StateDir() string
	CWD() string
	Settings() app.MachineSettings
	UpdateSettings(ctx context.Context, patch app.MachineSettingsPatch) (app.MachineSettings, error)
	GrantDiscovery(ctx context.Context, discovery app.DiscoverySettings) (app.MachineSettings, error)
	Init(ctx context.Context, opts InitCall) (app.InitResult, error)
	Register(ctx context.Context, opts app.RegisterOptions) (app.WorkspaceRecord, error)
	ListWorkspaces(ctx context.Context, opts app.ListOptions) ([]app.WorkspaceRecord, error)
	GetWorkspace(ctx context.Context, id string) (app.WorkspaceRecord, error)
	Repair(ctx context.Context, opts app.RepairOptions) (app.RepairResult, error)
	Attention(ctx context.Context, opts app.AttentionOptions) (app.AttentionReport, error)
	Search(ctx context.Context, opts app.SearchOptions) (app.SearchReport, error)
	Bind(ctx context.Context, id string) (*Workspace, error)
	BindRoot(ctx context.Context, root string) (*Workspace, error)
	InferCWD(ctx context.Context) (InferResult, error)
	BoardURL(workspaceID, projectKey string) (string, error)
	GrantPath(ctx context.Context, absPath, purpose string) (app.PathGrant, error)
	ConsumeGrant(ctx context.Context, id string) (app.PathGrant, error)
}

type MachineOpenOptions struct {
	Home            string
	StateDir        string
	CWD             string
	Now             func() time.Time
	Notice          io.Writer
	Getenv          func(string) string
	LookPath        func(string) (string, error)
	Host            app.HostInstaller
	CommandRunner   adapter.CommandRunner
	Executable      string
	SkipHostInstall bool
}

func OpenMachine(opts MachineOpenOptions) (Machine, error) {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if strings.TrimSpace(opts.Home) == "" {
		if home := strings.TrimSpace(opts.Getenv("HOME")); home != "" {
			opts.Home = home
		}
	}
	if strings.TrimSpace(opts.StateDir) == "" {
		dir, err := setup.DefaultStateDir(opts.Home, opts.Getenv)
		if err != nil {
			return nil, err
		}
		opts.StateDir = dir
	}
	if strings.TrimSpace(opts.Executable) == "" {
		opts.Executable, _ = os.Executable()
	}
	a, err := app.Open(app.Options{
		Home:            opts.Home,
		StateDir:        opts.StateDir,
		Now:             opts.Now,
		Notice:          opts.Notice,
		Getenv:          opts.Getenv,
		LookPath:        opts.LookPath,
		Host:            opts.Host,
		CommandRunner:   opts.CommandRunner,
		Executable:      opts.Executable,
		WriteClientCfg:  false, // Open does not write clients; Init does, via InitCall
		SkipHostInstall: true,  // serving MCP is not Home service install
	})
	if err != nil {
		return nil, err
	}
	return AdaptApp(a, opts.CWD), nil
}

type InitCall struct {
	Root           string
	GitMode        string
	Register       bool
	Agents         bool
	Backup         bool
	DefaultProject bool
	Actor          contracts.Actor
	GrantID        string
	WriteClientCfg bool
}

type InferResult struct {
	WorkspaceID string
	Root        string
	OK          bool
}

type RegistrationObservation struct {
	ConfigPresent   bool
	Command         string
	Args            []string
	ExpectedCommand string
	ExpectedArgs    []string
	InitializeOK    bool
}

func ClassifyRegistration(obs RegistrationObservation) string {
	if !obs.ConfigPresent {
		return "missing"
	}
	if !sameArgs(obs.Args, obs.ExpectedArgs) || (obs.ExpectedCommand != "" && obs.Command != obs.ExpectedCommand) {
		return "unverified"
	}
	if obs.InitializeOK {
		return "installed"
	}
	return "pending_restart"
}

func sameArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func publicWorkspace(rec app.WorkspaceRecord, includePath bool) map[string]any {
	out := map[string]any{
		"workspace_id": rec.WorkspaceID,
		"display_name": rec.DisplayName,
		"visibility":   rec.Visibility,
		"health":       rec.Health,
	}
	if rec.HealthDetail != "" {
		out["health_detail"] = redactMixedString(rec.HealthDetail, includePath)
	}
	if includePath && rec.Path != "" {
		out["path"] = rec.Path
	}
	if !rec.RegisteredAt.IsZero() {
		out["registered_at"] = rec.RegisteredAt
	}
	return out
}

func publicAttention(report app.AttentionReport, includePaths bool) map[string]any {
	items := make([]map[string]any, 0, len(report.Items))
	for _, item := range report.Items {
		row := map[string]any{
			"workspace_id": item.WorkspaceID,
			"display_name": item.DisplayName,
			"ticket_id":    item.TicketID,
			"project":      item.Project,
			"title":        item.Title,
			"category":     item.Category,
			"reason":       redactMixedString(item.Reason, includePaths),
			"health":       item.Health,
		}
		if includePaths && item.Path != "" {
			row["path"] = item.Path
		}
		items = append(items, row)
	}
	missing := make([]map[string]any, 0, len(report.Missing))
	for _, rec := range report.Missing {
		missing = append(missing, publicWorkspace(rec, includePaths))
	}
	kind := report.Kind
	if kind == "" {
		kind = "attention"
	}
	return map[string]any{"kind": kind, "items": items, "unavailable": missing}
}

func publicMachineSettings(settings app.MachineSettings) map[string]any {
	return map[string]any{
		"format":            settings.Format,
		"auto_register":     settings.AutoRegister,
		"default_project":   settings.DefaultProject,
		"local_checkpoints": settings.LocalCheckpoints,
		"git_mode":          settings.GitMode,
		"browser":           settings.Browser,
		"home":              settings.Home,
		"agents":            map[string]any{"auto_install": settings.Agents.AutoInstall},
		"service":           map[string]any{"bind": settings.Service.Bind, "port": settings.Service.Port, "enabled": settings.Service.Enabled},
		"discovery": map[string]any{
			"enabled":    settings.Discovery.Enabled,
			"max_depth":  settings.Discovery.MaxDepth,
			"root_count": len(settings.Discovery.Roots),
		},
	}
}
