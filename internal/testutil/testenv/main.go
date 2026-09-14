// Package testenv isolates machine-scoped Atlas state for test processes and
// any CLI children they launch. It is not used by the application.
package testenv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Run(run func() int) int {
	// Reuse installed toolchains without putting Go caches in the fixture HOME.
	if raw, err := exec.Command("go", "env", "GOCACHE", "GOPATH").Output(); err == nil {
		values := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(values) == 2 {
			_ = os.Setenv("GOCACHE", values[0])
			_ = os.Setenv("GOPATH", values[1])
		}
	}
	home, err := os.MkdirTemp("", "atlas-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(home)
	state := filepath.Join(home, "state", "atlas-tasker")
	if err = os.MkdirAll(state, 0o700); err == nil {
		err = os.WriteFile(filepath.Join(state, "settings.json"), []byte(`{"format":"atlas_machine_settings_v1","service":{"enabled":false,"auto_start":false},"agents":{"auto_install":false},"browser":{"open_home":false}}`), 0o600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for key, value := range map[string]string{"HOME": home, "XDG_STATE_HOME": filepath.Join(home, "state"), "XDG_CONFIG_HOME": filepath.Join(home, "config")} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return run()
}
