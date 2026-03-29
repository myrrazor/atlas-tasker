package contracts

import "testing"

func FuzzParseSearchQuery(f *testing.F) {
	for _, seed := range []string{
		"status=ready",
		"status=in_progress label=cli text~parser",
		"type=bug project=APP assignee=agent:builder-1",
		"status=",
		"wat=bad",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = ParseSearchQuery(raw)
	})
}
