package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/install"
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

// installWithHooks installs the agent with the recording converter and
// adds its hooks, which runs the converter once.
func installWithHooks(t *testing.T) paths.Paths {
	t.Helper()
	p := paths.ForHome(t.TempDir())
	saveRecord(t, p)
	if code, _, stderr := runSettings(t, p, "ext", "install", cliAgentFolder(t, "1.0.0", recordingConverter)); code != 0 {
		t.Fatalf("install: %s", stderr)
	}
	if code, _, stderr := runAgent(t, p, "", "hooks", "add", "acme.agent", "--yes"); code != 0 {
		t.Fatalf("hooks add: %s", stderr)
	}
	return p
}

// saveRecord writes the install record that `install` would.
func saveRecord(t *testing.T, p paths.Paths) {
	t.Helper()
	if err := install.SaveRecord(p.InstallRecord, install.Record{Version: install.RecordVersion, Bin: install.BinPath(p.Home)}); err != nil {
		t.Fatal(err)
	}
}

func TestAPlainInstallNeverRunsTheConverter(t *testing.T) {
	p := paths.ForHome(t.TempDir())

	code, _, stderr := runSettings(t, p, "ext", "install", cliAgentFolder(t, "1.0.0", recordingConverter))

	if code != 0 {
		t.Fatalf("install: %s", stderr)
	}
	if got := startedRuns(t, p); got != "" {
		t.Errorf("converter ran on a plain install, before any trust: %q", got)
	}
}

func TestAddingHooksRunsTheConverterOnceWithEmptyInput(t *testing.T) {
	p := installWithHooks(t)

	if got := startedRuns(t, p); got != "acme::0\n" {
		t.Errorf("converter runs = %q, want one run with the source, no event and empty stdin", got)
	}
}

func TestAnApprovedUpgradeWithHooksRunsTheNewConverterOnce(t *testing.T) {
	p := installWithHooks(t)

	code, _, stderr := runSettingsWithInput(t, p, "y\n", "ext", "install", cliAgentFolder(t, "1.1.0", recordingConverter+"# 1.1.0\n"))

	if code != 0 {
		t.Fatalf("upgrade: %s", stderr)
	}
	if got := startedRuns(t, p); got != "acme::0\nacme::0\n" {
		t.Errorf("converter runs = %q, want one for hooks add and one for the upgrade", got)
	}
}

func TestAnApprovedUpgradeWithoutHooksRunsNothing(t *testing.T) {
	p := withInstalledAgent(t)

	code, _, stderr := runSettingsWithInput(t, p, "y\n", "ext", "install", cliAgentFolder(t, "1.1.0", recordingConverter))

	if code != 0 {
		t.Fatalf("upgrade: %s", stderr)
	}
	if got := startedRuns(t, p); got != "" {
		t.Errorf("converter ran for an agent without hooks: %q", got)
	}
}

func TestARefusedUpgradeNeverRunsTheConverter(t *testing.T) {
	p := installWithHooks(t)

	code, _, _ := runSettingsWithInput(t, p, "n\n", "ext", "install", cliAgentFolder(t, "1.1.0", recordingConverter+"# 1.1.0\n"))

	if code != 1 {
		t.Errorf("code = %d, want a refusal", code)
	}
	if got := startedRuns(t, p); got != "acme::0\n" {
		t.Errorf("converter runs = %q, want only the one of hooks add", got)
	}
}

func TestAFailingFirstConverterRunIsOnlyAWarning(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	saveRecord(t, p)
	if code, _, stderr := runSettings(t, p, "ext", "install", cliAgentFolder(t, "1.0.0", "#!/bin/sh\nexit 3\n")); code != 0 {
		t.Fatalf("install: %s", stderr)
	}

	code, stdout, stderr := runAgent(t, p, "", "hooks", "add", "acme.agent", "--yes")

	if code != 0 {
		t.Fatalf("code = %d, stderr %s", code, stderr)
	}
	if !strings.Contains(stdout, "hooks added: acme.agent") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "warning: ") || !strings.Contains(stdout, "bin/convert") {
		t.Errorf("stdout = %q, want a warning that names the converter", stdout)
	}
}
