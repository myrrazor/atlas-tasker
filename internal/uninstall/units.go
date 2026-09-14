package uninstall

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

const (
	homeLaunchdLabel   = "com.atlas-tasker.home"
	homeSystemdService = "atlas-home.service"
	legacyLinuxService = "atlas-tasker.service"
	backupLaunchdPref  = "com.atlas-tasker.backup."
	backupSystemdPref  = "atlas-backup-"
	scheduleMarker     = "Atlas-owned backup scheduler. Do not edit by hand."
	homeServiceMarker  = "Atlas-owned Home service. Do not edit by hand."
	canonicalGlobalKey = "atlas-tasker"
	globalMCPServeSub  = "mcp"
	globalMCPServeCmd  = "serve"
	globalMCPServeFlag = "--global"
)

func allowedUnitName(name string) bool {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".plist")
	switch name {
	case homeLaunchdLabel, homeSystemdService, legacyLinuxService:
		return true
	}
	if strings.HasPrefix(name, backupLaunchdPref) {
		id := strings.TrimPrefix(name, backupLaunchdPref)
		return unitIDRe.MatchString(id) && !strings.Contains(id, "..")
	}
	if strings.HasPrefix(name, backupSystemdPref) {
		rest := strings.TrimPrefix(name, backupSystemdPref)
		rest = strings.TrimSuffix(strings.TrimSuffix(rest, ".timer"), ".service")
		return unitIDRe.MatchString(rest) && !strings.Contains(rest, "..")
	}
	return false
}

func unitLabel(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".plist")
}

func isGUIDomain(value string) bool {
	if !strings.HasPrefix(value, "gui/") {
		return false
	}
	uid := strings.TrimPrefix(value, "gui/")
	if uid == "" {
		return false
	}
	for _, r := range uid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func launchctlBootoutArgv(path string) (string, []string, error) {
	if !filepath.IsAbs(path) || !strings.HasSuffix(path, ".plist") {
		return "", nil, apperr.New(apperr.CodeConflict, "launchctl bootout requires an absolute plist path")
	}
	label := unitLabel(path)
	if !allowedUnitName(label) {
		return "", nil, apperr.New(apperr.CodeConflict, "service label is not allowlisted")
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	return "launchctl", []string{"bootout", domain, path}, nil
}

func systemdArgv(action string, unit string) (string, []string, error) {
	if action != "stop" && action != "disable" {
		return "", nil, apperr.New(apperr.CodeConflict, "unsupported systemctl action")
	}
	if !allowedUnitName(unit) {
		return "", nil, apperr.New(apperr.CodeConflict, "service unit is not allowlisted")
	}
	return "systemctl", []string{"--user", action, unit}, nil
}

func parsePlistProgramArgs(body string) []string {
	idx := strings.Index(body, "<key>ProgramArguments</key>")
	if idx < 0 {
		return nil
	}
	rest := body[idx:]
	start := strings.Index(rest, "<array>")
	end := strings.Index(rest, "</array>")
	if start < 0 || end < 0 || end <= start {
		return nil
	}
	chunk := rest[start:end]
	var args []string
	for {
		open := strings.Index(chunk, "<string>")
		close := strings.Index(chunk, "</string>")
		if open < 0 || close < 0 || close <= open {
			break
		}
		args = append(args, chunk[open+len("<string>"):close])
		chunk = chunk[close+len("</string>"):]
	}
	return args
}

func parseSystemdExecStart(body string) []string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ExecStart=") {
			return strings.Fields(strings.TrimPrefix(trimmed, "ExecStart="))
		}
	}
	return nil
}

func unitHasMarker(body string) bool {
	return strings.Contains(body, scheduleMarker) || strings.Contains(body, homeServiceMarker)
}

func exactProgramBinary(args []string, binary string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == binary
}

func pairedTimerPath(servicePath string) string {
	if !strings.HasSuffix(servicePath, ".service") {
		return ""
	}
	return strings.TrimSuffix(servicePath, ".service") + ".timer"
}

func pairedServicePath(timerPath string) string {
	if !strings.HasSuffix(timerPath, ".timer") {
		return ""
	}
	return strings.TrimSuffix(timerPath, ".timer") + ".service"
}
