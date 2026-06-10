package render

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	tableview "github.com/charmbracelet/lipgloss/table"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/theme"
	"golang.org/x/term"
)

type TableBorderMode int

const (
	TableBorderAuto TableBorderMode = iota
	TableBorderUnicode
	TableBorderASCII
)

type TableOptions struct {
	Title             string
	Width             int
	SelectedRow       int
	HighlightSelected bool
	Border            TableBorderMode
}

func colorEnabled() bool {
	return ColorEnabled()
}

func ColorEnabled() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func TerminalWidth(defaultWidth int) int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if width, err := strconv.Atoi(raw); err == nil && width > 0 {
			return width
		}
	}
	return defaultWidth
}

func terminalWidth(defaultWidth int) int {
	return TerminalWidth(defaultWidth)
}

func normalizedWidth(width int) int {
	if width <= 0 {
		return 80
	}
	return width
}

func markdownWidth(width int) int {
	width = normalizedWidth(width)
	if width < 16 {
		return 16
	}
	return width
}

func SanitizeDisplay(value string) string {
	return sanitizeDisplay(value, true, false)
}

// SanitizeTerminalOutput is the lenient variant for the final CLI output
// boundary only. Renderers strict-scrub every piece of user content before
// styling (SanitizeDisplay / SanitizeDisplayLine), so the only escapes left in
// an assembled pretty block are the SGR colors our own styles emitted --
// stripping those leaves "[90m" residue in every color-capable terminal.
func SanitizeTerminalOutput(value string) string {
	return sanitizeDisplay(value, true, true)
}

func SanitizeDisplayLine(value string) string {
	return strings.Join(strings.Fields(sanitizeDisplay(value, false, false)), " ")
}

// allowSGR lets plain color sequences (ESC '[' [0-9;:]* 'm') through; only
// SanitizeTerminalOutput sets it. Everything else stays strict so user
// content can never reach the output boundary with an intact escape byte.
func sanitizeDisplay(value string, preserveNewlines bool, allowSGR bool) string {
	if value == "" {
		return ""
	}
	value = stripRawC1CSI(value)
	value = strings.ToValidUTF8(value, "")
	runes := []rune(value)
	var out strings.Builder
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == 0x1b:
			if end, ok := sgrEnd(runes, i); ok {
				if allowSGR {
					for j := i; j <= end; j++ {
						out.WriteRune(runes[j])
					}
				}
				i = end
				continue
			}
			if end, ok := escapeSequenceEnd(runes, i); ok {
				i = end
			}
			// every other escape sequence stays banned
		case r == 0x9b:
			if end, ok := csiEnd(runes, i+1); ok {
				i = end
			}
		case r == '\n' || r == '\r':
			if preserveNewlines {
				out.WriteRune('\n')
			} else {
				out.WriteRune(' ')
			}
		case r == '\t':
			out.WriteRune(' ')
		case isBidiOverride(r):
			continue
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			continue
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func stripRawC1CSI(value string) string {
	bytes := []byte(value)
	var out strings.Builder
	for i := 0; i < len(bytes); i++ {
		if bytes[i] != 0x9b {
			out.WriteByte(bytes[i])
			continue
		}
		for i+1 < len(bytes) {
			i++
			if bytes[i] >= 0x40 && bytes[i] <= 0x7e {
				break
			}
		}
	}
	return out.String()
}

func escapeSequenceEnd(runes []rune, start int) (int, bool) {
	if start+1 >= len(runes) {
		return start, true
	}
	switch runes[start+1] {
	case '[':
		return csiEnd(runes, start+2)
	case ']':
		return oscEnd(runes, start+2), true
	default:
		return start + 1, true
	}
}

func csiEnd(runes []rune, start int) (int, bool) {
	for i := start; i < len(runes); i++ {
		if runes[i] >= 0x40 && runes[i] <= 0x7e {
			return i, true
		}
	}
	if len(runes) == 0 {
		return 0, false
	}
	return len(runes) - 1, true
}

func oscEnd(runes []rune, start int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == '\a' {
			return i
		}
		if runes[i] == 0x1b && i+1 < len(runes) && runes[i+1] == '\\' {
			return i + 1
		}
	}
	if len(runes) == 0 {
		return 0
	}
	return len(runes) - 1
}

// sgrEnd reports the index of the terminating 'm' when runes[start] opens a
// plain SGR sequence -- the only escape sanitizeDisplay ever lets through.
func sgrEnd(runes []rune, start int) (int, bool) {
	i := start + 1
	if i >= len(runes) || runes[i] != '[' {
		return 0, false
	}
	for i++; i < len(runes); i++ {
		switch {
		case runes[i] == 'm':
			return i, true
		case (runes[i] >= '0' && runes[i] <= '9') || runes[i] == ';' || runes[i] == ':':
			// still inside the parameter list
		default:
			return 0, false
		}
	}
	return 0, false
}

func isBidiOverride(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

func TicketPretty(ticket contracts.TicketSnapshot, comments []string) string {
	useColor := colorEnabled()
	titleStyle := lipgloss.NewStyle().Bold(true)
	mutedStyle := lipgloss.NewStyle()
	if useColor {
		titleStyle = titleStyle.Foreground(theme.Primary)
		mutedStyle = mutedStyle.Foreground(theme.Muted)
	}

	out := strings.Builder{}
	out.WriteString(titleStyle.Render(fmt.Sprintf("%s %s %s", SanitizeDisplayLine(ticket.ID), StatusBadge(ticket.Status), SanitizeDisplayLine(ticket.Title))))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render(fmt.Sprintf("Type: %s  Priority: %s  Assignee: %s", SanitizeDisplay(string(ticket.Type)), PriorityBadge(ticket.Priority), optionalString(SanitizeDisplay(string(ticket.Assignee)), "-"))))
	out.WriteString("\n\n")
	if strings.TrimSpace(ticket.Description) != "" {
		out.WriteString("Description:\n")
		out.WriteString(SanitizeDisplay(ticket.Description))
		out.WriteString("\n\n")
	}
	if len(ticket.AcceptanceCriteria) > 0 {
		out.WriteString("Acceptance Criteria:\n")
		for _, criterion := range ticket.AcceptanceCriteria {
			out.WriteString("- " + SanitizeDisplay(criterion) + "\n")
		}
		out.WriteString("\n")
	}
	if len(comments) > 0 {
		out.WriteString("Recent Comments:\n")
		for _, comment := range comments {
			out.WriteString("- " + SanitizeDisplay(comment) + "\n")
		}
	}
	return strings.TrimSpace(out.String())
}

func TicketsPretty(title string, tickets []contracts.TicketSnapshot) string {
	return TicketsPrettyWithWidth(title, tickets, terminalWidth(100))
}

func TicketsPrettyWithWidth(title string, tickets []contracts.TicketSnapshot, width int) string {
	if len(tickets) == 0 {
		return EmptyState(title, "No tickets found. Try creating one with `tracker ticket create`.")
	}
	width = normalizedWidth(width)
	if width >= 44 {
		return TicketsTable(title, tickets, width, -1)
	}
	lines := []string{SanitizeDisplayLine(title) + ":"}
	for _, ticket := range tickets {
		lines = append(lines, "- "+TicketSummary(ticket, width-2))
	}
	return strings.Join(lines, "\n")
}

func TicketsTable(title string, tickets []contracts.TicketSnapshot, width int, selectedRow int) string {
	rows := make([][]string, 0, len(tickets))
	for idx, ticket := range tickets {
		marker := ""
		if idx == selectedRow {
			marker = ">"
		}
		rows = append(rows, []string{
			marker,
			SanitizeDisplayLine(ticket.ID),
			optionalString(TypeBadge(ticket.Type), "-"),
			StatusBadge(ticket.Status),
			PriorityBadge(ticket.Priority),
			optionalString(SanitizeDisplayLine(ticket.Title), "(untitled)"),
		})
	}
	return RenderTable([]string{"", "ID", "Type", "Status", "Priority", "Title"}, rows, TableOptions{
		Title:             title,
		Width:             width,
		SelectedRow:       selectedRow,
		HighlightSelected: selectedRow >= 0,
	})
}

func BoardPretty(board contracts.BoardView) string {
	return BoardPrettyWithWidth(board, terminalWidth(100))
}

func BoardPrettyWithWidth(board contracts.BoardView, width int) string {
	width = normalizedWidth(width)
	ordered := []contracts.Status{
		contracts.StatusBacklog,
		contracts.StatusReady,
		contracts.StatusInProgress,
		contracts.StatusInReview,
		contracts.StatusBlocked,
		contracts.StatusDone,
	}
	labels := map[contracts.Status]string{
		contracts.StatusBacklog:    "Backlog",
		contracts.StatusReady:      "Ready",
		contracts.StatusInProgress: "In Progress",
		contracts.StatusInReview:   "In Review",
		contracts.StatusBlocked:    "Blocked",
		contracts.StatusDone:       "Done",
	}
	if width >= 54 {
		rows := make([][]string, 0)
		for _, status := range ordered {
			tickets := sortedTickets(board.Columns[status])
			if len(tickets) == 0 {
				// empty workflow columns are noise in a table; the kanban
				// sections below still show them on narrow terminals
				continue
			}
			column := fmt.Sprintf("%s (%d)", labels[status], len(tickets))
			if ColorEnabled() {
				column = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Render(column)
			}
			for idx, ticket := range tickets {
				label := ""
				if idx == 0 {
					label = column
				}
				rows = append(rows, []string{
					label,
					SanitizeDisplayLine(ticket.ID),
					optionalString(TypeBadge(ticket.Type), "-"),
					StatusBadge(ticket.Status),
					PriorityBadge(ticket.Priority),
					optionalString(SanitizeDisplayLine(ticket.Title), "(untitled)"),
				})
			}
		}
		if len(rows) == 0 {
			return EmptyState("Board", "No tickets yet.")
		}
		return RenderTable([]string{"Column", "ID", "Type", "Status", "Priority", "Title"}, rows, TableOptions{
			Title: "Board",
			Width: width,
		})
	}
	sections := make([]string, 0, len(ordered))
	for _, status := range ordered {
		tickets := sortedTickets(board.Columns[status])
		header := TruncateDisplay(fmt.Sprintf("%s (%d)", labels[status], len(tickets)), width)
		if ColorEnabled() {
			header = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Render(header)
		}
		section := []string{header}
		if len(tickets) == 0 {
			section = append(section, TruncateDisplay("  - (empty)", width))
		} else {
			for _, ticket := range tickets {
				summaryWidth := width - 4
				if summaryWidth < 1 {
					summaryWidth = 1
				}
				section = append(section, TruncateDisplay("  - "+TicketSummary(ticket, summaryWidth), width))
			}
		}
		sections = append(sections, strings.Join(section, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func sortedTickets(tickets []contracts.TicketSnapshot) []contracts.TicketSnapshot {
	out := append([]contracts.TicketSnapshot{}, tickets...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out
}

func Markdown(input string) string {
	return MarkdownWithWidth(input, terminalWidth(100)-4)
}

func MarkdownWithWidth(input string, width int) string {
	input = SanitizeDisplay(input)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(markdownWidth(width)),
	)
	if err != nil {
		return input
	}
	out, err := renderer.Render(input)
	if err != nil {
		return input
	}
	return out
}

func RenderTable(headers []string, rows [][]string, options TableOptions) string {
	width := normalizedWidth(options.Width)
	cleanHeaders := make([]string, 0, len(headers))
	for _, header := range headers {
		cleanHeaders = append(cleanHeaders, SanitizeDisplayLine(header))
	}
	cleanRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		clean := make([]string, 0, len(row))
		for _, cell := range row {
			// keep our own SGR badge styling; renderers strict-scrub user
			// content before any styling is applied (same invariant as the
			// output boundary)
			clean = append(clean, strings.Join(strings.Fields(sanitizeDisplay(cell, false, true)), " "))
		}
		cleanRows = append(cleanRows, clean)
	}

	border := lipgloss.RoundedBorder()
	if tableUsesASCII(options.Border) {
		border = lipgloss.ASCIIBorder()
	}
	styleCell := func(row, _ int) lipgloss.Style {
		style := lipgloss.NewStyle().Padding(0, 1)
		switch {
		case row == tableview.HeaderRow:
			style = style.Bold(true)
			if ColorEnabled() {
				style = style.Foreground(theme.Accent)
			}
		case options.HighlightSelected && options.SelectedRow >= 0 && row == options.SelectedRow:
			style = style.Bold(true)
			if ColorEnabled() {
				style = style.Foreground(theme.Primary)
			}
		}
		return style
	}
	makeTable := func(fixedWidth int) *tableview.Table {
		t := tableview.New().
			Border(border).
			BorderRow(false).
			BorderHeader(true).
			BorderColumn(true).
			Wrap(false).
			StyleFunc(styleCell).
			Headers(cleanHeaders...).
			Rows(cleanRows...)
		if fixedWidth > 0 {
			t = t.Width(fixedWidth)
		}
		if ColorEnabled() {
			t.BorderStyle(lipgloss.NewStyle().Foreground(theme.Muted))
		}
		return t
	}

	// hug the content like a real terminal table; only pin the width when the
	// natural layout overflows the terminal
	rendered := makeTable(0).String()
	for _, line := range strings.Split(rendered, "\n") {
		if lipgloss.Width(line) > width {
			rendered = makeTable(width).String()
			break
		}
	}
	title := strings.TrimSpace(SanitizeDisplayLine(options.Title))
	if title == "" {
		return rendered
	}
	titleText := TruncateDisplay(title, width)
	if ColorEnabled() {
		titleText = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Render(titleText)
	}
	return titleText + "\n" + rendered
}

func tableUsesASCII(mode TableBorderMode) bool {
	switch mode {
	case TableBorderASCII:
		return true
	case TableBorderUnicode:
		return false
	default:
		return !ColorEnabled()
	}
}

func StatusBadge(status contracts.Status) string {
	return valueBadge(string(status))
}

func TypeBadge(ticketType contracts.TicketType) string {
	if !ticketType.IsValid() {
		return ""
	}
	return valueBadge(string(ticketType))
}

func PriorityBadge(priority contracts.Priority) string {
	value := strings.TrimSpace(SanitizeDisplayLine(string(priority)))
	if value == "" {
		value = "unknown"
	}
	text := "[" + value + "]"
	if !ColorEnabled() {
		return text
	}
	// urgency scale instead of the status buckets: critical screams, high
	// glows, medium stays quiet, low fades
	if color, ok := theme.PriorityColor(value); ok {
		return lipgloss.NewStyle().Foreground(color).Render(text)
	}
	return text
}

func GateBadge(state any) string {
	return namedBadge("gate", fmt.Sprint(state))
}

func SignatureBadge(state any) string {
	return namedBadge("sig", fmt.Sprint(state))
}

func SyncBadge(state any) string {
	return namedBadge("sync", fmt.Sprint(state))
}

func TicketSummary(ticket contracts.TicketSnapshot, width int) string {
	head := strings.TrimSpace(strings.Join(compactNonEmpty(SanitizeDisplayLine(ticket.ID), TypeBadge(ticket.Type), StatusBadge(ticket.Status), PriorityBadge(ticket.Priority)), " "))
	title := strings.TrimSpace(SanitizeDisplayLine(ticket.Title))
	if title == "" {
		title = "(untitled)"
	}
	full := head + " " + title
	width = normalizedWidth(width)
	if lipgloss.Width(full) <= width {
		return full
	}
	titleWidth := width - lipgloss.Width(head) - 1
	if titleWidth <= 4 {
		return TruncateDisplay(head, width)
	}
	return head + " " + TruncateDisplay(title, titleWidth)
}

func compactNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func TruncateDisplay(value string, maxWidth int) string {
	// keep our own SGR styling intact while scrubbing everything else;
	// lipgloss.Width is ansi-aware so the fit check still measures glyphs
	value = strings.Join(strings.Fields(sanitizeDisplay(value, false, true)), " ")
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	// too wide: drop styling so the rune cut below can't slice an escape
	// sequence in half
	value = SanitizeDisplayLine(value)
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	suffix := "..."
	if maxWidth <= lipgloss.Width(suffix) {
		suffix = ""
	}
	limit := maxWidth - lipgloss.Width(suffix)
	var out strings.Builder
	for _, r := range value {
		next := out.String() + string(r)
		if lipgloss.Width(next) > limit {
			break
		}
		out.WriteRune(r)
	}
	return strings.TrimRight(out.String(), " ") + suffix
}

func EmptyState(title string, action string) string {
	title = strings.TrimSpace(SanitizeDisplayLine(title))
	action = strings.TrimSpace(SanitizeDisplayLine(action))
	if action == "" {
		return fmt.Sprintf("%s\n  (empty)", title)
	}
	return fmt.Sprintf("%s\n  (empty)\n  next: %s", title, action)
}

func optionalString(value string, fallback string) string {
	value = SanitizeDisplayLine(value)
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func valueBadge(value string) string {
	value = strings.TrimSpace(SanitizeDisplayLine(value))
	if value == "" {
		value = "unknown"
	}
	text := "[" + value + "]"
	if !ColorEnabled() {
		return text
	}
	return lipgloss.NewStyle().Foreground(theme.StatusColor(value)).Render(text)
}

func namedBadge(kind string, value string) string {
	kind = strings.TrimSpace(strings.ToLower(SanitizeDisplayLine(kind)))
	value = strings.TrimSpace(SanitizeDisplayLine(value))
	if kind == "" {
		kind = "state"
	}
	if value == "" {
		value = "unknown"
	}
	text := "[" + kind + ":" + value + "]"
	if !ColorEnabled() {
		return text
	}
	return lipgloss.NewStyle().Foreground(theme.StatusColor(value)).Render(text)
}
