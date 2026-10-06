package client

import (
	"regexp"
	"strings"
	"testing"
)

// Kept local so source archives can test the release without the server tree.
// tests/checks/test_release_version_parity.py checks the server and packaging.
const serverStableVersionPattern = `(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?`

func TestReleaseVersionMatchesServerStableGate(t *testing.T) {
	// Python uses fullmatch, so both ends must be anchored in Go as well.
	stable := regexp.MustCompile(`\A(?:` + serverStableVersionPattern + `)\z`)
	for version, want := range map[string]bool{
		"0.3.2": true, "0.3.3": true, "0.3.3+build-test.1": true,
		"0.3.3-candidate.1": false, "0.3.3-rc.1+build": false,
		"0.03.3": false, "0.3.3\n": false,
		"0.4.1": true, "0.4.1+build.1": true, "0.4.1-rc.1": false,
		"0.4.0": true, "0.4.0+build.1": true, "0.4.0-rc.1": false,
	} {
		if stable.MatchString(version) != want {
			t.Fatalf("server stable-version control failed for %q", version)
		}
	}
	for _, version := range []string{ClientVersion, Version} {
		if !stable.MatchString(version) || strings.Contains(strings.SplitN(version, "+", 2)[0], "-") {
			t.Fatalf("release version %q is not stable", version)
		}
		if !comparisonClientVersion(version) {
			t.Fatalf("release version %q disables comparison", version)
		}
	}
}
