package render

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"golang.org/x/term"
)

func colorEnabled() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func terminalWidth(defaultWidth int) int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return defaultWidth
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
	out.WriteString(titleStyle.Render(fmt.Sprintf("%s [%s] %s", ticket.ID, ticket.Status, ticket.Title)))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render(fmt.Sprintf("Type: %s  Priority: %s  Assignee: %s", ticket.Type, ticket.Priority, optionalString(string(ticket.Assignee), "-"))))
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
	if len(tickets) == 0 {
		return EmptyState(title, "No tickets found. Try creating one with `tracker ticket create`.")
	}
	lines := []string{title + ":"}
	for _, ticket := range tickets {
		lines = append(lines, fmt.Sprintf("- %s [%s] (%s) %s", ticket.ID, ticket.Status, ticket.Priority, ticket.Title))
	}
	return strings.Join(lines, "\n")
}

func BoardPretty(board contracts.BoardView) string {
	ordered := []contracts.Status{
		contracts.StatusBacklog,
		contracts.StatusReady,
		contracts.StatusInProgress,
		contracts.StatusInReview,
		contracts.StatusBlocked,
		contracts.StatusDone,
		contracts.StatusCanceled,
	}
	labels := map[contracts.Status]string{
		contracts.StatusBacklog:    "Backlog",
		contracts.StatusReady:      "Ready",
		contracts.StatusInProgress: "In Progress",
		contracts.StatusInReview:   "In Review",
		contracts.StatusBlocked:    "Blocked",
		contracts.StatusDone:       "Done",
		contracts.StatusCanceled:   "Canceled",
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
		section := []string{fmt.Sprintf("%s (%d)", labels[status], len(tickets))}
		if len(tickets) == 0 {
			section = append(section, "  - (empty)")
		} else {
			for _, ticket := range tickets {
				section = append(section, fmt.Sprintf("  - %s %s", ticket.ID, ticket.Title))
			}
		}
		sections = append(sections, strings.Join(section, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func Markdown(input string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(terminalWidth(100)-4),
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

func EmptyState(title string, action string) string {
	return fmt.Sprintf("%s\n\n%s", title, action)
}

func optionalString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
