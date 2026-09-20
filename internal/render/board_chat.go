package render

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// ChatBoardWidth is the paste width for Discord / Grokbot. Desktop code
// blocks are wider; this stays readable on a phone without wrapping.
const ChatBoardWidth = 52

// ChatPasteHint tells an agent what to do with the chat field.
const ChatPasteHint = "Paste the chat field verbatim in Discord, Grokbot, or any host that renders ANSI code blocks. Use markdown in Slack, Teams, or hosts that do not color ANSI."

// Discord-safe 3/4-bit SGR only. Discord ignores 256-color and truecolor.
const (
	chatReset       = "\x1b[0m"
	chatBoldWhite   = "1;37"
	chatBoldCyan    = "1;36"
	chatBoldGreen   = "1;32"
	chatBoldBlue    = "1;34"
	chatBoldMagenta = "1;35"
	chatBoldRed     = "1;31"
	chatBoldYellow  = "1;33"
	chatWhite       = "37"
	chatCyan        = "36"
	chatGreen       = "32"
	chatBlue        = "34"
	chatMagenta     = "35"
	chatRed         = "31"
)

// StatusChatView is the chat projection of atlas.status. Renderers must keep
// status names as text; color is extra, not the only signal.
type StatusChatView struct {
	Scope           string
	Project         string
	TicketID        string
	AgentID         string
	RunID           string
	Actor           string
	UnknownProject  bool
	Disambiguation  []string
	SetupErrors     []string
	Board           CompactBoard
	RecommendedNext []string
}

// CompactBoardChat is the paste-ready Discord/Grokbot board: a fenced
// ```ansi block with status colors. Default MCP markdown stays ANSI-free.
func CompactBoardChat(board CompactBoard) string {
	return fenceChat(chatBoardLines(chatHeadline(board), board, nil))
}

// StatusChat is the paste-ready Discord/Grokbot status card.
func StatusChat(view StatusChatView) string {
	if view.UnknownProject {
		return fenceChat(chatDisambiguation(view))
	}
	board := view.Board
	if view.Scope == "ticket" && strings.TrimSpace(view.TicketID) != "" {
		board = focusChatBoard(board, view.TicketID)
	}
	if len(view.RecommendedNext) > 0 {
		board.NextActions = append([]string(nil), view.RecommendedNext...)
	}
	return fenceChat(chatBoardLines(chatStatusHeadline(view, board), board, view.SetupErrors))
}

func fenceChat(body string) string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		body = chatPaint(chatWhite, "(empty)")
	}
	return "```ansi\n" + body + "\n```\n"
}

func chatHeadline(board CompactBoard) string {
	project := chatSafe(board.Project)
	if project == "" {
		project = chatSafe(strings.TrimPrefix(board.Title, "Board "))
	}
	if project == "" || strings.EqualFold(project, "Board") {
		return chatPaint(chatBoldCyan, "ATLAS")
	}
	return chatPaint(chatBoldCyan, "ATLAS") + chatPaint(chatWhite, "  ·  ") + chatPaint(chatBoldWhite, project)
}

func chatStatusHeadline(view StatusChatView, board CompactBoard) string {
	switch view.Scope {
	case "ticket":
		id := chatSafe(view.TicketID)
		if id == "" {
			id = chatSafe(board.Project)
		}
		return chatPaint(chatBoldCyan, "ATLAS") + chatPaint(chatWhite, "  ·  ") + chatPaint(chatBoldWhite, id)
	case "agent":
		who := chatSafe(view.AgentID)
		if who == "" {
			who = chatSafe(view.Actor)
		}
		if who == "" {
			who = "agent"
		}
		return chatPaint(chatBoldCyan, "ATLAS") + chatPaint(chatWhite, "  ·  ") + chatPaint(chatBoldWhite, who)
	case "run":
		id := chatSafe(view.RunID)
		if id == "" {
			id = "run"
		}
		return chatPaint(chatBoldCyan, "ATLAS") + chatPaint(chatWhite, "  ·  ") + chatPaint(chatBoldWhite, id)
	default:
		return chatHeadline(board)
	}
}

func chatDisambiguation(view StatusChatView) string {
	var lines []string
	lines = append(lines, chatPaint(chatBoldCyan, "ATLAS"))
	if requested := chatSafe(view.Project); requested != "" {
		lines = append(lines, chatPaint(chatBoldYellow, "Unknown project  "+requested))
	} else {
		lines = append(lines, chatPaint(chatWhite, "Name a project for project-scoped status."))
	}
	known := make([]string, 0, len(view.Disambiguation))
	for _, item := range view.Disambiguation {
		item = chatSafe(item)
		if item != "" {
			known = append(known, item)
		}
	}
	if len(known) == 0 {
		lines = append(lines, chatPaint(chatWhite, "Known projects: (none)"))
	} else {
		lines = append(lines, chatPaint(chatWhite, "Known projects: "+strings.Join(known, ", ")))
	}
	return strings.Join(lines, "\n")
}

func chatBoardLines(headline string, board CompactBoard, setupErrors []string) string {
	width := ChatBoardWidth
	var lines []string
	lines = append(lines, fitChat(headline, width))
	if board.TotalCards == 0 && len(populatedColumns(board)) == 0 && len(setupErrors) == 0 && len(board.NextActions) == 0 {
		lines = append(lines, chatPaint(chatWhite, "0 tickets"))
		lines = append(lines, "")
		lines = append(lines, chatPaint(chatWhite, "(empty)"))
		return strings.Join(lines, "\n")
	}
	if board.TotalCards > 0 || board.ShownCards > 0 {
		lines = append(lines, fitChat(chatPaint(chatWhite, ticketCountPhrase(board.TotalCards, board.ShownCards, board.Truncated)), width))
	}
	if len(board.Attention) > 0 {
		items := make([]string, 0, len(board.Attention))
		for _, item := range board.Attention {
			items = append(items, chatSafe(item))
		}
		lines = append(lines, chatPaint(chatBoldYellow, TruncateDisplay(strings.Join(items, "  ·  "), width)))
	}
	for _, err := range setupErrors {
		err = chatSafe(err)
		if err == "" {
			continue
		}
		lines = append(lines, chatPaint(chatBoldRed, TruncateDisplay("setup  "+err, width)))
	}
	populated := populatedColumns(board)
	if len(populated) == 0 && board.TotalCards == 0 {
		lines = append(lines, "")
		if len(board.NextActions) > 0 {
			lines = append(lines, chatPaint(chatBoldCyan, "NEXT"))
			lines = append(lines, chatPaint(chatCyan, TruncateDisplay("→  "+chatSafe(board.NextActions[0]), width)))
		} else {
			lines = append(lines, chatPaint(chatWhite, "(empty)"))
		}
		return strings.Join(lines, "\n")
	}
	for _, col := range populated {
		lines = append(lines, "")
		header := strings.ToUpper(chatSafe(col.Label))
		if col.Truncated {
			header = fmt.Sprintf("%s  %d  showing %d", header, col.Total, col.Shown)
		} else {
			header = fmt.Sprintf("%s  %d", header, col.Total)
		}
		lines = append(lines, fitChat(chatPaint(chatLaneBold(col.Status), header), width))
		for _, card := range col.Cards {
			lines = append(lines, chatCardLine(card, col.Status, width))
		}
	}
	if len(board.NextActions) > 0 {
		lines = append(lines, "")
		lines = append(lines, chatPaint(chatBoldCyan, "NEXT"))
		lines = append(lines, chatPaint(chatCyan, TruncateDisplay("→  "+chatSafe(board.NextActions[0]), width)))
	}
	for _, note := range board.Notes {
		note = chatSafe(note)
		if note == "" {
			continue
		}
		lines = append(lines, chatPaint(chatWhite, TruncateDisplay(note, width)))
	}
	return strings.Join(lines, "\n")
}

func chatCardLine(card CompactCard, status string, width int) string {
	bar := chatPaint(chatLane(status), "▎")
	id := chatSafe(card.ID)
	if id == "" {
		id = "?"
	}
	title := chatSafe(card.Title)
	if title == "" {
		title = "(untitled)"
	}
	pri := chatPriorityLabel(card)
	extra := chatExtraLabel(card)
	prefix := bar + " "
	gap := "  "
	used := DisplayWidth(prefix) + DisplayWidth(id) + DisplayWidth(gap)
	if pri != "" {
		used += DisplayWidth(gap) + DisplayWidth(pri)
	}
	titleWidth := maxInt(6, width-used)
	if extra != "" && titleWidth > 16 {
		extraWidth := minInt(DisplayWidth(extra), 14)
		if titleWidth-DisplayWidth(gap)-extraWidth >= 8 {
			used += DisplayWidth(gap) + extraWidth
			titleWidth = maxInt(6, width-used)
			extra = TruncateDisplay(extra, extraWidth)
		} else {
			extra = ""
		}
	} else {
		extra = ""
	}
	title = TruncateDisplay(title, titleWidth)
	line := prefix + chatPaint(chatBoldWhite, id) + gap + chatPaint(chatWhite, title)
	if pri != "" {
		line += gap + chatPaint(chatMetaCode(card), pri)
	}
	if extra != "" {
		line += gap + chatPaint(chatWhite, extra)
	}
	return fitChat(line, width)
}

func chatPriorityLabel(card CompactCard) string {
	if card.Priority == "" || card.Priority == string(contracts.PriorityMedium) {
		return ""
	}
	return strings.ToUpper(chatSafe(card.Priority))
}

func chatExtraLabel(card CompactCard) string {
	if card.Assignee != "" {
		return chatSafe(card.Assignee)
	}
	if len(card.BlockedBy) > 0 {
		return "needs " + chatSafe(card.BlockedBy[0])
	}
	return ""
}

func chatMetaCode(card CompactCard) string {
	switch strings.ToLower(strings.TrimSpace(card.Priority)) {
	case string(contracts.PriorityCritical):
		return chatBoldRed
	case string(contracts.PriorityHigh):
		return chatBoldYellow
	case string(contracts.PriorityLow):
		return chatWhite
	}
	if len(card.BlockedBy) > 0 {
		return chatRed
	}
	return chatWhite
}

func chatLane(status string) string {
	switch contracts.Status(strings.ToLower(strings.TrimSpace(status))) {
	case contracts.StatusReady:
		return chatGreen
	case contracts.StatusInProgress:
		return chatBlue
	case contracts.StatusInReview:
		return chatMagenta
	case contracts.StatusBlocked:
		return chatRed
	case contracts.StatusDone:
		return chatCyan
	case contracts.StatusCanceled:
		return chatWhite
	default:
		return chatWhite
	}
}

func chatLaneBold(status string) string {
	switch contracts.Status(strings.ToLower(strings.TrimSpace(status))) {
	case contracts.StatusReady:
		return chatBoldGreen
	case contracts.StatusInProgress:
		return chatBoldBlue
	case contracts.StatusInReview:
		return chatBoldMagenta
	case contracts.StatusBlocked:
		return chatBoldRed
	case contracts.StatusDone:
		return chatBoldCyan
	case contracts.StatusCanceled:
		return chatBoldWhite
	default:
		return chatBoldWhite
	}
}

func focusChatBoard(board CompactBoard, ticketID string) CompactBoard {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return board
	}
	for _, col := range board.Columns {
		for _, card := range col.Cards {
			if card.ID != ticketID {
				continue
			}
			focused := board
			focused.Title = ticketID
			focused.TotalCards = 1
			focused.ShownCards = 1
			focused.Truncated = false
			focused.Notes = nil
			focused.Columns = []CompactColumn{{
				Status: col.Status,
				Label:  col.Label,
				Total:  1,
				Shown:  1,
				Cards:  []CompactCard{card},
			}}
			return focused
		}
	}
	return board
}

func chatPaint(code, text string) string {
	if text == "" {
		return ""
	}
	return "\x1b[" + code + "m" + text + chatReset
}

func chatSafe(value string) string {
	value = SanitizeDisplayLine(value)
	value = strings.ReplaceAll(value, "`", "'")
	value = strings.ReplaceAll(value, "```", "'''")
	return value
}

func fitChat(value string, width int) string {
	if width <= 0 || DisplayWidth(value) <= width {
		return value
	}
	return TruncateDisplay(value, width)
}
