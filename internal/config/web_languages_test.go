package config

import (
	"strings"
	"testing"
)

func TestWebLanguageCatalogsRoundTrip(t *testing.T) {
	for _, lang := range []string{"en", "es", "id", "zh", "ja", "ko"} {
		t.Run(lang, func(t *testing.T) {
			root := t.TempDir()
			if err := Set(root, "web.lang", " "+strings.ToUpper(lang)+" "); err != nil {
				t.Fatalf("set supported language: %v", err)
			}
			got, err := Get(root, "web.lang")
			if err != nil || got != lang {
				t.Fatalf("language round trip = %q, %v; want %q", got, err, lang)
			}
			if err := Set(root, "web.lang", "unsupported"); err == nil {
				t.Fatal("unsupported language was accepted")
			}
			got, err = Get(root, "web.lang")
			if err != nil || got != lang {
				t.Fatalf("invalid update changed saved language: %q, %v", got, err)
			}
		})
	}
}
