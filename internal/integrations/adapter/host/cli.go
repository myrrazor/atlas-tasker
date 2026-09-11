package host

import (
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func clientInspectCommands(target integrations.Target, exe, serverName, workspaceRoot string) []adapter.Command {
	timeout := 15 * time.Second
	switch target {
	case integrations.TargetCodex:
		return optionalGet(adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "list"}, Timeout: timeout},
			exe, []string{"mcp", "get", serverName}, "", serverName, timeout)
	case integrations.TargetClaude:
		return optionalGet(adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "list"}, Timeout: timeout},
			exe, []string{"mcp", "get", serverName}, "", serverName, timeout)
	case integrations.TargetOpenClaw:
		cmds := []adapter.Command{}
		if serverName != "" {
			cmds = append(cmds, adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "doctor", serverName, "--probe"}, Timeout: 20 * time.Second})
			cmds = append(cmds, adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "show", serverName, "--json"}, Timeout: timeout})
		}
		return cmds
	case integrations.TargetGrok:
		cmds := []adapter.Command{{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "list", "--json"}, Dir: workspaceRoot, Timeout: timeout}}
		if serverName != "" {
			cmds = append(cmds, adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "doctor", serverName, "--json"}, Dir: workspaceRoot, Timeout: 20 * time.Second})
		}
		return cmds
	default:
		return nil
	}
}

func clientInventoryCommands(target integrations.Target, exe, serverName, workspaceRoot string) []adapter.Command {
	timeout := 15 * time.Second
	switch target {
	case integrations.TargetCodex, integrations.TargetClaude:
		return optionalGet(adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "list"}, Timeout: timeout},
			exe, []string{"mcp", "get", serverName}, "", serverName, timeout)
	case integrations.TargetOpenClaw:
		cmds := []adapter.Command{{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "list"}, Timeout: timeout}}
		if serverName != "" {
			cmds = append(cmds, adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "show", serverName, "--json"}, Timeout: timeout})
		}
		return cmds
	case integrations.TargetGrok:
		return []adapter.Command{{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: []string{"mcp", "list", "--json"}, Dir: workspaceRoot, Timeout: timeout}}
	default:
		return nil
	}
}

func optionalGet(list adapter.Command, exe string, getArgs []string, dir, serverName string, timeout time.Duration) []adapter.Command {
	cmds := []adapter.Command{list}
	if serverName == "" {
		return cmds
	}
	get := adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: exe, Args: getArgs, Dir: dir, Timeout: timeout}
	return append(cmds, get)
}

func clientRemoveCommand(target integrations.Target, exe, serverName, workspaceRoot string) *adapter.Command {
	if exe == "" || serverName == "" {
		return nil
	}
	timeout := 15 * time.Second
	switch target {
	case integrations.TargetClaude:
		return &adapter.Command{Purpose: adapter.CommandPurposeRemove, Executable: exe, Args: []string{"mcp", "remove", serverName}, Timeout: timeout}
	case integrations.TargetOpenClaw:
		return &adapter.Command{Purpose: adapter.CommandPurposeRemove, Executable: exe, Args: []string{"mcp", "unset", serverName}, Timeout: timeout}
	case integrations.TargetGrok:
		return &adapter.Command{Purpose: adapter.CommandPurposeRemove, Executable: exe, Args: []string{"mcp", "remove", "--scope", "project", serverName}, Dir: workspaceRoot, Timeout: timeout}
	default:
		return nil
	}
}

func methodForInspect(cmd adapter.Command) adapter.VerificationMethod {
	joined := strings.Join(cmd.Args, " ")
	switch {
	case strings.Contains(joined, "doctor"):
		return adapter.VerificationClientCLIDoctor
	case strings.Contains(joined, "get") || strings.Contains(joined, "show"):
		return adapter.VerificationClientCLIGet
	default:
		return adapter.VerificationClientCLIList
	}
}

func isListCommand(cmd adapter.Command) bool {
	return methodForInspect(cmd) == adapter.VerificationClientCLIList
}

func nativeProof(target integrations.Target, method adapter.VerificationMethod, ok bool, mentions bool, detail string) (proves, doctor bool) {
	if !ok || !mentions {
		return false, false
	}
	switch method {
	case adapter.VerificationClientCLIDoctor:
		if doctorProbeFailed(detail) {
			return false, false
		}
		return true, true
	case adapter.VerificationClientCLIGet:
		if target == integrations.TargetOpenClaw {
			return false, false
		}
		return true, false
	default:
		return false, false
	}
}

func doctorProbeFailed(detail string) bool {
	lower := strings.ToLower(detail)
	for _, marker := range []string{
		"probe failed", "connection refused", "unhealthy", "offline",
		"not running", "failed to", "error:", "timed out", "timeout",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
