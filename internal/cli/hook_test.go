package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
)

type panickingAdapter struct{}

func (panickingAdapter) Name() string { return "panicky" }
func (panickingAdapter) Map([]byte, int64) (event.Event, bool) {
	panic("boom")
}

func TestRunHookRecoversFromPanic(t *testing.T) {
	p := paths.ForHome(t.TempDir())

	code := RunHook(panickingAdapter{}, strings.NewReader("{}"), p, func() int64 { return 1 })

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	log, err := os.ReadFile(filepath.Join(p.DataDir, errorLogName))
	if err != nil || !strings.Contains(string(log), "panic: boom") {
		t.Errorf("error log = %q, err %v", log, err)
	}
}
