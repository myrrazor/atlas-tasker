package cli

import "github.com/myrrazor/atlas-tasker/internal/slashcmd"

// ParseSlashCommand converts slash input into tracker CLI args.
func ParseSlashCommand(input string) ([]string, error) {
	return slashcmd.Parse(input)
}
