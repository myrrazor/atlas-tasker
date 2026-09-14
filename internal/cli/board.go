package cli

import (
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func formatBoard(board contracts.BoardView, project string, style string, density string, backup service.BackupHealthSummary, haveBackup bool) (string, error) {
	parsedStyle, err := render.ParseBoardStyle(style)
	if err != nil {
		return "", err
	}
	parsedDensity, err := render.ParseBoardDensity(density)
	if err != nil {
		return "", err
	}
	width := render.TerminalWidth(100)
	if parsedStyle == render.BoardStyleLegacy {
		return render.BoardTableWithWidth(board, width), nil
	}
	project = strings.TrimSpace(project)
	if project == "" {
		project = render.UniqueProject(board.Columns)
	}
	compact := render.NewCompactBoard(project, board.Columns, -1, nil)
	if haveBackup {
		render.AttachBackup(&compact, backupSignal(backup))
	}
	return render.RenderBoard(compact, render.BoardRenderOptions{
		Width:       width,
		Style:       parsedStyle,
		Density:     parsedDensity,
		ShowSummary: true,
	}), nil
}

func backupSignal(h service.BackupHealthSummary) render.BackupSignal {
	return render.BackupSignal{
		Configured:             h.Configured,
		SnapshotCount:          h.SnapshotCount,
		WorkerState:            h.WorkerState,
		LastErrorClass:         h.LastErrorClass,
		AutomaticEnabled:       h.AutomaticEnabled,
		VerifiedRemote:         h.VerifiedRemote,
		UnbackedEventCount:     h.UnbackedEventCount,
		LastLocalCheckpointID:  h.LastLocalCheckpointID,
		LastRemoteCheckpointID: h.LastRemoteCheckpointID,
		Notes:                  append([]string(nil), h.Notes...),
	}
}
