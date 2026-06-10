package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newTeamCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "team", Short: "Apply ready-made agent team presets"}
	cmd.AddCommand(newTeamListCommand())
	cmd.AddCommand(newTeamShowCommand())
	cmd.AddCommand(newTeamApplyCommand())
	return cmd
}

func newTeamListCommand() *cobra.Command {
	flags := &outputFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available team presets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			presets, err := service.TeamPresets("")
			if err != nil {
				return err
			}
			payload := map[string]any{"kind": "team_preset_list", "generated_at": time.Now().UTC(), "presets": presets}
			lines := []string{"team presets:"}
			for _, preset := range presets {
				lines = append(lines, fmt.Sprintf("- %s: %s", preset.Name, preset.Summary))
			}
			lines = append(lines, "", "apply one: tracker team apply <preset> --actor human:owner --reason \"team setup\"")
			text := strings.Join(lines, "\n")
			return writeCommandOutput(cmd, payload, text, text)
		},
	}
	addReadOutputFlags(cmd, flags)
	return cmd
}

func newTeamShowCommand() *cobra.Command {
	flags := &outputFlags{}
	provider := ""
	cmd := &cobra.Command{
		Use:   "show <PRESET>",
		Short: "Show what a team preset would set up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			preset, err := service.TeamPresetByName(args[0], provider)
			if err != nil {
				return err
			}
			payload := map[string]any{"kind": "team_preset", "generated_at": time.Now().UTC(), "preset": preset}
			lines := []string{fmt.Sprintf("%s (%s)", preset.DisplayName, preset.Name), preset.Summary, ""}
			lines = append(lines, fmt.Sprintf("completion mode: %s", preset.Completion), "agents:")
			for _, agent := range preset.Agents {
				roles := make([]string, 0, len(agent.PreferredRoles))
				for _, role := range agent.PreferredRoles {
					roles = append(roles, string(role))
				}
				lines = append(lines, fmt.Sprintf("- %s (%s) roles=%s", agent.AgentID, agent.Provider, strings.Join(roles, ",")))
			}
			for _, runbook := range preset.Runbooks {
				lines = append(lines, fmt.Sprintf("runbook: %s (%d stages)", runbook.Name, len(runbook.Stages)))
			}
			for _, profile := range preset.Profiles {
				lines = append(lines, fmt.Sprintf("permissions: %s", profile.ProfileID))
			}
			text := strings.Join(lines, "\n")
			return writeCommandOutput(cmd, payload, text, text)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Preview with a provider: claude, codex, or mixed")
	addReadOutputFlags(cmd, flags)
	return cmd
}

func newTeamApplyCommand() *cobra.Command {
	flags := &outputFlags{}
	mutation := &mutationFlags{Actor: "human:owner"}
	provider := ""
	dryRun := false
	cmd := &cobra.Command{
		Use:   "apply <PRESET>",
		Short: "Create the preset's agents, runbook, permissions, and completion mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace, err := openWorkspace()
			if err != nil {
				return err
			}
			defer workspace.close()
			result, err := workspace.actions.ApplyTeamPreset(cmd.Context(), args[0], provider, dryRun, contracts.Actor(strings.TrimSpace(mutation.Actor)), mutation.Reason)
			if err != nil {
				return err
			}
			payload := map[string]any{"kind": "team_apply_result", "generated_at": time.Now().UTC(), "result": result}
			lines := []string{fmt.Sprintf("team preset %s", result.Preset)}
			if result.DryRun {
				lines[0] += " (dry run)"
			}
			for _, item := range result.Created {
				verb := "created"
				if result.DryRun {
					verb = "would create"
				}
				lines = append(lines, fmt.Sprintf("- %s %s", verb, item))
			}
			for _, item := range result.Skipped {
				lines = append(lines, "- skipped "+item)
			}
			if len(result.NextSteps) > 0 {
				lines = append(lines, "", "next steps:")
				for _, step := range result.NextSteps {
					lines = append(lines, "- "+step)
				}
			}
			text := strings.Join(lines, "\n")
			return writeCommandOutput(cmd, payload, text, text)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Agent provider: claude, codex, or mixed")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without creating anything")
	addMutationFlags(cmd, mutation)
	addReadOutputFlags(cmd, flags)
	return cmd
}
