package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/setup"
	"github.com/spf13/cobra"
)

func newSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Plan and apply Atlas agent setup for this workspace",
		Long:  "Inspect the workspace and installed agents, produce a read-only plan, and apply per-provider transactions. Planning never writes. Backup is a separate consent group.",
		RunE:  runSetup,
	}
	cmd.Flags().Bool("plan", false, "Print the setup plan and write nothing")
	cmd.Flags().Bool("yes", false, "Apply the plan without prompting; does not consent to backup or unnamed machine-wide scopes")
	cmd.Flags().String("agents", "", "Comma-separated targets, or 'all'")
	cmd.Flags().String("mode", "", "Managed project mode: guidance|managed|disabled")
	cmd.Flags().String("team", "", "Requested team policy (pair, etc.); does not overwrite existing agent roles")
	cmd.Flags().Bool("backup", false, "Include the backup group; --yes is not backup consent")
	cmd.Flags().String("backup-target", "", "Backup target ID; separate consent from --yes")
	addReadOutputFlags(cmd, &outputFlags{})

	status := &cobra.Command{Use: "status", Short: "Show setup and integration state", RunE: runSetupStatus}
	addReadOutputFlags(status, &outputFlags{})

	repair := &cobra.Command{Use: "repair", Short: "Recover interrupted setup and refresh drifted Atlas-owned files", RunE: runSetupRepair}
	repair.Flags().Bool("yes", false, "Apply the repair plan")
	addReadOutputFlags(repair, &outputFlags{})

	cmd.AddCommand(status, repair)
	return cmd
}

func runSetup(cmd *cobra.Command, _ []string) error {
	engine, err := setupEngineFromCWD()
	if err != nil {
		return err
	}
	planOnly, _ := cmd.Flags().GetBool("plan")
	yes, _ := cmd.Flags().GetBool("yes")
	agentsRaw, _ := cmd.Flags().GetString("agents")
	modeRaw, _ := cmd.Flags().GetString("mode")
	backup, _ := cmd.Flags().GetBool("backup")
	backupTarget, _ := cmd.Flags().GetString("backup-target")
	team, _ := cmd.Flags().GetString("team")
	jsonMode, _ := cmd.Flags().GetBool("json")

	agents, err := parseSetupAgents(agentsRaw)
	if err != nil {
		return err
	}
	agentsAll := agentsRaw == "all"
	var mode contracts.ManagedMode
	if strings.TrimSpace(modeRaw) != "" {
		mode = contracts.ManagedMode(strings.TrimSpace(modeRaw))
		if !mode.IsValid() {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid managed mode %q", modeRaw))
		}
	}

	interactive := !planOnly && !yes && !jsonMode && canPromptIntegrations(cmd)
	if !planOnly && !yes && !interactive {
		if jsonMode {
			return apperr.New(apperr.CodeInvalidInput, "noninteractive setup requires --yes or --plan")
		}
		return apperr.New(apperr.CodeInvalidInput, "noninteractive setup requires --yes or --plan; EOF is not consent")
	}
	if !planOnly && (backup || strings.TrimSpace(backupTarget) != "") && strings.TrimSpace(backupTarget) == "" && !interactive {
		return apperr.New(apperr.CodeInvalidInput, "noninteractive backup requires --backup-target; --yes is not backup consent")
	}

	planOpts := setup.PlanOptions{Agents: agents, AgentsAll: agentsAll, Mode: mode, Backup: backup, BackupTarget: backupTarget, Team: team}
	if interactive {
		prepared, err := engine.Plan(planOpts)
		if err != nil {
			return err
		}
		accepted, err := confirmSetupPlan(cmd, prepared)
		if err != nil {
			return err
		}
		if !accepted {
			return writeCommandOutput(cmd, map[string]any{"kind": "setup_cancelled", "status": "cancelled", "wrote": false}, "cancelled; nothing written", "cancelled; nothing written")
		}
		if backup && strings.TrimSpace(backupTarget) == "" {
			ok, err := confirmBackupConsent(cmd)
			if err != nil {
				return err
			}
			if !ok {
				planOpts.Backup = false
				prepared, err = engine.Plan(planOpts)
				if err != nil {
					return err
				}
			}
		}
		report, err := engine.ApplyPrepared(commandContext(cmd), prepared, setup.ApplyOptions{Interactive: true, AllowMachineWide: containsOpenClaw(agents) || agentsAll})
		return writeSetupReport(cmd, &prepared.Plan, report, err)
	}

	prepared, err := engine.Plan(planOpts)
	if err != nil {
		return err
	}
	if planOnly {
		return writeSetupReport(cmd, &prepared.Plan, engine.PlanReport(prepared), nil)
	}
	report, err := engine.ApplyPrepared(commandContext(cmd), prepared, setup.ApplyOptions{Yes: true, AllowMachineWide: containsOpenClaw(agents) || agentsAll})
	return writeSetupReport(cmd, &prepared.Plan, report, err)
}

func runSetupStatus(cmd *cobra.Command, _ []string) error {
	engine, err := setupEngineFromCWD()
	if err != nil {
		return err
	}
	report, err := engine.StatusReport()
	if err != nil {
		return err
	}
	return writeSetupReport(cmd, nil, report, setupStatusError(report))
}

func runSetupRepair(cmd *cobra.Command, _ []string) error {
	engine, err := setupEngineFromCWD()
	if err != nil {
		return err
	}
	yes, _ := cmd.Flags().GetBool("yes")
	report, err := engine.Repair(commandContext(cmd), "", yes)
	if err != nil && report == nil {
		return err
	}
	return writeSetupReport(cmd, nil, report, err)
}

func setupEngineFromCWD() (*setup.Engine, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	engine, err := setup.NewEngine(cwd)
	if err != nil {
		return nil, err
	}
	engine.Hooks.BackupFirstCheckpoint = runSetupFirstBackup
	return engine, nil
}

func runSetupFirstBackup(ctx context.Context, targetID string) (setup.FirstBackupResult, error) {
	w, err := openWorkspace()
	if err != nil {
		return setup.FirstBackupResult{}, err
	}
	defer w.close()
	if _, err := w.actions.EnableAutoBackup(ctx, targetID); err != nil {
		return setup.FirstBackupResult{}, err
	}
	view, err := w.actions.BackupTick(ctx, true)
	if err != nil {
		return setup.FirstBackupResult{}, err
	}
	result := setup.FirstBackupResult{CheckpointID: view.CheckpointID}
	if status, statusErr := w.actions.AutoBackupStatus(ctx); statusErr == nil {
		if status.LastRemoteCheckpointID != "" {
			result.CheckpointID = status.LastRemoteCheckpointID
		} else if status.LastLocalCheckpointID != "" {
			result.CheckpointID = status.LastLocalCheckpointID
		}
		result.Verified = status.VerifiedRemote
		return result, nil
	}
	result.Verified = view.State == "verified"
	return result, nil
}

func parseSetupAgents(raw string) ([]integrations.Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "all" {
		return nil, nil
	}
	targets, err := integrations.ParseTargetList(raw)
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidInput, err.Error())
	}
	return targets, nil
}

func containsOpenClaw(targets []integrations.Target) bool {
	for _, target := range targets {
		if target == integrations.TargetOpenClaw {
			return true
		}
	}
	return false
}

func confirmSetupPlan(cmd *cobra.Command, prepared *setup.PreparedSetup) (bool, error) {
	fmt.Fprintln(cmd.OutOrStdout(), "Setup plan:")
	for _, provider := range prepared.Plan.Providers {
		if !provider.Selected {
			continue
		}
		state := "no-op"
		if !provider.NoOp {
			state = string(provider.ResultingState)
		}
		extra := ""
		if provider.MachineWide {
			extra = " (machine-wide)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "- %s %s%s\n", provider.Target, state, extra)
	}
	if prepared.Plan.ManagedMode != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "- managed mode %s\n", prepared.Plan.ManagedMode.Declared)
	}
	fmt.Fprint(cmd.OutOrStdout(), "Apply this setup plan? [y/N] ")
	return readYesNo(cmd)
}

func confirmBackupConsent(cmd *cobra.Command) (bool, error) {
	fmt.Fprint(cmd.OutOrStdout(), "Configure off-device backup as a separate transaction? Atlas-owned tickets and events would leave this machine. [y/N] ")
	return readYesNo(cmd)
}

func readYesNo(cmd *cobra.Command) (bool, error) {
	reader := bufio.NewReader(cmd.InOrStdin())
	cmd.SetIn(reader)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	if err == io.EOF && strings.TrimSpace(line) == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func writeSetupReport(cmd *cobra.Command, plan *setup.SetupPlan, report *setup.RunReport, runErr error) error {
	data := map[string]any{}
	if report != nil {
		data["kind"] = report.Kind
		data["status"] = report.Status
		data["workspace_id"] = report.WorkspaceID
		data["workspace_root"] = report.WorkspaceRoot
		data["providers"] = report.Providers
		data["remaining_dependencies"] = report.RemainingDependencies
		if report.ManagedMode != nil {
			data["managed_mode"] = report.ManagedMode
		}
		if report.Backup != nil {
			data["backup"] = report.Backup
		}
		if report.Fingerprint != "" {
			data["fingerprint"] = report.Fingerprint
		}
		if report.RepairReason != "" {
			data["repair_reason"] = report.RepairReason
		}
		if report.BackupWorker != "" {
			data["backup_worker_state"] = report.BackupWorker
		}
		if report.LastVerified != "" {
			data["last_verified_checkpoint"] = report.LastVerified
		}
	}
	if plan != nil && plan.Team != nil {
		data["team"] = plan.Team
	}
	if plan != nil {
		data["plan"] = plan
		data["inspection"] = plan.Inspection
	}
	pretty := "setup"
	if report != nil {
		pretty = fmt.Sprintf("setup status=%s providers=%d", report.Status, len(report.Providers))
	}
	if err := writeCommandOutput(cmd, data, pretty, pretty); err != nil {
		return err
	}
	return runErr
}

func runIntegrationsStatus(cmd *cobra.Command, _ []string) error {
	engine, err := setupEngineFromCWD()
	if err != nil {
		return err
	}
	report, err := engine.StatusReport()
	if err != nil {
		return err
	}
	if report != nil {
		report.Kind = "integrations_status"
	}
	return writeSetupReport(cmd, nil, report, setupStatusError(report))
}

func runIntegrationsRepair(cmd *cobra.Command, args []string) error {
	engine, err := setupEngineFromCWD()
	if err != nil {
		return err
	}
	target, err := parseSetupTarget(args[0])
	if err != nil {
		return err
	}
	yes, _ := cmd.Flags().GetBool("yes")
	report, err := engine.Repair(commandContext(cmd), target, yes)
	if err != nil && report == nil {
		return err
	}
	if report != nil {
		report.Kind = "integrations_repair"
	}
	return writeSetupReport(cmd, nil, report, err)
}

func runIntegrationsDisconnect(cmd *cobra.Command, args []string) error {
	engine, err := setupEngineFromCWD()
	if err != nil {
		return err
	}
	target, err := parseSetupTarget(args[0])
	if err != nil {
		return err
	}
	yes, _ := cmd.Flags().GetBool("yes")
	report, err := engine.Disconnect(commandContext(cmd), target, yes)
	if err != nil && report == nil {
		return err
	}
	return writeSetupReport(cmd, nil, report, err)
}

func parseSetupTarget(raw string) (integrations.Target, error) {
	targets, err := integrations.ParseTargetList(raw)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidInput, err.Error())
	}
	if len(targets) != 1 {
		return "", apperr.New(apperr.CodeInvalidInput, "expected exactly one integration target")
	}
	return targets[0], nil
}

func setupStatusError(report *setup.RunReport) error {
	if report == nil {
		return nil
	}
	if report.RepairReason != "" {
		return apperr.New(apperr.CodeRepairNeeded, report.RepairReason)
	}
	return nil
}
