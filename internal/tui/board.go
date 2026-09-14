package tui

import (
	"github.com/myrrazor/atlas-tasker/internal/render"
)

func (m model) boardView() string {
	width := m.width
	if width <= 0 {
		width = 88
	}
	height := m.height
	project := render.UniqueProject(m.board.Board.Columns)
	board := render.NewCompactBoard(project, m.board.Board.Columns, -1, nil)
	color := renderEnabled()
	density := render.DensityComfortable
	bodyHeight := boardBodyHeight(height, width)
	style := m.boardStyle
	if style == "" {
		style = render.BoardStyleTable
	}
	if style.IsKanban() && height > 0 && height < 18 && m.selectedID != "" {
		density = render.DensityFocus
	}
	opts := render.BoardRenderOptions{
		Width:       width,
		Height:      bodyHeight,
		Style:       style,
		Density:     density,
		SelectedID:  m.selectedID,
		ShowSummary: true,
		Color:       &color,
	}
	return render.RenderBoard(board, opts)
}

func boardBodyHeight(height int, width int) int {
	if height <= 0 {
		return 0
	}
	chrome := 7
	if width > 0 && width < 44 {
		chrome = 6
	}
	body := height - chrome
	if body < 6 {
		return 6
	}
	return body
}
