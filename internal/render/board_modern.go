package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/theme"
)

type BoardStyle string

const (
	// BoardStyleTable is the default terminal/TUI board: aligned rows, not lanes.
	BoardStyleTable BoardStyle = "table"
	// BoardStyleKanban is the optional side-by-side card/lane board.
	BoardStyleKanban BoardStyle = "kanban"
	// BoardStyleModern is accepted as an alias for kanban.
	BoardStyleModern BoardStyle = "modern"
	// BoardStyleLegacy is the old boxed ASCII grid; CLI-only via --style legacy.
	BoardStyleLegacy BoardStyle = "legacy"
	// BoardStyleChat is the paste-ready Discord/Grokbot ANSI board.
	BoardStyleChat BoardStyle = "chat"
	// BoardStyleHTML is a self-contained HTML fragment for capable chat hosts.
	BoardStyleHTML BoardStyle = "html"
	// BoardStyleMarkdown is a readable board for Markdown chat hosts.
	BoardStyleMarkdown BoardStyle = "markdown"
)

type BoardDensity string

const (
	DensityComfortable BoardDensity = "comfortable"
	DensityCompact     BoardDensity = "compact"
	DensityFocus       BoardDensity = "focus"
)

// BoardRenderOptions controls a terminal snapshot of CompactBoard.
type BoardRenderOptions struct {
	Width       int
	Height      int
	Style       BoardStyle
	Density     BoardDensity
	SelectedID  string
	ShowSummary bool
	Color       *bool
}

func (o BoardRenderOptions) color() bool {
	if o.Color != nil {
		return *o.Color
	}
	return ColorEnabled()
}

func (o BoardRenderOptions) normalized() BoardRenderOptions {
	if o.Width <= 0 {
		o.Width = 80
	}
	switch o.Style {
	case BoardStyleTable, BoardStyleKanban, BoardStyleModern, BoardStyleLegacy, BoardStyleChat, BoardStyleHTML, BoardStyleMarkdown:
	default:
		o.Style = BoardStyleTable
	}
	switch o.Density {
	case DensityCompact, DensityFocus, DensityComfortable:
	default:
		o.Density = DensityComfortable
	}
	return o
}

func ParseBoardStyle(raw string) (BoardStyle, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "table":
		return BoardStyleTable, nil
	case "kanban", "modern", "card", "cards", "lanes":
		return BoardStyleKanban, nil
	case "legacy", "grid":
		return BoardStyleLegacy, nil
	case "chat", "discord", "ansi":
		return BoardStyleChat, nil
	case "html":
		return BoardStyleHTML, nil
	case "markdown":
		return BoardStyleMarkdown, nil
	default:
		return "", fmt.Errorf("board style must be table, kanban, legacy, chat, html, or markdown")
	}
}

func (s BoardStyle) IsKanban() bool {
	return s == BoardStyleKanban || s == BoardStyleModern
}

func ParseBoardDensity(raw string) (BoardDensity, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "comfortable":
		return DensityComfortable, nil
	case "compact":
		return DensityCompact, nil
	case "focus":
		return DensityFocus, nil
	default:
		return "", fmt.Errorf("board density must be comfortable, compact, or focus")
	}
}

// RenderBoard presents CompactBoard for a human terminal or capable chat host.
// Markdown and JSON callers already have structured fields.
func RenderBoard(board CompactBoard, opts BoardRenderOptions) string {
	opts = opts.normalized()
	if opts.Style == BoardStyleHTML {
		return CompactBoardWidgetFragment(board)
	}
	if opts.Style == BoardStyleMarkdown {
		board.BoardURL = "" // A relative local board path is not a useful chat link.
		return CompactBoardMarkdown(board)
	}
	if opts.Style == BoardStyleChat {
		return CompactBoardChat(board)
	}
	if opts.Style.IsKanban() {
		return renderModernBoard(board, opts)
	}
	return renderCompactTable(board, opts)
}

func renderModernBoard(board CompactBoard, opts BoardRenderOptions) string {
	width := opts.Width
	populated := populatedColumns(board)
	if board.TotalCards == 0 || len(populated) == 0 {
		action := "Try creating one with `tracker ticket create`."
		if len(board.NextActions) > 0 {
			action = board.NextActions[0]
		}
		return fitLines(EmptyState(board.Title, action), width)
	}

	density := opts.Density
	// Comfortable stays comfortable even on a 40-col terminal — stack instead
	// of collapsing into inline compact text. Focus is for short TUI heights.
	if density == DensityComfortable && opts.Height > 0 && opts.Height < 16 && opts.SelectedID != "" {
		density = DensityFocus
	}

	var chunks []string
	if opts.ShowSummary {
		if summary := renderBoardSummary(board, opts, width); summary != "" {
			chunks = append(chunks, summary)
		}
	}

	lanes := populated
	if density == DensityFocus {
		chunks = append(chunks, renderFocusRail(populated, opts, width))
		if focused, ok := focusColumn(populated, opts.SelectedID); ok {
			lanes = []CompactColumn{focused}
		}
	}

	perRow := laneColumns(width, density)
	if perRow < 1 {
		perRow = 1
	}
	for i := 0; i < len(lanes); i += perRow {
		end := i + perRow
		if end > len(lanes) {
			end = len(lanes)
		}
		row := lanes[i:end]
		laneWidth := width
		if len(row) > 1 {
			laneWidth = (width - laneGutter*(len(row)-1)) / len(row)
			if laneWidth < 12 {
				laneWidth = 12
			}
		}
		rendered := make([]string, 0, len(row))
		maxCards := cardsPerLane(opts.Height, density, len(lanes), perRow, opts.ShowSummary)
		for _, col := range row {
			rendered = append(rendered, renderLane(board, col, opts, density, laneWidth, maxCards))
		}
		if len(rendered) == 1 {
			chunks = append(chunks, rendered[0])
			continue
		}
		chunks = append(chunks, joinLaneRow(rendered, laneGutter))
	}

	return strings.Join(chunks, "\n\n")
}

func renderBoardSummary(board CompactBoard, opts BoardRenderOptions, width int) string {
	color := opts.color()
	title := TruncateDisplay(SanitizeDisplayLine(board.Title), width)
	if color {
		title = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Render(title)
	}
	lines := []string{title}

	countBits := []string{fmt.Sprintf("%d tickets", board.TotalCards)}
	if board.Truncated {
		countBits[0] = fmt.Sprintf("%d of %d tickets", board.ShownCards, board.TotalCards)
	}
	countBits = append(countBits, board.Attention...)
	lede := strings.Join(countBits, "  ·  ")
	if !color {
		lede = strings.Join(countBits, " | ")
	}
	lines = append(lines, styleMuted(TruncateDisplay(lede, width), color))

	if len(board.NextActions) > 0 {
		next := "Next  " + board.NextActions[0]
		lines = append(lines, styleMuted(TruncateDisplay(next, width), color))
	}
	if board.Backup != nil {
		lines = append(lines, styleMuted(TruncateDisplay("Backup  "+board.Backup.SummaryLine(), width), color))
	}
	for _, note := range board.Notes {
		lines = append(lines, styleMuted(TruncateDisplay(note, width), color))
	}
	return strings.Join(lines, "\n")
}

func renderFocusRail(columns []CompactColumn, opts BoardRenderOptions, width int) string {
	color := opts.color()
	parts := make([]string, 0, len(columns))
	for _, col := range columns {
		label := fmt.Sprintf("%s %d", col.Label, col.Total)
		selected := columnContains(col, opts.SelectedID)
		if color {
			style := lipgloss.NewStyle().Foreground(theme.LaneColor(col.Status))
			if selected {
				style = style.Bold(true)
			}
			label = style.Render(label)
		} else if selected {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	sep := "  ·  "
	if !color {
		sep = " | "
	}
	return TruncateDisplay(strings.Join(parts, sep), width)
}

func renderLane(board CompactBoard, col CompactColumn, opts BoardRenderOptions, density BoardDensity, width int, maxCards int) string {
	color := opts.color()
	header := TruncateDisplay(fmt.Sprintf("%s  %d", col.Label, col.Total), width)
	if col.Truncated {
		header = TruncateDisplay(fmt.Sprintf("%s  %d  +%d", col.Label, col.Shown, col.Total-col.Shown), width)
	}
	if color {
		header = lipgloss.NewStyle().Bold(true).Foreground(theme.LaneColor(col.Status)).Render(header)
	}

	shown, hiddenAbove, hiddenBelow := windowCards(col.Cards, opts.SelectedID, maxCards)
	lines := []string{header}
	if hiddenAbove > 0 {
		lines = append(lines, styleMuted(TruncateDisplay(fmt.Sprintf("%d above", hiddenAbove), width), color))
	}
	for i, card := range shown {
		selected := opts.SelectedID != "" && card.ID == opts.SelectedID
		lines = append(lines, renderCard(board, card, opts, density, width, selected)...)
		if density != DensityCompact && i < len(shown)-1 {
			lines = append(lines, "")
		}
	}
	if hiddenBelow > 0 {
		lines = append(lines, styleMuted(TruncateDisplay(fmt.Sprintf("+%d more", hiddenBelow), width), color))
	}
	return strings.Join(lines, "\n")
}

func joinLaneRow(blocks []string, gutter int) string {
	if len(blocks) == 0 {
		return ""
	}
	if len(blocks) == 1 {
		return blocks[0]
	}
	if gutter < 1 {
		gutter = 1
	}
	cols := make([][]string, len(blocks))
	widths := make([]int, len(blocks))
	height := 0
	for i, block := range blocks {
		cols[i] = strings.Split(block, "\n")
		if len(cols[i]) > height {
			height = len(cols[i])
		}
		for _, line := range cols[i] {
			if w := DisplayWidth(line); w > widths[i] {
				widths[i] = w
			}
		}
	}
	gap := strings.Repeat(" ", gutter)
	rows := make([]string, 0, height)
	for r := 0; r < height; r++ {
		var row strings.Builder
		for i := range cols {
			if i > 0 {
				row.WriteString(gap)
			}
			line := ""
			if r < len(cols[i]) {
				line = cols[i][r]
			}
			row.WriteString(line)
			pad := widths[i] - DisplayWidth(line)
			if pad > 0 {
				row.WriteString(strings.Repeat(" ", pad))
			}
		}
		rows = append(rows, strings.TrimRight(row.String(), " "))
	}
	return strings.Join(rows, "\n")
}

func renderCard(board CompactBoard, card CompactCard, opts BoardRenderOptions, density BoardDensity, width int, selected bool) []string {
	color := opts.color()
	accent := laneAccent(card.Status, selected, color)
	prefix := accent + " "
	if selected && !color {
		prefix = "> "
	}
	idText := SanitizeDisplayLine(card.ID)
	if idText == "" {
		idText = "?"
	}
	idStyle := lipgloss.NewStyle().Bold(true)
	if color {
		idStyle = idStyle.Foreground(theme.Primary)
		if selected {
			idStyle = idStyle.Foreground(theme.Accent)
		}
	}

	idWidth := maxInt(1, width-DisplayWidth(prefix))
	idLines := wrapToken(idText, idWidth)
	styledID := make([]string, len(idLines))
	for i, piece := range idLines {
		styledID[i] = idStyle.Render(piece)
	}

	priority := ""
	if card.Priority != "" && card.Priority != string(contracts.PriorityMedium) {
		priority = SanitizeDisplayLine(card.Priority)
	}

	var lines []string
	first := prefix + styledID[0]
	if density != DensityCompact && priority != "" {
		candidate := first + "  " + priority
		if DisplayWidth(candidate) <= width {
			first = candidate
			priority = ""
		}
	}
	lines = append(lines, first)
	indent := strings.Repeat(" ", DisplayWidth(prefix))
	for _, piece := range styledID[1:] {
		lines = append(lines, indent+piece)
	}
	if priority != "" {
		lines = append(lines, padMeta(prefix, priority, width, color))
	}

	title := optionalString(SanitizeDisplayLine(card.Title), "(untitled)")
	titleWidth := maxInt(1, width-DisplayWidth(prefix))
	if density == DensityCompact {
		// ID already on line 1; put the title beside it when it fits.
		combined := lines[0] + "  " + title
		if DisplayWidth(combined) <= width {
			lines[0] = combined
		} else {
			for _, part := range wrapTitle(title, titleWidth, 1) {
				lines = append(lines, padIndented(prefix, part, width, color, false))
			}
		}
		return lines
	}

	for _, part := range wrapTitle(title, titleWidth, 2) {
		lines = append(lines, padIndented(prefix, part, width, color, false))
	}
	if meta := cardMetaLine(board, card, density); meta != "" {
		lines = append(lines, padIndented(prefix, TruncateDisplay(meta, titleWidth), width, color, true))
	}
	return lines
}

func wrapToken(value string, width int) []string {
	value = SanitizeDisplayLine(value)
	if value == "" {
		return []string{"?"}
	}
	if width <= 0 {
		return splitLongToken(value, 1)
	}
	if DisplayWidth(value) <= width {
		return []string{value}
	}
	parts := splitLongToken(value, width)
	if len(parts) == 0 {
		return []string{value}
	}
	return parts
}

func wrapTitle(title string, width int, maxLines int) []string {
	if maxLines < 1 {
		maxLines = 1
	}
	parts := WrapDisplay(title, width)
	if len(parts) == 0 {
		return []string{TruncateDisplay(title, width)}
	}
	if len(parts) <= maxLines {
		return parts
	}
	head := append([]string{}, parts[:maxLines-1]...)
	rest := strings.Join(parts[maxLines-1:], " ")
	return append(head, TruncateDisplay(rest, width))
}

func cardMetaLine(board CompactBoard, card CompactCard, density BoardDensity) string {
	bits := make([]string, 0, 8)
	if card.Project != "" && card.Project != board.Project {
		bits = append(bits, SanitizeDisplayLine(card.Project))
	}
	if card.Type != "" && card.Type != string(contracts.TicketTypeTask) {
		bits = append(bits, SanitizeDisplayLine(card.Type))
	}
	if card.Assignee != "" {
		bits = append(bits, SanitizeDisplayLine(card.Assignee))
	}
	if card.Reviewer != "" {
		bits = append(bits, "rev "+SanitizeDisplayLine(card.Reviewer))
	}
	if density == DensityCompact {
		return strings.Join(bits, " · ")
	}
	for _, label := range card.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		bits = append(bits, SanitizeDisplayLine(label))
		if len(bits) >= 5 {
			break
		}
	}
	if card.LatestRunID != "" {
		bits = append(bits, SanitizeDisplayLine(card.LatestRunID))
	}
	if card.ChildrenTotal > 0 {
		bits = append(bits, fmt.Sprintf("%d of %d children", card.ChildrenDone, card.ChildrenTotal))
	}
	if len(card.Attention) > 0 {
		bits = append(bits, SanitizeDisplayLine(card.Attention[0]))
	}
	return strings.Join(bits, " · ")
}

func padMeta(prefix string, meta string, width int, color bool) string {
	return padIndented(prefix, meta, width, color, true)
}

func padIndented(prefix string, text string, width int, color bool, muted bool) string {
	indentWidth := DisplayWidth(prefix)
	if indentWidth < 2 {
		indentWidth = 2
	}
	indent := strings.Repeat(" ", indentWidth)
	if color {
		if muted {
			text = styleMuted(text, true)
		} else {
			text = lipgloss.NewStyle().Foreground(theme.Text).Render(text)
		}
	}
	return fitLine(indent+text, width)
}

const (
	laneGutter           = 3
	comfortableMinLane   = 36
	compactMinLane       = 28
	comfortableCardLines = 5
)

func laneAccent(status string, selected bool, color bool) string {
	if !color {
		if selected {
			return ">"
		}
		return "|"
	}
	ch := "▏"
	if selected {
		ch = "▌"
	}
	return lipgloss.NewStyle().Foreground(theme.LaneColor(status)).Render(ch)
}

func laneColumns(width int, density BoardDensity) int {
	if density == DensityFocus {
		return 1
	}
	minLane := comfortableMinLane
	if density == DensityCompact {
		minLane = compactMinLane
	}
	maxN := 1
	for n := 2; n <= 4; n++ {
		need := n*minLane + (n-1)*laneGutter
		if need <= width {
			maxN = n
		}
	}
	return maxN
}

func cardsPerLane(height int, density BoardDensity, laneCount int, perRow int, summary bool) int {
	if height <= 0 {
		return 0
	}
	rows := (laneCount + perRow - 1) / perRow
	if rows < 1 {
		rows = 1
	}
	chrome := 3
	if summary {
		chrome += 4
	}
	available := height - chrome
	if available < 4 {
		available = 4
	}
	perLaneHeight := available / rows
	cardRows := comfortableCardLines
	if density == DensityCompact {
		cardRows = 2
	}
	n := (perLaneHeight - 1) / cardRows
	if n < 1 {
		return 1
	}
	return n
}

// JoinBlocks places rendered columns side by side with a real gutter.
// lipgloss.JoinHorizontal trims trailing spaces and eats padding.
func JoinBlocks(blocks []string, gutter int) string {
	return joinLaneRow(blocks, gutter)
}

func windowCards(cards []CompactCard, selectedID string, maxCards int) ([]CompactCard, int, int) {
	if maxCards <= 0 || len(cards) <= maxCards {
		return cards, 0, 0
	}
	idx := 0
	for i, card := range cards {
		if selectedID != "" && card.ID == selectedID {
			idx = i
			break
		}
	}
	start := idx - maxCards/3
	if start < 0 {
		start = 0
	}
	end := start + maxCards
	if end > len(cards) {
		end = len(cards)
		start = end - maxCards
		if start < 0 {
			start = 0
		}
	}
	return cards[start:end], start, len(cards) - end
}

func focusColumn(columns []CompactColumn, selectedID string) (CompactColumn, bool) {
	if selectedID != "" {
		for _, col := range columns {
			if columnContains(col, selectedID) {
				return col, true
			}
		}
	}
	if len(columns) > 0 {
		return columns[0], true
	}
	return CompactColumn{}, false
}

func columnContains(col CompactColumn, id string) bool {
	if id == "" {
		return false
	}
	for _, card := range col.Cards {
		if card.ID == id {
			return true
		}
	}
	return false
}

func styleMuted(text string, color bool) string {
	if !color || text == "" {
		return text
	}
	return lipgloss.NewStyle().Foreground(theme.Muted).Render(text)
}

func fitLines(value string, width int) string {
	if width <= 0 {
		return value
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if DisplayWidth(line) > width {
			lines[i] = TruncateDisplay(line, width)
		}
	}
	return strings.Join(lines, "\n")
}

func fitLine(value string, width int) string {
	if width <= 0 || DisplayWidth(value) <= width {
		return value
	}
	return TruncateDisplay(value, width)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const (
	tableGutter    = 2
	tableMarkerW   = 2
	tableMinTitle  = 8
	tablePriW      = 8
	tableAssignMin = 12
	tableAssignMax = 18
	tableStatusW   = 11 // "In Progress"
	tableColMinW   = 36
)

type tableColKind int

const (
	tableColID tableColKind = iota
	tableColStatus
	tableColPri
	tableColAssignee
	tableColTitle
)

type tableCol struct {
	kind   tableColKind
	header string
	width  int
}

type tableTicketRow struct {
	id    string
	lines []string
}

func renderCompactTable(board CompactBoard, opts BoardRenderOptions) string {
	width := opts.Width
	cards := tableCards(board)
	if len(cards) == 0 {
		action := "Try creating one with `tracker ticket create`."
		if len(board.NextActions) > 0 {
			action = board.NextActions[0]
		}
		return fitLines(EmptyState(board.Title, action), width)
	}

	var chunks []string
	if opts.ShowSummary {
		if summary := renderBoardSummary(board, opts, width); summary != "" {
			chunks = append(chunks, summary)
		}
	}

	cols := planTableCols(width, cards)
	rows := make([]tableTicketRow, 0, len(cards))
	for _, card := range cards {
		rows = append(rows, buildTableRow(board, card, cols, opts, width))
	}

	header := renderTableHeader(cols, opts, width)
	rule := renderTableRule(cols, opts, width)
	chrome := 0
	if opts.ShowSummary && len(chunks) > 0 {
		chrome += strings.Count(chunks[0], "\n") + 1
		chrome++ // blank before table
	}
	chrome += 2 // header + rule
	if opts.Height > 0 {
		chrome++ // position line
	}
	budget := 0
	if opts.Height > 0 {
		budget = opts.Height - chrome
		if budget < 1 {
			budget = 1
		}
	}

	shown, from, to := windowTableRows(rows, opts.SelectedID, budget)
	body := make([]string, 0, 2+len(shown)*2)
	body = append(body, header, rule)
	for _, row := range shown {
		body = append(body, row.lines...)
	}
	if opts.Height > 0 && len(rows) > 0 {
		body = append(body, styleMuted(fitLine(fmt.Sprintf("showing %d to %d of %d", from+1, to, len(rows)), width), opts.color()))
	}
	chunks = append(chunks, strings.Join(body, "\n"))
	return strings.Join(chunks, "\n\n")
}

func tableCards(board CompactBoard) []CompactCard {
	n := 0
	for _, col := range board.Columns {
		n += len(col.Cards)
	}
	out := make([]CompactCard, 0, n)
	for _, col := range board.Columns {
		for _, card := range col.Cards {
			if strings.TrimSpace(card.Status) == "" {
				card.Status = col.Status
			}
			out = append(out, card)
		}
	}
	return out
}

func planTableCols(width int, cards []CompactCard) []tableCol {
	inner := width - tableMarkerW
	if inner < 6 {
		inner = 6
	}
	longestID := DisplayWidth("ID")
	longestAssign := DisplayWidth("ASSIGNEE")
	anyAssignee := false
	for _, card := range cards {
		if w := DisplayWidth(SanitizeDisplayLine(card.ID)); w > longestID {
			longestID = w
		}
		if who := strings.TrimSpace(SanitizeDisplayLine(card.Assignee)); who != "" {
			anyAssignee = true
			if w := DisplayWidth(who); w > longestAssign {
				longestAssign = w
			}
		}
	}
	assignW := tableAssignMin
	if longestAssign > assignW {
		assignW = minInt(longestAssign, tableAssignMax)
	}

	// stacked ID/status/title when a 3-column row cannot exist
	if width < tableColMinW {
		idW := minInt(longestID, maxInt(2, inner))
		return []tableCol{
			{tableColID, "ID", idW},
			{tableColStatus, "STATUS", minInt(tableStatusW, maxInt(4, inner))},
			{tableColTitle, "TITLE", maxInt(1, inner)},
		}
	}

	gutters := 2 * tableGutter
	idBudget := inner - tableStatusW - tableMinTitle - gutters
	if idBudget < 2 {
		idBudget = 2
	}
	idW := longestID
	if idW > idBudget {
		idW = idBudget
	}
	if idW < 2 {
		idW = 2
	}
	used := idW + tableStatusW + tableMinTitle + gutters
	leftover := inner - used
	if leftover < 0 {
		leftover = 0
	}

	// title keeps leftover first; extra columns only when they still leave
	// a usable title (Carbon: title gets the width, not the metadata)
	includePri := leftover >= tablePriW+tableGutter+12
	if includePri {
		leftover -= tablePriW + tableGutter
	}
	includeAssignee := anyAssignee && leftover >= assignW+tableGutter+8
	if includeAssignee {
		leftover -= assignW + tableGutter
	}
	titleW := tableMinTitle + leftover
	if titleW < 1 {
		titleW = 1
	}

	cols := []tableCol{
		{tableColID, "ID", idW},
		{tableColStatus, "STATUS", tableStatusW},
	}
	if includePri {
		cols = append(cols, tableCol{tableColPri, "PRI", tablePriW})
	}
	if includeAssignee {
		cols = append(cols, tableCol{tableColAssignee, "ASSIGNEE", assignW})
	}
	cols = append(cols, tableCol{tableColTitle, "TITLE", titleW})
	return cols
}

func buildTableRow(board CompactBoard, card CompactCard, cols []tableCol, opts BoardRenderOptions, width int) tableTicketRow {
	color := opts.color()
	selected := opts.SelectedID != "" && card.ID == opts.SelectedID
	marker := "  "
	if selected {
		marker = "> "
	}

	stacked := width < tableColMinW
	if stacked {
		return buildStackedRow(board, card, opts, width, selected, marker)
	}

	cells := make([][]string, len(cols))
	hasPri, hasAssignee := false, false
	for i, col := range cols {
		switch col.kind {
		case tableColID:
			idText := SanitizeDisplayLine(card.ID)
			if idText == "" {
				idText = "?"
			}
			parts := wrapToken(idText, col.width)
			styled := make([]string, len(parts))
			idStyle := lipgloss.NewStyle().Bold(true)
			if color {
				if selected {
					idStyle = idStyle.Foreground(theme.Accent)
				} else {
					idStyle = idStyle.Foreground(theme.Primary)
				}
			}
			for j, part := range parts {
				styled[j] = idStyle.Render(part)
			}
			cells[i] = styled
		case tableColStatus:
			label := tableStatusLabel(card.Status)
			cells[i] = []string{styleTableStatus(TruncateDisplay(label, col.width), card.Status, color)}
		case tableColPri:
			hasPri = true
			cells[i] = []string{styleTablePri(TruncateDisplay(tablePriorityLabel(card.Priority), col.width), card.Priority, color)}
		case tableColAssignee:
			hasAssignee = true
			who := strings.TrimSpace(SanitizeDisplayLine(card.Assignee))
			if who == "" {
				who = "—"
			}
			cells[i] = wrapTitle(who, col.width, 2)
		case tableColTitle:
			title := optionalString(SanitizeDisplayLine(card.Title), "(untitled)")
			maxLines := 2
			if col.width >= 24 {
				maxLines = 1
			}
			parts := wrapTitle(title, col.width, maxLines)
			if color {
				titleStyle := lipgloss.NewStyle().Foreground(theme.Text)
				if selected {
					titleStyle = titleStyle.Bold(true)
				}
				for j, part := range parts {
					parts[j] = titleStyle.Render(part)
				}
			}
			cells[i] = parts
		}
	}

	n := 1
	for _, cell := range cells {
		if len(cell) > n {
			n = len(cell)
		}
	}
	gutter := strings.Repeat(" ", tableGutter)
	lines := make([]string, 0, n+1)
	for r := 0; r < n; r++ {
		var b strings.Builder
		if r == 0 {
			b.WriteString(marker)
		} else {
			b.WriteString("  ")
		}
		for i, col := range cols {
			if i > 0 {
				b.WriteString(gutter)
			}
			piece := ""
			if r < len(cells[i]) {
				piece = cells[i][r]
			}
			b.WriteString(padDisplay(piece, col.width))
		}
		lines = append(lines, fitLine(strings.TrimRight(b.String(), " "), width))
	}
	if meta := tableSecondaryMeta(board, card, hasPri, hasAssignee); meta != "" {
		indent := tableMarkerW + cols[0].width + tableGutter
		if indent < tableMarkerW {
			indent = tableMarkerW
		}
		pad := strings.Repeat(" ", indent)
		lines = append(lines, fitLine(pad+styleMuted(TruncateDisplay(meta, maxInt(1, width-indent)), color), width))
	}
	return tableTicketRow{id: card.ID, lines: lines}
}

func buildStackedRow(board CompactBoard, card CompactCard, opts BoardRenderOptions, width int, selected bool, marker string) tableTicketRow {
	color := opts.color()
	idText := SanitizeDisplayLine(card.ID)
	if idText == "" {
		idText = "?"
	}
	idWidth := maxInt(1, width-tableMarkerW)
	idParts := wrapToken(idText, idWidth)
	idStyle := lipgloss.NewStyle().Bold(true)
	if color {
		if selected {
			idStyle = idStyle.Foreground(theme.Accent)
		} else {
			idStyle = idStyle.Foreground(theme.Primary)
		}
	}

	var lines []string
	lines = append(lines, fitLine(marker+idStyle.Render(idParts[0]), width))
	indent := strings.Repeat(" ", tableMarkerW)
	for _, part := range idParts[1:] {
		lines = append(lines, fitLine(indent+idStyle.Render(part), width))
	}

	status := tableStatusLabel(card.Status)
	lines = append(lines, fitLine(indent+styleTableStatus(TruncateDisplay(status, maxInt(1, width-tableMarkerW)), card.Status, color), width))

	title := optionalString(SanitizeDisplayLine(card.Title), "(untitled)")
	titleWidth := maxInt(1, width-tableMarkerW)
	titleStyle := lipgloss.NewStyle()
	if color {
		titleStyle = titleStyle.Foreground(theme.Text)
		if selected {
			titleStyle = titleStyle.Bold(true)
		}
	}
	for _, part := range wrapTitle(title, titleWidth, 2) {
		lines = append(lines, fitLine(indent+titleStyle.Render(part), width))
	}
	if meta := tableSecondaryMeta(board, card, false, false); meta != "" {
		lines = append(lines, fitLine(indent+styleMuted(TruncateDisplay(meta, titleWidth), color), width))
	}
	return tableTicketRow{id: card.ID, lines: lines}
}

func renderTableHeader(cols []tableCol, opts BoardRenderOptions, width int) string {
	color := opts.color()
	if width < tableColMinW {
		heads := make([]string, 0, len(cols))
		for _, col := range cols {
			heads = append(heads, col.header)
		}
		line := strings.Join(heads, "  ")
		if color {
			line = lipgloss.NewStyle().Bold(true).Foreground(theme.Subtle).Render(line)
		}
		return fitLine(line, width)
	}
	gutter := strings.Repeat(" ", tableGutter)
	var b strings.Builder
	b.WriteString("  ")
	for i, col := range cols {
		if i > 0 {
			b.WriteString(gutter)
		}
		head := col.header
		if color {
			head = lipgloss.NewStyle().Bold(true).Foreground(theme.Subtle).Render(head)
		}
		b.WriteString(padDisplay(head, col.width))
	}
	return fitLine(strings.TrimRight(b.String(), " "), width)
}

func renderTableRule(cols []tableCol, opts BoardRenderOptions, width int) string {
	ruleW := width
	if width >= tableColMinW {
		ruleW = tableMarkerW
		for i, col := range cols {
			if i > 0 {
				ruleW += tableGutter
			}
			ruleW += col.width
		}
		if ruleW > width {
			ruleW = width
		}
	}
	if ruleW < 1 {
		ruleW = 1
	}
	ch := "─"
	if !opts.color() {
		ch = "-"
	}
	rule := strings.Repeat(ch, ruleW)
	if opts.color() {
		rule = lipgloss.NewStyle().Foreground(theme.Subtle).Render(rule)
	}
	return fitLine(rule, width)
}

func windowTableRows(rows []tableTicketRow, selectedID string, maxLines int) ([]tableTicketRow, int, int) {
	if maxLines <= 0 || len(rows) == 0 {
		return rows, 0, len(rows)
	}
	total := 0
	for _, row := range rows {
		total += len(row.lines)
	}
	if total <= maxLines {
		return rows, 0, len(rows)
	}
	sel := 0
	for i, row := range rows {
		if selectedID != "" && row.id == selectedID {
			sel = i
			break
		}
	}
	start, end := sel, sel+1
	used := len(rows[sel].lines)
	if used > maxLines {
		return rows[start:end], start, end
	}
	// keep a little context above the selection, then fill downward
	aboveBudget := maxLines / 3
	for start > 0 {
		need := len(rows[start-1].lines)
		if used+need > maxLines || used+need > aboveBudget+len(rows[sel].lines) {
			break
		}
		start--
		used += need
	}
	for end < len(rows) {
		need := len(rows[end].lines)
		if used+need > maxLines {
			break
		}
		used += need
		end++
	}
	for start > 0 {
		need := len(rows[start-1].lines)
		if used+need > maxLines {
			break
		}
		start--
		used += need
	}
	return rows[start:end], start, end
}

func tableStatusLabel(status string) string {
	if label, ok := compactColumnLabels[contracts.Status(status)]; ok {
		return label
	}
	status = strings.TrimSpace(SanitizeDisplayLine(status))
	if status == "" {
		return "Unknown"
	}
	return status
}

func tablePriorityLabel(priority string) string {
	priority = strings.TrimSpace(SanitizeDisplayLine(priority))
	if priority == "" || priority == string(contracts.PriorityMedium) {
		return ""
	}
	return priority
}

func tableSecondaryMeta(board CompactBoard, card CompactCard, hasPri, hasAssignee bool) string {
	bits := make([]string, 0, 4)
	if !hasPri {
		pri := strings.TrimSpace(card.Priority)
		if pri != "" && pri != string(contracts.PriorityMedium) {
			bits = append(bits, SanitizeDisplayLine(pri))
		}
	}
	if !hasAssignee {
		who := strings.TrimSpace(SanitizeDisplayLine(card.Assignee))
		if who != "" {
			bits = append(bits, who)
		}
	}
	if card.Project != "" && card.Project != board.Project {
		bits = append(bits, SanitizeDisplayLine(card.Project))
	}
	return strings.Join(bits, " · ")
}

func styleTableStatus(label, status string, color bool) string {
	if !color || label == "" {
		return label
	}
	return lipgloss.NewStyle().Foreground(theme.LaneColor(status)).Render(label)
}

func styleTablePri(label, priority string, color bool) string {
	if !color || label == "" {
		return label
	}
	if c, ok := theme.PriorityColor(priority); ok {
		return lipgloss.NewStyle().Foreground(c).Render(label)
	}
	return lipgloss.NewStyle().Foreground(theme.Subtle).Render(label)
}

func padDisplay(value string, width int) string {
	w := DisplayWidth(value)
	if w >= width {
		return value
	}
	return value + strings.Repeat(" ", width-w)
}
