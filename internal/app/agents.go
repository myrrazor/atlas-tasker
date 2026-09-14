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
	wrote := false
	for _, d := range detections {
		if !d.Found || d.Target == integrations.TargetGeneric {
			continue
		}
		client := a.registerDetectedClient(ctx, d.Target, command, GlobalMCPArgsFor(d.Target))
		report.Clients = append(report.Clients, client)
		if client.Status == AgentWritten || client.Status == AgentPendingClientRestart {
			wrote = true
		}
	}
	genericPath := filepath.Join(a.stateDir, "integrations", "atlas-mcp.json")
	genericArgs := GlobalMCPArgs()
	if err := writePortableDescriptor(genericPath, command, genericArgs); err != nil {
		report.Notes = append(report.Notes, "generic portable descriptor: "+err.Error())
	} else {
		status := verifyGlobalMCPJSON(genericPath, command, genericArgs)
		report.Clients = append(report.Clients, AgentClientReport{
			Target:     integrations.TargetGeneric,
			Status:     status,
			Command:    command,
			Args:       genericArgs,
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
		_ = a.recordUninstallClientActions(command, GlobalMCPArgs(), report.Clients)
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

func GlobalMCPArgsFor(target integrations.Target) []string {
	args := GlobalMCPArgs()
	if target == integrations.TargetGrok {
		return append(args, GlobalMCPToolNameStyleFlag, GlobalMCPToolNameStylePortable)
	}
	return args
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
	command := a.opts.Executable
	if command == "" {
		command = "tracker"
	}
	report := AgentSetupReport{Attempted: true}
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
		if client, ok := a.inspectUserClient(spec.target, spec.path, spec.toml, command, GlobalMCPArgsFor(spec.target)); ok {
			report.Clients = append(report.Clients, client)
		}
	}
	for _, ref := range a.knownWorkspaceRoots(ctx) {
		if client := a.inspectProjectGrok(ref.root, ref.workspaceID, command); client != nil {
			report.Clients = append(report.Clients, *client)
		}
	}
	return report
}

type workspaceRef struct {
	root        string
	workspaceID string
}

func (a *App) knownWorkspaceRoots(ctx context.Context) []workspaceRef {
	seen := map[string]struct{}{}
	var out []workspaceRef
	add := func(root, id string) {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || !filepath.IsAbs(root) {
			return
		}
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return
		}
		if _, ok := seen[root]; ok {
			return
		}
		if id == "" {
			id = readWorkspaceID(root)
		}
		if id == "" {
			return
		}
		seen[root] = struct{}{}
		out = append(out, workspaceRef{root: root, workspaceID: id})
	}
	listed, err := a.ListWorkspaces(ctx, ListOptions{IncludeHidden: true})
	if err == nil {
		for _, rec := range listed {
			add(rec.Path, rec.WorkspaceID)
		}
	}
	for _, grant := range a.ListPendingGrants() {
		add(grant.Path, "")
	}
	return out
}

func (a *App) inspectUserClient(target integrations.Target, path string, toml bool, command string, args []string) (AgentClientReport, bool) {
	client := AgentClientReport{
		Target:     target,
		Command:    command,
		Args:       append([]string(nil), args...),
		ConfigPath: path,
		Scope:      AgentScopeUser,
		Binding:    AgentBindingGlobal,
		Provenance: AgentProvenanceNative,
		ServerName: GlobalMCPServerName,
	}
	if target == integrations.TargetGeneric {
		client.Provenance = AgentProvenancePortable
		client.Binding = ""
	}
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return client, false
	}
	if err != nil {
		client.Status = AgentUnverified
		client.Detail = err.Error()
		return client, true
	}
	if toml {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			client.Status = AgentUnverified
			client.Detail = readErr.Error()
			return client, true
		}
		entry, parsed, present, usable := host.InspectTOMLServer(raw, GlobalMCPServerName)
		if !parsed {
			client.Status = AgentUnverified
			client.Detail = "user-scoped config is malformed"
			return client, true
		}
		if !present {
			return client, false
		}
		client.Command = entry.Command
		client.Args = append([]string(nil), entry.Args...)
		if !usable {
			client.Status = AgentUnverified
			client.Detail = "user-scoped Atlas entry is invalid"
			return client, true
		}
		if commandMatchesAtlas(entry.Command, command, a.lookPath()) && slicesEqual(entry.Args, args) {
			client.Status = AgentPendingClientRestart
			client.Detail = "user-scoped file matches; client restart may be required"
			return client, true
		}
		client.Status = AgentUnverified
		client.Detail = "user-scoped config does not match Atlas-managed argv"
		return client, true
	}
	gotCmd, gotArgs, parsed, present, usable := inspectJSONNamed(path, GlobalMCPServerName)
	if !parsed {
		client.Status = AgentUnverified
		client.Detail = "user-scoped config is malformed"
		return client, true
	}
	if !present {
		return client, false
	}
	client.Command = gotCmd
	client.Args = append([]string(nil), gotArgs...)
	if !usable {
		client.Status = AgentUnverified
		client.Detail = "user-scoped Atlas entry is invalid"
		return client, true
	}
	if commandMatchesAtlas(gotCmd, command, a.lookPath()) && slicesEqual(gotArgs, args) {
		if target == integrations.TargetGeneric {
			client.Status = AgentWritten
			client.Detail = "portable descriptor only; not a live client config"
			return client, true
		}
		client.Status = AgentPendingClientRestart
		client.Detail = "user-scoped file matches; client restart may be required"
		return client, true
	}
	client.Status = AgentUnverified
	client.Detail = "user-scoped config does not match Atlas-managed argv"
	return client, true
}

func (a *App) inspectProjectGrok(root, workspaceID, executable string) *AgentClientReport {
	path := filepath.Join(root, ".grok", "config.toml")
	if _, err := os.Lstat(path); err != nil {
		return nil
	}
	name, err := adapter.ServerNameFor(workspaceID)
	if err != nil {
		return nil
	}
	expectedArgs, err := expectedGrokProjectArgs(workspaceID)
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return &AgentClientReport{
			Target:        integrations.TargetGrok,
			Status:        AgentUnverified,
			ConfigPath:    path,
			Scope:         AgentScopeProject,
			WorkspaceID:   workspaceID,
			WorkspaceRoot: root,
			Binding:       string(adapter.WorkspaceBindingVerifiedCwd),
			Provenance:    AgentProvenanceNative,
			ServerName:    name,
			Detail:        err.Error(),
		}
	}
	entry, ok := host.TOMLServerEntry(raw, name)
	if !ok {
		return nil
	}
	client := AgentClientReport{
		Target:        integrations.TargetGrok,
		Command:       entry.Command,
		Args:          append([]string(nil), entry.Args...),
		ConfigPath:    path,
		Scope:         AgentScopeProject,
		WorkspaceID:   workspaceID,
		WorkspaceRoot: root,
		Binding:       string(adapter.WorkspaceBindingVerifiedCwd),
		Provenance:    AgentProvenanceNative,
		ServerName:    name,
	}
	if commandMatchesAtlas(entry.Command, executable, a.lookPath()) && slicesEqual(entry.Args, expectedArgs) {
		client.Status = AgentPendingClientRestart
		client.Detail = "project-scoped Grok entry matches this workspace; client restart may be required"
		return &client
	}
	client.Status = AgentUnverified
	client.Detail = "project-scoped Grok entry does not match this workspace binding, profile, bounds, or command"
	return &client
}

func expectedGrokProjectArgs(workspaceID string) ([]string, error) {
	reg, err := adapter.NewRegistration(adapter.PortableExecutableName, workspaceID, adapter.WorkspaceBinding{Kind: adapter.WorkspaceBindingVerifiedCwd}, "", true)
	if err != nil {
		return nil, err
	}
	styled, err := reg.WithToolNameStyle(adapter.ToolNameStylePortable)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), styled.Args...), nil
}

func commandMatchesAtlas(got, intended string, look func(string) (string, error)) bool {
	got = strings.TrimSpace(got)
	intended = strings.TrimSpace(intended)
	if got == "" || intended == "" {
		return false
	}
	if got != adapter.PortableExecutableName {
		return sameCommandPath(got, intended)
	}
	if look == nil {
		return false
	}
	found, err := look(adapter.PortableExecutableName)
	if err != nil || strings.TrimSpace(found) == "" {
		return false
	}
	return sameCommandPath(found, intended)
}

func sameCommandPath(got, intended string) bool {
	if got == intended || filepath.Clean(got) == filepath.Clean(intended) {
		return true
	}
	return host.SameExecutable(got, intended)
}

func inspectJSONNamed(path, name string) (command string, args []string, parsed, present, usable bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, false, false, false
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return "", nil, false, false, false
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		return "", nil, true, false, false
	}
	value, exists := servers[name]
	if !exists {
		return "", nil, true, false, false
	}
	entry, _ := value.(map[string]any)
	if entry == nil {
		return "", nil, true, true, false
	}
	command, _ = entry["command"].(string)
	args, ok := jsonStringSlice(entry["args"])
	if strings.TrimSpace(command) == "" || !ok {
		return command, args, true, true, false
	}
	return command, args, true, true, true
}

func readWorkspaceID(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, ".tracker", "workspace.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if json.Unmarshal(raw, &meta) != nil {
		return ""
	}
	return strings.TrimSpace(meta.WorkspaceID)
}
