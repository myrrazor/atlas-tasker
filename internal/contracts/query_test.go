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

func TestParseSearchQuerySupportsMultiWordText(t *testing.T) {
	query, err := ParseSearchQuery("project=AUTH text~logout flow")
	if err != nil {
		t.Fatalf("parse search query: %v", err)
	}
	if len(query.Terms) != 2 {
		t.Fatalf("expected two terms, got %#v", query.Terms)
	}
	if query.Terms[0] != (SearchTerm{Kind: SearchTermProject, Value: "AUTH"}) {
		t.Fatalf("unexpected project term: %#v", query.Terms[0])
	}
	if query.Terms[1] != (SearchTerm{Kind: SearchTermTextLike, Value: "logout flow"}) {
		t.Fatalf("unexpected text term: %#v", query.Terms[1])
	}
}

func TestParseSearchQuerySupportsQuotedMultiWordText(t *testing.T) {
	query, err := ParseSearchQuery(`project=AUTH text~"logout flow"`)
	if err != nil {
		t.Fatalf("parse search query: %v", err)
	}
	if len(query.Terms) != 2 {
		t.Fatalf("expected two terms, got %#v", query.Terms)
	}
	if query.Terms[1] != (SearchTerm{Kind: SearchTermTextLike, Value: "logout flow"}) {
		t.Fatalf("unexpected text term: %#v", query.Terms[1])
	}
}

func TestParseSearchQueryTextStopsAtNextStructuredTerm(t *testing.T) {
	query, err := ParseSearchQuery("text~logout flow status=ready")
	if err != nil {
		t.Fatalf("parse search query: %v", err)
	}
	if len(query.Terms) != 2 {
		t.Fatalf("expected two terms, got %#v", query.Terms)
	}
	if query.Terms[0] != (SearchTerm{Kind: SearchTermTextLike, Value: "logout flow"}) {
		t.Fatalf("unexpected text term: %#v", query.Terms[0])
	}
	if query.Terms[1] != (SearchTerm{Kind: SearchTermStatus, Value: "ready"}) {
		t.Fatalf("unexpected status term: %#v", query.Terms[1])
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
