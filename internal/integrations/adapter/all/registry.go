package all

import (
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/claude"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/codex"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/cursor"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/generic"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/grok"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/openclaw"
)

// Options bind engine-local paths onto the six adapters.
type Options struct {
	StateDir string
	Runner   adapter.CommandRunner
}

// New returns a registry with every Sprint 114.2 adapter.
func New(opts Options) (*adapter.Registry, error) {
	runner := opts.Runner
	if runner == nil {
		runner = host.DefaultRunner{}
	}
	reg := adapter.NewRegistry()
	for _, item := range []adapter.AgentIntegrationAdapter{
		codex.New().WithStateDir(opts.StateDir).WithRunner(runner),
		claude.New().WithStateDir(opts.StateDir).WithRunner(runner),
		cursor.New().WithStateDir(opts.StateDir).WithRunner(runner),
		openclaw.New().WithStateDir(opts.StateDir).WithRunner(runner),
		grok.New().WithStateDir(opts.StateDir).WithRunner(runner),
		generic.New().WithStateDir(opts.StateDir).WithRunner(runner),
	} {
		if err := reg.Register(item); err != nil {
			return nil, err
		}
	}
	return reg, nil
}
