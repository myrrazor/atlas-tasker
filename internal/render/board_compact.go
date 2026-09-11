package render

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const DefaultBoardCardLimit = 20

// CompactBoard is the structured board shared by MCP Markdown, the MCP App,
// and web-board linking. Markdown and HTML are derived only from these fields.
type CompactBoard struct {
	Title       string            `json:"title"`
	Project     string            `json:"project,omitempty"`
	BoardURL    string            `json:"board_url,omitempty"`
	TotalCards  int               `json:"total_cards"`
	ShownCards  int               `json:"shown_cards"`
	Truncated   bool              `json:"truncated"`
	Limit       int               `json:"limit"`
	Columns     []CompactColumn   `json:"columns"`
	Notes       []string          `json:"notes,omitempty"`
	NextCursors map[string]string `json:"next_cursors,omitempty"`
}

// CompactColumn is one accessible status column.
type CompactColumn struct {
	Status    string        `json:"status"`
	Label     string        `json:"label"`
	Total     int           `json:"total"`
	Shown     int           `json:"shown"`
	Truncated bool          `json:"truncated"`
	Cards     []CompactCard `json:"cards"`
}

// CompactCard is one ticket on the compact board. Status is always written
// as text so blocked/review states are not color-only.
type CompactCard struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority,omitempty"`
	Type        string   `json:"type,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

var compactColumnOrder = []contracts.Status{
	contracts.StatusBacklog,
	contracts.StatusReady,
	contracts.StatusInProgress,
	contracts.StatusInReview,
	contracts.StatusBlocked,
	contracts.StatusDone,
	contracts.StatusCanceled,
}

var compactColumnLabels = map[contracts.Status]string{
	contracts.StatusBacklog:    "Backlog",
	contracts.StatusReady:      "Ready",
	contracts.StatusInProgress: "In Progress",
	contracts.StatusInReview:   "In Review",
	contracts.StatusBlocked:    "Blocked",
	contracts.StatusDone:       "Done",
	contracts.StatusCanceled:   "Canceled",
}

// NewCompactBoard builds a bounded board from Atlas columns.
func NewCompactBoard(project string, columns map[contracts.Status][]contracts.TicketSnapshot, limit int, nextCursors map[string]string) CompactBoard {
	if limit <= 0 {
		limit = DefaultBoardCardLimit
	}
	title := "Board"
	if strings.TrimSpace(project) != "" {
		title = "Board " + strings.TrimSpace(project)
	}
	board := CompactBoard{
		Title:       title,
		Project:     strings.TrimSpace(project),
		Limit:       limit,
		NextCursors: nextCursors,
	}
	if board.Project != "" {
		board.BoardURL = "/board?project=" + board.Project
	} else {
		board.BoardURL = "/board"
	}
	for _, status := range compactColumnOrder {
		tickets := append([]contracts.TicketSnapshot(nil), columns[status]...)
		sort.Slice(tickets, func(i, j int) bool {
			if tickets[i].UpdatedAt.Equal(tickets[j].UpdatedAt) {
				return tickets[i].ID < tickets[j].ID
			}
			return tickets[i].UpdatedAt.After(tickets[j].UpdatedAt)
		})
		col := CompactColumn{
			Status: string(status),
			Label:  compactColumnLabels[status],
			Total:  len(tickets),
		}
		shown := tickets
		if len(shown) > limit {
			shown = shown[:limit]
			col.Truncated = true
			board.Truncated = true
		}
		col.Shown = len(shown)
		for _, ticket := range shown {
			titleText := strings.TrimSpace(ticket.Title)
			if titleText == "" {
				titleText = "(untitled)"
			}
			col.Cards = append(col.Cards, CompactCard{
				ID:       ticket.ID,
				Title:    titleText,
				Status:   string(ticket.Status),
				Priority: string(ticket.Priority),
				Type:     string(ticket.Type),
				Assignee: string(ticket.Assignee),
			})
		}
		board.Columns = append(board.Columns, col)
		board.TotalCards += col.Total
		board.ShownCards += col.Shown
	}
	if board.Truncated {
		board.Notes = append(board.Notes, fmt.Sprintf("showing %d of %d cards; use cursor or a named project to page", board.ShownCards, board.TotalCards))
	}
	return board
}

// CompactBoardMarkdown is deterministic compact Markdown derived only from board.
func CompactBoardMarkdown(board CompactBoard) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(SanitizeDisplayLine(board.Title))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Cards %d (showing %d).", board.TotalCards, board.ShownCards))
	if board.Truncated {
		b.WriteString(" Truncated.")
	}
	b.WriteString("\n")
	if board.BoardURL != "" {
		b.WriteString("Web board (human session): `")
		b.WriteString(SanitizeDisplayLine(board.BoardURL))
		b.WriteString("` via `tracker web serve`.\n")
	}
	for _, note := range board.Notes {
		b.WriteString("- ")
		b.WriteString(SanitizeDisplayLine(note))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	for _, col := range board.Columns {
		b.WriteString(fmt.Sprintf("## %s (%d)", SanitizeDisplayLine(col.Label), col.Total))
		if col.Truncated {
			b.WriteString(fmt.Sprintf(" showing %d, truncated", col.Shown))
		}
		b.WriteString("\n")
		if len(col.Cards) == 0 {
			b.WriteString("- (empty)\n\n")
			continue
		}
		for _, card := range col.Cards {
			b.WriteString("- ")
			b.WriteString(compactCardLine(card))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func compactCardLine(card CompactCard) string {
	parts := []string{
		SanitizeDisplayLine(card.ID),
		"[" + SanitizeDisplayLine(card.Status) + "]",
	}
	if card.Priority != "" {
		parts = append(parts, SanitizeDisplayLine(card.Priority))
	}
	parts = append(parts, SanitizeDisplayLine(card.Title))
	if card.Assignee != "" {
		parts = append(parts, "assignee="+SanitizeDisplayLine(card.Assignee))
	}
	if len(card.ReasonCodes) > 0 {
		parts = append(parts, "reasons="+SanitizeDisplayLine(strings.Join(card.ReasonCodes, ",")))
	}
	return strings.Join(parts, " ")
}

// BoardAppCSP is the Content-Security-Policy for the first MCP App board.
// No scripts, no network, no forms, inline CSS only.
const BoardAppCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src 'none'; font-src 'none'; script-src 'none'; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'"

// CompactBoardAppHTML is a self-contained read-only MCP App document.
func CompactBoardAppHTML(board CompactBoard) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="`)
	b.WriteString(BoardAppCSP)
	b.WriteString("\">\n<title>")
	b.WriteString(html.EscapeString(board.Title))
	b.WriteString("</title>\n<style>")
	b.WriteString(`body{font-family:system-ui,sans-serif;margin:1rem;color:#111;background:#fff}
h1,h2{margin:0 0 .5rem}
section{margin:1rem 0}
.status{font-weight:700}
.status-blocked,.status-in_review{text-decoration:underline}
.meta{color:#333}
ul{padding-left:1.2rem}
li{margin:.25rem 0}`)
	b.WriteString("</style>\n</head>\n<body>\n")
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(board.Title))
	b.WriteString("</h1>\n<p class=\"meta\">")
	b.WriteString(html.EscapeString(fmt.Sprintf("Cards %d, showing %d.", board.TotalCards, board.ShownCards)))
	if board.Truncated {
		b.WriteString(" Truncated.")
	}
	if board.BoardURL != "" {
		b.WriteString(" Web board path ")
		b.WriteString(html.EscapeString(board.BoardURL))
		b.WriteString(" (human session).")
	}
	b.WriteString("</p>\n")
	for _, note := range board.Notes {
		b.WriteString("<p class=\"meta\">")
		b.WriteString(html.EscapeString(note))
		b.WriteString("</p>\n")
	}
	for _, col := range board.Columns {
		b.WriteString(`<section aria-label="`)
		b.WriteString(html.EscapeString(col.Label))
		b.WriteString(`">`)
		b.WriteString("<h2>")
		b.WriteString(html.EscapeString(fmt.Sprintf("%s (%d)", col.Label, col.Total)))
		if col.Truncated {
			b.WriteString(html.EscapeString(fmt.Sprintf(" showing %d, truncated", col.Shown)))
		}
		b.WriteString("</h2>\n")
		if len(col.Cards) == 0 {
			b.WriteString("<p>(empty)</p>\n</section>\n")
			continue
		}
		b.WriteString("<ul>\n")
		for _, card := range col.Cards {
			b.WriteString("<li><span class=\"status status-")
			b.WriteString(html.EscapeString(card.Status))
			b.WriteString("\">")
			b.WriteString(html.EscapeString(card.Status))
			b.WriteString("</span> ")
			b.WriteString(html.EscapeString(compactCardLine(card)))
			b.WriteString("</li>\n")
		}
		b.WriteString("</ul>\n</section>\n")
	}
	b.WriteString("</body>\n</html>\n")
	return b.String()
}
