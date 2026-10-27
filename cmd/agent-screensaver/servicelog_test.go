package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/logfile"
)

func TestServiceLogRotatesWhenStderrIsTheLogFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "daemon.log")
	stderr, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	var redirected int

	out, closeLog := serviceLog(logPath, stderr, func(*os.File) error { redirected++; return nil })
	defer closeLog()

	if _, ok := out.(*logfile.Rotating); !ok {
		t.Fatalf("log writer = %T, want a rotating log", out)
	}
	line := strings.Repeat("x", logfile.MaxSize-10) + "\n"
	for range 2 {
		if _, err := out.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Errorf("no daemon.log.1 after passing the limit: %v", err)
	}
	if redirected != 1 {
		t.Errorf("stderr redirected %d times, want 1", redirected)
	}
}

func TestServiceLogKeepsATerminalStderr(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "daemon.log")
	other, err := os.Create(filepath.Join(t.TempDir(), "terminal"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	out, closeLog := serviceLog(logPath, other, nil)
	defer closeLog()

	if out != other {
		t.Errorf("log writer = %v, want stderr itself", out)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("daemon.log created for a run outside launchd: %v", err)
	}
}
