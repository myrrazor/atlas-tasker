package slashcmd

import "testing"

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"/ticket view APP-1",
		"/ticket comment APP-1 --body \"hello world\"",
		"/search status=ready label=cli",
		"/broken \"quote",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = Parse(input)
	})
}
