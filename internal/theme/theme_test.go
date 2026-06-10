package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStatusColorBucketsMatchLegacy(t *testing.T) {
	// same buckets render.colorFor used with ANSI-16; the theme just swaps
	// the palette underneath
	cases := map[string]lipgloss.TerminalColor{
		"ready":                     Success,
		" ACTIVE ":                  Success,
		"approved":                  Success,
		"trusted_valid":             Success,
		"completed":                 Success,
		"resolved":                  Success,
		"passing":                   Success,
		"success":                   Success,
		"enabled":                   Success,
		"blocked":                   Danger,
		"failed":                    Danger,
		"rejected":                  Danger,
		"invalid_signature":         Danger,
		"payload_hash_mismatch":     Danger,
		"canonicalization_mismatch": Danger,
		"dirty":                     Danger,
		"in_progress":               Warning,
		"in_review":                 Warning,
		"open":                      Warning,
		"running":                   Warning,
		"verifying":                 Warning,
		"publishing":                Warning,
		"planned":                   Warning,
		"valid_untrusted":           Warning,
		"valid_unknown_key":         Warning,
		"done":                      Info,
		"merged":                    Info,
		"synced":                    Info,
		"unknown_thing":             Muted,
		"":                          Muted,
	}
	for in, want := range cases {
		if got := StatusColor(in); got != want {
			t.Fatalf("StatusColor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPriorityColorScale(t *testing.T) {
	if c, ok := PriorityColor("critical"); !ok || c != Danger {
		t.Fatalf("critical should map to Danger, got %v ok=%v", c, ok)
	}
	if c, ok := PriorityColor("high"); !ok || c != Orange {
		t.Fatalf("high should map to Orange, got %v ok=%v", c, ok)
	}
	if c, ok := PriorityColor("low"); !ok || c != Muted {
		t.Fatalf("low should map to Muted, got %v ok=%v", c, ok)
	}
	if _, ok := PriorityColor("medium"); ok {
		t.Fatal("medium stays unstyled")
	}
	if _, ok := PriorityColor(""); ok {
		t.Fatal("empty priority stays unstyled")
	}
}
