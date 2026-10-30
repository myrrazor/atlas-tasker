package update

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCheckGHVersion(t *testing.T) {
	for _, tc := range []struct {
		out string
		ok  bool
	}{
		{"gh version 2.45.0 (2024-03-31)\nhttps://github.com/cli/cli/releases/tag/v2.45.0\n", false},
		{"gh version 2.96.0 (2026-07-02)\n", false},
		{"gh version 2.101.1 (2026-09-20)\n", false},
		{"gh version 2.102.0 (2026-09-30)\n", true},
		{"gh version 2.110.0 (2026-12-01)\n", true},
		{"gh version 3.0 (2027-01-01)\n", true},
		{"not a gh banner\n", false},
	} {
		err := checkGHVersion(tc.out)
		if tc.ok != (err == nil) {
			t.Fatalf("%q: ok=%v err=%v", tc.out, tc.ok, err)
		}
		if err != nil && !strings.HasPrefix(err.Error(), GHTooOldMessage) {
			t.Fatalf("error must start with the installer message: %v", err)
		}
	}
}

func TestGHMinVersionMatchesInstaller(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	min := regexp.MustCompile(`(?m)^GH_MIN_VERSION="([^"]+)"$`).FindStringSubmatch(script)
	msg := regexp.MustCompile(`(?m)^GH_TOO_OLD_MESSAGE="([^"]+)"$`).FindStringSubmatch(script)
	if min == nil || min[1] != GHMinVersion || msg == nil || msg[1] != GHTooOldMessage {
		t.Fatalf("install.sh and tracker update disagree: %v %v", min, msg)
	}
}
