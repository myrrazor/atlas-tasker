package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/setup"
)

// Options configure a machine-level App. Tests inject Home/StateDir/Host/Process/LookPath.
type Options struct {
	Home            string
	StateDir        string
	Now             func() time.Time
	LookPath        func(string) (string, error)
	Getenv          func(string) string
	Executable      string
	Host            HostInstaller
	Process         ProcessSpawner
	Probe           ServiceProber
	Listen          func(host string, port int) (net.Listener, string, error)
	OpenBrowser     func(url string) error
	WriteClientCfg  bool
	SkipHostInstall bool
	CommandRunner   adapter.CommandRunner
	Notice          io.Writer
	GOOS            string
}

// App is the machine-wide Atlas application: settings, registry, hub, service.
type App struct {
	opts     Options
	home     string
	stateDir string
	mu       sync.Mutex
	settings MachineSettings
	hub      *Hub
}

func Open(opts Options) (*App, error) {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.Notice == nil {
		opts.Notice = io.Discard
	}
	home := opts.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	home = filepath.Clean(home)
	if !filepath.IsAbs(home) {
		return nil, apperr.New(apperr.CodeInvalidInput, "home must be an absolute path")
	}
	stateDir := opts.StateDir
	if stateDir == "" {
		var err error
		stateDir, err = setup.DefaultStateDir(home, opts.Getenv)
		if err != nil {
			return nil, err
		}
	} else if !filepath.IsAbs(stateDir) {
		return nil, apperr.New(apperr.CodeInvalidInput, "state dir must be an absolute path")
	}
	stateDir = filepath.Clean(stateDir)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("chmod state dir: %w", err)
	}
	a := &App{
		opts:     opts,
		home:     home,
		stateDir: stateDir,
	}
	release, err := setup.AcquireSetupLockWait(stateDir, "machine settings")
	if err != nil {
		return nil, err
	}
	settings, err := a.loadSettings()
	_ = release()
	if err != nil {
		return nil, err
	}
	a.settings = cloneSettings(settings)
	a.hub = newHub(a)
	if opts.Executable == "" {
		if exe, err := os.Executable(); err == nil {
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}
			a.opts.Executable = exe
		}
	}
	return a, nil
}

func (a *App) Close() error {
	if a == nil || a.hub == nil {
		return nil
	}
	return a.hub.Close()
}

func (a *App) StateDir() string { return a.stateDir }
func (a *App) Home() string     { return a.home }
func (a *App) Hub() *Hub        { return a.hub }
func (a *App) now() time.Time   { return a.opts.Now().UTC() }

func (a *App) getenv() func(string) string {
	if a.opts.Getenv != nil {
		return a.opts.Getenv
	}
	return os.Getenv
}

func (a *App) lookPath() func(string) (string, error) {
	if a.opts.LookPath != nil {
		return a.opts.LookPath
	}
	return exec.LookPath
}

func randomID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(raw[:])
}

func atomicJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".atlas-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func Capabilities() []Capability {
	return []Capability{
		{Name: CapInit, Scope: ScopeMachine, Mutating: true},
		{Name: CapRegister, Scope: ScopeMachine, Mutating: true},
		{Name: CapList, Scope: ScopeMachine, Mutating: false},
		{Name: CapRepair, Scope: ScopeMachine, Mutating: true},
		{Name: CapSettings, Scope: ScopeMachine, Mutating: true},
		{Name: CapAttention, Scope: ScopeMachine, Mutating: false},
		{Name: CapSearch, Scope: ScopeMachine, Mutating: false},
		{Name: CapProject, Scope: ScopeWorkspace, Mutating: true},
		{Name: CapTicket, Scope: ScopeWorkspace, Mutating: true},
		{Name: CapBoard, Scope: ScopeWorkspace, Mutating: false},
		{Name: CapBackup, Scope: ScopeWorkspace, Mutating: true},
		{Name: CapRestore, Scope: ScopeWorkspace, Mutating: true},
	}
}

func (a *App) lockMachine(purpose string) (func() error, error) {
	return setup.AcquireSetupLock(a.stateDir, purpose)
}

func (a *App) lockMachineWait(purpose string) (func() error, error) {
	return setup.AcquireSetupLockWait(a.stateDir, purpose)
}

func (a *App) withMachineLock(purpose string, fn func() error) error {
	release, err := a.lockMachine(purpose)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	return fn()
}

func GlobalMCPArgs() []string {
	return []string{GlobalMCPSubcommand, GlobalMCPServe, GlobalMCPFlag, GlobalMCPProfileFlag, GlobalMCPProfile}
}
