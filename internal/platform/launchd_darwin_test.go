package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recorder struct {
	commands [][]string
	failOn   string // an argument that makes the call fail, for example "print"
	loaded   bool   // print succeeds until bootout ran, like a real service
	gone     bool   // set by bootout: print fails from then on
}

func noSleep(time.Duration) {}

func (r *recorder) run(name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	if r.loaded {
		switch args[0] {
		case "bootout":
			r.loaded, r.gone = false, true
		case "print":
			return nil
		}
	}
	if r.gone && args[0] == "print" {
		return errors.New("exit status 113")
	}
	if r.failOn != "" && len(args) > 0 && args[0] == r.failOn {
		return errors.New("exit status 113")
	}
	return nil
}

// scriptedLaunchd is a fake launchctl. printLoaded is how many `print` calls
// still say "loaded" (the first one decides whether bootout runs);
// bootstrapFailures is how many bootstrap calls fail before one succeeds
// (-1 means always).
type scriptedLaunchd struct {
	printLoaded       int
	bootstrapFailures int
	commands          [][]string
	slept             []time.Duration
}

func (s *scriptedLaunchd) run(name string, args ...string) error {
	s.commands = append(s.commands, append([]string{name}, args...))
	switch args[0] {
	case "print":
		if s.printLoaded > 0 {
			s.printLoaded--
			return nil
		}
		return errors.New("exit status 113")
	case "bootstrap":
		if s.bootstrapFailures != 0 {
			if s.bootstrapFailures > 0 {
				s.bootstrapFailures--
			}
			return errors.New("exit status 5: Bootstrap failed: 5: Input/output error")
		}
	}
	return nil
}

func (s *scriptedLaunchd) sleep(d time.Duration) { s.slept = append(s.slept, d) }

func (s *scriptedLaunchd) count(verb string) int {
	n := 0
	for _, command := range s.commands {
		if command[1] == verb {
			n++
		}
	}
	return n
}

func TestLaunchdWaitsAndRetries(t *testing.T) {
	tests := []struct {
		name          string
		fake          scriptedLaunchd
		wantErr       bool
		wantPrints    int
		wantBootstrap int
		wantSleeps    int
	}{
		{"gone after 2 polls, bootstrap once",
			scriptedLaunchd{printLoaded: 3}, false, 4, 1, 2},
		{"bootstrap fails twice with status 5, then loads",
			scriptedLaunchd{printLoaded: 1, bootstrapFailures: 2}, false, 2, 3, 2},
		{"bootstrap always fails",
			scriptedLaunchd{printLoaded: 1, bootstrapFailures: -1}, true, 2, 5, 4},
		{"service never goes away, bootstrap still tried",
			scriptedLaunchd{printLoaded: 1000, bootstrapFailures: 0}, false, 31, 1, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := tt.fake
			launchd := Launchd{Home: t.TempDir(), Run: fake.run, Sleep: fake.sleep}

			_, err := launchd.Install("x.y", []string{"/bin/x", "daemon"}, "/tmp/x.log")

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				for _, want := range []string{"Bootstrap failed: 5", "launchctl bootstrap gui/$(id -u)", "re-run install"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error lacks %q: %v", want, err)
					}
				}
			}
			if got := fake.count("print"); got != tt.wantPrints {
				t.Errorf("print calls = %d, want %d", got, tt.wantPrints)
			}
			if got := fake.count("bootstrap"); got != tt.wantBootstrap {
				t.Errorf("bootstrap calls = %d, want %d", got, tt.wantBootstrap)
			}
			if got := len(fake.slept); got != tt.wantSleeps {
				t.Errorf("sleeps = %d, want %d", got, tt.wantSleeps)
			}
		})
	}
}

func TestLaunchdPlistAndCommands(t *testing.T) {
	label := "dev.agentscreensaver.daemon"
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	tests := []struct {
		name          string
		failOn        string
		loaded        bool
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
			name:   "already loaded",
			loaded: true,
			wantInstall: [][]string{
				{"launchctl", "print", target},
				{"launchctl", "bootout", target},
				{"launchctl", "print", target},
				{"launchctl", "bootstrap", domain, ""},
			},
			wantUninstall: [][]string{
				{"launchctl", "print", target},
				{"launchctl", "bootout", target},
				{"launchctl", "print", target},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			fake := &recorder{failOn: tt.failOn, loaded: tt.loaded}
			launchd := Launchd{Home: home, Run: fake.run, Sleep: noSleep}
			bin := filepath.Join(home, "Library", "Application Support", "agent-screensaver", "bin", "agent-screensaver")

			logFile := filepath.Join(home, "Library", "Application Support", "agent-screensaver", "some.log")
			plist, err := launchd.Install(label, []string{bin, "daemon"}, logFile)
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
				"<key>StandardErrorPath</key>\n\t<string>" + logFile + "</string>",
			} {
				if !strings.Contains(string(content), want) {
					t.Errorf("plist lacks %q:\n%s", want, content)
				}
			}

			fake.commands = nil
			fake.loaded, fake.gone = tt.loaded, false
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
	launchd := Launchd{Home: home, Run: (&recorder{failOn: "print"}).run, Sleep: noSleep}
	plist, err := launchd.Install("x.y", []string{"/a&b/<bin>", "daemon"}, "/tmp/x.log")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(plist)
	if !strings.Contains(string(content), "/a&amp;b/&lt;bin&gt;") {
		t.Errorf("not escaped:\n%s", content)
	}
}

func TestLaunchdBootstrapFailureIsReported(t *testing.T) {
	launchd := Launchd{Home: t.TempDir(), Run: (&recorder{failOn: "bootstrap"}).run, Sleep: noSleep}
	if _, err := launchd.Install("x.y", []string{"/bin/x", "daemon"}, "/tmp/x.log"); err == nil {
		t.Error("want an error, got nil")
	}
}

func TestKeepAliveByService(t *testing.T) {
	tests := []struct {
		name, label, want string
	}{
		{"daemon is always restarted", "dev.agentscreensaver.daemon", "<key>KeepAlive</key>\n\t<true/>"},
		{"tray stays quit after a clean exit", "dev.agentscreensaver.tray",
			"<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plist := string(renderPlist(tt.label, []string{"/bin/x"}, "/log"))
			if !strings.Contains(plist, tt.want) {
				t.Errorf("plist lacks %q:\n%s", tt.want, plist)
			}
		})
	}
}

func TestLaunchdStartKickstartsWithoutRestart(t *testing.T) {
	fake := &recorder{}
	launchd := Launchd{Home: t.TempDir(), Run: fake.run, Sleep: noSleep}
	if err := launchd.Start("dev.agentscreensaver.tray"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"launchctl", "kickstart", domain() + "/dev.agentscreensaver.tray"}}
	if !reflect.DeepEqual(fake.commands, want) {
		t.Errorf("commands = %q, want %q (no -k, so a running menu bar app is left alone)", fake.commands, want)
	}
}
