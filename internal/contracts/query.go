package contracts

import (
	"fmt"
	"strings"
)

// SearchTermKind defines a single supported v1 query primitive.
type SearchTermKind string

const (
	SearchTermStatus   SearchTermKind = "status"
	SearchTermType     SearchTermKind = "type"
	SearchTermProject  SearchTermKind = "project"
	SearchTermAssignee SearchTermKind = "assignee"
	SearchTermLabel    SearchTermKind = "label"
	SearchTermTextLike SearchTermKind = "text"
)

// SearchTerm is one parsed token in a v1 search query.
type SearchTerm struct {
	Kind  SearchTermKind `json:"kind"`
	Value string         `json:"value"`
}

// SearchQuery is a simple AND query over terms.
type SearchQuery struct {
	Terms []SearchTerm `json:"terms"`
}

func ParseSearchQuery(raw string) (SearchQuery, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return SearchQuery{}, fmt.Errorf("query is empty")
	}

	tokens := strings.Fields(trimmed)
	terms := make([]SearchTerm, 0, len(tokens))

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch {
		case strings.HasPrefix(token, "status="):
			value := strings.TrimPrefix(token, "status=")
			if value == "" {
				return SearchQuery{}, fmt.Errorf("status query missing value")
			}
			terms = append(terms, SearchTerm{Kind: SearchTermStatus, Value: value})
		case strings.HasPrefix(token, "type="):
			value := strings.TrimPrefix(token, "type=")
			if value == "" {
				return SearchQuery{}, fmt.Errorf("type query missing value")
			}
			terms = append(terms, SearchTerm{Kind: SearchTermType, Value: value})
		case strings.HasPrefix(token, "project="):
			value := strings.TrimPrefix(token, "project=")
			if value == "" {
				return SearchQuery{}, fmt.Errorf("project query missing value")
			}
			terms = append(terms, SearchTerm{Kind: SearchTermProject, Value: value})
		case strings.HasPrefix(token, "assignee="):
			value := strings.TrimPrefix(token, "assignee=")
			if value == "" {
				return SearchQuery{}, fmt.Errorf("assignee query missing value")
			}
			terms = append(terms, SearchTerm{Kind: SearchTermAssignee, Value: value})
		case strings.HasPrefix(token, "label="):
			value := strings.TrimPrefix(token, "label=")
			if value == "" {
				return SearchQuery{}, fmt.Errorf("label query missing value")
			}
			terms = append(terms, SearchTerm{Kind: SearchTermLabel, Value: value})
		case strings.HasPrefix(token, "text~"):
			value := strings.TrimPrefix(token, "text~")
			for i+1 < len(tokens) && !isSearchTermStart(tokens[i+1]) {
				i++
				value += " " + tokens[i]
			}
			value = cleanSearchValue(value)
			if value == "" {
				return SearchQuery{}, fmt.Errorf("text query missing value")
			}
			terms = append(terms, SearchTerm{Kind: SearchTermTextLike, Value: value})
		default:
			return SearchQuery{}, fmt.Errorf("unsupported query token: %s", token)
		}
	}

	return SearchQuery{Terms: terms}, nil
}

func isSearchTermStart(token string) bool {
	for _, prefix := range []string{"status=", "type=", "project=", "assignee=", "label=", "text~"} {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}

func cleanSearchValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}
