package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type integrationInstallSummary struct {
	Kind     string                       `json:"kind"`
	Targets  []string                     `json:"targets"`
	Results  []integrations.InstallResult `json:"results"`
	Detected []integrations.Detection     `json:"detected,omitempty"`
	Skipped  bool                         `json:"skipped,omitempty"`
	Message  string                       `json:"message"`
}

func runIntegrationsDetect(cmd *cobra.Command, _ []string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	detections := integrations.Detect(integrations.DetectOptions{Workspace: root})
	found := integrations.DetectedTargets(integrations.DetectOptions{Workspace: root})
	payload := map[string]any{
		"kind":       "integrations_detect",
		"detections": detections,
		"found":      found,
	}
	var b strings.Builder
	if len(found) == 0 {
		b.WriteString("no coding agents detected")
	} else {
		b.WriteString("detected coding agents:\n")
		for i, item := range found {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(fmt.Sprintf("- %s (%s)", item.Target, strings.Join(item.Reasons, "; ")))
		}
	}
	pretty := b.String()
	return writeCommandOutput(cmd, payload, pretty, pretty)
}

func runIntegrationsInstallWizard(cmd *cobra.Command, targets []integrations.Target, force bool, global bool, interactive bool) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := ensureInitArtifacts(root); err != nil {
		return err
	}

	detections := integrations.Detect(integrations.DetectOptions{Workspace: root})
	if len(targets) == 0 && interactive {
		selected, err := integrations.SelectTargetsInteractively(integrations.SelectOptions{
			Detections:        detections,
			Stdout:            cmd.OutOrStdout(),
			Stdin:             cmd.InOrStdin(),
			DefaultToDetected: true,
		})
		if err != nil {
			return err
		}
		if selected == nil {
			summary := integrationInstallSummary{
				Kind:     "integrations_install",
				Detected: detections,
				Skipped:  true,
				Message:  "installation cancelled",
				Targets:  []string{},
				Results:  []integrations.InstallResult{},
			}
			return writeCommandOutput(cmd, summary, summary.Message, summary.Message)
		}
		targets = selected
	}
	if len(targets) == 0 {
		return apperr.New(apperr.CodeInvalidInput, "no integration targets selected; pass a target, --targets, or run in a TTY")
	}
	if global && (len(targets) != 1 || targets[0] != integrations.TargetOpenClaw) {
		return apperr.New(apperr.CodeInvalidInput, "--global is only supported when installing openclaw")
	}

	results := make([]integrations.InstallResult, 0, len(targets))
	names := make([]string, 0, len(targets))
	lines := make([]string, 0, len(targets))
	installer := integrations.Installer{Root: root}
	for _, target := range targets {
		result, err := installer.InstallOpts(target, integrations.InstallOptions{
			Force:  force,
			Global: global && target == integrations.TargetOpenClaw,
		})
		if err != nil {
			return err
		}
		results = append(results, result)
		names = append(names, string(target))
		lines = append(lines, fmt.Sprintf("installed %s guidance into %s", target, result.InstructionFile))
	}
	summary := integrationInstallSummary{
		Kind:     "integrations_install",
		Targets:  names,
		Results:  results,
		Detected: detections,
		Message:  strings.Join(lines, "\n"),
	}
	return writeCommandOutput(cmd, summary, summary.Message, summary.Message)
}

func stdinIsInteractive(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func stdoutIsInteractive(cmd *cobra.Command) bool {
	file, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func canPromptIntegrations(cmd *cobra.Command) bool {
	jsonMode, _ := cmd.Flags().GetBool("json")
	if jsonMode {
		return false
	}
	return stdinIsInteractive(cmd) && stdoutIsInteractive(cmd)
}

func confirmIntegrationsSetup(cmd *cobra.Command) (bool, error) {
	fmt.Fprint(cmd.OutOrStdout(), "Set up coding-agent integrations now? [Y/n] ")
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" || line == "y" || line == "yes" {
		return true, nil
	}
	if line == "n" || line == "no" {
		return false, nil
	}
	return false, apperr.New(apperr.CodeInvalidInput, "please answer y or n")
}
