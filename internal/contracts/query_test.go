package contracts

import "testing"

func TestParseSearchQuery(t *testing.T) {
	query, err := ParseSearchQuery("status=in_progress label=cli text~parser")
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if len(query.Terms) != 3 {
		t.Fatalf("expected 3 terms, got %d", len(query.Terms))
	}
	if query.Terms[0].Kind != SearchTermStatus || query.Terms[0].Value != "in_progress" {
		t.Fatalf("unexpected first term: %#v", query.Terms[0])
	}
}

func TestParseSearchQuerySupportsAllV1Operators(t *testing.T) {
	raw := "status=ready type=task project=APP assignee=agent:builder-1 label=cli text~parser"
	query, err := ParseSearchQuery(raw)
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if len(query.Terms) != 6 {
		t.Fatalf("expected 6 terms, got %d", len(query.Terms))
	}
}

func TestParseSearchQueryRejectsUnsupportedToken(t *testing.T) {
	_, err := ParseSearchQuery("foo=bar")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseSearchQueryRejectsEmptyInput(t *testing.T) {
	_, err := ParseSearchQuery("   ")
	if err == nil {
		t.Fatal("expected empty query error")
	}
}
