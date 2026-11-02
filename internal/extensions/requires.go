package extensions

import (
	"fmt"
	"regexp"
	"strconv"
)

// RunningVersion is the version of the running build. The program sets it
// once at start-up, before any extension is read. "dev", and any value that
// does not start with x.y.z, mark a build that is not a release: it does not
// check Manifest.Requires.
var RunningVersion = "dev"

var (
	requiresPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	// releasePattern reads the leading x.y.z of a build version. The release
	// script uses `git describe`, so a build can be "v0.4.4", "v0.4.4-3-gabc"
	// or "v0.4.4-dirty".
	releasePattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)
)

// semver is a version as three numbers, so 0.10.0 is above 0.9.0.
type semver [3]int

func (v semver) less(other semver) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] < other[i]
		}
	}
	return false
}

// parseNumbers turns the three captured digit groups into a semver.
func parseNumbers(groups []string) (semver, bool) {
	var v semver
	for i, group := range groups {
		n, err := strconv.Atoi(group)
		if err != nil {
			return semver{}, false
		}
		v[i] = n
	}
	return v, true
}

// validateRequires checks the format of Manifest.Requires and then compares
// it with the running version. An empty Requires needs nothing.
func validateRequires(m Manifest) error {
	if m.Requires == "" {
		return nil
	}
	if !requiresPattern.MatchString(m.Requires) {
		return fmt.Errorf("requires %q must look like \"0.5.0\" (three numbers separated by dots)", m.Requires)
	}
	needed, ok := parseNumbers(releasePattern.FindStringSubmatch(m.Requires)[1:])
	if !ok {
		return fmt.Errorf("requires %q is too large", m.Requires)
	}
	groups := releasePattern.FindStringSubmatch(RunningVersion)
	if groups == nil {
		return nil // a build that is not a release skips the check
	}
	running, ok := parseNumbers(groups[1:])
	if ok && running.less(needed) {
		return fmt.Errorf("%s needs agent-screensaver %s or later; this is %s. Update agent-screensaver first.", m.ID, m.Requires, RunningVersion)
	}
	return nil
}
