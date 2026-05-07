package render

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"golang.org/x/term"
)

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

func TicketPretty(ticket contracts.TicketSnapshot, comments []string) string {
	useColor := colorEnabled()
	titleStyle := lipgloss.NewStyle().Bold(true)
	mutedStyle := lipgloss.NewStyle()
	if useColor {
		titleStyle = titleStyle.Foreground(lipgloss.Color("10"))
		mutedStyle = mutedStyle.Foreground(lipgloss.Color("8"))
	}

	out := strings.Builder{}
	out.WriteString(titleStyle.Render(fmt.Sprintf("%s %s %s", ticket.ID, StatusBadge(ticket.Status), ticket.Title)))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render(fmt.Sprintf("Type: %s  Priority: %s  Assignee: %s", ticket.Type, PriorityBadge(ticket.Priority), optionalString(string(ticket.Assignee), "-"))))
	out.WriteString("\n\n")
	if strings.TrimSpace(ticket.Description) != "" {
		out.WriteString("Description:\n")
		out.WriteString(ticket.Description)
		out.WriteString("\n\n")
	}
	if len(ticket.AcceptanceCriteria) > 0 {
		out.WriteString("Acceptance Criteria:\n")
		for _, criterion := range ticket.AcceptanceCriteria {
			out.WriteString("- " + criterion + "\n")
		}
		out.WriteString("\n")
	}
	if len(comments) > 0 {
		out.WriteString("Recent Comments:\n")
		for _, comment := range comments {
			out.WriteString("- " + comment + "\n")
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
	lines := []string{title + ":"}
	for _, ticket := range tickets {
		lines = append(lines, "- "+TicketSummary(ticket, width-2))
	}
	return strings.Join(lines, "\n")
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
	sections := make([]string, 0, len(ordered))
	for _, status := range ordered {
		tickets := board.Columns[status]
		sort.Slice(tickets, func(i, j int) bool {
			if tickets[i].UpdatedAt.Equal(tickets[j].UpdatedAt) {
				return tickets[i].ID < tickets[j].ID
			}
			return tickets[i].UpdatedAt.Before(tickets[j].UpdatedAt)
		})
		section := []string{TruncateDisplay(fmt.Sprintf("%s (%d)", labels[status], len(tickets)), width)}
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

func Markdown(input string) string {
	return MarkdownWithWidth(input, terminalWidth(100)-4)
}

func MarkdownWithWidth(input string, width int) string {
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

func StatusBadge(status contracts.Status) string {
	return valueBadge(string(status))
}

func PriorityBadge(priority contracts.Priority) string {
	return valueBadge(string(priority))
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
	head := strings.TrimSpace(fmt.Sprintf("%s %s %s", ticket.ID, StatusBadge(ticket.Status), PriorityBadge(ticket.Priority)))
	title := strings.TrimSpace(ticket.Title)
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

func TruncateDisplay(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
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
	title = strings.TrimSpace(title)
	action = strings.TrimSpace(action)
	if action == "" {
		return fmt.Sprintf("%s\n  (empty)", title)
	}
	return fmt.Sprintf("%s\n  (empty)\n  next: %s", title, action)
}

func optionalString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func valueBadge(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown"
	}
	text := "[" + value + "]"
	if !ColorEnabled() {
		return text
	}
	return lipgloss.NewStyle().Foreground(colorFor(value)).Render(text)
}

func namedBadge(kind string, value string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	value = strings.TrimSpace(value)
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
	return lipgloss.NewStyle().Foreground(colorFor(value)).Render(text)
}

func colorFor(value string) lipgloss.Color {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ready", "active", "approved", "trusted_valid", "completed", "resolved", "passing", "success", "enabled":
		return lipgloss.Color("10")
	case "blocked", "failed", "rejected", "invalid_signature", "payload_hash_mismatch", "canonicalization_mismatch", "dirty":
		return lipgloss.Color("9")
	case "in_progress", "in_review", "open", "running", "verifying", "publishing", "planned", "valid_untrusted", "valid_unknown_key":
		return lipgloss.Color("11")
	case "done", "merged", "synced":
		return lipgloss.Color("14")
	default:
		return lipgloss.Color("8")
	}
}
