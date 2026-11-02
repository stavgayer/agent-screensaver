package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testBlock = "# >>> agent-screensaver >>>\nexport PATH=\"$HOME/.local/bin:$PATH\"\n# <<< agent-screensaver <<<\n"

func linkPath(home string) string { return filepath.Join(home, ".local", "bin", "agent-screensaver") }

// installRun is one run of install: the plan and the notes are read
// first, as the command does, then Apply runs. A temp home only.
type installRun struct {
	plan      string
	notes     []string
	artifacts string
	binary    string
	record    Record
}

func runInstall(t *testing.T, home string, opts Options) installRun {
	t.Helper()
	withTools(t, home)
	artifacts, binary := artifactsFixture(t)
	return runInstallWith(t, home, artifacts, binary, opts)
}

func runInstallWith(t *testing.T, home, artifacts, binary string, opts Options) installRun {
	t.Helper()
	plan, err := Plan(home, artifacts, opts)
	if err != nil {
		t.Fatal(err)
	}
	notes := CommandNotes(home, opts)
	record, err := Apply(home, binary, artifacts, &fakeServices{}, opts)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return installRun{plan: planText(plan), notes: notes, artifacts: artifacts, binary: binary, record: record}
}

func readOrEmpty(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func TestInstallPutsTheCommandOnThePath(t *testing.T) {
	const existing = "export EDITOR=vim\nalias ll='ls -l'\n"
	tests := []struct {
		name        string
		shell, path string
		profileFile string // under home; "" means the shell has none
		before      *string
		wantProfile string // "" means the file is not there
		wantBackup  bool
		wantPlan    []string // {home} stands for the temp home
		notWantPlan []string
		wantNotes   []string
	}{
		{name: "zsh, not on the PATH, profile exists", shell: "/bin/zsh", path: "/usr/bin:/bin",
			profileFile: ".zprofile", before: ptr(existing), wantProfile: existing + testBlock, wantBackup: true,
			wantPlan: []string{"Link the agent-screensaver command at {home}/.local/bin/agent-screensaver",
				"Add ~/.local/bin to your PATH in {home}/.zprofile (backup: {home}/.zprofile.agent-screensaver.bak)"},
			wantNotes: []string{"Open a new Terminal window to use the agent-screensaver command."}},
		{name: "zsh, no profile yet", shell: "/bin/zsh", path: "/usr/bin",
			profileFile: ".zprofile", wantProfile: testBlock,
			wantPlan:  []string{"Add ~/.local/bin to your PATH in {home}/.zprofile (the file does not exist yet and is created)"},
			wantNotes: []string{"Open a new Terminal window"}},
		{name: "profile without a final newline", shell: "/bin/zsh", path: "/usr/bin",
			profileFile: ".zprofile", before: ptr("export A=1"), wantProfile: "export A=1\n" + testBlock, wantBackup: true,
			wantNotes: []string{"Open a new Terminal window"}},
		{name: "bash", shell: "/usr/local/bin/bash", path: "/usr/bin",
			profileFile: ".bash_profile", before: ptr(existing), wantProfile: existing + testBlock, wantBackup: true,
			wantPlan:  []string{"Add ~/.local/bin to your PATH in {home}/.bash_profile (backup: {home}/.bash_profile.agent-screensaver.bak)"},
			wantNotes: []string{"Open a new Terminal window"}},
		{name: "already on the PATH", shell: "/bin/zsh", path: "/usr/bin:{home}/.local/bin:/bin",
			profileFile: ".zprofile", before: ptr(existing), wantProfile: existing,
			wantPlan: []string{"Link the agent-screensaver command at {home}/.local/bin/agent-screensaver"}, notWantPlan: []string{"PATH in", "yourself"}},
		{name: "already on the PATH, unclean entry", shell: "/bin/zsh", path: "/usr/bin:{home}/./.local//bin/",
			profileFile: ".zprofile", wantProfile: "", notWantPlan: []string{"PATH in"}},
		{name: "already on the PATH, written with a tilde", shell: "/bin/zsh", path: "~/.local/bin:/usr/bin",
			profileFile: ".zprofile", wantProfile: "", notWantPlan: []string{"PATH in"}},
		{name: "already on the PATH, written with $HOME", shell: "/bin/zsh", path: "$HOME/.local/bin",
			profileFile: ".zprofile", wantProfile: "", notWantPlan: []string{"PATH in"}},
		{name: "a lookalike entry is not it", shell: "/bin/zsh", path: "{home}/.local/bin2:{home}/local/bin:.local/bin",
			profileFile: ".zprofile", wantProfile: testBlock, wantNotes: []string{"Open a new Terminal window"}},
		{name: "fish: no edit, the user adds it", shell: "/opt/homebrew/bin/fish", path: "/usr/bin",
			wantPlan:  []string{"your shell (fish) is not zsh or bash. Add ~/.local/bin to your PATH yourself"},
			wantNotes: []string{"Add ~/.local/bin to your PATH to use the agent-screensaver command."}},
		{name: "unknown shell", shell: "", path: "/usr/bin",
			wantPlan:  []string{"install does not know your shell. Add ~/.local/bin to your PATH yourself"},
			wantNotes: []string{"Add ~/.local/bin to your PATH"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, file := range []string{".zprofile", ".bash_profile"} {
				if tt.profileFile == file && tt.before != nil {
					writeFile(t, filepath.Join(home, file), []byte(*tt.before))
				}
			}
			opts := Options{Shell: tt.shell, Path: strings.ReplaceAll(tt.path, "{home}", home)}

			run := runInstall(t, home, opts)

			t.Logf("plan:\n%s", run.plan)
			if target, err := os.Readlink(linkPath(home)); err != nil || target != locationsFor(home).bin {
				t.Errorf("link -> %q, %v; want the installed binary", target, err)
			}
			if run.record.CommandLink != linkPath(home) {
				t.Errorf("record.CommandLink = %q", run.record.CommandLink)
			}
			for _, want := range tt.wantPlan {
				if !strings.Contains(run.plan, strings.ReplaceAll(want, "{home}", home)) {
					t.Errorf("plan lacks %q:\n%s", want, run.plan)
				}
			}
			for _, bad := range tt.notWantPlan {
				if strings.Contains(run.plan, bad) {
					t.Errorf("plan has %q:\n%s", bad, run.plan)
				}
			}
			if got := strings.Join(run.notes, "\n"); (len(tt.wantNotes) == 0) != (got == "") {
				t.Errorf("notes = %q", got)
			}
			for _, want := range tt.wantNotes {
				if !strings.Contains(strings.Join(run.notes, "\n"), want) {
					t.Errorf("notes lack %q: %v", want, run.notes)
				}
			}
			for _, file := range []string{".zprofile", ".bash_profile"} {
				got, exists := readOrEmpty(t, filepath.Join(home, file))
				want, wantExists := "", false
				if file == tt.profileFile {
					want, wantExists = tt.wantProfile, tt.wantProfile != ""
				}
				if exists != wantExists || got != want {
					t.Errorf("%s = %q (exists %v), want %q", file, got, exists, want)
				}
				backup, hasBackup := readOrEmpty(t, filepath.Join(home, file+backupSuffix))
				if wantBackup := file == tt.profileFile && tt.wantBackup; hasBackup != wantBackup || (wantBackup && backup != *tt.before) {
					t.Errorf("%s backup = %q (exists %v), want exists %v with the original", file, backup, hasBackup, wantBackup)
				}
			}
			if strings.HasSuffix(tt.wantProfile, testBlock) {
				if len(run.record.Profiles) != 1 || run.record.Profiles[0] != filepath.Join(home, tt.profileFile) {
					t.Errorf("record.Profiles = %v", run.record.Profiles)
				}
			} else if len(run.record.Profiles) != 0 {
				t.Errorf("record.Profiles = %v, want none", run.record.Profiles)
			}
		})
	}
}

func ptr(text string) *string { return &text }

func TestInstallLeavesAProfileWithOurBlockAlone(t *testing.T) {
	const profile = "export A=1\n" + testBlock + "export B=2\n"
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".zprofile"), []byte(profile))

	run := runInstall(t, home, Options{Shell: "/bin/zsh", Path: "/usr/bin"})

	if got, _ := readOrEmpty(t, filepath.Join(home, ".zprofile")); got != profile {
		t.Errorf("profile changed:\n%s", got)
	}
	if _, has := readOrEmpty(t, filepath.Join(home, ".zprofile"+backupSuffix)); has {
		t.Error("a backup was made although nothing changed")
	}
	if strings.Contains(run.plan, "PATH in") || len(run.notes) != 0 {
		t.Errorf("plan or notes talk about the PATH:\n%s%v", run.plan, run.notes)
	}
}

func TestInstallAgainChangesNothingMore(t *testing.T) {
	const original = "export A=1\n"
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".zprofile"), []byte(original))
	opts := Options{Shell: "/bin/zsh", Path: "/usr/bin"}
	first := runInstall(t, home, opts)

	second := runInstallWith(t, home, first.artifacts, first.binary, opts)

	if got, _ := readOrEmpty(t, filepath.Join(home, ".zprofile")); got != original+testBlock || strings.Count(got, "agent-screensaver >>>") != 1 {
		t.Errorf("profile after two installs:\n%s", got)
	}
	if backup, _ := readOrEmpty(t, filepath.Join(home, ".zprofile"+backupSuffix)); backup != original {
		t.Errorf("the backup is %q, want the original file", backup)
	}
	if strings.Contains(second.plan, "PATH in") || len(second.notes) != 0 {
		t.Errorf("second plan or notes talk about the PATH:\n%s%v", second.plan, second.notes)
	}
	if len(second.record.Profiles) != 1 {
		t.Errorf("the second record forgot the profile: %v", second.record.Profiles)
	}
	if target, err := os.Readlink(linkPath(home)); err != nil || target != locationsFor(home).bin {
		t.Errorf("link -> %q, %v", target, err)
	}
}

func TestInstallLeavesAForeignCommandAlone(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, link string)
	}{
		{"a file", func(t *testing.T, link string) { writeFile(t, link, []byte("#!/bin/sh\necho mine\n")) }},
		{"a link to another program", func(t *testing.T, link string) {
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/usr/bin/true", link); err != nil {
				t.Fatal(err)
			}
		}},
		{"a folder", func(t *testing.T, link string) {
			if err := os.MkdirAll(link, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tt.build(t, linkPath(home))
			before, err := os.Lstat(linkPath(home))
			if err != nil {
				t.Fatal(err)
			}
			beforeTarget, _ := os.Readlink(linkPath(home))

			run := runInstall(t, home, Options{Shell: "/bin/zsh", Path: "/usr/bin"})

			t.Logf("plan:\n%s", run.plan)
			if want := "Skip the agent-screensaver command: " + linkPath(home) + " already exists and is not ours, so it stays as it is"; !strings.Contains(run.plan, want) {
				t.Errorf("plan lacks %q:\n%s", want, run.plan)
			}
			after, err := os.Lstat(linkPath(home))
			if err != nil || after.Mode() != before.Mode() || after.Size() != before.Size() {
				t.Errorf("the foreign command changed: %v, %v", after, err)
			}
			if target, _ := os.Readlink(linkPath(home)); target != beforeTarget {
				t.Errorf("link now points at %q", target)
			}
			if _, has := readOrEmpty(t, filepath.Join(home, ".zprofile")); has {
				t.Error("the profile was edited for a command that was skipped")
			}
			if run.record.CommandLink != "" || len(run.record.Profiles) != 0 || len(run.notes) != 0 {
				t.Errorf("record or notes: %q %v %v", run.record.CommandLink, run.record.Profiles, run.notes)
			}
		})
	}
}

func TestUninstallRemovesTheLinkAndOnlyOurBlock(t *testing.T) {
	const original = "# my zsh\nexport A=1\n\nalias ll='ls -l'\n"
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".zprofile"), []byte(original))
	run := runInstall(t, home, Options{Shell: "/bin/zsh", Path: "/usr/bin"})
	// The user adds a line after ours; it must survive.
	profile := filepath.Join(home, ".zprofile")
	if data, _ := os.ReadFile(profile); string(data) != original+testBlock {
		t.Fatalf("install wrote %q", data)
	}
	later := original + testBlock + "export LATER=1\n"
	writeFile(t, profile, []byte(later))
	_ = run

	plan, err := PlanUninstall(home)
	if err != nil {
		t.Fatal(err)
	}
	text := planText(plan)
	t.Logf("uninstall plan:\n%s", text)
	for _, want := range []string{
		"Delete the agent-screensaver command link " + linkPath(home),
		"Remove the lines of agent-screensaver that add ~/.local/bin to your PATH from " + profile + " (your other lines stay)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("uninstall plan lacks %q:\n%s", want, text)
		}
	}

	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(linkPath(home)); !os.IsNotExist(err) {
		t.Errorf("the link is still there (err %v)", err)
	}
	if info, err := os.Stat(filepath.Dir(linkPath(home))); err != nil || !info.IsDir() {
		t.Errorf("~/.local/bin must stay (err %v)", err)
	}
	if got, _ := readOrEmpty(t, profile); got != original+"export LATER=1\n" {
		t.Errorf("profile after uninstall = %q", got)
	}
}

func TestUninstallGivesBackAProfileInstallMadeFromNothing(t *testing.T) {
	home := t.TempDir()
	runInstall(t, home, Options{Shell: "/bin/bash", Path: "/usr/bin"})

	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}

	if got, _ := readOrEmpty(t, filepath.Join(home, ".bash_profile")); got != "" {
		t.Errorf("profile after uninstall = %q, want no lines of ours", got)
	}
}

func TestUninstallLeavesAForeignLinkAndKeepsTheRest(t *testing.T) {
	home := t.TempDir()
	runInstall(t, home, Options{Shell: "/bin/zsh", Path: "/usr/bin"})
	// The user replaces our link with a program of their own.
	if err := os.Remove(linkPath(home)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, linkPath(home), []byte("mine"))

	plan, err := PlanUninstall(home)
	if err != nil {
		t.Fatal(err)
	}
	text := planText(plan)
	t.Logf("uninstall plan:\n%s", text)
	if want := "Leave " + linkPath(home) + " where it is: it is not the link that install made"; !strings.Contains(text, want) {
		t.Errorf("plan lacks %q:\n%s", want, text)
	}
	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}

	if got, _ := readOrEmpty(t, linkPath(home)); got != "mine" {
		t.Errorf("the foreign command became %q", got)
	}
}

func TestUninstallOfAnOlderInstallTouchesNoCommand(t *testing.T) {
	home := t.TempDir()
	runInstall(t, home, Options{})
	// An older record knows nothing of the command.
	record, err := LoadRecord(locationsFor(home).paths.InstallRecord, nil)
	if err != nil {
		t.Fatal(err)
	}
	record.CommandLink, record.Profiles = "", nil
	if err := SaveRecord(locationsFor(home).paths.InstallRecord, record); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".zprofile"), []byte(testBlock))
	if err := os.Remove(linkPath(home)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, linkPath(home), []byte("mine"))

	plan, err := PlanUninstall(home)
	if err != nil {
		t.Fatal(err)
	}
	if text := planText(plan); strings.Contains(text, linkPath(home)) || strings.Contains(text, "PATH") {
		t.Errorf("the plan mentions the command:\n%s", text)
	}
	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}

	if got, _ := readOrEmpty(t, linkPath(home)); got != "mine" {
		t.Errorf("the command became %q", got)
	}
	if got, _ := readOrEmpty(t, filepath.Join(home, ".zprofile")); got != testBlock {
		t.Errorf("a profile that the record does not name changed: %q", got)
	}
}

func TestUninstallIgnoresARecordedProfileThatIsNotOurs(t *testing.T) {
	home := t.TempDir()
	runInstall(t, home, Options{})
	other := filepath.Join(home, "notes.txt")
	writeFile(t, other, []byte(testBlock))
	record, err := LoadRecord(locationsFor(home).paths.InstallRecord, nil)
	if err != nil {
		t.Fatal(err)
	}
	record.Profiles = []string{other}
	if err := SaveRecord(locationsFor(home).paths.InstallRecord, record); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}

	if got, _ := readOrEmpty(t, other); got != testBlock {
		t.Errorf("a file that is not a shell profile of ours changed: %q", got)
	}
}

func TestWithoutBlocks(t *testing.T) {
	tests := []struct {
		name, in, want string
		found          bool
	}{
		{"no block", "a\nb\n", "a\nb\n", false},
		{"block between lines", "a\n" + testBlock + "b\n", "a\nb\n", true},
		{"block at the end without a newline", "a\n# >>> agent-screensaver >>>\nx\n# <<< agent-screensaver <<<", "a\n", true},
		{"two blocks", testBlock + "a\n" + testBlock, "a\n", true},
		{"begin without an end stays", "a\n# >>> agent-screensaver >>>\nb\n", "a\n# >>> agent-screensaver >>>\nb\n", false},
		{"end without a begin stays", "a\n# <<< agent-screensaver <<<\nb\n", "a\n# <<< agent-screensaver <<<\nb\n", false},
		{"a stray begin does not eat the lines before a real block", "# >>> agent-screensaver >>>\nkeep\n" + testBlock, "# >>> agent-screensaver >>>\nkeep\n", true},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := withoutBlocks([]byte(tt.in))
			if string(got) != tt.want || found != tt.found {
				t.Errorf("withoutBlocks(%q) = %q, %v; want %q, %v", tt.in, got, found, tt.want, tt.found)
			}
		})
	}
}

func TestOnPath(t *testing.T) {
	const home = "/Users/x"
	tests := []struct {
		path string
		want bool
	}{
		{"/usr/bin:/Users/x/.local/bin", true},
		{"/Users/x/.local/bin/", true},
		{"/Users/x/.local/../.local/bin", true},
		{"~/.local/bin", true},
		{"$HOME/.local/bin", true},
		{"${HOME}/.local/bin", true},
		{"/Users/y/.local/bin", false},
		{"/Users/x/.local", false},
		{".local/bin", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := onPath(home, tt.path); got != tt.want {
			t.Errorf("onPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
