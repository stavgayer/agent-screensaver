package main

import (
	"io"
	"log"
	"os"
	"syscall"

	"agent-screensaver/internal/logfile"
)

// serviceLog returns the writer for the daemon's log. When stderr is
// logPath (launchd's StandardErrorPath), it is a rotating log that keeps
// the file under logfile.MaxSize, and redirect points stderr at each new
// file so a crash report lands there too. Otherwise, as in a terminal,
// stderr is used as it is. The returned func closes what it opened.
func serviceLog(logPath string, stderr *os.File, redirect func(*os.File) error) (io.Writer, func()) {
	if !sameFile(logPath, stderr) {
		return stderr, func() {}
	}
	rotating, err := logfile.Open(logPath, logfile.MaxSize, redirect)
	if err != nil {
		return stderr, func() {}
	}
	return rotating, func() { _ = rotating.Close() }
}

func sameFile(path string, file *os.File) bool {
	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}
	pathInfo, err := os.Stat(path)
	return err == nil && os.SameFile(fileInfo, pathInfo)
}

// redirectStderr makes file descriptor 2 write to file.
func redirectStderr(file *os.File) error {
	return syscall.Dup2(int(file.Fd()), int(os.Stderr.Fd()))
}

// useServiceLog sends the standard logger to the daemon's log.
func useServiceLog(logPath string) (io.Writer, func()) {
	out, closeLog := serviceLog(logPath, os.Stderr, redirectStderr)
	log.SetOutput(out)
	return out, closeLog
}
