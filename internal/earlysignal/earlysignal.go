// Package earlysignal registers for SIGINT and SIGTERM during package
// initialization when the process is a long-running server (`tracker serve`
// or `tracker web serve`).
//
// When a process starts with SIGINT ignored (a background job, some
// supervisors and terminal harnesses), the kernel discards any SIGINT that
// arrives before the Go runtime installs a handler, and the runtime only does
// that on signal.Notify. Registering from main()'s command handler left a
// window of tens of milliseconds while other packages initialized and the
// command tree was built. This package imports only the standard library so it
// initializes before the rest of the CLI, and signals that arrive before the
// server asks for them are kept in a buffered channel instead of being lost.
package earlysignal

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	mu      sync.Mutex
	pending chan os.Signal
)

func init() {
	if IsServeCommand(os.Args[1:]) {
		pending = make(chan os.Signal, 2)
		signal.Notify(pending, os.Interrupt, syscall.SIGTERM)
	}
}

// IsServeCommand reports whether args (without the program name) run
// `serve` or `web serve`. Flags before the subcommand are skipped; `mcp serve`
// and every other command return false.
func IsServeCommand(args []string) bool {
	var words []string
	for _, arg := range args {
		if len(arg) > 0 && arg[0] == '-' {
			continue
		}
		words = append(words, arg)
		if len(words) == 2 {
			break
		}
	}
	switch {
	case len(words) >= 1 && words[0] == "serve":
		return true
	case len(words) >= 2 && words[0] == "web" && words[1] == "serve":
		return true
	}
	return false
}

// Take returns the channel registered at init (nil if none) and hands its
// ownership to the caller, who must eventually call signal.Stop on it. Any
// SIGINT/SIGTERM that arrived since init is still buffered in it.
func Take() chan os.Signal {
	mu.Lock()
	defer mu.Unlock()
	ch := pending
	pending = nil
	return ch
}
