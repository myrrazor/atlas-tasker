package cli

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newCompactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "compact", Short: "Remove compactable derived artifacts", RunE: runCompact}
	cmd.Flags().Bool("yes", false, "Apply compaction without prompting")
	addMutationFlags(cmd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func runCompact(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	yes, _ := cmd.Flags().GetBool("yes")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	result, err := workspace.actions.CompactWorkspace(commandContext(cmd), yes, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatCompactResult(result)
	data := map[string]any{"kind": "compact_result", "generated_at": result.GeneratedAt, "payload": result}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatCompactResult(result service.CompactResult) string {
	line := fmt.Sprintf("compact removed=%d bytes=%d", len(result.RemovedPaths), result.BytesFreed)
	if len(result.SkippedPaths) > 0 {
		line += " skipped=" + strings.Join(result.SkippedPaths, ",")
	}
	return line
}
