package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/paths"
)

// recordingConverter appends "<source>:<event>:<stdin bytes>" to
// $HOME/started each time it runs.
const recordingConverter = "#!/bin/sh\nn=$(wc -c | tr -d ' ')\nprintf '%s:%s:%s\\n' \"$AGS_SOURCE\" \"$AGS_EVENT\" \"$n\" >> \"$HOME/started\"\n"

func startedRuns(t *testing.T, p paths.Paths) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.Home, "started"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestExtInstallRunsTheConverterOnceWithEmptyInput(t *testing.T) {
	p := paths.ForHome(t.TempDir())

	code, _, stderr := runSettings(t, p, "ext", "install", cliAgentFolder(t, "1.0.0", recordingConverter))

	if code != 0 {
		t.Fatalf("install: %s", stderr)
	}
	if got := startedRuns(t, p); got != "acme::0\n" {
		t.Errorf("converter runs = %q, want one run with the source, no event and empty stdin", got)
	}
}

func TestAnApprovedUpgradeRunsTheNewConverterOnce(t *testing.T) {
	p := withInstalledAgent(t)

	code, _, stderr := runSettingsWithInput(t, p, "y\n", "ext", "install", cliAgentFolder(t, "1.1.0", recordingConverter))

	if code != 0 {
		t.Fatalf("upgrade: %s", stderr)
	}
	if got := startedRuns(t, p); got != "acme::0\n" {
		t.Errorf("converter runs = %q, want one", got)
	}
}

func TestARefusedUpgradeNeverRunsTheConverter(t *testing.T) {
	p := withInstalledAgent(t)

	code, _, _ := runSettingsWithInput(t, p, "n\n", "ext", "install", cliAgentFolder(t, "1.1.0", recordingConverter))

	if code != 1 {
		t.Errorf("code = %d, want a refusal", code)
	}
	if got := startedRuns(t, p); got != "" {
		t.Errorf("converter ran for a refused install: %q", got)
	}
}

func TestAFailingFirstConverterRunIsOnlyAWarning(t *testing.T) {
	p := paths.ForHome(t.TempDir())

	code, stdout, stderr := runSettings(t, p, "ext", "install", cliAgentFolder(t, "1.0.0", "#!/bin/sh\nexit 3\n"))

	if code != 0 {
		t.Fatalf("code = %d, stderr %s", code, stderr)
	}
	if !strings.Contains(stdout, "installed: acme.agent 1.0.0") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "warning: ") || !strings.Contains(stdout, "bin/convert") {
		t.Errorf("stdout = %q, want a warning that names the converter", stdout)
	}
}
