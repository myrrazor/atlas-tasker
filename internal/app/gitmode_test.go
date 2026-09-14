package app

import "testing"

func TestDefaultProjectKeyKeepsShortBasenamesAndFirstWordForLongNames(t *testing.T) {
	cases := map[string]string{
		"widgets":                 "WIDGETS",
		"app":                     "APP",
		"12":                      "P12",
		"***":                     "MAIN",
		"atlas-tasker-monorepo":   "ATLAS",
		"atlas_tasker_monorepo":   "ATLAS",
		"a-very-long-hyphen-name": "VERY",
		"supercalifragilistic":    "MAIN",
	}
	for in, want := range cases {
		if got := DefaultProjectKey(in); got != want {
			t.Fatalf("DefaultProjectKey(%q)=%q want %q", in, got, want)
		}
	}
}
