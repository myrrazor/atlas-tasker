package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/config"
)

func TestEveryCatalogCanBeTheWorkspaceLanguage(t *testing.T) {
	h := newWebHarness(t, false)
	for lang := range messageCatalogs {
		t.Run(lang, func(t *testing.T) {
			if err := config.Set(h.root, "web.lang", lang); err != nil {
				t.Fatalf("set catalog language: %v", err)
			}
			for _, route := range []string{"/", "/board"} {
				response := h.doAuthed(t, http.MethodGet, route, "", nil)
				if response.code != http.StatusOK || !strings.Contains(response.body, `<html lang="`+lang+`">`) {
					t.Fatalf("%s did not render configured %s: status=%d", route, lang, response.code)
				}
			}
		})
	}
}
