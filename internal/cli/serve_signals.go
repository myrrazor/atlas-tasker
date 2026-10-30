package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// serveShutdownGrace bounds how long a long-running server may take to stop
// after the first SIGINT/SIGTERM before the process exits anyway.
var serveShutdownGrace = 10 * time.Second

// serveExit is swappable for tests.
var serveExit = os.Exit

// serveSignalContext cancels the returned context on the first SIGINT or
// SIGTERM so the server shuts down gracefully. Registering with
// signal.Notify also re-enables SIGINT when the process was started with it
// ignored (background jobs, some supervisors and PTY harnesses), which is
// why Ctrl-C used to leave `tracker serve` running. A second signal, or a
// shutdown that outlives serveShutdownGrace, exits immediately so the
// process can't stay wedged.
func serveSignalContext(parent context.Context, stderr io.Writer) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
		case <-done:
			return
		}
		cancel()
		timer := time.NewTimer(serveShutdownGrace)
		defer timer.Stop()
		select {
		case <-signals:
			fmt.Fprintln(stderr, "second interrupt; exiting without waiting for shutdown")
			serveExit(130)
		case <-timer.C:
			fmt.Fprintf(stderr, "shutdown did not finish within %s; exiting\n", serveShutdownGrace)
			serveExit(1)
		case <-done:
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		close(done)
		cancel()
	}
}
