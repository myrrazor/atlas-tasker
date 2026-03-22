package cli

import (
	"fmt"
	"strings"
)

// ParseSlashCommand converts slash input into tracker CLI args.
func ParseSlashCommand(input string) ([]string, error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return nil, nil
	}
	if !strings.HasPrefix(normalized, "/") {
		return nil, fmt.Errorf("slash commands must start with '/'")
	}
	raw := strings.TrimSpace(strings.TrimPrefix(normalized, "/"))
	if raw == "" {
		return nil, nil
	}

	args, err := splitArgs(raw)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, nil
	}
	return args, nil
}

func splitArgs(input string) ([]string, error) {
	args := make([]string, 0)
	var token strings.Builder
	inSingle := false
	inDouble := false
	escape := false

	flush := func() {
		if token.Len() > 0 {
			args = append(args, token.String())
			token.Reset()
		}
	}

	for _, ch := range input {
		if escape {
			token.WriteRune(ch)
			escape = false
			continue
		}
		switch ch {
		case '\\':
			escape = true
		case '\'':
			if inDouble {
				token.WriteRune(ch)
			} else {
				inSingle = !inSingle
			}
		case '"':
			if inSingle {
				token.WriteRune(ch)
			} else {
				inDouble = !inDouble
			}
		case ' ', '\t':
			if inSingle || inDouble {
				token.WriteRune(ch)
			} else {
				flush()
			}
		default:
			token.WriteRune(ch)
		}
	}

	if escape || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote or escape")
	}
	flush()
	return args, nil
}
