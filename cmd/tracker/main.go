package main

import (
	"os"

	"github.com/myrrazor/atlas-tasker/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
