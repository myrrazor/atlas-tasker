// Package theme is the one place colors live. The palette comes from the
// brand assets (electric-blue chrome on near-black, see assets/brand/) plus
// GitHub-dark-style semantic hues picked to clear 4.5:1 contrast on both
// light and dark terminal backgrounds. lipgloss/termenv downsample the hex
// values to 256/16 colors on lesser terminals, and render.ColorEnabled()
// remains the hard NO_COLOR gate at every call site.
package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// brand
	Primary = lipgloss.AdaptiveColor{Light: "#0B5FD9", Dark: "#4D9FFF"} // electric blue
	Accent  = lipgloss.AdaptiveColor{Light: "#0A3F8F", Dark: "#9CC8FF"} // ice highlight
	Steel   = lipgloss.AdaptiveColor{Light: "#27408B", Dark: "#1E5ACC"} // deep bevel blue
	Muted   = lipgloss.AdaptiveColor{Light: "#57606A", Dark: "#7D8590"}

	// semantic
	Success = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	Warning = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	Danger  = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	Info    = lipgloss.AdaptiveColor{Light: "#0969DA", Dark: "#58A6FF"}
	Orange  = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FF8A3D"}
)

// StatusColor buckets workflow-ish state strings into semantic colors. Keep
// the buckets in sync with what the badges actually print; "done" family maps
// to Info on purpose -- finished work reads brand-blue, not green.
func StatusColor(value string) lipgloss.TerminalColor {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ready", "active", "approved", "trusted_valid", "completed", "resolved", "passing", "success", "enabled":
		return Success
	case "blocked", "failed", "rejected", "invalid_signature", "payload_hash_mismatch", "canonicalization_mismatch", "dirty":
		return Danger
	case "in_progress", "in_review", "open", "running", "verifying", "publishing", "planned", "valid_untrusted", "valid_unknown_key":
		return Warning
	case "done", "merged", "synced":
		return Info
	default:
		return Muted
	}
}

// PriorityColor returns the urgency scale; ok=false means leave it unstyled
// (medium is the baseline and shouldn't shout).
func PriorityColor(value string) (lipgloss.TerminalColor, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return Danger, true
	case "high":
		return Orange, true
	case "low":
		return Muted, true
	default:
		return nil, false
	}
}
