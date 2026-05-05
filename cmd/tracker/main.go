package main

import (
	"os"

	"github.com/myrrazor/atlas-tasker/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
