package cli

import (
	"reflect"
	"testing"
)

func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "simple", input: "/project list", want: []string{"project", "list"}},
		{name: "quoted", input: "/project create APP \"App Project\"", want: []string{"project", "create", "APP", "App Project"}},
		{name: "single quoted", input: "/ticket comment APP-1 --body 'needs parser fix'", want: []string{"ticket", "comment", "APP-1", "--body", "needs parser fix"}},
		{name: "escaped", input: "/search text~parser\\ v2", want: []string{"search", "text~parser v2"}},
		{name: "missing slash", input: "ticket list", wantErr: true},
		{name: "unterminated quote", input: "/ticket comment APP-1 --body \"oops", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSlashCommand(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected parse error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected parse result\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}
