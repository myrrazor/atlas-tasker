package earlysignal

import "testing"

func TestIsServeCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"serve"}, true},
		{[]string{"serve", "--port", "0"}, true},
		{[]string{"--plain", "serve"}, true},
		{[]string{"web", "serve", "--no-browser"}, true},
		{[]string{"web", "open"}, false},
		{[]string{"mcp", "serve"}, false},
		{[]string{"home", "status"}, false},
		{[]string{"init"}, false},
		{nil, false},
	} {
		if got := IsServeCommand(tc.args); got != tc.want {
			t.Errorf("IsServeCommand(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
