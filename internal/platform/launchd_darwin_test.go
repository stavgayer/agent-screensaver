package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recorder struct {
	commands [][]string
	failOn   string // an argument that makes the call fail, for example "print"
}

func (r *recorder) run(name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	if r.failOn != "" && len(args) > 0 && args[0] == r.failOn {
		return errors.New("exit status 113")
	}
	return nil
}

func TestLaunchdPlistAndCommands(t *testing.T) {
	label := "dev.agentscreensaver.daemon"
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	tests := []struct {
		name          string
		failOn        string
		wantInstall   [][]string
		wantUninstall [][]string
	}{
		{
			name:   "not loaded yet",
			failOn: "print",
			wantInstall: [][]string{
				{"launchctl", "print", target},
				{"launchctl", "bootstrap", domain, ""},
			},
			wantUninstall: [][]string{{"launchctl", "print", target}},
		},
		{
			name: "already loaded",
			wantInstall: [][]string{
				{"launchctl", "print", target},
				{"launchctl", "bootout", target},
				{"launchctl", "bootstrap", domain, ""},
			},
			wantUninstall: [][]string{
				{"launchctl", "print", target},
				{"launchctl", "bootout", target},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			fake := &recorder{failOn: tt.failOn}
			launchd := Launchd{Home: home, Run: fake.run}
			bin := filepath.Join(home, "Library", "Application Support", "agent-screensaver", "bin", "agent-screensaver")

			plist, err := launchd.Install(label, []string{bin, "daemon"})
			if err != nil {
				t.Fatal(err)
			}

			wantPlist := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
			if plist != wantPlist {
				t.Errorf("plist path = %s, want %s", plist, wantPlist)
			}
			assertCommands(t, fake.commands, tt.wantInstall, plist)
			content, err := os.ReadFile(plist)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"<key>Label</key>\n\t<string>" + label + "</string>",
				"<string>" + bin + "</string>\n\t\t<string>daemon</string>",
				"<key>RunAtLoad</key>\n\t<true/>",
				"<key>KeepAlive</key>\n\t<true/>",
				"<key>ProcessType</key>\n\t<string>Background</string>",
				"<key>LowPriorityIO</key>\n\t<true/>",
				"<key>StandardErrorPath</key>\n\t<string>" + filepath.Join(home, "Library", "Application Support", "agent-screensaver", "daemon.log") + "</string>",
			} {
				if !strings.Contains(string(content), want) {
					t.Errorf("plist lacks %q:\n%s", want, content)
				}
			}

			fake.commands = nil
			if err := launchd.Uninstall(label); err != nil {
				t.Fatal(err)
			}
			assertCommands(t, fake.commands, tt.wantUninstall, plist)
			if _, err := os.Stat(plist); !os.IsNotExist(err) {
				t.Errorf("plist still exists (err %v)", err)
			}
		})
	}
}

// assertCommands compares recorded commands; an empty last argument in want
// stands for the plist path, which the test only learns from Install.
func assertCommands(t *testing.T, got, want [][]string, plist string) {
	t.Helper()
	expected := make([][]string, len(want))
	for position, command := range want {
		expected[position] = append([]string{}, command...)
		if last := len(command) - 1; command[last] == "" {
			expected[position][last] = plist
		}
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("commands = %v, want %v", got, expected)
	}
}

func TestLaunchdPlistEscapesXML(t *testing.T) {
	home := t.TempDir()
	launchd := Launchd{Home: home, Run: (&recorder{failOn: "print"}).run}
	plist, err := launchd.Install("x.y", []string{"/a&b/<bin>", "daemon"})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(plist)
	if !strings.Contains(string(content), "/a&amp;b/&lt;bin&gt;") {
		t.Errorf("not escaped:\n%s", content)
	}
}

func TestLaunchdBootstrapFailureIsReported(t *testing.T) {
	launchd := Launchd{Home: t.TempDir(), Run: (&recorder{failOn: "bootstrap"}).run}
	if _, err := launchd.Install("x.y", []string{"/bin/x", "daemon"}); err == nil {
		t.Error("want an error, got nil")
	}
}
