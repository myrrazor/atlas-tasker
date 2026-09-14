package app

import (
	"context"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

// SilentRunner never execs a client binary. Tests inject this so Detect/Plan
// cannot hang on host claude/codex/cursor.
type SilentRunner struct{}

func (SilentRunner) Run(_ context.Context, _ adapter.Command) (adapter.CommandResult, error) {
	return adapter.CommandResult{ExitCode: -1}, nil
}

type RecordingRunner struct {
	Commands []adapter.Command
}

func (r *RecordingRunner) Run(_ context.Context, command adapter.Command) (adapter.CommandResult, error) {
	r.Commands = append(r.Commands, command)
	return adapter.CommandResult{ExitCode: 0, Stdout: []byte("ok")}, nil
}
