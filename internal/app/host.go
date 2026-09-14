package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type ServiceUnit struct {
	Label       string
	Executable  string
	Args        []string
	Environment map[string]string
	Home        string
	StateDir    string
	Port        int
}

type HostPlan struct {
	Platform string `json:"platform"`
	Path     string `json:"path"`
	Content  string `json:"content"`
}

type HostStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Path      string `json:"path,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type HostCommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, err error)
}

type HostInstaller interface {
	Plan(spec ServiceUnit) (HostPlan, error)
	Install(ctx context.Context, spec ServiceUnit) error
	Uninstall(ctx context.Context, spec ServiceUnit) error
	Status(ctx context.Context, spec ServiceUnit) (HostStatus, error)
}

type ExecHostRunner struct{}

func (ExecHostRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type RecordingHostRunner struct {
	Calls [][]string
}

func (r *RecordingHostRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.Calls = append(r.Calls, append([]string{name}, args...))
	return "", nil
}

type NativeHostInstaller struct {
	UnitDir string
	Runner  HostCommandRunner
	GOOS    string
}

func NewNativeHostInstaller(home string, runner HostCommandRunner) *NativeHostInstaller {
	goos := runtime.GOOS
	unitDir := ""
	switch goos {
	case "darwin":
		unitDir = filepath.Join(home, "Library", "LaunchAgents")
	default:
		unitDir = filepath.Join(home, ".config", "systemd", "user")
	}
	if runner == nil {
		runner = ExecHostRunner{}
	}
	return &NativeHostInstaller{UnitDir: unitDir, Runner: runner, GOOS: goos}
}

func (h *NativeHostInstaller) unitPath(spec ServiceUnit) string {
	if h.GOOS == "darwin" {
		return filepath.Join(h.UnitDir, specLabel(spec)+".plist")
	}
	return filepath.Join(h.UnitDir, HomeSystemdService)
}

func (h *NativeHostInstaller) Plan(spec ServiceUnit) (HostPlan, error) {
	path := h.unitPath(spec)
	content := hostUnitContent(h.GOOS, spec)
	platform := "linux-systemd"
	if h.GOOS == "darwin" {
		platform = "macos-launchd"
	}
	return HostPlan{Platform: platform, Path: path, Content: content}, nil
}

func (h *NativeHostInstaller) Install(ctx context.Context, spec ServiceUnit) error {
	plan, err := h.Plan(spec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return err
	}
	if raw, err := os.ReadFile(plan.Path); err == nil && !strings.Contains(string(raw), HomeServiceMarker) {
		return fmt.Errorf("refusing to overwrite unrelated service definition at %s", plan.Path)
	}
	if err := os.WriteFile(plan.Path, []byte(plan.Content), 0o644); err != nil {
		return err
	}
	if h.GOOS == "darwin" {
		if _, err := h.Runner.Run(ctx, "launchctl", "bootout", "gui/"+userID(), plan.Path); err != nil {
			// first install has nothing to boot out
			_ = err
		}
		if _, err := h.Runner.Run(ctx, "launchctl", "bootstrap", "gui/"+userID(), plan.Path); err != nil {
			return fmt.Errorf("launchctl bootstrap: %w", err)
		}
		if _, err := h.Runner.Run(ctx, "launchctl", "kickstart", "-k", "gui/"+userID()+"/"+specLabel(spec)); err != nil {
			return fmt.Errorf("launchctl kickstart: %w", err)
		}
		return nil
	}
	if _, err := h.Runner.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if _, err := h.Runner.Run(ctx, "systemctl", "--user", "enable", "--now", HomeSystemdService); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	return nil
}

func (h *NativeHostInstaller) Uninstall(ctx context.Context, spec ServiceUnit) error {
	path := h.unitPath(spec)
	if h.GOOS == "darwin" {
		_, _ = h.Runner.Run(ctx, "launchctl", "bootout", "gui/"+userID(), path)
	} else {
		_, _ = h.Runner.Run(ctx, "systemctl", "--user", "disable", "--now", HomeSystemdService)
	}
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (h *NativeHostInstaller) Status(ctx context.Context, spec ServiceUnit) (HostStatus, error) {
	path := h.unitPath(spec)
	_, err := os.Stat(path)
	status := HostStatus{Path: path, Installed: err == nil}
	if !status.Installed {
		return status, nil
	}
	if h.GOOS == "darwin" {
		out, runErr := h.Runner.Run(ctx, "launchctl", "print", "gui/"+userID()+"/"+specLabel(spec))
		status.Running = runErr == nil && strings.Contains(out, "state = running")
		if runErr != nil {
			status.Detail = runErr.Error()
		}
		return status, nil
	}
	out, runErr := h.Runner.Run(ctx, "systemctl", "--user", "is-active", HomeSystemdService)
	status.Running = runErr == nil && strings.TrimSpace(out) == "active"
	if runErr != nil {
		status.Detail = runErr.Error()
	}
	return status, nil
}

func specLabel(spec ServiceUnit) string {
	if spec.Label != "" {
		return spec.Label
	}
	return HomeLaunchdLabel
}

func userID() string {
	return strconv.Itoa(os.Getuid())
}

func hostUnitContent(goos string, spec ServiceUnit) string {
	exe := spec.Executable
	args := spec.Args
	if len(args) == 0 {
		args = []string{"serve"}
	}
	env := spec.Environment
	if env == nil {
		env = map[string]string{}
	}
	if goos == "darwin" {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<!-- ` + HomeServiceMarker + ` -->
<dict>
  <key>Label</key>
  <string>` + xmlEscape(specLabel(spec)) + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + xmlEscape(exe) + `</string>
`)
		for _, arg := range args {
			b.WriteString("    <string>" + xmlEscape(arg) + "</string>\n")
		}
		b.WriteString(`  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
`)
		if spec.Home != "" {
			b.WriteString("  <key>WorkingDirectory</key>\n  <string>" + xmlEscape(spec.Home) + "</string>\n")
		}
		if len(env) > 0 {
			b.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
			for k, v := range env {
				b.WriteString("    <key>" + xmlEscape(k) + "</key>\n    <string>" + xmlEscape(v) + "</string>\n")
			}
			b.WriteString("  </dict>\n")
		}
		b.WriteString("</dict>\n</plist>\n")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("# " + HomeServiceMarker + "\n[Unit]\nDescription=Atlas Tasker Home\n\n[Service]\nType=simple\nExecStart=" + systemdQuote(exe))
	for _, arg := range args {
		b.WriteString(" " + systemdQuote(arg))
	}
	b.WriteString("\nRestart=on-failure\n")
	if spec.Home != "" {
		b.WriteString("WorkingDirectory=" + systemdQuote(spec.Home) + "\n")
	}
	for k, v := range env {
		b.WriteString("Environment=" + systemdQuote(k+"="+v) + "\n")
	}
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'\\$") {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `$$`)
	return `"` + s + `"`
}
