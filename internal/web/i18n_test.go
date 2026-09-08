package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/config"
)

func TestMessageCatalogsHaveIdenticalKeys(t *testing.T) {
	english := messageCatalogs[defaultLanguage]
	for lang, catalog := range messageCatalogs {
		for key := range english {
			if _, ok := catalog[key]; !ok {
				t.Errorf("%s catalog missing %q", lang, key)
			}
		}
		for key := range catalog {
			if _, ok := english[key]; !ok {
				t.Errorf("%s catalog has extra key %q", lang, key)
			}
		}
	}
}

func TestResolveLanguagePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		configured string
		accepted   string
		want       string
	}{
		{name: "query wins", query: "es-MX", configured: "id", accepted: "en", want: "es"},
		{name: "config wins", configured: "id", accepted: "es", want: "id"},
		{name: "accept language base", accepted: "fr-FR, id-ID;q=0.8, es;q=0.7", want: "id"},
		{name: "accept language quality", accepted: "es;q=0.4, id;q=0.9", want: "id"},
		{name: "unsupported falls back", query: "fr", configured: "de", accepted: "pt", want: "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveLanguage(tt.query, tt.configured, tt.accepted); got != tt.want {
				t.Fatalf("resolveLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderedWelcomeAndBoardUseConfiguredSpanish(t *testing.T) {
	h := newWebHarness(t, false)
	if err := config.Set(h.root, "web.owner_name", "Ada Lovelace"); err != nil {
		t.Fatalf("set owner name: %v", err)
	}
	if err := config.Set(h.root, "web.lang", "es"); err != nil {
		t.Fatalf("set web language: %v", err)
	}

	welcome := h.doAuthed(t, http.MethodGet, "/", "", nil)
	for _, want := range []string{
		`<html lang="es">`,
		"Resumen del espacio de trabajo",
		"Te damos la bienvenida, Ada Lovelace",
		"Cambios recientes",
		h.ticketID + " se creó",
		`href="/?lang=id"`,
	} {
		if !strings.Contains(welcome.body, want) {
			t.Fatalf("Spanish welcome missing %q:\n%s", want, welcome.body)
		}
	}

	board := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	for _, want := range []string{
		`<html lang="es">`,
		">Filtros</summary>",
		">Pendiente</span>",
		">En curso</span>",
		"No hay tickets",
		"Notas y actividad",
		`href="/board?lang=id"`,
	} {
		if !strings.Contains(board.body, want) {
			t.Fatalf("Spanish board missing %q:\n%s", want, board.body)
		}
	}

	css := h.doAuthed(t, http.MethodGet, "/static/app.css", "", nil)
	if !strings.Contains(css.body, ".drawer-actions button") || !strings.Contains(css.body, "white-space: normal") {
		t.Fatalf("drawer actions must allow translated labels to wrap:\n%s", css.body)
	}
	if !strings.Contains(css.body, "repeat(4, minmax(120px, 1fr))") {
		t.Fatalf("compact drawer tabs must reserve room for translated labels:\n%s", css.body)
	}
}
