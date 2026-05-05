package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/spf13/cobra"
)

func newShellCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Interactive slash-command shell",
		RunE: func(_ *cobra.Command, _ []string) error {
			scanner := bufio.NewScanner(os.Stdin)
			for {
				fmt.Fprint(os.Stdout, "tracker> ")
				if !scanner.Scan() {
					break
				}
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				if line == "/exit" || line == "/quit" {
					return nil
				}
				args, err := ParseSlashCommand(line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "shell parse error: %v\n", err)
					continue
				}
				if len(args) == 0 {
					continue
				}
				if err := executeArgsWithSurface(args, contracts.EventSurfaceShell); err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
				}
			}
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}
