package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

// Execute runs the CLI and returns the process exit code.
func Execute(args []string, stdout io.Writer, stderr io.Writer) int {
	return executeWithSurface(args, stdout, stderr, contracts.EventSurfaceCLI)
}

func executeWithSurface(args []string, stdout io.Writer, stderr io.Writer, surface contracts.EventSurface) int {
	root := NewRootCommand()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	root.SetContext(service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: surface}))
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
