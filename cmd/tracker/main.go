package main

import (
	"os"

	// Imported first, and standard-library only, so a server registers for
	// SIGINT/SIGTERM before the rest of the CLI initializes.
	_ "github.com/myrrazor/atlas-tasker/internal/earlysignal"

	"github.com/myrrazor/atlas-tasker/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
