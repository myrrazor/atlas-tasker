package mcp

import "github.com/myrrazor/atlas-tasker/internal/service"

type backupStatusPayload struct {
	Kind          string                      `json:"kind"`
	BackupHealth  service.BackupHealthSummary `json:"backup_health"`
	Auto          service.AutoBackupStatus    `json:"automatic"`
}

func backupStatusTool(tc ToolContext, _ map[string]any) (any, error) {
	health, err := tc.Server.Workspace.Queries.BackupHealth(tc.Context)
	if err != nil {
		return nil, err
	}
	auto, err := tc.Server.Workspace.Queries.AutoBackupStatus(tc.Context)
	if err != nil {
		return nil, err
	}
	return backupStatusPayload{Kind: "atlas.backup.status", BackupHealth: health, Auto: auto}, nil
}
