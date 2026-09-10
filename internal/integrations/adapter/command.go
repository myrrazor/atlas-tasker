package adapter

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CommandPurpose classifies why an adapter runs an external client binary.
// Every purpose is a direct argv execution; the contract has no shell purpose.
type CommandPurpose string

const (
	CommandPurposeDetectVersion CommandPurpose = "detect_version"
	CommandPurposeInspect       CommandPurpose = "inspect"
	CommandPurposeRegister      CommandPurpose = "register"
	CommandPurposeProbe         CommandPurpose = "probe"
	CommandPurposeReload        CommandPurpose = "reload"
	CommandPurposeRemove        CommandPurpose = "remove"
)

func (p CommandPurpose) IsValid() bool {
	switch p {
	case CommandPurposeDetectVersion, CommandPurposeInspect, CommandPurposeRegister, CommandPurposeProbe, CommandPurposeReload, CommandPurposeRemove:
		return true
	default:
		return false
	}
}

// Mutates reports whether commands with this purpose change client-side state
// and therefore need rollback material or an irreversible classification.
func (p CommandPurpose) Mutates() bool {
	switch p {
	case CommandPurposeRegister, CommandPurposeReload, CommandPurposeRemove:
		return true
	default:
		return false
	}
}

// MaxCommandTimeout bounds every external client invocation. Unbounded waits
// are a plan validation error, not a runtime surprise.
const MaxCommandTimeout = 10 * time.Minute

// EnvVar is one explicitly provided environment entry. Runners never inherit
// the parent environment implicitly beyond the documented pass-through set;
// anything an adapter needs is listed here so it appears in the plan.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Command is the structured, non-shell model for invoking a client binary.
// Executable is always an absolute path; Args are passed as argv without any
// interpolation, quoting, or expansion.
type Command struct {
	Purpose    CommandPurpose `json:"purpose"`
	Executable string         `json:"executable"`
	Args       []string       `json:"args"`
	Dir        string         `json:"dir,omitempty"`
	Env        []EnvVar       `json:"env,omitempty"`
	Timeout    time.Duration  `json:"timeout_ns"`
}

// shellInterpreters are refused as Executable so an adapter cannot smuggle a
// shell string through the structured model.
var shellInterpreters = map[string]struct{}{
	"sh": {}, "bash": {}, "zsh": {}, "dash": {}, "ksh": {}, "fish": {}, "csh": {}, "tcsh": {}, "ash": {},
	"cmd": {}, "cmd.exe": {}, "powershell": {}, "powershell.exe": {}, "pwsh": {}, "pwsh.exe": {},
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretEnvMarkers is a conservative guard, not a classifier: plans are
// rendered in --json output, so an environment name that announces a secret
// is refused outright.
var secretEnvMarkers = []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "API_KEY", "APIKEY", "PRIVATE_KEY", "CREDENTIAL"}

func (c Command) Validate() error {
	if !c.Purpose.IsValid() {
		return fmt.Errorf("invalid command purpose: %q", c.Purpose)
	}
	if err := validateAbsoluteExecutable(c.Executable); err != nil {
		return err
	}
	if _, shell := shellInterpreters[strings.ToLower(filepath.Base(c.Executable))]; shell {
		return fmt.Errorf("command executable %q is a shell interpreter; adapters must invoke the client binary directly", c.Executable)
	}
	for i, arg := range c.Args {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("command arg %d contains a NUL byte", i)
		}
		if containsCredentialURL(arg) {
			return fmt.Errorf("command arg %d embeds credentials in a URL", i)
		}
	}
	if c.Dir != "" {
		if !filepath.IsAbs(c.Dir) || filepath.Clean(c.Dir) != c.Dir {
			return fmt.Errorf("command dir must be a clean absolute path: %q", c.Dir)
		}
	}
	seen := map[string]struct{}{}
	for _, entry := range c.Env {
		if !envNamePattern.MatchString(entry.Name) {
			return fmt.Errorf("invalid environment variable name: %q", entry.Name)
		}
		if _, dup := seen[entry.Name]; dup {
			return fmt.Errorf("duplicate environment variable: %s", entry.Name)
		}
		seen[entry.Name] = struct{}{}
		upper := strings.ToUpper(entry.Name)
		for _, marker := range secretEnvMarkers {
			if strings.Contains(upper, marker) {
				return fmt.Errorf("environment variable %s looks like a secret and cannot appear in a plan", entry.Name)
			}
		}
		if strings.ContainsRune(entry.Value, 0) {
			return fmt.Errorf("environment variable %s contains a NUL byte", entry.Name)
		}
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("command timeout must be positive")
	}
	if c.Timeout > MaxCommandTimeout {
		return fmt.Errorf("command timeout %s exceeds the %s bound", c.Timeout, MaxCommandTimeout)
	}
	return nil
}

// Mutates reports whether the command changes client-side state.
func (c Command) Mutates() bool { return c.Purpose.Mutates() }

// Display renders the argv for plans and logs. Each element is quoted with Go
// string syntax so the rendering is unambiguous and never valid shell input by
// accident.
func (c Command) Display() string {
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, strconv.Quote(c.Executable))
	for _, arg := range c.Args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
}

func validateAbsoluteExecutable(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("executable path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("executable must be an absolute path: %q", path)
	}
	if filepath.Clean(path) != path {
		return fmt.Errorf("executable path must be clean: %q", path)
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("executable path contains a NUL byte")
	}
	return nil
}

// containsCredentialURL detects userinfo in URLs (scheme://user:pass@host).
// Backup targets and MCP registrations both refuse this form.
func containsCredentialURL(value string) bool {
	idx := strings.Index(value, "://")
	if idx < 0 {
		return false
	}
	rest := value[idx+3:]
	end := strings.IndexAny(rest, "/?#")
	if end >= 0 {
		rest = rest[:end]
	}
	return strings.Contains(rest, "@")
}

// CommandResult is what a CommandRunner reports back. Stdout and Stderr are
// captured for verification parsing and must be sanitized before they reach
// status output.
type CommandResult struct {
	ExitCode int           `json:"exit_code"`
	Stdout   []byte        `json:"-"`
	Stderr   []byte        `json:"-"`
	Duration time.Duration `json:"duration_ns"`
	TimedOut bool          `json:"timed_out"`
}

// CommandRunner executes a validated Command. Tests inject fakes; the
// production runner uses os/exec with the argv exactly as given.
type CommandRunner interface {
	Run(ctx context.Context, command Command) (CommandResult, error)
}

// ProbeRecord is the plan-visible trace of a read-only command executed during
// detection or verification, so planning output can show every external call.
type ProbeRecord struct {
	Command  Command `json:"command"`
	ExitCode int     `json:"exit_code"`
	TimedOut bool    `json:"timed_out"`
	Summary  string  `json:"summary,omitempty"`
}
