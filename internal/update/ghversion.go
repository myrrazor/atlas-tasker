package update

import (
	"fmt"
	"regexp"

	"golang.org/x/mod/semver"
)

// GHMinVersion matches GH_MIN_VERSION in scripts/install.sh. 2.102.0 is the
// first gh whose `attestation verify` has both signer-policy fixes
// (GHSA-mm27-mwq9-fr5g in 2.97.0, GHSA-4mq3-hpgx-9cx8 in 2.102.0).
const GHMinVersion = "2.102.0"

// GHTooOldMessage is word for word the message scripts/install.sh prints.
const GHTooOldMessage = "Atlas Tasker needs GitHub CLI (gh) " + GHMinVersion + " or newer. Upgrade it from https://cli.github.com/ (distro packages are often older) and re-run."

var ghVersionLine = regexp.MustCompile(`(?m)^gh version (\d+\.\d+(?:\.\d+)?)`)

// checkGHVersion parses `gh --version` output and returns an error carrying
// GHTooOldMessage when it is unreadable or older than GHMinVersion.
func checkGHVersion(output string) error {
	m := ghVersionLine.FindStringSubmatch(output)
	if m == nil {
		return fmt.Errorf("%s Could not read the version from gh --version", GHTooOldMessage)
	}
	found := normalizeSemver(m[1])
	if found == "" || semver.Compare(found, "v"+GHMinVersion) < 0 {
		return fmt.Errorf("%s Found gh %s; minimum is %s", GHTooOldMessage, m[1], GHMinVersion)
	}
	return nil
}
