package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
	"github.com/myrrazor/atlas-tasker/internal/setup"
)

type ProbeResult struct {
	Occupied   bool
	Atlas      bool
	InstanceID string
	URL        string
	Detail     string
}

type ServiceProber interface {
	Probe(ctx context.Context, host string, port int) (ProbeResult, error)
}

type HTTPProber struct {
	Client *http.Client
}

func (p HTTPProber) Probe(ctx context.Context, host string, port int) (ProbeResult, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 800 * time.Millisecond}
	}
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect:") {
			return ProbeResult{Occupied: false}, nil
		}
		return ProbeResult{Occupied: true, Detail: err.Error()}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	result := ProbeResult{Occupied: true, URL: "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"}
	if svc := resp.Header.Get("X-Atlas-Service"); svc == "atlas-home" {
		result.Atlas = true
		result.InstanceID = resp.Header.Get("X-Atlas-Instance")
	}
	if !result.Atlas && resp.Header.Get("Content-Type") == "application/json" {
		var payload struct {
			Kind       string `json:"kind"`
			InstanceID string `json:"instance_id"`
		}
		if json.Unmarshal(body, &payload) == nil && payload.Kind == "atlas_home" {
			result.Atlas = true
			result.InstanceID = payload.InstanceID
		}
	}
	if !result.Atlas && strings.TrimSpace(string(body)) == "ok" {
		// text health without identity is treated as a foreign occupant
		result.Detail = "port is occupied without Atlas Home identity"
	}
	return result, nil
}

type ProcessSpawner interface {
	Start(ctx context.Context, exe string, args []string, env []string) (pid int, err error)
}

type ExecSpawner struct{}

func (ExecSpawner) Start(_ context.Context, exe string, args []string, env []string) (int, error) {
	cmd := exec.Command(exe, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

type NoopSpawner struct{}

func (NoopSpawner) Start(context.Context, string, []string, []string) (int, error) {
	return 0, apperr.New(apperr.CodeBusy, "process spawn disabled")
}

// RecordingSpawner is a test fake. Production never uses it.
type RecordingSpawner struct {
	mu      sync.Mutex
	Calls   int
	Exe     string
	Args    []string
	Env     []string
	PID     int
	Err     error
	OnStart func()
}

func (s *RecordingSpawner) Start(_ context.Context, exe string, args []string, env []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls++
	s.Exe = exe
	s.Args = append([]string(nil), args...)
	s.Env = append([]string(nil), env...)
	if s.OnStart != nil {
		s.OnStart()
	}
	if s.Err != nil {
		return 0, s.Err
	}
	if s.PID != 0 {
		return s.PID, nil
	}
	return 1, nil
}

type LatchProber struct {
	Instance string
	Running  func() bool
}

func (p LatchProber) Probe(context.Context, string, int) (ProbeResult, error) {
	if p.Running != nil && p.Running() {
		return ProbeResult{Occupied: true, Atlas: true, InstanceID: p.Instance}, nil
	}
	return ProbeResult{Occupied: false}, nil
}

func looksLikeTestBinary(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
}

func homeURL(host string, port int) string {
	if host == "" {
		host = DefaultHomeBind
	}
	if port == 0 {
		port = DefaultHomePort
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"
}

func (a *App) serviceUnit() ServiceUnit {
	settings := a.snapshotSettings()
	exe := a.opts.Executable
	if exe == "" {
		exe = "tracker"
	}
	bind := settings.Service.Bind
	if bind == "" {
		bind = DefaultHomeBind
	}
	port := settings.Service.Port
	if port == 0 {
		port = DefaultHomePort
	}
	return ServiceUnit{
		Label:      HomeLaunchdLabel,
		Executable: exe,
		Args:       []string{"serve", "--state-dir", a.stateDir, "--host", bind, "--port", strconv.Itoa(port)},
		Environment: map[string]string{
			"HOME": a.home,
		},
		Home:     a.home,
		StateDir: a.stateDir,
		Port:     settings.Service.Port,
	}
}

func (a *App) ServiceStatus(ctx context.Context) (ServiceStatus, error) {
	settings := a.snapshotSettings()
	host := settings.Service.Bind
	if host == "" {
		host = DefaultHomeBind
	}
	port := settings.Service.Port
	if port == 0 {
		port = DefaultHomePort
	}
	status := ServiceStatus{
		Kind:       "atlas_home_status",
		Host:       host,
		Port:       port,
		InstanceID: settings.InstanceID,
	}
	probe, err := a.probe(ctx, host, port)
	if err != nil {
		return status, err
	}
	status.Running = probe.Occupied && probe.Atlas && probe.InstanceID == settings.InstanceID
	status.IdentityOK = probe.Atlas && probe.InstanceID == settings.InstanceID
	status.Reused = status.Running
	status.URL = homeURL(host, port)
	status.Detail = probe.Detail
	return status, nil
}

func (a *App) probe(ctx context.Context, host string, port int) (ProbeResult, error) {
	prober := a.opts.Probe
	if prober == nil {
		prober = HTTPProber{}
	}
	return prober.Probe(ctx, host, port)
}

func (a *App) EnsureService(ctx context.Context, opts ServiceOptions) (ServiceStatus, error) {
	release, err := setup.AcquireServiceStartLock(a.stateDir)
	if err != nil {
		return ServiceStatus{}, err
	}
	defer func() { _ = release() }()
	settings := a.snapshotSettings()
	host := settings.Service.Bind
	if host == "" {
		host = DefaultHomeBind
	}
	port := settings.Service.Port
	if port == 0 {
		port = DefaultHomePort
	}
	if !settings.Service.Enabled && !opts.Foreground {
		status := ServiceStatus{
			Kind:       "atlas_home_status",
			Host:       host,
			Port:       port,
			URL:        homeURL(host, port),
			InstanceID: settings.InstanceID,
			Detail:     "Home service is disabled in settings",
		}
		return status, nil
	}
	probe, err := a.probe(ctx, host, port)
	if err != nil {
		return ServiceStatus{}, err
	}
	if probe.Occupied && probe.Atlas && probe.InstanceID == settings.InstanceID {
		status := ServiceStatus{
			Kind:       "atlas_home_status",
			Running:    true,
			Reused:     true,
			Host:       host,
			Port:       port,
			URL:        homeURL(host, port),
			InstanceID: settings.InstanceID,
			IdentityOK: true,
		}
		if claim, err := a.ClaimURL(status.URL); err == nil {
			status.ClaimURL = claim
		}
		return status, a.maybeOpen(opts, status)
	}
	if probe.Occupied && probe.Atlas && probe.InstanceID != settings.InstanceID {
		return ServiceStatus{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("port %s is owned by a different Atlas instance %s", net.JoinHostPort(host, strconv.Itoa(port)), probe.InstanceID))
	}
	if probe.Occupied {
		return ServiceStatus{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("port %s is occupied by another application; Atlas will not pick a different port", net.JoinHostPort(host, strconv.Itoa(port))))
	}

	unit := a.serviceUnit()
	if looksLikeTestBinary(unit.Executable) && a.opts.Process == nil {
		status := ServiceStatus{
			Kind:       "atlas_home_status",
			Host:       host,
			Port:       port,
			URL:        homeURL(host, port),
			InstanceID: settings.InstanceID,
			Detail:     "refusing to spawn a test binary as Home",
		}
		return status, nil
	}
	if !a.opts.SkipHostInstall && settings.Service.Enabled && settings.Service.AutoStart && !opts.Foreground {
		installer := a.opts.Host
		if installer == nil {
			installer = NewNativeHostInstaller(a.home, nil)
		}
		if err := installer.Install(ctx, unit); err != nil {
			probe.Detail = err.Error()
		} else {
			_ = a.recordHomeServiceUninstallAction(unit)
			status, waitErr := a.waitForSelf(ctx, host, port)
			if waitErr == nil {
				status.Detail = probe.Detail
				return status, a.maybeOpen(opts, status)
			}
		}
	}

	spawner := a.opts.Process
	if spawner == nil {
		spawner = ExecSpawner{}
	}
	env := []string{
		"HOME=" + a.home,
	}
	pid, err := spawner.Start(ctx, unit.Executable, unit.Args, env)
	if err != nil {
		return ServiceStatus{}, err
	}
	status, err := a.waitForSelf(ctx, host, port)
	if err != nil {
		status.PID = pid
		status.Detail = err.Error()
		return status, err
	}
	status.PID = pid
	return status, a.maybeOpen(opts, status)
}

func (a *App) waitForSelf(ctx context.Context, host string, port int) (ServiceStatus, error) {
	settings := a.snapshotSettings()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		probe, err := a.probe(ctx, host, port)
		if err == nil && probe.Atlas && probe.InstanceID == settings.InstanceID {
			status := ServiceStatus{
				Kind:       "atlas_home_status",
				Running:    true,
				Host:       host,
				Port:       port,
				URL:        homeURL(host, port),
				InstanceID: settings.InstanceID,
				IdentityOK: true,
			}
			if claim, err := a.ClaimURL(status.URL); err == nil {
				status.ClaimURL = claim
			}
			return status, nil
		}
		select {
		case <-ctx.Done():
			return ServiceStatus{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return ServiceStatus{
		Kind:       "atlas_home_status",
		Host:       host,
		Port:       port,
		InstanceID: settings.InstanceID,
	}, apperr.New(apperr.CodeBusy, "Atlas Home did not become reachable on the configured port")
}

func (a *App) maybeOpen(opts ServiceOptions, status ServiceStatus) error {
	if !opts.OpenBrowser || !a.snapshotSettings().Browser.OpenHome {
		return nil
	}
	open := a.opts.OpenBrowser
	if open == nil {
		return nil
	}
	url := status.ClaimURL
	if url == "" {
		url = status.URL
	}
	return open(url)
}

func HealthIdentityHeaders(instanceID string) map[string]string {
	return map[string]string{
		"X-Atlas-Service":  "atlas-home",
		"X-Atlas-Instance": instanceID,
	}
}

func HealthJSON(instanceID string, port int) map[string]any {
	return map[string]any{
		"kind":        "atlas_home",
		"instance_id": instanceID,
		"version":     buildinfo.Current().Version,
		"port":        port,
	}
}
