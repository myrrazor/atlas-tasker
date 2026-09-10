package adapter

import (
	"strings"
	"testing"
	"time"
)

func validCommand() Command {
	return Command{
		Purpose:    CommandPurposeInspect,
		Executable: "/usr/local/bin/codex",
		Args:       []string{"mcp", "list", "--json"},
		Dir:        "/srv/workspace",
		Env:        []EnvVar{{Name: "HOME", Value: "/tmp/fake-home"}},
		Timeout:    30 * time.Second,
	}
}

func TestCommandValidateAccepts(t *testing.T) {
	if err := validCommand().Validate(); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
	if validCommand().Mutates() {
		t.Fatal("inspect must not count as a mutation")
	}
	register := validCommand()
	register.Purpose = CommandPurposeRegister
	if !register.Mutates() {
		t.Fatal("register must count as a mutation")
	}
}

func TestCommandValidateRejects(t *testing.T) {
	cases := map[string]func(*Command){
		"purpose":            func(c *Command) { c.Purpose = "shell" },
		"relative exe":       func(c *Command) { c.Executable = "codex" },
		"unclean exe":        func(c *Command) { c.Executable = "/usr/local/bin/../bin/codex" },
		"empty exe":          func(c *Command) { c.Executable = "" },
		"sh":                 func(c *Command) { c.Executable = "/bin/sh" },
		"bash":               func(c *Command) { c.Executable = "/usr/bin/bash" },
		"cmd.exe":            func(c *Command) { c.Executable = "/c/Windows/System32/cmd.exe" },
		"powershell":         func(c *Command) { c.Executable = "/usr/bin/PowerShell" },
		"nul arg":            func(c *Command) { c.Args = []string{"a\x00b"} },
		"credential url arg": func(c *Command) { c.Args = []string{"https://user:pass@example.com/repo.git"} },
		"relative dir":       func(c *Command) { c.Dir = "workspace" },
		"unclean dir":        func(c *Command) { c.Dir = "/srv/workspace/" },
		"bad env name":       func(c *Command) { c.Env = []EnvVar{{Name: "1BAD", Value: "x"}} },
		"secret env":         func(c *Command) { c.Env = []EnvVar{{Name: "GITHUB_TOKEN", Value: "x"}} },
		"api key env":        func(c *Command) { c.Env = []EnvVar{{Name: "MY_API_KEY", Value: "x"}} },
		"dup env":            func(c *Command) { c.Env = []EnvVar{{Name: "HOME", Value: "a"}, {Name: "HOME", Value: "b"}} },
		"nul env value":      func(c *Command) { c.Env = []EnvVar{{Name: "HOME", Value: "a\x00"}} },
		"zero timeout":       func(c *Command) { c.Timeout = 0 },
		"negative timeout":   func(c *Command) { c.Timeout = -time.Second },
		"huge timeout":       func(c *Command) { c.Timeout = MaxCommandTimeout + time.Second },
	}
	for name, mutate := range cases {
		cmd := validCommand()
		mutate(&cmd)
		if err := cmd.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestCommandDisplayQuotesEveryElement(t *testing.T) {
	cmd := Command{Executable: "/usr/local/bin/tracker", Args: []string{"mcp", "serve", "--workspace", "/path with space", "$(rm -rf /)"}}
	got := cmd.Display()
	want := `"/usr/local/bin/tracker" "mcp" "serve" "--workspace" "/path with space" "$(rm -rf /)"`
	if got != want {
		t.Fatalf("Display() = %s, want %s", got, want)
	}
	if strings.Contains(got, "\n") {
		t.Fatal("display must be single-line")
	}
}

func TestContainsCredentialURL(t *testing.T) {
	yes := []string{"https://alice:secret@github.com/org/repo.git", "ssh://git@host/repo", "http://token@host"}
	no := []string{"https://github.com/org/repo.git", "git@github.com:org/repo.git", "--workspace", "user@host", "https://host/path?x=a@b"}
	for _, value := range yes {
		if !containsCredentialURL(value) {
			t.Errorf("%q should be detected as a credential URL", value)
		}
	}
	for _, value := range no {
		if containsCredentialURL(value) {
			t.Errorf("%q should not be detected as a credential URL", value)
		}
	}
}

func TestProbeRecordsMustBeReadOnlyInDetection(t *testing.T) {
	cmd := validCommand()
	cmd.Purpose = CommandPurposeRegister
	detection := Detection{Target: "codex", Installed: true, ExecutablePath: "/usr/local/bin/codex", VersionSupport: VersionUnknown, Probes: []ProbeRecord{{Command: cmd}}}
	if err := detection.Validate(); err == nil {
		t.Fatal("mutating probe must be rejected in detection")
	}
	cmd.Purpose = CommandPurposeDetectVersion
	detection.Probes = []ProbeRecord{{Command: cmd, ExitCode: 0}}
	if err := detection.Validate(); err != nil {
		t.Fatalf("read-only probe rejected: %v", err)
	}
}
