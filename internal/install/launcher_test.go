package install

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/paths"
)

// launcherApp is where install puts the launcher app for home.
func launcherApp(home string) string {
	return filepath.Join(home, "Applications", "Agent Screensaver.app")
}

// registerSpy records the apps install asks Launch Services about, so no test
// runs the real lsregister.
type registerSpy struct{ apps []string }

func (r *registerSpy) register(app string) { r.apps = append(r.apps, app) }

// applyLauncher installs into home with a fake binary. A temp home only.
func applyLauncher(t *testing.T, home, version string, spy *registerSpy) (Record, string) {
	t.Helper()
	withTools(t, home)
	artifacts, binary := artifactsFixture(t)
	record, err := Apply(home, binary, artifacts, &fakeServices{}, Options{Version: version, Register: spy.register})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return record, artifacts
}

func TestInstallCreatesValidLauncherApp(t *testing.T) {
	home := t.TempDir()
	spy := &registerSpy{}

	record, _ := applyLauncher(t, home, "1.2.3", spy)

	app := launcherApp(home)
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if id, err := bundleID(filepath.Join(app, "Contents", "Info.plist")); err != nil || id != LauncherBundleID {
		t.Fatalf("bundle id = %q, %v", id, err)
	}
	for _, want := range []string{
		"<key>CFBundleName</key>\n\t<string>Agent Screensaver</string>",
		"<key>CFBundleDisplayName</key>\n\t<string>Agent Screensaver</string>",
		"<key>CFBundlePackageType</key>\n\t<string>APPL</string>",
		"<key>CFBundleExecutable</key>\n\t<string>Agent Screensaver</string>",
		"<key>CFBundleIconFile</key>\n\t<string>AppIcon</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>1.2.3</string>",
		"<key>LSMinimumSystemVersion</key>\n\t<string>14.0</string>",
		"<key>LSUIElement</key>\n\t<true/>",
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}

	script := filepath.Join(app, "Contents", "MacOS", "Agent Screensaver")
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("launcher script is not executable: %v", info.Mode())
	}
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	wantScript := "#!/bin/sh\nexec '" + paths.ForHome(home).DataDir + "/bin/agent-screensaver' settings\n"
	if string(data) != wantScript {
		t.Errorf("script = %q, want %q", data, wantScript)
	}
	if !strings.Contains(wantScript, "Application Support") {
		t.Fatal("the test path should hold a space")
	}

	icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "AppIcon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(icon, []byte("icns")) {
		t.Errorf("AppIcon.icns does not start with the icns magic: %q", icon[:4])
	}

	if len(spy.apps) != 1 || spy.apps[0] != app {
		t.Errorf("registered = %v, want [%s]", spy.apps, app)
	}
	if record.Launcher != app {
		t.Errorf("record.Launcher = %q, want %q", record.Launcher, app)
	}
	saved, err := os.ReadFile(paths.ForHome(home).InstallRecord)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(saved, &onDisk); err != nil || onDisk["launcher"] != app {
		t.Errorf("install record launcher = %v (%v)", onDisk["launcher"], err)
	}
	entries, _ := os.ReadDir(filepath.Dir(app))
	if len(entries) != 1 {
		t.Errorf("~/Applications holds more than the app (a staging folder is left): %v", entries)
	}
}

// The script must start the installed binary with the argument settings,
// whatever the spaces in its path.
func TestLauncherScriptRunsInstalledBinaryWithSettings(t *testing.T) {
	home := t.TempDir()
	withTools(t, home)
	artifacts, binary := artifactsFixture(t)
	out := filepath.Join(t.TempDir(), "args.txt")
	fake := "#!/bin/sh\necho \"$@\" > '" + out + "'\n"
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(home, binary, artifacts, &fakeServices{}, Options{}); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(launcherApp(home), "Contents", "MacOS", "Agent Screensaver")
	if err := exec.Command(script).Run(); err != nil {
		t.Fatalf("run launcher: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil || strings.TrimSpace(string(got)) != "settings" {
		t.Errorf("binary got args %q (%v), want settings", got, err)
	}
}

func TestReinstallReplacesOurLauncher(t *testing.T) {
	home := t.TempDir()
	applyLauncher(t, home, "1.0.0", &registerSpy{})
	stale := filepath.Join(launcherApp(home), "Contents", "stale.txt")
	writeFile(t, stale, []byte("old"))
	spy := &registerSpy{}

	applyLauncher(t, home, "2.0.0", spy)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("old file survived the replace (err %v)", err)
	}
	plist, _ := os.ReadFile(filepath.Join(launcherApp(home), "Contents", "Info.plist"))
	if !strings.Contains(string(plist), "<string>2.0.0</string>") {
		t.Errorf("version not updated:\n%s", plist)
	}
	if len(spy.apps) != 1 {
		t.Errorf("registered = %v", spy.apps)
	}
}

// foreignApps are things at the launcher path that are not our app.
func foreignApps(t *testing.T) map[string]func(app string) {
	return map[string]func(app string){
		"another bundle id": func(app string) {
			writeFile(t, filepath.Join(app, "Contents", "Info.plist"), []byte(strings.Replace(launcherInfoPlist("9"), LauncherBundleID, "com.example.other", 1)))
		},
		"no Info.plist": func(app string) {
			writeFile(t, filepath.Join(app, "Contents", "readme.txt"), []byte("mine"))
		},
		"unreadable Info.plist": func(app string) {
			writeFile(t, filepath.Join(app, "Contents", "Info.plist"), []byte("bplist00\x00\x01 not xml"))
		},
		"a plain file": func(app string) {
			writeFile(t, app, []byte("mine"))
		},
	}
}

// snapshot lists every path under root with its content, to compare before
// and after.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[path] = "<dir>"
			return nil
		}
		data, _ := os.ReadFile(path)
		files[path] = string(data)
		return nil
	})
	return files
}

func TestForeignLauncherIsLeftAloneAndPlanSaysSo(t *testing.T) {
	for name, build := range foreignApps(t) {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			app := launcherApp(home)
			build(app)
			before := snapshot(t, filepath.Dir(app))
			spy := &registerSpy{}
			withTools(t, home)
			artifacts, _ := artifactsFixture(t)

			plan, err := Plan(home, artifacts)
			if err != nil {
				t.Fatal(err)
			}
			if text := planText(plan); !strings.Contains(text, "Skip the launcher app: "+app) || !strings.Contains(text, "not ours") {
				t.Errorf("plan does not say the app is skipped:\n%s", text)
			}
			record, _ := applyLauncher(t, home, "1.0.0", spy)

			after := snapshot(t, filepath.Dir(app))
			if len(before) != len(after) {
				t.Fatalf("~/Applications changed:\nbefore %v\nafter %v", before, after)
			}
			for path, content := range before {
				if after[path] != content {
					t.Errorf("%s changed", path)
				}
			}
			if len(spy.apps) != 0 {
				t.Errorf("registered a foreign app: %v", spy.apps)
			}
			if record.Launcher != "" {
				t.Errorf("record.Launcher = %q, want empty", record.Launcher)
			}
		})
	}
}

func planText(plan []PlannedStep) string {
	var text strings.Builder
	for _, step := range plan {
		text.WriteString(step.Description + "\n")
	}
	return text.String()
}

func TestPlanListsLauncherApp(t *testing.T) {
	home := t.TempDir()
	withTools(t, home)
	artifacts, _ := artifactsFixture(t)
	app := launcherApp(home)

	plan, err := Plan(home, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	text := planText(plan)
	if !strings.Contains(text, "Create the launcher app at "+app) || !strings.Contains(text, "does not exist yet and is created too") {
		t.Errorf("plan for a missing ~/Applications:\n%s", text)
	}

	if err := os.MkdirAll(filepath.Dir(app), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, _ = Plan(home, artifacts)
	if text := planText(plan); !strings.Contains(text, "Create the launcher app at "+app) || strings.Contains(text, "created too") {
		t.Errorf("plan for an existing ~/Applications:\n%s", text)
	}

	applyLauncher(t, home, "1.0.0", &registerSpy{})
	plan, _ = Plan(home, artifacts)
	if text := planText(plan); !strings.Contains(text, "Replace the launcher app at "+app) {
		t.Errorf("plan for a re-install:\n%s", text)
	}
}

func TestUninstallRemovesOurLauncherAndKeepsApplicationsFolder(t *testing.T) {
	home := t.TempDir()
	applyLauncher(t, home, "1.0.0", &registerSpy{})
	plan, err := PlanUninstall(home)
	if err != nil {
		t.Fatal(err)
	}
	if text := planText(plan); !strings.Contains(text, "Delete the launcher app "+launcherApp(home)) {
		t.Errorf("uninstall plan:\n%s", text)
	}

	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(launcherApp(home)); !os.IsNotExist(err) {
		t.Errorf("launcher app still there (err %v)", err)
	}
	if info, err := os.Stat(filepath.Join(home, "Applications")); err != nil || !info.IsDir() {
		t.Errorf("~/Applications must stay (err %v)", err)
	}
}

func TestUninstallLeavesForeignLauncher(t *testing.T) {
	for name, build := range foreignApps(t) {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			applyLauncher(t, home, "1.0.0", &registerSpy{})
			app := launcherApp(home)
			if err := os.RemoveAll(app); err != nil { // the user put their own app there after install
				t.Fatal(err)
			}
			build(app)
			before := snapshot(t, filepath.Dir(app))
			plan, err := PlanUninstall(home)
			if err != nil {
				t.Fatal(err)
			}
			if text := planText(plan); !strings.Contains(text, "Leave "+app+" where it is") {
				t.Errorf("uninstall plan does not say the app stays:\n%s", text)
			}

			if err := Uninstall(home, &fakeServices{}); err != nil {
				t.Fatal(err)
			}

			after := snapshot(t, filepath.Dir(app))
			if len(before) != len(after) {
				t.Fatalf("~/Applications changed:\nbefore %v\nafter %v", before, after)
			}
			for path, content := range before {
				if after[path] != content {
					t.Errorf("%s changed", path)
				}
			}
		})
	}
}

func TestUninstallWithoutLauncherSaysNothingAboutIt(t *testing.T) {
	home := t.TempDir()
	applyLauncher(t, home, "1.0.0", &registerSpy{})
	if err := os.RemoveAll(launcherApp(home)); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanUninstall(home)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(planText(plan), "launcher") {
		t.Errorf("plan mentions a launcher that is not there:\n%s", planText(plan))
	}
	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}
}
