package mcp

import (
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/render"
)

// mcpAppDocument is progressive enhancement for hosts that render MCP Apps.
// Hosts without App support use Markdown. The same CompactBoard backs both.
type mcpAppDocument struct {
	MIMEType string `json:"mime_type"`
	CSP      string `json:"content_security_policy"`
	HTML     string `json:"html"`
	ReadOnly bool   `json:"read_only"`
}

func newBoardApp(board render.CompactBoard) *mcpAppDocument {
	return &mcpAppDocument{
		MIMEType: "text/html",
		CSP:      render.BoardAppCSP,
		HTML:     render.CompactBoardAppHTML(board),
		ReadOnly: true,
	}
}

func boardAppPassesCSP(doc *mcpAppDocument) bool {
	if doc == nil {
		return false
	}
	html := strings.ToLower(doc.HTML)
	if !strings.Contains(doc.HTML, render.BoardAppCSP) {
		return false
	}
	if !doc.ReadOnly {
		return false
	}
	banned := []string{"<script", "javascript:", "http://", "https://", "<form", "onclick=", "onerror=", "fetch(", "xmlhttprequest"}
	for _, needle := range banned {
		if strings.Contains(html, needle) {
			return false
		}
	}
	return true
}
