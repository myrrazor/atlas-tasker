package render

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const DefaultBoardCardLimit = 20

// CompactBoard is the structured board shared by the terminal snapshot, TUI,
// MCP Markdown, and the MCP App. Renderers derive presentation from these
// fields; they do not scrape each other's strings.
type CompactBoard struct {
	Title       string            `json:"title"`
	Project     string            `json:"project,omitempty"`
	BoardURL    string            `json:"board_url,omitempty"`
	TotalCards  int               `json:"total_cards"`
	ShownCards  int               `json:"shown_cards"`
	Truncated   bool              `json:"truncated"`
	Limit       int               `json:"limit"`
	Columns     []CompactColumn   `json:"columns"`
	Attention   []string          `json:"attention,omitempty"`
	NextActions []string          `json:"next_actions,omitempty"`
	Backup      *BackupSignal     `json:"backup,omitempty"`
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
	ID               string   `json:"id"`
	Project          string   `json:"project,omitempty"`
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	Priority         string   `json:"priority,omitempty"`
	Type             string   `json:"type,omitempty"`
	Assignee         string   `json:"assignee,omitempty"`
	Reviewer         string   `json:"reviewer,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	BlockedBy        []string `json:"blocked_by,omitempty"`
	Blocks           []string `json:"blocks,omitempty"`
	Parent           string   `json:"parent,omitempty"`
	Description      string   `json:"description,omitempty"`
	Acceptance       []string `json:"acceptance_criteria,omitempty"`
	ReviewState      string   `json:"review_state,omitempty"`
	LatestRunID      string   `json:"latest_run_id,omitempty"`
	ChangeReadyState string   `json:"change_ready_state,omitempty"`
	ChildrenTotal    int      `json:"children_total,omitempty"`
	ChildrenDone     int      `json:"children_done,omitempty"`
	ChildrenBlocked  int      `json:"children_blocked,omitempty"`
	ReasonCodes      []string `json:"reason_codes,omitempty"`
	Attention        []string `json:"attention,omitempty"`
}

// BackupSignal is a path-free backup/health snapshot for board chrome.
// Nil means the renderer should omit backup entirely rather than guess.
type BackupSignal struct {
	Configured             bool     `json:"configured"`
	SnapshotCount          int      `json:"snapshot_count,omitempty"`
	WorkerState            string   `json:"worker_state,omitempty"`
	LastErrorClass         string   `json:"last_error_class,omitempty"`
	AutomaticEnabled       bool     `json:"automatic_enabled,omitempty"`
	VerifiedRemote         bool     `json:"verified_remote,omitempty"`
	UnbackedEventCount     int      `json:"unbacked_event_count,omitempty"`
	LastLocalCheckpointID  string   `json:"last_local_checkpoint_id,omitempty"`
	LastRemoteCheckpointID string   `json:"last_remote_checkpoint_id,omitempty"`
	Notes                  []string `json:"notes,omitempty"`
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
// limit < 0 means show every card (CLI/TUI). limit == 0 uses DefaultBoardCardLimit (MCP).
func NewCompactBoard(project string, columns map[contracts.Status][]contracts.TicketSnapshot, limit int, nextCursors map[string]string) CompactBoard {
	unlimited := limit < 0
	if limit == 0 {
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
	if unlimited {
		board.Limit = 0
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
		if !unlimited && len(shown) > limit {
			shown = shown[:limit]
			col.Truncated = true
			board.Truncated = true
		}
		col.Shown = len(shown)
		for _, ticket := range shown {
			col.Cards = append(col.Cards, compactCardFromTicket(ticket))
		}
		board.Columns = append(board.Columns, col)
		board.TotalCards += col.Total
		board.ShownCards += col.Shown
	}
	if board.Truncated {
		board.Notes = append(board.Notes, fmt.Sprintf("showing %d of %d cards; use cursor or a named project to page", board.ShownCards, board.TotalCards))
	}
	deriveBoardSignals(&board)
	return board
}

// UniqueProject returns the only project key on the board, or "" when mixed
// or empty. Used so a single-project workspace heading can say DEMO without
// repeating it on every card.
func UniqueProject(columns map[contracts.Status][]contracts.TicketSnapshot) string {
	seen := ""
	for _, tickets := range columns {
		for _, ticket := range tickets {
			project := strings.TrimSpace(ticket.Project)
			if project == "" {
				continue
			}
			if seen == "" {
				seen = project
				continue
			}
			if project != seen {
				return ""
			}
		}
	}
	return seen
}

func compactCardFromTicket(ticket contracts.TicketSnapshot) CompactCard {
	titleText := strings.TrimSpace(ticket.Title)
	if titleText == "" {
		titleText = "(untitled)"
	}
	card := CompactCard{
		ID:               ticket.ID,
		Project:          strings.TrimSpace(ticket.Project),
		Title:            titleText,
		Status:           string(ticket.Status),
		Priority:         string(ticket.Priority),
		Type:             string(ticket.Type),
		Assignee:         string(ticket.Assignee),
		Reviewer:         string(ticket.Reviewer),
		Labels:           append([]string(nil), ticket.Labels...),
		BlockedBy:        append([]string(nil), ticket.BlockedBy...),
		Blocks:           append([]string(nil), ticket.Blocks...),
		Parent:           strings.TrimSpace(ticket.Parent),
		Description:      ticket.Description,
		Acceptance:       append([]string(nil), ticket.AcceptanceCriteria...),
		ReviewState:      strings.TrimSpace(string(ticket.ReviewState)),
		LatestRunID:      strings.TrimSpace(ticket.LatestRunID),
		ChangeReadyState: strings.TrimSpace(string(ticket.ChangeReadyState)),
		ReasonCodes:      nil,
	}
	if ticket.Progress.TotalChildren > 0 {
		card.ChildrenTotal = ticket.Progress.TotalChildren
		card.ChildrenDone = ticket.Progress.DoneChildren
		card.ChildrenBlocked = ticket.Progress.BlockedChildren
	}
	var attention []string
	if len(ticket.BlockedBy) > 0 {
		attention = append(attention, "blocked-by "+strings.Join(ticket.BlockedBy, ","))
	}
	if len(ticket.OpenGateIDs) > 0 {
		attention = append(attention, fmt.Sprintf("%d open gates", len(ticket.OpenGateIDs)))
	}
	if card.ChildrenBlocked > 0 {
		attention = append(attention, fmt.Sprintf("%d blocked children", card.ChildrenBlocked))
	}
	card.Attention = attention
	return card
}

func deriveBoardSignals(board *CompactBoard) {
	idIn := func(status contracts.Status) string {
		for _, col := range board.Columns {
			if col.Status == string(status) && len(col.Cards) > 0 {
				return col.Cards[0].ID
			}
		}
		return ""
	}
	totalOf := func(status contracts.Status) int {
		for _, col := range board.Columns {
			if col.Status == string(status) {
				return col.Total
			}
		}
		return 0
	}
	var attention []string
	if n := totalOf(contracts.StatusBlocked); n > 0 {
		attention = append(attention, fmt.Sprintf("%d blocked", n))
	}
	if n := totalOf(contracts.StatusInReview); n > 0 {
		attention = append(attention, fmt.Sprintf("%d in review", n))
	}
	for _, col := range board.Columns {
		for _, card := range col.Cards {
			if card.Priority == string(contracts.PriorityCritical) {
				attention = append(attention, "critical "+card.ID)
			}
		}
	}
	if board.Backup != nil {
		if board.Backup.LastErrorClass != "" {
			attention = append(attention, "backup "+board.Backup.LastErrorClass)
		}
		if board.Backup.UnbackedEventCount > 0 {
			attention = append(attention, fmt.Sprintf("%d unbacked events", board.Backup.UnbackedEventCount))
		}
	}
	if len(attention) > 8 {
		attention = attention[:8]
	}
	board.Attention = attention

	// State-led, not permission-led: never "claim"/"review" unless the
	// caller has resolved actor policy. Ready work beats a blocked caution.
	var next []string
	if id := idIn(contracts.StatusReady); id != "" {
		next = append(next, "ready work "+id)
	}
	if id := idIn(contracts.StatusInProgress); id != "" {
		next = append(next, "in progress "+id)
	}
	if id := idIn(contracts.StatusInReview); id != "" {
		next = append(next, "in review "+id)
	}
	if id := idIn(contracts.StatusBlocked); id != "" && len(next) == 0 {
		next = append(next, "blocked "+id+" — do not start until blockers clear")
	}
	if len(next) == 0 && board.TotalCards == 0 {
		next = append(next, "create a ticket in this workspace")
	}
	if len(next) == 0 {
		next = append(next, "no actionable Atlas work in this scope")
	}
	board.NextActions = next
}

// AttachBackup copies a path-free backup snapshot onto the board and
// re-derives attention so backup errors show up beside blocked work.
func AttachBackup(board *CompactBoard, signal BackupSignal) {
	copySignal := signal
	board.Backup = &copySignal
	deriveBoardSignals(board)
}

func (s BackupSignal) SummaryLine() string {
	parts := make([]string, 0, 6)
	if !s.AutomaticEnabled {
		parts = append(parts, "local checkpoints off")
	} else {
		parts = append(parts, "automatic")
	}
	if strings.TrimSpace(s.LastLocalCheckpointID) != "" {
		parts = append(parts, "local checkpoint present")
	}
	if s.UnbackedEventCount > 0 {
		parts = append(parts, fmt.Sprintf("%d unbacked events", s.UnbackedEventCount))
	}
	if s.VerifiedRemote {
		parts = append(parts, "remote verified")
	}
	if s.SnapshotCount > 0 {
		parts = append(parts, fmt.Sprintf("%d snapshots", s.SnapshotCount))
	}
	if s.LastErrorClass != "" {
		parts = append(parts, SanitizeDisplayLine(s.LastErrorClass))
	}
	// Raw worker_state "pending" is not a human status when automatic is off;
	// unbacked events already cover pending work when automatic is on.
	state := strings.TrimSpace(s.WorkerState)
	if s.AutomaticEnabled && state != "" && state != string(contracts.BackupOutboxPending) &&
		state != string(contracts.BackupOutboxVerified) {
		parts = append(parts, SanitizeDisplayLine(state))
	}
	if len(parts) == 0 {
		if !s.Configured {
			return "not configured"
		}
		if len(s.Notes) > 0 {
			return SanitizeDisplayLine(s.Notes[0])
		}
		return "configured"
	}
	return strings.Join(parts, ", ")
}

func populatedColumns(board CompactBoard) []CompactColumn {
	out := make([]CompactColumn, 0, len(board.Columns))
	for _, col := range board.Columns {
		if col.Total == 0 {
			continue
		}
		out = append(out, col)
	}
	return out
}

// CompactBoardMarkdown is the ANSI-free chat board for Slack, Teams, and
// coding-agent transcripts: heading, counts, attention, one next action,
// backup, populated lanes. Ticket text is escaped so titles cannot inject
// images, HTML, or links. Discord/Grokbot color uses CompactBoardChat.
// CLI `--md` does not use this.
func CompactBoardMarkdown(board CompactBoard) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(escapeMarkdown(board.Title))
	b.WriteString("\n\n")
	if board.TotalCards == 0 {
		b.WriteString("0 tickets.\n\n(empty)\n")
		return b.String()
	}
	b.WriteString(ticketCountPhrase(board.TotalCards, board.ShownCards, board.Truncated))
	b.WriteString("\n")
	if len(board.Attention) > 0 {
		items := make([]string, 0, len(board.Attention))
		for _, item := range board.Attention {
			items = append(items, escapeMarkdown(item))
		}
		b.WriteString("Attention: ")
		b.WriteString(strings.Join(items, "; "))
		b.WriteString(".\n")
	}
	if len(board.NextActions) > 0 {
		b.WriteString("Next: ")
		b.WriteString(escapeMarkdown(board.NextActions[0]))
		b.WriteString(".\n")
	}
	if board.Backup != nil {
		b.WriteString("Backup: ")
		b.WriteString(escapeMarkdown(board.Backup.SummaryLine()))
		b.WriteString(".\n")
	}
	if line := markdownBoardPath(board.BoardURL); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, note := range board.Notes {
		b.WriteString(escapeMarkdown(note))
		b.WriteString("\n")
	}
	populated := populatedColumns(board)
	if len(populated) == 0 {
		b.WriteString("\n(empty)\n")
		return b.String()
	}
	b.WriteString("\n")
	for _, col := range populated {
		if col.Truncated {
			b.WriteString(fmt.Sprintf("## %s (%d, showing %d)\n", escapeMarkdown(col.Label), col.Total, col.Shown))
		} else {
			b.WriteString(fmt.Sprintf("## %s (%d)\n", escapeMarkdown(col.Label), col.Total))
		}
		for _, card := range col.Cards {
			b.WriteString("- ")
			b.WriteString(markdownCardItem(card))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func markdownBoardPath(raw string) string {
	raw = strings.TrimSpace(SanitizeDisplayLine(raw))
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "`", "")
	return "Board `" + raw + "`"
}

func markdownCardItem(card CompactCard) string {
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = "(untitled)"
	}
	line := "**" + escapeMarkdown(card.ID) + "** " + escapeMarkdown(title)
	meta := markdownCardMeta(card)
	if meta == "" {
		return line
	}
	return line + "\n  " + meta
}

func markdownCardMeta(card CompactCard) string {
	bits := make([]string, 0, 6)
	if card.Priority != "" && card.Priority != string(contracts.PriorityMedium) {
		bits = append(bits, escapeMarkdown(card.Priority))
	}
	if card.Assignee != "" {
		bits = append(bits, escapeMarkdown(card.Assignee))
	}
	if card.Reviewer != "" {
		bits = append(bits, escapeMarkdown(card.Reviewer))
	}
	for _, label := range card.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		bits = append(bits, escapeMarkdown(label))
		if len(bits) >= 5 {
			break
		}
	}
	if len(card.BlockedBy) > 0 {
		bits = append(bits, "blocked by "+escapeMarkdown(strings.Join(card.BlockedBy, ", ")))
	}
	if card.LatestRunID != "" {
		bits = append(bits, escapeMarkdown(card.LatestRunID))
	}
	if card.ChildrenTotal > 0 {
		bits = append(bits, fmt.Sprintf("%d of %d children done", card.ChildrenDone, card.ChildrenTotal))
	}
	return strings.Join(bits, " · ")
}

func ticketCountPhrase(total int, shown int, truncated bool) string {
	if total == 0 {
		return "0 tickets."
	}
	if truncated {
		return fmt.Sprintf("%d of %d tickets.", shown, total)
	}
	if total == 1 {
		return "1 ticket."
	}
	return fmt.Sprintf("%d tickets.", total)
}

func escapeMarkdown(value string) string {
	value = SanitizeDisplayLine(value)
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"#", "\\#",
		"<", "\\<",
		">", "\\>",
		"!", "\\!",
		"|", "\\|",
	)
	return replacer.Replace(value)
}

// BoardAppCSP is the Content-Security-Policy for the first MCP App board.
// No scripts, no network, no forms, inline CSS only.
const BoardAppCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src 'none'; font-src 'none'; script-src 'none'; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'"

// CompactBoardAppHTML is a self-contained read-only MCP App document.
func CompactBoardAppHTML(board CompactBoard) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString("\n")
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="`)
	b.WriteString(BoardAppCSP)
	b.WriteString("\">\n<title>")
	b.WriteString(html.EscapeString(board.Title))
	b.WriteString("</title>\n<style>")
	b.WriteString(BoardAppCSS)
	b.WriteString("</style>\n</head>\n<body>\n")
	b.WriteString(CompactBoardAppBody(board))
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// CompactBoardAppBody is the inner board markup shared with the chat fragment.
func CompactBoardAppBody(board CompactBoard) string {
	return boardWidgetBody(board)
}
