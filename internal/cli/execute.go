package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

// Execute runs the CLI and returns the process exit code.
func Execute(args []string, stdout io.Writer, stderr io.Writer) int {
	root := NewRootCommand()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if wantsJSON(args) {
			raw, marshalErr := json.MarshalIndent(apperr.Envelope(err), "", "  ")
			if marshalErr == nil {
				fmt.Fprintln(stderr, string(raw))
			} else {
				fmt.Fprintln(stderr, err.Error())
			}
		} else {
			fmt.Fprintln(stderr, err.Error())
		}
		return apperr.ExitCode(err)
	}
	return 0
}

func wantsJSON(args []string) bool {
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "--json" {
			return true
		}
		if !strings.HasPrefix(arg, "--json=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(arg, "--json="))
		if value == "" {
			return true
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return true
		}
		return parsed
	}
	return false
}
