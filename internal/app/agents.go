package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func (a *App) setupAgents(ctx context.Context, ws *Workspace) AgentSetupReport {
	report := AgentSetupReport{Attempted: true}
	detections := integrations.Detect(integrations.DetectOptions{
		Workspace: ws.Root,
		Home:      a.home,
		LookPath:  a.lookPath(),
		Getenv:    a.getenv(),
	})
	command := a.opts.Executable
	if command == "" {
		command = "tracker"
	}
	args := GlobalMCPArgs()
	wrote := false
	for _, d := range detections {
		if !d.Found || d.Target == integrations.TargetGeneric {
			continue
		}
		client := a.registerDetectedClient(ctx, d.Target, command, args)
		report.Clients = append(report.Clients, client)
		if client.Status == AgentWritten || client.Status == AgentPendingClientRestart {
			wrote = true
		}
	}
	genericPath := filepath.Join(a.stateDir, "integrations", "atlas-mcp.json")
	if err := writePortableDescriptor(genericPath, command, args); err != nil {
		report.Notes = append(report.Notes, "generic portable descriptor: "+err.Error())
	} else {
		status := verifyGlobalMCPJSON(genericPath, command, args)
		report.Clients = append(report.Clients, AgentClientReport{
			Target:     integrations.TargetGeneric,
			Status:     status,
			Command:    command,
			Args:       args,
			ConfigPath: genericPath,
			Detail:     "portable descriptor only; not a live client config",
		})
		wrote = wrote || status == AgentWritten
	}
	if len(report.Clients) == 1 && report.Clients[0].Target == integrations.TargetGeneric && !wrote {
		report.Notes = append(report.Notes, "no supported coding agents detected")
	}
	if err := a.installWorkspaceSkills(ws, detections); err != nil {
		report.Notes = append(report.Notes, "workspace skills: "+err.Error())
	}
	if wrote {
		_ = a.recordUninstallClientActions(command, args, report.Clients)
	}
	return report
}

func (a *App) registerDetectedClient(ctx context.Context, target integrations.Target, command string, args []string) AgentClientReport {
	client := AgentClientReport{Target: target, Command: command, Args: args}
	look := a.lookPath()
	exe := ""
	if look != nil {
		if path, err := look(clientExecutableName(target)); err == nil {
			exe = path
		}
	}
	switch target {
	case integrations.TargetCursor:
		path := filepath.Join(a.home, ".cursor", "mcp.json")
		client.ConfigPath = path
		return a.writeJSONRegistration(path, command, args, client, "Cursor loads new MCP servers after a restart")
	case integrations.TargetCodex:
		path := filepath.Join(a.home, ".codex", "config.toml")
		client.ConfigPath = path
		return a.writeTOMLRegistration(path, command, args, client)
	case integrations.TargetClaude:
		client.ConfigPath = filepath.Join(a.home, ".claude.json")
		if exe == "" {
			client.Status = AgentNotDetected
			client.Detail = "claude CLI is not installed"
			return client
		}
		return a.runClientRegistration(ctx, client, exe, []string{"mcp", "add", GlobalMCPServerName, "--scope", "user", "--", command}, args)
	case integrations.TargetGrok:
		client.ConfigPath = filepath.Join(a.home, ".grok", "config.toml")
		if exe == "" {
			client.Status = AgentNotDetected
			client.Detail = "grok CLI is not installed"
			return client
		}
		return a.runClientRegistration(ctx, client, exe, []string{"mcp", "add", "--scope", "user", GlobalMCPServerName, "--", command}, args)
	case integrations.TargetOpenClaw:
		if exe == "" {
			client.Status = AgentNotDetected
			client.Detail = "openclaw CLI is not installed"
			return client
		}
		cliArgs := []string{"mcp", "add", GlobalMCPServerName, "--command", command}
		for _, arg := range args {
			cliArgs = append(cliArgs, "--arg", arg)
		}
		return a.runClientRegistration(ctx, client, exe, cliArgs, nil)
	default:
		client.Status = AgentNotDetected
		return client
	}
}

func clientExecutableName(target integrations.Target) string {
	switch target {
	case integrations.TargetCodex:
		return "codex"
	case integrations.TargetClaude:
		return "claude"
	case integrations.TargetCursor:
		return "cursor"
	case integrations.TargetOpenClaw:
		return "openclaw"
	case integrations.TargetGrok:
		return "grok"
	default:
		return ""
	}
}

func (a *App) writeJSONRegistration(path, command string, args []string, client AgentClientReport, restartNote string) AgentClientReport {
	if configEntrySymlinked(path) {
		client.Status = AgentUnverified
		client.Detail = "refusing symlinked MCP config"
		return client
	}
	existing, err := host.ReadExisting(path)
	if err != nil {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client
	}
	if unmanagedJSONCollision(existing, command, args) {
		client.Status = AgentUnverified
		client.Detail = "existing unmanaged atlas-tasker entry"
		return client
	}
	entry := adapter.StandardServerEntry{Type: adapter.RegistrationTransportStdio, Command: command, Args: append([]string(nil), args...)}
	merged, err := host.MergeJSONServer(existing, GlobalMCPServerName, entry)
	if err != nil {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client
	}
	if string(existing) != string(merged) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			client.Status = AgentUnverified
			client.Detail = err.Error()
			return client
		}
		if err := os.WriteFile(path, append(merged, '\n'), 0o600); err != nil {
			client.Status = AgentUnverified
			client.Detail = err.Error()
			return client
		}
	}
	client.Status = verifyGlobalMCPJSON(path, command, args)
	if client.Status == AgentWritten && restartNote != "" {
		client.Status = AgentPendingClientRestart
		client.Detail = restartNote
	}
	return client
}

func (a *App) writeTOMLRegistration(path, command string, args []string, client AgentClientReport) AgentClientReport {
	if configEntrySymlinked(path) {
		client.Status = AgentUnverified
		client.Detail = "refusing symlinked Codex config"
		return client
	}
	existing, err := host.ReadExisting(path)
	if err != nil {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client
	}
	required := false
	merged, err := host.MergeTOMLServer(existing, GlobalMCPServerName, []host.TOMLField{
		{Key: "command", Value: command},
		{Key: "args", Array: append([]string(nil), args...)},
		{Key: "required", Bool: &required},
	})
	if err != nil {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client
	}
	if string(existing) != string(merged) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			client.Status = AgentUnverified
			client.Detail = err.Error()
			return client
		}
		if err := os.WriteFile(path, merged, 0o600); err != nil {
			client.Status = AgentUnverified
			client.Detail = err.Error()
			return client
		}
	}
	if verifyTOMLRegistration(path, command, args) {
		client.Status = AgentWritten
	} else {
		client.Status = AgentUnverified
		client.Detail = "codex config.toml did not contain the Atlas-managed entry"
	}
	return client
}

func (a *App) runClientRegistration(ctx context.Context, client AgentClientReport, exe string, prefix, extra []string) AgentClientReport {
	cmd := adapter.Command{
		Purpose:    adapter.CommandPurposeRegister,
		Executable: exe,
		Args:       append(append([]string{}, prefix...), extra...),
		Timeout:    30 * time.Second,
	}
	if err := cmd.Validate(); err != nil {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client
	}
	runner := a.opts.CommandRunner
	if runner == nil {
		runner = host.DefaultRunner{}
	}
	result, err := runner.Run(ctx, cmd)
	if err != nil && result.ExitCode != 0 {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client
	}
	client.Status = AgentPendingClientRestart
	client.Detail = "Atlas-managed CLI registration; client restart may be required"
	return client
}

func configEntrySymlinked(path string) bool {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	info, err = os.Lstat(filepath.Dir(path))
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func writePortableDescriptor(path, command string, args []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	existing, err := host.ReadExisting(path)
	if err != nil {
		return err
	}
	entry := adapter.StandardServerEntry{Type: adapter.RegistrationTransportStdio, Command: command, Args: append([]string(nil), args...)}
	merged, err := host.MergeJSONServer(existing, GlobalMCPServerName, entry)
	if err != nil {
		return err
	}
	if string(existing) == string(merged) {
		return nil
	}
	return os.WriteFile(path, append(merged, '\n'), 0o600)
}

func unmanagedJSONCollision(existing []byte, command string, args []string) bool {
	if len(strings.TrimSpace(string(existing))) == 0 {
		return false
	}
	var doc map[string]any
	if json.Unmarshal(existing, &doc) != nil {
		return false
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	entry, _ := servers[GlobalMCPServerName].(map[string]any)
	if entry == nil {
		return false
	}
	gotCmd, _ := entry["command"].(string)
	gotArgs, ok := jsonStringSlice(entry["args"])
	if gotCmd == command && ok && slicesEqual(gotArgs, args) {
		return false
	}
	if gotCmd == "" {
		return false
	}
	return true
}

func verifyGlobalMCPJSON(path, command string, args []string) AgentClientStatus {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AgentUnverified
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return AgentUnverified
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	entry, _ := servers[GlobalMCPServerName].(map[string]any)
	if entry == nil {
		return AgentUnverified
	}
	gotCmd, _ := entry["command"].(string)
	if filepath.Clean(gotCmd) != filepath.Clean(command) && gotCmd != command {
		return AgentUnverified
	}
	gotArgs, ok := jsonStringSlice(entry["args"])
	if !ok || !slicesEqual(gotArgs, args) {
		return AgentUnverified
	}
	return AgentWritten
}

func verifyTOMLRegistration(path, command string, args []string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	body := string(raw)
	if !strings.Contains(body, "[mcp_servers."+GlobalMCPServerName+"]") && !strings.Contains(body, `[mcp_servers."`+GlobalMCPServerName+`"]`) {
		return false
	}
	if !strings.Contains(body, command) {
		return false
	}
	for _, arg := range args {
		if !strings.Contains(body, arg) {
			return false
		}
	}
	return true
}

func jsonStringSlice(v any) ([]string, bool) {
	switch typed := v.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (a *App) installWorkspaceSkills(ws *Workspace, detections []integrations.Detection) error {
	installer := integrations.Installer{Root: ws.Root}
	for _, d := range detections {
		if !d.Found || d.Target == integrations.TargetGeneric {
			continue
		}
		if _, err := installer.Install(d.Target, false); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ListAgentClients(ctx context.Context) AgentSetupReport {
	_ = ctx
	command := a.opts.Executable
	if command == "" {
		command = "tracker"
	}
	args := GlobalMCPArgs()
	report := AgentSetupReport{Attempted: true}
	look := a.lookPath()
	detected := map[integrations.Target]bool{}
	for _, d := range integrations.Detect(integrations.DetectOptions{
		Workspace: a.home,
		Home:      a.home,
		LookPath:  look,
		Getenv:    a.getenv(),
	}) {
		detected[d.Target] = d.Found
	}
	specs := []struct {
		target integrations.Target
		path   string
		toml   bool
	}{
		{integrations.TargetCursor, filepath.Join(a.home, ".cursor", "mcp.json"), false},
		{integrations.TargetCodex, filepath.Join(a.home, ".codex", "config.toml"), true},
		{integrations.TargetGrok, filepath.Join(a.home, ".grok", "config.toml"), true},
		{integrations.TargetClaude, filepath.Join(a.home, ".claude.json"), false},
		{integrations.TargetGeneric, filepath.Join(a.stateDir, "integrations", "atlas-mcp.json"), false},
	}
	for _, spec := range specs {
		client := AgentClientReport{Target: spec.target, Command: command, Args: args, ConfigPath: spec.path}
		_, err := os.Lstat(spec.path)
		switch {
		case os.IsNotExist(err) && !detected[spec.target] && spec.target != integrations.TargetGeneric:
			continue
		case os.IsNotExist(err):
			if detected[spec.target] {
				client.Status = AgentUnverified
				client.Detail = "client is present but the Atlas MCP entry is missing"
			} else {
				client.Status = AgentNotDetected
			}
		case spec.toml:
			if verifyTOMLRegistration(spec.path, command, args) {
				client.Status = AgentWritten
			} else {
				client.Status = AgentUnverified
				client.Detail = "config does not match Atlas-managed argv"
			}
		default:
			client.Status = verifyGlobalMCPJSON(spec.path, command, args)
			if client.Status == AgentWritten {
				client.Status = AgentPendingClientRestart
				client.Detail = "file matches; client restart may be required"
			}
		}
		report.Clients = append(report.Clients, client)
	}
	return report
}
