package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

// DisplayWidth is the terminal cell width of value. lipgloss is ANSI-aware and
// uses the same uniseg tables we truncate with, so layout math stays honest.
func DisplayWidth(value string) int {
	return lipgloss.Width(value)
}

func truncateGraphemes(value string, limit int) string {
	if limit <= 0 || value == "" {
		return ""
	}
	if uniseg.StringWidth(value) <= limit {
		return value
	}
	var b strings.Builder
	state := -1
	width := 0
	rest := value
	for len(rest) > 0 {
		cluster, next, w, newState := uniseg.FirstGraphemeClusterInString(rest, state)
		if w < 0 {
			w = 0
		}
		if width+w > limit {
			break
		}
		b.WriteString(cluster)
		width += w
		rest = next
		state = newState
	}
	return strings.TrimRight(b.String(), " ")
}

// WrapDisplay wraps sanitized text to width using grapheme-aware word breaks.
// Long tokens (CJK, emoji, IDs) split on cluster boundaries rather than bytes.
func WrapDisplay(value string, width int) []string {
	value = strings.TrimSpace(SanitizeDisplay(value))
	if value == "" {
		return nil
	}
	if width <= 0 {
		return []string{""}
	}
	words := strings.Fields(value)
	lines := make([]string, 0, len(words))
	current := ""
	flush := func() {
		if current == "" {
			return
		}
		lines = append(lines, current)
		current = ""
	}
	for _, word := range words {
		switch {
		case current == "":
			if DisplayWidth(word) <= width {
				current = word
				continue
			}
			lines = append(lines, splitLongToken(word, width)...)
		case DisplayWidth(current+" "+word) <= width:
			current += " " + word
		default:
			flush()
			if DisplayWidth(word) <= width {
				current = word
				continue
			}
			lines = append(lines, splitLongToken(word, width)...)
		}
	}
	flush()
	if len(lines) == 0 {
		return []string{truncateGraphemes(value, width)}
	}
	return lines
}

func splitLongToken(word string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	rest := word
	for rest != "" {
		piece := truncateGraphemes(rest, width)
		if piece == "" {
			// one cluster wider than the column; skip it so we don't loop
			_, rest, _, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
			continue
		}
		lines = append(lines, piece)
		rest = strings.TrimPrefix(rest, piece)
	}
	return lines
}
